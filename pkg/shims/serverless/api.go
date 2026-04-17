package serverless

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
)

// ─────────────────────────────────────────────────────────────────────────────
// Resource types — Cloud Functions v2 & Cloud Run v2
// ─────────────────────────────────────────────────────────────────────────────

// Function mirrors the Cloud Functions v2 Function resource.
type Function struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	BuildConfig *BuildConfig      `json:"buildConfig,omitempty"`
	ServiceConfig *ServiceConfig  `json:"serviceConfig,omitempty"`
	State       string            `json:"state"` // DEPLOYING, ACTIVE, FAILED, DELETING
	StateMessages []StateMessage  `json:"stateMessages,omitempty"`
	UpdateTime  string            `json:"updateTime"`
	Labels      map[string]string `json:"labels,omitempty"`
	Url         string            `json:"url"`
}

type BuildConfig struct {
	Runtime       string `json:"runtime"` // nodejs20, python312, go122, etc.
	EntryPoint    string `json:"entryPoint"`
	Source        *Source `json:"source,omitempty"`
}

type Source struct {
	StorageSource *StorageSource  `json:"storageSource,omitempty"`
	RepoSource    *RepoSource     `json:"repoSource,omitempty"`
}

type StorageSource struct {
	Bucket string `json:"bucket"`
	Object string `json:"object"`
}

type RepoSource struct {
	ProjectId  string `json:"projectId"`
	RepoName   string `json:"repoName"`
	BranchName string `json:"branchName"`
}

type ServiceConfig struct {
	MaxInstanceCount int               `json:"maxInstanceCount"`
	MinInstanceCount int               `json:"minInstanceCount"`
	AvailableMemory  string            `json:"availableMemory"`
	TimeoutSeconds   int               `json:"timeoutSeconds"`
	EnvironmentVariables map[string]string `json:"environmentVariables,omitempty"`
	IngressSettings  string            `json:"ingressSettings"` // ALLOW_ALL, ALLOW_INTERNAL_ONLY
	Uri              string            `json:"uri"`
}

type StateMessage struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

// Service mirrors the Cloud Run v2 Service resource.
type Service struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Uid           string            `json:"uid"`
	Generation    string            `json:"generation"`
	Labels        map[string]string `json:"labels,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	CreateTime    string            `json:"createTime"`
	UpdateTime    string            `json:"updateTime"`
	Creator       string            `json:"creator"`
	LastModifier  string            `json:"lastModifier"`
	Ingress       string            `json:"ingress"` // INGRESS_TRAFFIC_ALL, INGRESS_TRAFFIC_INTERNAL_ONLY
	LaunchStage   string            `json:"launchStage"` // GA, BETA
	Template      *RevisionTemplate `json:"template,omitempty"`
	TrafficStatuses []TrafficStatus `json:"trafficStatuses,omitempty"`
	Uri           string            `json:"uri"`
	// Reconciling is true during deploy
	Reconciling bool   `json:"reconciling"`
	Conditions  []Condition `json:"conditions,omitempty"`
	ObservedGeneration string `json:"observedGeneration"`
}

type RevisionTemplate struct {
	Containers []Container       `json:"containers,omitempty"`
	Scaling    *ScalingConfig    `json:"scaling,omitempty"`
	MaxInstanceRequestConcurrency int `json:"maxInstanceRequestConcurrency,omitempty"`
}

type Container struct {
	Image   string            `json:"image"`
	Env     []EnvVar          `json:"env,omitempty"`
	Resources *ResourceRequirements `json:"resources,omitempty"`
	Ports   []ContainerPort   `json:"ports,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

type ResourceRequirements struct {
	Limits map[string]string `json:"limits,omitempty"`
}

type ContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int    `json:"containerPort"`
}

type ScalingConfig struct {
	MinInstanceCount int `json:"minInstanceCount"`
	MaxInstanceCount int `json:"maxInstanceCount"`
}

type TrafficStatus struct {
	Type     string `json:"type"`
	Revision string `json:"revision,omitempty"`
	Percent  int    `json:"percent"`
}

