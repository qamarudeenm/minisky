package appengine

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
	"minisky/pkg/shims/logging"
	"minisky/pkg/shims/serverless"
)

var (
	singletonAPI *API
	once         sync.Once
)

func init() {
	f := func(ctx *registry.Context) http.Handler {
		once.Do(func() {
			// App Engine needs access to the Serverless backend for Buildpacks
			var serverlessAPI *serverless.API
			if s, ok := ctx.GetShim("cloudfunctions.googleapis.com").(*serverless.API); ok {
				serverlessAPI = s
			}
			// App Engine emits structured logs into Cloud Logging
			var logAPI *logging.API
			if l, ok := ctx.GetShim("logging.googleapis.com").(*logging.API); ok {
				logAPI = l
			}
			singletonAPI = NewAPI(ctx.OpMgr, ctx.SvcMgr, serverlessAPI, logAPI)
		})
		return singletonAPI
	}
	registry.Register("appengine.googleapis.com", f)
}

// AppEngine Resources
type App struct {
	Id              string `json:"id"`
	LocationId      string `json:"locationId"`
	DefaultHostname string `json:"defaultHostname"`
}

type Service struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type Version struct {
	Id           string            `json:"id"`
	Name         string            `json:"name"`
	Runtime      string            `json:"runtime"`
	State        string            `json:"servingStatus"` // SERVING, STOPPED
	Deployment   *Deployment       `json:"deployment,omitempty"`
	Entrypoint   *Entrypoint       `json:"entrypoint,omitempty"`
	EnvVariables map[string]string `json:"envVariables,omitempty"`
	CreateTime   string            `json:"createTime"`
}

type Deployment struct {
	Files map[string]File `json:"files,omitempty"`
}

type File struct {
	SourceUrl string `json:"sourceUrl"`
}

type Entrypoint struct {
	Shell string `json:"shell"`
}

type API struct {
	mu         sync.RWMutex
	opMgr      *orchestrator.OperationManager
	svcMgr     *orchestrator.ServiceManager
	serverless *serverless.API
	logAPI     *logging.API
	apps       map[string]*App
	services   map[string]map[string]*Service                 // appId -> serviceId -> Service
	versions   map[string]map[string]map[string]*Version      // appId -> serviceId -> versionId -> Version
}

func NewAPI(opMgr *orchestrator.OperationManager, sm *orchestrator.ServiceManager, serverless *serverless.API, logAPI *logging.API) *API {
	return &API{
		opMgr:      opMgr,
		svcMgr:     sm,
		serverless: serverless,
		logAPI:     logAPI,
		apps:       make(map[string]*App),
		services:   make(map[string]map[string]*Service),
		versions:   make(map[string]map[string]map[string]*Version),
	}
}

// pushLog emits a structured log entry to Cloud Logging (no-op if logAPI is nil)
func (api *API) pushLog(severity, service, text string) {
	if api.logAPI == nil {
		return
	}
	api.logAPI.PushLog(severity, "gae_app", service, text)
}


func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: AppEngine] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path
	// Mock App Engine v1 API
	// projects/{projectId}/apps
	// projects/{projectId}/apps/{appId}/services
	// projects/{projectId}/apps/{appId}/services/{serviceId}/versions

	switch {
	case strings.HasSuffix(path, "/apps"):
		api.handleApps(w, r)
	case strings.Contains(path, "/services"):
		if strings.Contains(path, "/versions") {
			api.handleVersions(w, r)
		} else {
			api.handleServices(w, r)
		}
	case strings.Contains(path, "/operations/"):
		api.handleOperations(w, r)
	case strings.Contains(path, "/deploy"): // MiniSky Direct Deploy extension
		api.handleDirectDeploy(w, r)
	default:
		// Default mock: if app doesn't exist, return 404
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    404,
				"message": "Resource not found",
				"status":  "NOT_FOUND",
			},
		})
	}
}

func (api *API) handleApps(w http.ResponseWriter, r *http.Request) {
	project := extractSegmentAfter(r.URL.Path, "projects")
	if r.Method == http.MethodGet {
		api.mu.RLock()
		app, ok := api.apps[project]
		api.mu.RUnlock()

		if !ok {
			// In MiniSky, we auto-create the app for the project if requested
			app = &App{
				Id:           project,
				LocationId:   "us-central1",
				DefaultHostname: fmt.Sprintf("%s.appspot.com", project),
			}
			api.mu.Lock()
			api.apps[project] = app
			api.mu.Unlock()
		}
		json.NewEncoder(w).Encode(app)
	}
}

func (api *API) handleServices(w http.ResponseWriter, r *http.Request) {
	appId := extractSegmentAfter(r.URL.Path, "apps")
	if r.Method == http.MethodGet {
		api.mu.RLock()
		svcs := api.services[appId]
		api.mu.RUnlock()

		items := []*Service{}
		for _, s := range svcs {
			items = append(items, s)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"services": items})
	}
}

