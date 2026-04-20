package dataproc

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/config"
	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("dataproc.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.OpMgr, ctx.SvcMgr)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// Cluster mirrors the Dataproc v1 Cluster resource.
type Cluster struct {
	ProjectId   string        `json:"projectId"`
	ClusterName string        `json:"clusterName"`
	ClusterUuid string        `json:"clusterUuid"`
	Config      ClusterConfig `json:"config"`
	Status      ClusterStatus `json:"status"`
	StatusHistory []ClusterStatus `json:"statusHistory,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type ClusterConfig struct {
	MasterConfig   *InstanceGroupConfig `json:"masterConfig,omitempty"`
	WorkerConfig   *InstanceGroupConfig `json:"workerConfig,omitempty"`
	SoftwareConfig *SoftwareConfig      `json:"softwareConfig,omitempty"`
}

type InstanceGroupConfig struct {
	NumInstances   int    `json:"numInstances"`
	MachineTypeUri string `json:"machineTypeUri"`
	DiskConfig     *DiskConfig `json:"diskConfig,omitempty"`
}

type DiskConfig struct {
	BootDiskSizeGb int `json:"bootDiskSizeGb"`
}

type SoftwareConfig struct {
	ImageVersion string            `json:"imageVersion"`
	Properties   map[string]string `json:"properties,omitempty"`
}

type ClusterStatus struct {
	State          string `json:"state"` // CREATING, RUNNING, DELETING, ERROR
	Detail         string `json:"detail,omitempty"`
	StateStartTime string `json:"stateStartTime"`
}

// Job mirrors the Dataproc v1 Job resource.
type Job struct {
	Reference  JobReference `json:"reference"`
	Placement  JobPlacement `json:"placement"`
	Status     JobStatus    `json:"status"`
	SparkJob   *SparkJob    `json:"sparkJob,omitempty"`
	PysparkJob *PySparkJob  `json:"pysparkJob,omitempty"`
	HiveJob    *HiveJob     `json:"hiveJob,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type JobReference struct {
	ProjectId string `json:"projectId"`
	JobId     string `json:"jobId"`
}

type JobPlacement struct {
	ClusterName string `json:"clusterName"`
	ClusterUuid string `json:"clusterUuid,omitempty"`
}

type JobStatus struct {
	State          string `json:"state"` // PENDING, SETUP_DONE, RUNNING, DONE, ERROR
	StateStartTime string `json:"stateStartTime"`
	Details        string `json:"details,omitempty"`
}

type SparkJob struct {
	MainClass  string   `json:"mainClass,omitempty"`
	MainJarFileUri string `json:"mainJarFileUri,omitempty"`
	Args       []string `json:"args,omitempty"`
	JarFileUris []string `json:"jarFileUris,omitempty"`
}

type PySparkJob struct {
	MainPythonFileUri string   `json:"mainPythonFileUri"`
	Args              []string `json:"args,omitempty"`
	PythonFileUris    []string `json:"pythonFileUris,omitempty"`
}

type HiveJob struct {
	QueryList *QueryList `json:"queryList,omitempty"`
	QueryFileUri string  `json:"queryFileUri,omitempty"`
}

type QueryList struct {
	Queries []string `json:"queries"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

// API is the high-fidelity Dataproc v1 shim.
type API struct {
	mu       sync.RWMutex
	opMgr    *orchestrator.OperationManager
	svcMgr   *orchestrator.ServiceManager
	clusters map[string]*Cluster // key: project:region:clusterName
	jobs     map[string]*Job     // key: project:region:jobId
}

func NewAPI(opMgr *orchestrator.OperationManager, svcMgr *orchestrator.ServiceManager) *API {
	return &API{
		opMgr:    opMgr,
		svcMgr:   svcMgr,
		clusters: make(map[string]*Cluster),
		jobs:     make(map[string]*Job),
	}
}

// ServeHTTP dispatches Dataproc v1 paths.
//
// Supported paths (dataproc.googleapis.com):
//   POST   /v1/projects/{project}/regions/{region}/clusters
//   GET    /v1/projects/{project}/regions/{region}/clusters
//   GET    /v1/projects/{project}/regions/{region}/clusters/{cluster}
//   DELETE /v1/projects/{project}/regions/{region}/clusters/{cluster}
//   POST   /v1/projects/{project}/regions/{region}/jobs:submit
//   GET    /v1/projects/{project}/regions/{region}/jobs
//   GET    /v1/projects/{project}/regions/{region}/jobs/{jobId}
//   GET    /v1/projects/{project}/regions/{region}/operations/{operation}
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Dataproc] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/operations/"):
		api.getOperation(w, r, path)
	case strings.Contains(path, "/jobs"):
		api.routeJobs(w, r, path)
	case strings.Contains(path, "/clusters"):
		api.routeClusters(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Dataproc resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Clusters
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeClusters(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	region := extractSegmentAfter(path, "regions")
	clusterName := extractSegmentAfter(path, "clusters")

	switch r.Method {
	case http.MethodPost:
		api.createCluster(w, r, project, region)
	case http.MethodGet:
		if clusterName != "" {
			api.getCluster(w, project, region, clusterName)
		} else {
			api.listClusters(w, project, region)
		}
	case http.MethodDelete:
		api.deleteCluster(w, r, project, region, clusterName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createCluster(w http.ResponseWriter, r *http.Request, project, region string) {
	var body struct {
		ClusterName string        `json:"clusterName"`
		Config      ClusterConfig `json:"config"`
		Labels      map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}
	if body.ClusterName == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "clusterName is required")
		return
	}

	// Defaults
	cfg := body.Config
	if cfg.MasterConfig == nil {
		cfg.MasterConfig = &InstanceGroupConfig{
			NumInstances:   1,
			MachineTypeUri: "n1-standard-4",
			DiskConfig:     &DiskConfig{BootDiskSizeGb: 500},
		}
	}
	if cfg.WorkerConfig == nil {
		cfg.WorkerConfig = &InstanceGroupConfig{
			NumInstances:   2,
			MachineTypeUri: "n1-standard-4",
			DiskConfig:     &DiskConfig{BootDiskSizeGb: 500},
		}
	}
	if cfg.SoftwareConfig == nil {
		cfg.SoftwareConfig = &SoftwareConfig{ImageVersion: "2.1-debian11"}
	}

	clusterUuid := fmt.Sprintf("%x-%x", time.Now().UnixNano(), time.Now().UnixNano()/2)
	cl := &Cluster{
		ProjectId:   project,
		ClusterName: body.ClusterName,
		ClusterUuid: clusterUuid,
		Config:      cfg,
		Labels:      body.Labels,
		Status: ClusterStatus{
			State:          "CREATING",
			StateStartTime: time.Now().UTC().Format(time.RFC3339),
		},
	}

	key := clusterKey(project, region, body.ClusterName)
	api.mu.Lock()
	api.clusters[key] = cl
	api.mu.Unlock()

	targetLink := fmt.Sprintf(
		"https://dataproc.googleapis.com/v1/projects/%s/regions/%s/clusters/%s",
		project, region, body.ClusterName)
	op := api.opMgr.Register("dataproc#operation", "CREATE", targetLink, "", region)

	api.opMgr.RunAsync(op.Name, func() error {
		clusterStr := body.ClusterName
		reg := config.GetImageRegistry()

		// Provision the Master Node
		masterImage := reg.Dataproc.DefaultImage
		reqVersion := cfg.SoftwareConfig.ImageVersion
		for _, v := range reg.Dataproc.Versions {
			if strings.Contains(reqVersion, v.Version) {
				masterImage = v.Image
				break
			}
		}

		// Connectivity configuration for Cloud Storage and BigQuery emulators
		connectivityEnv := []string{
			"SPARK_HADOOP_fs_gs_impl=com.google.cloud.hadoop.fs.gcs.GoogleHadoopFileSystem",
			"SPARK_HADOOP_google_cloud_auth_service_account_enable=false",
			"SPARK_HADOOP_fs_gs_endpoint=http://minisky-gcs:4443",
			"BIGQUERY_REST_ENDPOINT=http://host.docker.internal:8080/bigquery/v2",
		}

		masterName := fmt.Sprintf("minisky-dataproc-%s-m", clusterStr)
		api.svcMgr.ProvisionComputeVM(masterName, masterImage, "default", reg.Dataproc.MasterPorts, connectivityEnv)

		// Provision Worker Nodes
		numWorkers := 2
		if cfg.WorkerConfig != nil {
			numWorkers = cfg.WorkerConfig.NumInstances
		}
		for i := 0; i < numWorkers; i++ {
			workerName := fmt.Sprintf("minisky-dataproc-%s-w-%d", clusterStr, i)
			api.svcMgr.ProvisionComputeVM(workerName, masterImage, "default", []string{}, connectivityEnv)
		}

		api.mu.Lock()
		if c, ok := api.clusters[key]; ok {
			c.Status.State = "RUNNING"
			c.Status.StateStartTime = time.Now().UTC().Format(time.RFC3339)
		}
		api.mu.Unlock()
		return nil
	})

	// Dataproc uses google.longrunning.Operation format
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toLRO(op, project, region))
}

func (api *API) getCluster(w http.ResponseWriter, project, region, name string) {
	key := clusterKey(project, region, name)
	api.mu.RLock()
	cl, ok := api.clusters[key]
	api.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Cluster "+name+" not found")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cl)
}

func (api *API) listClusters(w http.ResponseWriter, project, region string) {
	prefix := clusterKey(project, region, "")
	api.mu.RLock()
	items := []*Cluster{}
	for k, v := range api.clusters {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"clusters": items})
}

func (api *API) deleteCluster(w http.ResponseWriter, r *http.Request, project, region, name string) {
	key := clusterKey(project, region, name)
	api.mu.Lock()
	cl, ok := api.clusters[key]
	if ok {
		cl.Status.State = "DELETING"
		// Delay actual map deletion to allow UI to see DELETING state briefly, 
		// but since we want to align with previous exact behavior, we just copy properties.
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Cluster "+name+" not found")
		return
	}

	numWorkers := 2
	if cl.Config.WorkerConfig != nil {
		numWorkers = cl.Config.WorkerConfig.NumInstances
	}

	targetLink := fmt.Sprintf(
		"https://dataproc.googleapis.com/v1/projects/%s/regions/%s/clusters/%s",
		project, region, name)
	op := api.opMgr.Register("dataproc#operation", "DELETE", targetLink, "", region)
	
	api.opMgr.RunAsync(op.Name, func() error { 
		// Teardown physical containers
		api.svcMgr.DeleteComputeVM(fmt.Sprintf("minisky-dataproc-%s-m", name))
		for i := 0; i < numWorkers; i++ {
			api.svcMgr.DeleteComputeVM(fmt.Sprintf("minisky-dataproc-%s-w-%d", name, i))
		}

		api.mu.Lock()
		delete(api.clusters, key)
		api.mu.Unlock()
		return nil 
	})
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toLRO(op, project, region))
}

// ─────────────────────────────────────────────────────────────────────────────
// Jobs
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeJobs(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	region := extractSegmentAfter(path, "regions")

	// jobs:submit is a POST with a colon-verb
	if strings.HasSuffix(strings.TrimRight(path, "/"), ":submit") || r.Method == http.MethodPost {
		api.submitJob(w, r, project, region)
		return
	}

	jobId := extractSegmentAfter(path, "jobs")
	if r.Method == http.MethodGet && jobId != "" {
		api.getJob(w, project, region, jobId)
		return
	}

	if r.Method == http.MethodGet {
		api.listJobs(w, project, region)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (api *API) submitJob(w http.ResponseWriter, r *http.Request, project, region string) {
	var body struct {
		Job Job `json:"job"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}

	job := body.Job
	jobId := fmt.Sprintf("job-%x", time.Now().UnixNano())
	job.Reference.ProjectId = project
	job.Reference.JobId = jobId
	job.Status = JobStatus{
		State:          "PENDING",
		StateStartTime: time.Now().UTC().Format(time.RFC3339),
	}

	key := jobKey(project, region, jobId)
	api.mu.Lock()
	api.jobs[key] = &job
	api.mu.Unlock()

	// Drive job state: PENDING → RUNNING → DONE
	go func() {
		time.Sleep(500 * time.Millisecond)
		api.mu.Lock()
		j, ok := api.jobs[key]
		if !ok {
			api.mu.Unlock()
			return
		}
		j.Status.State = "RUNNING"
		j.Status.StateStartTime = time.Now().UTC().Format(time.RFC3339)
		clusterName := j.Placement.ClusterName
		api.mu.Unlock()

		masterName := fmt.Sprintf("minisky-dataproc-%s-m", clusterName)
		
		var cmd []string
		if j.PysparkJob != nil {
			cmd = []string{"spark-submit", "--master", "spark://localhost:7077", j.PysparkJob.MainPythonFileUri}
			cmd = append(cmd, j.PysparkJob.Args...)
		} else if j.SparkJob != nil {
			cmd = []string{"spark-submit", "--master", "spark://localhost:7077"}
			if j.SparkJob.MainClass != "" {
				cmd = append(cmd, "--class", j.SparkJob.MainClass)
			}
			cmd = append(cmd, j.SparkJob.MainJarFileUri)
			cmd = append(cmd, j.SparkJob.Args...)
		} else {
			// No-op for unsupported types or mocks
			time.Sleep(2 * time.Second)
		}

		if len(cmd) > 0 {
			out, err := api.svcMgr.RunCommandInContainer(masterName, cmd)
			api.mu.Lock()
			if j, ok := api.jobs[key]; ok {
				if err != nil {
					j.Status.State = "ERROR"
					j.Status.Details = fmt.Sprintf("Spark-submit failed: %v\nOutput: %s", err, out)
				} else {
					j.Status.State = "DONE"
					j.Status.Details = out
				}
				j.Status.StateStartTime = time.Now().UTC().Format(time.RFC3339)
			}
			api.mu.Unlock()
		} else {
			api.mu.Lock()
			if j, ok := api.jobs[key]; ok {
				j.Status.State = "DONE"
			}
			api.mu.Unlock()
		}
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&job)
}