type Condition struct {
	Type               string `json:"type"`
	State              string `json:"state"` // CONDITION_SUCCEEDED, CONDITION_FAILED, CONDITION_RECONCILING
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime"`
	Severity           string `json:"severity,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

// API handles both cloudfunctions.googleapis.com and run.googleapis.com traffic.
type API struct {
	mu        sync.RWMutex
	opMgr     *orchestrator.OperationManager
	backend   *BuildpacksBackend
	functions map[string]*Function // key: project:location:name
	services  map[string]*Service  // key: project:location:name
}

func NewAPI(opMgr *orchestrator.OperationManager) *API {
	return &API{
		opMgr:     opMgr,
		backend:   NewBuildpacksBackend(),
		functions: make(map[string]*Function),
		services:  make(map[string]*Service),
	}
}

// GetBackend exposes the backend for dynamic dashboard configuration.
func (api *API) GetBackend() *BuildpacksBackend {
	return api.backend
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Serverless] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/functions"):
		api.routeFunctions(w, r, path)
	case strings.Contains(path, "/services"):
		api.routeServices(w, r, path)
	case strings.Contains(path, "/operations/"):
		api.getOperation(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Serverless resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Cloud Functions
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeFunctions(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	location := firstOf(extractSegmentAfter(path, "locations"), "us-central1")
	fnName := extractSegmentAfter(path, "functions")

	switch r.Method {
	case http.MethodPost:
		api.createFunction(w, r, project, location)
	case http.MethodGet:
		if fnName != "" {
			api.getFunction(w, project, location, fnName)
		} else {
			api.listFunctions(w, project, location)
		}
	case http.MethodDelete:
		api.deleteFunction(w, r, project, location, fnName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createFunction(w http.ResponseWriter, r *http.Request, project, location string) {
	functionId := r.URL.Query().Get("functionId")
	var body Function
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}

	if functionId == "" {
		// Extract name from resource name if provided
		parts := strings.Split(body.Name, "/")
		if len(parts) > 0 {
			functionId = parts[len(parts)-1]
		}
	}
	if functionId == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "functionId query param or name is required")
		return
	}

	fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, functionId)
	serviceUri := fmt.Sprintf("http://localhost:8080/%s", functionId)

	fn := &Function{
		Name:        fullName,
		Description: body.Description,
		BuildConfig: body.BuildConfig,
		ServiceConfig: body.ServiceConfig,
		State:       "DEPLOYING",
		UpdateTime:  time.Now().UTC().Format(time.RFC3339),
		Labels:      body.Labels,
		Url:         serviceUri,
	}
	if fn.ServiceConfig == nil {
		fn.ServiceConfig = &ServiceConfig{
			MaxInstanceCount: 100,
			AvailableMemory:  "256Mi",
			TimeoutSeconds:   60,
			IngressSettings:  "ALLOW_ALL",
			Uri:              serviceUri,
		}
	}

	key := serverlessKey(project, location, functionId)
	api.mu.Lock()
	api.functions[key] = fn
	api.mu.Unlock()

	op := api.opMgr.Register("cloudfunctions#operation", "CREATE", fullName, "", location)
	api.opMgr.RunAsync(op.Name, func() error {
		time.Sleep(2 * time.Second)
		api.mu.Lock()
		if f, ok := api.functions[key]; ok {
			f.State = "ACTIVE"
		}
		api.mu.Unlock()
		return nil
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toCloudFunctionsLRO(op, project, location))
}

func (api *API) getFunction(w http.ResponseWriter, project, location, name string) {
	key := serverlessKey(project, location, name)
	api.mu.RLock()
	fn, ok := api.functions[key]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Function "+name+" not found")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(fn)
}

func (api *API) listFunctions(w http.ResponseWriter, project, location string) {
	prefix := serverlessKey(project, location, "")
	api.mu.RLock()
	items := []*Function{}
	for k, v := range api.functions {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"functions": items,
	})
}

func (api *API) deleteFunction(w http.ResponseWriter, r *http.Request, project, location, name string) {
	key := serverlessKey(project, location, name)
	api.mu.Lock()
	_, ok := api.functions[key]
	if ok {
		delete(api.functions, key)
	}
	api.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Function "+name+" not found")
		return
	}
	fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, name)
	op := api.opMgr.Register("cloudfunctions#operation", "DELETE", fullName, "", location)
	api.opMgr.RunAsync(op.Name, func() error { return nil })
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toCloudFunctionsLRO(op, project, location))
}

// ─────────────────────────────────────────────────────────────────────────────
// Cloud Run Services
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeServices(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	location := firstOf(extractSegmentAfter(path, "locations"), "us-central1")
	svcName := extractSegmentAfter(path, "services")

	switch r.Method {
	case http.MethodPost:
		api.createService(w, r, project, location)
	case http.MethodGet:
		if svcName != "" {
			api.getService(w, project, location, svcName)
		} else {
			api.listServices(w, project, location)
		}
	case http.MethodDelete:
		api.deleteService(w, r, project, location, svcName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createService(w http.ResponseWriter, r *http.Request, project, location string) {
	serviceId := r.URL.Query().Get("serviceId")
	var body Service
	json.NewDecoder(r.Body).Decode(&body)
	if serviceId == "" {
		parts := strings.Split(body.Name, "/")
		if len(parts) > 0 {
			serviceId = parts[len(parts)-1]
		}
	}
	if serviceId == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "serviceId is required")
		return
	}

	fullName := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceId)
	svcUri := fmt.Sprintf("https://%s-%s.run.app", serviceId, project)

	svc := &Service{
		Name:         fullName,
		Description:  body.Description,
		Uid:          fmt.Sprintf("%x", time.Now().UnixNano()),
		Generation:   "1",
		Labels:       body.Labels,
		Annotations:  body.Annotations,
		CreateTime:   time.Now().UTC().Format(time.RFC3339),
		UpdateTime:   time.Now().UTC().Format(time.RFC3339),
		Creator:      "minisky-local@local-dev.iam.gserviceaccount.com",
		LastModifier: "minisky-local@local-dev.iam.gserviceaccount.com",
		Ingress:      firstOf(body.Ingress, "INGRESS_TRAFFIC_ALL"),
		LaunchStage:  "GA",
		Template:     body.Template,
		Reconciling:  true,
		Uri:          svcUri,
		ObservedGeneration: "0",
	}

	key := serverlessKey(project, location, serviceId)
	api.mu.Lock()
	api.services[key] = svc
	api.mu.Unlock()

	op := api.opMgr.Register("run#operation", "CREATE", fullName, "", location)
	api.opMgr.RunAsync(op.Name, func() error {
		time.Sleep(2 * time.Second)
		api.mu.Lock()
		if s, ok := api.services[key]; ok {
			s.Reconciling = false
			s.ObservedGeneration = "1"
			s.Conditions = []Condition{{
				Type:               "Ready",
				State:              "CONDITION_SUCCEEDED",
				LastTransitionTime: time.Now().UTC().Format(time.RFC3339),
			}}
			s.TrafficStatuses = []TrafficStatus{
				{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", Percent: 100},
			}
		}
		api.mu.Unlock()
		return nil
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toRunLRO(op, project, location))
}

func (api *API) getService(w http.ResponseWriter, project, location, name string) {
	key := serverlessKey(project, location, name)
	api.mu.RLock()
	svc, ok := api.services[key]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Service "+name+" not found")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(svc)
}

func (api *API) listServices(w http.ResponseWriter, project, location string) {
	prefix := serverlessKey(project, location, "")
	api.mu.RLock()
	items := []*Service{}
	for k, v := range api.services {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"services": items})
}

func (api *API) deleteService(w http.ResponseWriter, r *http.Request, project, location, name string) {
	key := serverlessKey(project, location, name)
	api.mu.Lock()
	_, ok := api.services[key]
	if ok {
		delete(api.services, key)
	}
	api.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Service "+name+" not found")
		return
	}
	fullName := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, name)
	op := api.opMgr.Register("run#operation", "DELETE", fullName, "", location)
	api.opMgr.RunAsync(op.Name, func() error { return nil })
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toRunLRO(op, project, location))
}

// ─────────────────────────────────────────────────────────────────────────────
// Operations
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) getOperation(w http.ResponseWriter, r *http.Request, path string) {
	opName := extractSegmentAfter(path, "operations")
	op := api.opMgr.Get(opName)
	if op == nil {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Operation not found: "+opName)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":     op.Name,
		"done":     op.Done,
		"metadata": map[string]string{"@type": "type.googleapis.com/google.cloud.functions.v2.OperationMetadata"},
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

func toCloudFunctionsLRO(op *orchestrator.Operation, project, location string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, op.Name),
		"metadata": map[string]string{
			"@type": "type.googleapis.com/google.cloud.functions.v2.OperationMetadata",
		},
		"done": op.Done,
	}
}

func toRunLRO(op *orchestrator.Operation, project, location string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, op.Name),
		"metadata": map[string]string{
			"@type": "type.googleapis.com/google.cloud.run.v2.Service",
		},
		"done": op.Done,
	}
}

func serverlessKey(project, location, name string) string {
	return project + ":" + location + ":" + name
}

func extractSegmentAfter(path, keyword string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == keyword && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func writeError(w http.ResponseWriter, code int, status, message string) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{"code": code, "status": status, "message": message},
	})
}