func (api *API) handleVersions(w http.ResponseWriter, r *http.Request) {
	appId := extractSegmentAfter(r.URL.Path, "apps")
	serviceId := extractSegmentAfter(r.URL.Path, "services")
	versionId := extractSegmentAfter(r.URL.Path, "versions")

	switch r.Method {
	case http.MethodGet:
		if versionId != "" {
			api.mu.RLock()
			v := api.versions[appId][serviceId][versionId]
			api.mu.RUnlock()
			if v == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(v)
		} else {
			api.mu.RLock()
			vers := api.versions[appId][serviceId]
			api.mu.RUnlock()
			items := []*Version{}
			for _, v := range vers {
				items = append(items, v)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"versions": items})
		}
	case http.MethodDelete:
		api.mu.Lock()
		delete(api.versions[appId][serviceId], versionId)
		api.mu.Unlock()
		// Cleanup container
		containerName := fmt.Sprintf("minisky-appengine-%s-%s-%s", appId, serviceId, versionId)
		api.svcMgr.DeleteComputeVM(containerName)
		api.pushLog("INFO", serviceId, fmt.Sprintf("Deleted version %s", versionId))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"done": true})
	}
}

// handleDirectDeploy is a MiniSky-specific extension for the Dashboard
func (api *API) handleDirectDeploy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project    string `json:"project"`
		Service    string `json:"service"`
		Version    string `json:"version"`
		Runtime    string `json:"runtime"`
		Code       string `json:"code"`
		Entrypoint string `json:"entrypoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		return
	}

	if req.Service == "" { req.Service = "default" }
	if req.Version == "" { req.Version = fmt.Sprintf("v-%d", time.Now().Unix()) }

	fullName := fmt.Sprintf("apps/%s/services/%s/versions/%s", req.Project, req.Service, req.Version)
	op := api.opMgr.Register("appengine#operation", "CREATE", fullName, "", "us-central1")

	api.pushLog("INFO", req.Service, fmt.Sprintf("Starting deployment of version %s (runtime: %s)", req.Version, req.Runtime))

	// Initialize state
	api.mu.Lock()
	if api.services[req.Project] == nil { api.services[req.Project] = make(map[string]*Service) }
	if api.versions[req.Project] == nil { api.versions[req.Project] = make(map[string]map[string]*Version) }
	if api.versions[req.Project][req.Service] == nil { api.versions[req.Project][req.Service] = make(map[string]*Version) }

	api.services[req.Project][req.Service] = &Service{Id: req.Service, Name: "apps/"+req.Project+"/services/"+req.Service}
	api.versions[req.Project][req.Service][req.Version] = &Version{
		Id: req.Version,
		Name: fullName,
		Runtime: req.Runtime,
		State: "SERVING",
		CreateTime: time.Now().Format(time.RFC3339),
	}
	api.mu.Unlock()

	api.opMgr.RunAsync(op.Name, func() error {
		// Leverage Serverless Backend
		if api.serverless == nil { return fmt.Errorf("serverless backend not initialized") }
		backend := api.serverless.GetBackend()

		tmpDir, err := os.MkdirTemp("", "minisky-gae-*")
		if err != nil { return err }
		defer os.RemoveAll(tmpDir)

		fileName := "main.py"
		if strings.HasPrefix(req.Runtime, "node") { fileName = "index.js" }
		if strings.HasPrefix(req.Runtime, "go") { fileName = "main.go" }

		os.WriteFile(tmpDir+"/"+fileName, []byte(req.Code), 0644)

		image, err := backend.BuildFunction("gae-"+req.Service+"-"+req.Version, tmpDir, req.Entrypoint)
		if err != nil { return err }

		// Provision as a Serverless VM
		containerName := fmt.Sprintf("minisky-appengine-%s-%s-%s", req.Project, req.Service, req.Version)
		_, err = api.svcMgr.ProvisionServerlessVM(containerName, image, []string{"PORT=8080", "GAE_SERVICE="+req.Service, "GAE_VERSION="+req.Version})
		if err != nil {
			api.pushLog("ERROR", req.Service, fmt.Sprintf("Deployment failed for version %s: %v", req.Version, err))
			return err
		}
		api.pushLog("INFO", req.Service, fmt.Sprintf("Version %s deployed successfully", req.Version))
		return nil
	})

	json.NewEncoder(w).Encode(op)
}

func (api *API) handleOperations(w http.ResponseWriter, r *http.Request) {
	opName := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
	opName = strings.Split(opName, "/operations/")[1]
	op := api.opMgr.Get(opName)
	if op == nil {
		w.WriteHeader(404)
		return
	}
	json.NewEncoder(w).Encode(op)
}

func extractSegmentAfter(path, segment string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == segment && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