func (api *API) getJob(w http.ResponseWriter, project, region, jobId string) {
	key := jobKey(project, region, jobId)
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

func (api *API) listJobs(w http.ResponseWriter, project, region string) {
	prefix := jobKey(project, region, "")
	api.mu.RLock()
	items := []*Job{}
	for k, v := range api.jobs {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"jobs": items})
}

// ─────────────────────────────────────────────────────────────────────────────
// Operations
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) getOperation(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	region := extractSegmentAfter(path, "regions")
	opName := extractSegmentAfter(path, "operations")

	op := api.opMgr.Get(opName)
	if op == nil {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Operation not found: "+opName)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toLRO(op, project, region))
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

func toLRO(op *orchestrator.Operation, project, region string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("projects/%s/regions/%s/operations/%s", project, region, op.Name),
		"metadata": map[string]interface{}{
			"@type":       "type.googleapis.com/google.cloud.dataproc.v1.ClusterOperationMetadata",
			"clusterName": "",
			"status": map[string]interface{}{
				"state": string(op.Status),
			},
		},
		"done":  op.Done,
		"error": op.Error,
	}
}

func clusterKey(project, region, name string) string { return project + ":" + region + ":" + name }
func jobKey(project, region, id string) string       { return project + ":" + region + ":" + id }

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
