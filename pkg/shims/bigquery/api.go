package bigquery

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("bigquery.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.OpMgr)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// Dataset mirrors the BigQuery Dataset resource.
type Dataset struct {
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	DatasetReference DatasetRef  `json:"datasetReference"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Location    string            `json:"location"`
	CreationTime string           `json:"creationTime"`
	LastModifiedTime string       `json:"lastModifiedTime"`
	Etag        string            `json:"etag"`
	SelfLink    string            `json:"selfLink"`
}

type DatasetRef struct {
	ProjectId string `json:"projectId"`
	DatasetId string `json:"datasetId"`
}

// Table mirrors the BigQuery Table resource.
type Table struct {
	Kind             string     `json:"kind"`
	ID               string     `json:"id"`
	TableReference   TableRef   `json:"tableReference"`
	Schema           *TableSchema `json:"schema,omitempty"`
	Description      string     `json:"description,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Location         string     `json:"location"`
	CreationTime     string     `json:"creationTime"`
	LastModifiedTime string     `json:"lastModifiedTime"`
	NumRows          string     `json:"numRows"`
	NumBytes         string     `json:"numBytes"`
	Type             string     `json:"type"` // TABLE, VIEW, EXTERNAL
	Etag             string     `json:"etag"`
	SelfLink         string     `json:"selfLink"`
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
	Name        string       `json:"name"`
	Type        string       `json:"type"` // STRING, INTEGER, FLOAT, BOOLEAN, RECORD, TIMESTAMP, DATE, etc.
	Mode        string       `json:"mode"` // NULLABLE, REQUIRED, REPEATED
	Description string       `json:"description,omitempty"`
	Fields      []FieldSchema `json:"fields,omitempty"` // nested RECORD
}

// Job mirrors the BigQuery Job resource.
type Job struct {
	Kind         string       `json:"kind"`
	ID           string       `json:"id"`
	JobReference JobRef       `json:"jobReference"`
	Status       JobStatus    `json:"status"`
	Statistics   JobStatistics `json:"statistics"`
	Configuration JobConfig   `json:"configuration"`

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
	State string      `json:"state"` // PENDING, RUNNING, DONE
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
}

type QueryConfig struct {
	Query                   string `json:"query"`
	UseLegacySql            bool   `json:"useLegacySql"`
	DefaultDataset          *DatasetRef `json:"defaultDataset,omitempty"`
	DestinationTable        *TableRef   `json:"destinationTable,omitempty"`
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
	return &API{
		opMgr:    opMgr,
		backend:  NewDuckDBBackend(),
		datasets: make(map[string]*Dataset),
		tables:   make(map[string]*Table),
		jobs:     make(map[string]*Job),
	}
}

// GetBackend exposes the backend for dynamic dashboard configuration.
func (api *API) GetBackend() *DuckDBBackend {
	return api.backend
}

// ServeHTTP dispatches BigQuery v2 paths.
//
// Supported paths (bigquery.googleapis.com):
//   POST   /bigquery/v2/projects/{project}/datasets
//   GET    /bigquery/v2/projects/{project}/datasets
//   GET    /bigquery/v2/projects/{project}/datasets/{dataset}
//   DELETE /bigquery/v2/projects/{project}/datasets/{dataset}
//   POST   /bigquery/v2/projects/{project}/datasets/{dataset}/tables
//   GET    /bigquery/v2/projects/{project}/datasets/{dataset}/tables
//   GET    /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}
//   DELETE /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}
//   POST   /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/insertAll
//   POST   /bigquery/v2/projects/{project}/jobs
//   GET    /bigquery/v2/projects/{project}/jobs/{jobId}
//   GET    /bigquery/v2/projects/{project}/jobs/{jobId}/results (getQueryResults)
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: BigQuery] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/insertAll"):
		api.insertAll(w, r, path)
	case strings.Contains(path, "/tables") && strings.Contains(path, "/datasets"):
		api.routeTables(w, r, path)
	case strings.Contains(path, "/datasets"):
		api.routeDatasets(w, r, path)
	case strings.Contains(path, "/jobs") && strings.Contains(path, "/results"):
		api.getQueryResults(w, r, path)
	case strings.Contains(path, "/jobs"):
		api.routeJobs(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "BigQuery resource not found: "+path)
	}
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
		api.mu.Unlock()
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

	case http.MethodDelete:
		key := project + ":" + datasetId
		api.mu.Lock()
		_, ok := api.datasets[key]
		if ok {
			delete(api.datasets, key)
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
			TableReference TableRef     `json:"tableReference"`
			Schema         *TableSchema `json:"schema"`
			Description    string       `json:"description"`
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

	case http.MethodDelete:
		key := tableKey(project, datasetId, tableId)
		api.mu.Lock()
		_, ok := api.tables[key]
		if ok {
			delete(api.tables, key)
		}
		api.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Table "+tableId+" not found")
			return
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
		Status: JobStatus{State: "RUNNING"},
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
	go func() {
		var execErr error
		var rows []map[string]interface{}
		
		if api.backend.Enabled() && body.Configuration.Query != nil && body.Configuration.Query.Query != "" {
			rows, execErr = api.backend.ExecuteQuery(body.Configuration.Query.Query)
		} else {
			time.Sleep(500 * time.Millisecond) // Simulate mock execution
		}

		api.mu.Lock()
		if j, ok := api.jobs[key]; ok {
			j.Status.State = "DONE"
			j.Statistics.EndTime = fmt.Sprintf("%d", time.Now().UnixMilli())
			if execErr != nil {
				j.Status.ErrorResult = &ErrorProto{Reason: "backendError", Message: execErr.Error()}
			} else if rows != nil {
				j.RawRows = rows
				// Dynamically build schema from first row
				if len(rows) > 0 {
					schema := &TableSchema{Fields: []FieldSchema{}}
					for colName := range rows[0] {
						schema.Fields = append(schema.Fields, FieldSchema{
							Name: colName,
							Type: "STRING", // fallback dynamic mapping
							Mode: "NULLABLE",
						})
					}
					j.Schema = schema
				}
			}
		}
		api.mu.Unlock()
	}()

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

// getQueryResults returns rows stored in the destination table (if available).
func (api *API) getQueryResults(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	jobId := extractSegmentAfter(path, "jobs")
	key := project + ":" + jobId

	api.mu.RLock()
	job, ok := api.jobs[key]
	api.mu.RUnlock()

	done := false
	if ok && job.Status.State == "DONE" {
		done = true
	}

	var schema map[string]interface{} = map[string]interface{}{"fields": []interface{}{}}
	var outRows []interface{} = []interface{}{}
	var numRows int = 0

	if ok && job.Schema != nil {
		schemaFields := []interface{}{}
		for _, f := range job.Schema.Fields {
			schemaFields = append(schemaFields, map[string]interface{}{
				"name": f.Name,
				"type": f.Type,
				"mode": f.Mode,
			})
		}
		schema["fields"] = schemaFields
	}

	if ok && job.RawRows != nil {
		numRows = len(job.RawRows)
		for _, rawRow := range job.RawRows {
			var fList []interface{}
			if job.Schema != nil {
				for _, f := range job.Schema.Fields {
					fList = append(fList, map[string]interface{}{"v": fmt.Sprintf("%v", rawRow[f.Name])})
				}
			}
			outRows = append(outRows, map[string]interface{}{"f": fList})
		}
	}

	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"kind":                "bigquery#getQueryResultsResponse",
		"jobComplete":         done,
		"totalRows":           fmt.Sprintf("%d", numRows),
		"schema":              schema,
		"rows":                outRows,
		"totalBytesProcessed": "0",
	}
	
	if ok && job.Status.ErrorResult != nil {
		response["errors"] = []interface{}{job.Status.ErrorResult}
	}

	json.NewEncoder(w).Encode(response)
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
