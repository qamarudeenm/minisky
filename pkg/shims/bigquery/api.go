package bigquery

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"minisky/pkg/config"
	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("bigquery.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.OpMgr)
	})

	registry.RegisterRoutes("bigquery.googleapis.com",
		"/bigquery/v2",
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// Dataset mirrors the BigQuery Dataset resource.
type Dataset struct {
	Kind             string            `json:"kind"`
	ID               string            `json:"id"`
	DatasetReference DatasetRef        `json:"datasetReference"`
	FriendlyName     string            `json:"friendlyName,omitempty"`
	Description      string            `json:"description,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Location         string            `json:"location"`
	CreationTime     string            `json:"creationTime"`
	LastModifiedTime string            `json:"lastModifiedTime"`
	Etag             string            `json:"etag"`
	SelfLink         string            `json:"selfLink"`
}

type DatasetRef struct {
	ProjectId string `json:"projectId"`
	DatasetId string `json:"datasetId"`
}

// Table mirrors the BigQuery Table resource.
type Table struct {
	Kind             string            `json:"kind"`
	ID               string            `json:"id"`
	TableReference   TableRef          `json:"tableReference"`
	Schema           *TableSchema      `json:"schema,omitempty"`
	FriendlyName     string            `json:"friendlyName,omitempty"`
	Description      string            `json:"description,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Location         string            `json:"location"`
	CreationTime     string            `json:"creationTime"`
	LastModifiedTime string            `json:"lastModifiedTime"`
	NumRows          string            `json:"numRows"`
	NumBytes         string            `json:"numBytes"`
	Type             string            `json:"type"` // TABLE, VIEW, EXTERNAL
	Etag             string            `json:"etag"`
	SelfLink         string            `json:"selfLink"`
	// In-memory row storage (for insertAll)
	rows []map[string]interface{}
}

type TableRef struct {
	ProjectId string `json:"projectId"`
	DatasetId string `json:"datasetId"`
	TableId   string `json:"tableId"`
}

type TableSchema struct {
	Fields []FieldSchema `json:"fields"`
}

type FieldSchema struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"` // STRING, INTEGER, FLOAT, BOOLEAN, RECORD, TIMESTAMP, DATE, etc.
	Mode        string        `json:"mode"` // NULLABLE, REQUIRED, REPEATED
	Description string        `json:"description,omitempty"`
	Fields      []FieldSchema `json:"fields,omitempty"` // nested RECORD
}

// Job mirrors the BigQuery Job resource.
type Job struct {
	Kind          string        `json:"kind"`
	ID            string        `json:"id"`
	JobReference  JobRef        `json:"jobReference"`
	Status        JobStatus     `json:"status"`
	Statistics    JobStatistics `json:"statistics"`
	Configuration JobConfig     `json:"configuration"`

	// Internal state
	RawRows []map[string]interface{} `json:"-"`
	Schema  *TableSchema             `json:"-"`
}

type JobRef struct {
	ProjectId string `json:"projectId"`
	JobId     string `json:"jobId"`
	Location  string `json:"location"`
}

type JobStatus struct {
	State       string      `json:"state"` // PENDING, RUNNING, DONE
	ErrorResult *ErrorProto `json:"errorResult,omitempty"`
}

type ErrorProto struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type JobStatistics struct {
	CreationTime        string `json:"creationTime"`
	StartTime           string `json:"startTime,omitempty"`
	EndTime             string `json:"endTime,omitempty"`
	TotalBytesProcessed string `json:"totalBytesProcessed"`
	TotalSlotMs         string `json:"totalSlotMs"`
}

type JobConfig struct {
	JobType string       `json:"jobType"` // QUERY, LOAD, EXTRACT, COPY
	Query   *QueryConfig `json:"query,omitempty"`
	Load    *LoadConfig  `json:"load,omitempty"`
}

type QueryConfig struct {
	Query            string      `json:"query"`
	UseLegacySql     bool        `json:"useLegacySql"`
	DefaultDataset   *DatasetRef `json:"defaultDataset,omitempty"`
	DestinationTable *TableRef   `json:"destinationTable,omitempty"`
}

type LoadConfig struct {
	SourceUris       []string `json:"sourceUris"`
	DestinationTable TableRef `json:"destinationTable"`
	SourceFormat     string   `json:"sourceFormat"` // CSV, JSON, NEWLINE_DELIMITED_JSON, PARQUET
	Autodetect       bool     `json:"autodetect"`
}

// QueryResultRow is a single row in a query response.
type QueryResultRow struct {
	F []QueryResultCell `json:"f"`
}

type QueryResultCell struct {
	V interface{} `json:"v"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

// API is the high-fidelity BigQuery v2 shim.
// Query execution is stubbed (returns empty results); table/dataset state is fully tracked.
type API struct {
	mu       sync.RWMutex
	opMgr    *orchestrator.OperationManager
	backend  *DuckDBBackend
	datasets map[string]*Dataset // key: project:datasetId
	tables   map[string]*Table   // key: project:datasetId:tableId
	jobs     map[string]*Job     // key: project:jobId
}

func NewAPI(opMgr *orchestrator.OperationManager) *API {
	api := &API{
		opMgr:    opMgr,
		backend:  NewDuckDBBackend(),
		datasets: make(map[string]*Dataset),
		tables:   make(map[string]*Table),
		jobs:     make(map[string]*Job),
	}
	api.restore()
	return api
}

// GetBackend exposes the backend for dynamic dashboard configuration.
func (api *API) GetBackend() *DuckDBBackend {
	return api.backend
}

// ServeHTTP dispatches BigQuery v2 paths.
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: BigQuery] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/insertAll"):
		api.insertAll(w, r, path)
	case strings.Contains(path, "/upload"):
		api.handleUpload(w, r)
	case strings.Contains(path, "/tables") && strings.Contains(path, "/datasets"):
		api.routeTables(w, r, path)
	case strings.Contains(path, "/datasets"):
		api.routeDatasets(w, r, path)
	case strings.Contains(path, "/queries"):
		// The official surface: POST /projects/{p}/queries (jobs.query) and
		// GET /projects/{p}/queries/{jobId} (jobs.getQueryResults). Every
		// google-cloud client reads results from here.
		api.routeQueries(w, r, path)
	case strings.Contains(path, "/jobs") && strings.Contains(path, "/results"):
		api.getQueryResults(w, r, path)
	case strings.Contains(path, "/jobs"):
		api.routeJobs(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "BigQuery resource not found: "+path)
	}
}

func (api *API) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(50 << 20) // 50MB max
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "INVALID_ARGUMENT", "Error retrieving file: "+err.Error())
		return
	}
	defer file.Close()

	uploadDir := filepath.Join(config.GetMiniskyDir(), "uploads")
	os.MkdirAll(uploadDir, 0755)

	destPath := filepath.Join(uploadDir, handler.Filename)
	dst, err := os.Create(destPath)
	if err != nil {
		writeError(w, 500, "INTERNAL", "Error creating file: "+err.Error())
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, 500, "INTERNAL", "Error saving file: "+err.Error())
		return
	}

	absPath, _ := filepath.Abs(destPath)
	log.Printf("[BigQuery] File uploaded to: %s", absPath)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"path":     absPath,
		"filename": handler.Filename,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Datasets
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeDatasets(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	datasetId := extractSegmentAfter(path, "datasets")

	switch r.Method {
	case http.MethodPost:
		var body struct {
			DatasetReference DatasetRef        `json:"datasetReference"`
			FriendlyName     string            `json:"friendlyName"`
			Description      string            `json:"description"`
			Labels           map[string]string `json:"labels"`
			Location         string            `json:"location"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.DatasetReference.DatasetId == "" {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "INVALID_ARGUMENT", "datasetReference.datasetId is required")
			return
		}
		dsID := body.DatasetReference.DatasetId
		location := body.Location
		if location == "" {
			location = "US"
		}
		nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())
		ds := &Dataset{
			Kind:             "bigquery#dataset",
			ID:               fmt.Sprintf("%s:%s", project, dsID),
			DatasetReference: DatasetRef{ProjectId: project, DatasetId: dsID},
			FriendlyName:     body.FriendlyName,
			Description:      body.Description,
			Labels:           body.Labels,
			Location:         location,
			CreationTime:     nowMs,
			LastModifiedTime: nowMs,
			Etag:             newEtag(),
			SelfLink:         fmt.Sprintf("https://bigquery.googleapis.com/bigquery/v2/projects/%s/datasets/%s", project, dsID),
		}
		key := project + ":" + dsID
		api.mu.Lock()
		api.datasets[key] = ds
		api.saveLocked()
		api.mu.Unlock()

		// The SQL translator needs the dataset name to tell `dataset.table`
		// apart from `alias.column`.
		api.backend.SetKnownDatasets([]string{dsID})

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ds)

	case http.MethodGet:
		if datasetId != "" {
			key := project + ":" + datasetId
			api.mu.RLock()
			ds, ok := api.datasets[key]
			api.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				writeError(w, 404, "NOT_FOUND", "Dataset "+datasetId+" not found")
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(ds)
		} else {
			prefix := project + ":"
			api.mu.RLock()
			items := []*Dataset{}
			for k, v := range api.datasets {
				if strings.HasPrefix(k, prefix) {
					items = append(items, v)
				}
			}
			api.mu.RUnlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"kind":     "bigquery#datasetList",
				"datasets": items,
			})
		}

	case http.MethodPatch, http.MethodPut:
		// Terraform PATCHes a dataset to reconcile metadata. Echoing the stored
		// resource back would leave the update unapplied, and the next plan would
		// propose the identical change forever — so the mutable fields are
		// actually written here.
		var body struct {
			FriendlyName *string           `json:"friendlyName"`
			Description  *string           `json:"description"`
			Labels       map[string]string `json:"labels"`
			Location     *string           `json:"location"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		key := project + ":" + datasetId
		api.mu.Lock()
		ds, ok := api.datasets[key]
		if ok {
			if body.FriendlyName != nil {
				ds.FriendlyName = *body.FriendlyName
			}
			if body.Description != nil {
				ds.Description = *body.Description
			}
			if body.Labels != nil {
				ds.Labels = body.Labels
			}
			if body.Location != nil && *body.Location != "" {
				ds.Location = *body.Location
			}
			ds.LastModifiedTime = fmt.Sprintf("%d", time.Now().UnixMilli())
			ds.Etag = newEtag()
			api.saveLocked()
		}
		api.mu.Unlock()

		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Dataset "+datasetId+" not found")
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ds)

	case http.MethodDelete:
		key := project + ":" + datasetId
		api.mu.Lock()
		_, ok := api.datasets[key]
		if ok {
			delete(api.datasets, key)
			api.saveLocked()
		}
		api.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Dataset "+datasetId+" not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tables
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeTables(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	datasetId := extractSegmentAfter(path, "datasets")
	tableId := extractSegmentAfter(path, "tables")

	switch r.Method {
	case http.MethodPost:
		var body struct {
			TableReference TableRef          `json:"tableReference"`
			Schema         *TableSchema      `json:"schema"`
			FriendlyName   string            `json:"friendlyName"`
			Description    string            `json:"description"`
			Labels         map[string]string `json:"labels"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.TableReference.TableId == "" {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "INVALID_ARGUMENT", "tableReference.tableId is required")
			return
		}
		tID := body.TableReference.TableId
		nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())
		t := &Table{
			Kind:             "bigquery#table",
			ID:               fmt.Sprintf("%s:%s.%s", project, datasetId, tID),
			TableReference:   TableRef{ProjectId: project, DatasetId: datasetId, TableId: tID},
			Schema:           body.Schema,
			FriendlyName:     body.FriendlyName,
			Description:      body.Description,
			Labels:           body.Labels,
			Location:         "US",
			CreationTime:     nowMs,
			LastModifiedTime: nowMs,
			NumRows:          "0",
			NumBytes:         "0",
			Type:             "TABLE",
			Etag:             newEtag(),
			SelfLink: fmt.Sprintf("https://bigquery.googleapis.com/bigquery/v2/projects/%s/datasets/%s/tables/%s",
				project, datasetId, tID),
		}
		key := tableKey(project, datasetId, tID)
		api.mu.Lock()
		api.tables[key] = t
		api.saveLocked()
		api.mu.Unlock()

		// Wire to DuckDB backend if enabled
		if api.backend.Enabled() && t.Schema != nil {
			if err := api.backend.CreateTable(project, datasetId, tID, t.Schema); err != nil {
				log.Printf("[Shim: BigQuery] CreateTable failed for %s.%s: %v", datasetId, tID, err)
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(t)

	case http.MethodGet:
		if tableId != "" {
			key := tableKey(project, datasetId, tableId)
			api.mu.RLock()
			t, ok := api.tables[key]
			api.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				writeError(w, 404, "NOT_FOUND", "Table "+tableId+" not found")
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(t)
		} else {
			prefix := tableKey(project, datasetId, "")
			api.mu.RLock()
			items := []*Table{}
			for k, v := range api.tables {
				if strings.HasPrefix(k, prefix) {
					items = append(items, v)
				}
			}
			api.mu.RUnlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"kind":       "bigquery#tableList",
				"totalItems": len(items),
				"tables":     items,
			})
		}

	case http.MethodPatch, http.MethodPut:
		// Same reasoning as datasets: an update that is not applied makes every
		// subsequent terraform plan propose it again.
		var body struct {
			Schema       *TableSchema      `json:"schema"`
			FriendlyName *string           `json:"friendlyName"`
			Description  *string           `json:"description"`
			Labels       map[string]string `json:"labels"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		key := tableKey(project, datasetId, tableId)
		api.mu.Lock()
		t, ok := api.tables[key]
		if ok {
			if body.Schema != nil {
				t.Schema = body.Schema
			}
			if body.FriendlyName != nil {
				t.FriendlyName = *body.FriendlyName
			}
			if body.Description != nil {
				t.Description = *body.Description
			}
			if body.Labels != nil {
				t.Labels = body.Labels
			}
			t.LastModifiedTime = fmt.Sprintf("%d", time.Now().UnixMilli())
			t.Etag = newEtag()
			api.saveLocked()
		}
		api.mu.Unlock()

		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Table "+tableId+" not found")
			return
		}
		if api.backend.Enabled() && body.Schema != nil {
			if err := api.backend.CreateTable(project, datasetId, tableId, body.Schema); err != nil {
				log.Printf("[Shim: BigQuery] CreateTable failed for %s.%s: %v", datasetId, tableId, err)
			}
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(t)

	case http.MethodDelete:
		key := tableKey(project, datasetId, tableId)
		api.mu.Lock()
		_, ok := api.tables[key]
		if ok {
			delete(api.tables, key)
			api.saveLocked()
		}
		api.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Table "+tableId+" not found")
			return
		}
		// Deleting the metadata without the data would leave a table that
		// tables.get denies but a query still returns rows from.
		if api.backend.Enabled() {
			if err := api.backend.DropTable(datasetId, tableId); err != nil {
				log.Printf("[Shim: BigQuery] DropTable failed for %s.%s: %v", datasetId, tableId, err)
			}
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// insertAll handles tabledata.insertAll (streaming inserts).
func (api *API) insertAll(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	datasetId := extractSegmentAfter(path, "datasets")
	tableId := extractSegmentAfter(path, "tables")

	var body struct {
		Rows []struct {
			InsertId string                 `json:"insertId"`
			Json     map[string]interface{} `json:"json"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}

	key := tableKey(project, datasetId, tableId)
	api.mu.Lock()
	if t, ok := api.tables[key]; ok {
		for _, row := range body.Rows {
			t.rows = append(t.rows, row.Json)
		}
		t.NumRows = fmt.Sprintf("%d", len(t.rows))
		api.saveLocked()
	}
	api.mu.Unlock()

	// GCP returns 200 with empty insertErrors on success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":         "bigquery#tableDataInsertAllResponse",
		"insertErrors": []interface{}{},
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Jobs
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeJobs(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	jobId := extractSegmentAfter(path, "jobs")

	switch r.Method {
	case http.MethodPost:
		api.insertJob(w, r, project)
	case http.MethodGet:
		if jobId != "" {
			api.getJob(w, project, jobId)
		} else {
			api.listJobs(w, project)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) insertJob(w http.ResponseWriter, r *http.Request, project string) {
	var body struct {
		JobReference  JobRef    `json:"jobReference"`
		Configuration JobConfig `json:"configuration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}

	jobId := body.JobReference.JobId
	if jobId == "" {
		jobId = fmt.Sprintf("job_minisky_%x", time.Now().UnixNano())
	}
	location := body.JobReference.Location
	if location == "" {
		location = "US"
	}
	nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())

	job := &Job{
		Kind: "bigquery#job",
		ID:   fmt.Sprintf("%s:%s", project, jobId),
		JobReference: JobRef{
			ProjectId: project,
			JobId:     jobId,
			Location:  location,
		},
		Configuration: body.Configuration,
		Status:        JobStatus{State: "RUNNING"},
		Statistics: JobStatistics{
			CreationTime:        nowMs,
			StartTime:           nowMs,
			TotalBytesProcessed: "0",
			TotalSlotMs:         "0",
		},
	}

	key := project + ":" + jobId
	api.mu.Lock()
	api.jobs[key] = job
	api.mu.Unlock()

	// Finish job asynchronously
	go api.runJob(project, key)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func (api *API) getJob(w http.ResponseWriter, project, jobId string) {
	key := project + ":" + jobId
	api.mu.RLock()
	job, ok := api.jobs[key]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Job "+jobId+" not found")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func (api *API) listJobs(w http.ResponseWriter, project string) {
	prefix := project + ":"
	api.mu.RLock()
	items := []*Job{}
	for k, v := range api.jobs {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind": "bigquery#jobList",
		"jobs": items,
	})
}

// runJob executes a job's configured work and records the outcome. Query jobs
// run against the DuckDB backend; load jobs ingest their source URI.
func (api *API) runJob(project, key string) {
	api.mu.RLock()
	job, ok := api.jobs[key]
	var cfg JobConfig
	if ok {
		cfg = job.Configuration
	}
	api.mu.RUnlock()
	if !ok {
		return
	}

	var execErr error
	var result *QueryResult

	if api.backend.Enabled() {
		// Give the SQL translator the dataset names it needs to resolve
		// `dataset.table` references before the statement runs.
		api.backend.SetKnownDatasets(api.datasetIDs(project))

		switch {
		case cfg.Query != nil && cfg.Query.Query != "":
			result, execErr = api.backend.ExecuteQueryWithSchema(cfg.Query.Query)
		case cfg.Load != nil && len(cfg.Load.SourceUris) > 0:
			load := cfg.Load
			execErr = api.backend.LoadData(
				load.DestinationTable.ProjectId,
				load.DestinationTable.DatasetId,
				load.DestinationTable.TableId,
				load.SourceUris[0],
				load.SourceFormat,
			)
		}
	} else {
		time.Sleep(500 * time.Millisecond) // Simulate mock execution
	}

	api.mu.Lock()
	if j, ok := api.jobs[key]; ok {
		j.Status.State = "DONE"
		j.Statistics.EndTime = fmt.Sprintf("%d", time.Now().UnixMilli())
		if execErr != nil {
			j.Status.ErrorResult = &ErrorProto{Reason: "backendError", Message: execErr.Error()}
		} else if result != nil {
			j.RawRows = result.Rows
			j.Schema = queryResultSchema(result)
		}
	}
	api.mu.Unlock()

	// A statement like CREATE TABLE AS — every dbt model — materialises a table
	// the metadata API has never seen. Reconcile so tables.get/list can find it.
	if execErr == nil && api.backend.Enabled() {
		api.reconcileTables(project)
	}
}

// datasetIDs lists the dataset names registered for a project.
func (api *API) datasetIDs(project string) []string {
	prefix := project + ":"
	api.mu.RLock()
	defer api.mu.RUnlock()

	ids := make([]string, 0, len(api.datasets))
	for key, ds := range api.datasets {
		if strings.HasPrefix(key, prefix) {
			ids = append(ids, ds.DatasetReference.DatasetId)
		}
	}
	return ids
}

// queryResultSchema converts the backend's ordered column list into a
// BigQuery table schema. Column order is preserved because the REST response
// renders each row positionally against it.
func queryResultSchema(result *QueryResult) *TableSchema {
	schema := &TableSchema{Fields: make([]FieldSchema, 0, len(result.Columns))}
	for _, column := range result.Columns {
		schema.Fields = append(schema.Fields, FieldSchema{
			Name: column.Name,
			Type: column.Type,
			Mode: "NULLABLE",
		})
	}
	return schema
}

// reconcileTables registers tables that exist in the backend but not in the
// metadata map, and refreshes the schema of ones that do.
func (api *API) reconcileTables(project string) {
	tables, err := api.backend.ListTables()
	if err != nil {
		log.Printf("[Shim: BigQuery] table reconciliation skipped: %v", err)
		return
	}

	nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())

	api.mu.Lock()
	defer api.mu.Unlock()

	for _, backendTable := range tables {
		key := tableKey(project, backendTable.Dataset, backendTable.Table)
		if existing, ok := api.tables[key]; ok {
			existing.Schema = backendTable.Schema
			existing.LastModifiedTime = nowMs
			continue
		}
		api.tables[key] = &Table{
			Kind:           "bigquery#table",
			ID:             fmt.Sprintf("%s:%s.%s", project, backendTable.Dataset, backendTable.Table),
			TableReference: TableRef{ProjectId: project, DatasetId: backendTable.Dataset, TableId: backendTable.Table},
			Schema:         backendTable.Schema,
			Description:    "Created by a query job",
			Location:       "US",
			CreationTime:   nowMs, LastModifiedTime: nowMs,
			NumRows: "0", NumBytes: "0",
			Type: "TABLE",
			Etag: newEtag(),
			SelfLink: fmt.Sprintf("https://bigquery.googleapis.com/bigquery/v2/projects/%s/datasets/%s/tables/%s",
				project, backendTable.Dataset, backendTable.Table),
		}
		log.Printf("[Shim: BigQuery] registered query-created table %s.%s", backendTable.Dataset, backendTable.Table)
	}
	api.saveLocked()
}

// routeQueries serves the official query surface:
//
//	POST /projects/{project}/queries           → jobs.query (synchronous)
//	GET  /projects/{project}/queries/{jobId}   → jobs.getQueryResults
//
// google-cloud clients — and therefore dbt, bq and every SDK — read results
// from here rather than from /jobs/{jobId}/results.
func (api *API) routeQueries(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	jobId := extractSegmentAfter(path, "queries")

	switch r.Method {
	case http.MethodGet:
		if jobId == "" {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "INVALID_ARGUMENT", "jobId is required")
			return
		}
		api.writeQueryResults(w, project, jobId)
	case http.MethodPost:
		api.query(w, r, project)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// query handles jobs.query — run the statement and return its rows in one call.
func (api *API) query(w http.ResponseWriter, r *http.Request, project string) {
	var body struct {
		Query          string      `json:"query"`
		UseLegacySql   *bool       `json:"useLegacySql"`
		DryRun         bool        `json:"dryRun"`
		Location       string      `json:"location"`
		RequestId      string      `json:"requestId"`
		DefaultDataset *DatasetRef `json:"defaultDataset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}
	if body.Query == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Field 'query' is required")
		return
	}

	location := body.Location
	if location == "" {
		location = "US"
	}
	jobId := fmt.Sprintf("job_minisky_%x", time.Now().UnixNano())
	nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())

	job := &Job{
		Kind:         "bigquery#job",
		ID:           fmt.Sprintf("%s:%s", project, jobId),
		JobReference: JobRef{ProjectId: project, JobId: jobId, Location: location},
		Configuration: JobConfig{
			JobType: "QUERY",
			Query:   &QueryConfig{Query: body.Query, DefaultDataset: body.DefaultDataset},
		},
		Status: JobStatus{State: "RUNNING"},
		Statistics: JobStatistics{
			CreationTime: nowMs, StartTime: nowMs,
			TotalBytesProcessed: "0", TotalSlotMs: "0",
		},
	}

	key := project + ":" + jobId
	api.mu.Lock()
	api.jobs[key] = job
	api.mu.Unlock()

	if body.DryRun {
		api.mu.Lock()
		job.Status.State = "DONE"
		api.mu.Unlock()
	} else {
		api.runJob(project, key) // synchronous: jobs.query returns the rows itself
	}

	api.writeQueryResults(w, project, jobId)
}

// getQueryResults serves MiniSky's original /jobs/{jobId}/results path, kept
// for backwards compatibility with clients written against it.
func (api *API) getQueryResults(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	jobId := extractSegmentAfter(path, "jobs")
	api.writeQueryResults(w, project, jobId)
}

// writeQueryResults renders a finished job's rows in the BigQuery REST shape.
func (api *API) writeQueryResults(w http.ResponseWriter, project, jobId string) {
	key := project + ":" + jobId

	api.mu.RLock()
	job, ok := api.jobs[key]
	api.mu.RUnlock()

	done := ok && job.Status.State == "DONE"

	schema := map[string]interface{}{"fields": []interface{}{}}
	outRows := []interface{}{}
	numRows := 0

	if ok && job.Schema != nil {
		fields := make([]interface{}, 0, len(job.Schema.Fields))
		for _, f := range job.Schema.Fields {
			fields = append(fields, map[string]interface{}{
				"name": f.Name,
				"type": f.Type,
				"mode": f.Mode,
			})
		}
		schema["fields"] = fields
	}

	if ok && job.RawRows != nil {
		numRows = len(job.RawRows)
		for _, rawRow := range job.RawRows {
			cells := []interface{}{}
			if job.Schema != nil {
				for _, f := range job.Schema.Fields {
					cells = append(cells, map[string]interface{}{"v": formatCell(rawRow[f.Name], f.Type)})
				}
			}
			outRows = append(outRows, map[string]interface{}{"f": cells})
		}
	}

	response := map[string]interface{}{
		"kind":                "bigquery#getQueryResultsResponse",
		"jobComplete":         done,
		"totalRows":           fmt.Sprintf("%d", numRows),
		"schema":              schema,
		"rows":                outRows,
		"totalBytesProcessed": "0",
		"cacheHit":            false,
		"jobReference": map[string]interface{}{
			"projectId": project,
			"jobId":     jobId,
			"location":  "US",
		},
	}

	if ok && job.Status.ErrorResult != nil {
		response["errors"] = []interface{}{job.Status.ErrorResult}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// formatCell renders a value the way the BigQuery REST API does: every cell is
// a string (or null), with dates, times and timestamps in their wire formats.
func formatCell(value interface{}, bqType string) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		switch bqType {
		case "DATE":
			return v.Format("2006-01-02")
		case "TIME":
			return v.Format("15:04:05")
		case "DATETIME":
			return v.Format("2006-01-02T15:04:05")
		default:
			// TIMESTAMP is reported as epoch seconds.
			return strconv.FormatFloat(float64(v.UnixNano())/1e9, 'f', -1, 64)
		}
	case []byte:
		return string(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

func tableKey(project, dataset, table string) string { return project + ":" + dataset + ":" + table }

func extractSegmentAfter(path, keyword string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == keyword && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func writeError(w http.ResponseWriter, code int, status, message string) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{"code": code, "status": status, "message": message},
	})
}

func newEtag() string {
	return fmt.Sprintf("BQETAG%x", time.Now().UnixNano())
}
