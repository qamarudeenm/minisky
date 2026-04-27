package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"minisky/pkg/config"
	"minisky/pkg/orchestrator"
	"minisky/pkg/shims/appengine"
	"minisky/pkg/shims/bigquery"
	"minisky/pkg/shims/gke"
	"minisky/pkg/shims/logging"
	"minisky/pkg/shims/memorystore"
	"minisky/pkg/shims/monitoring"
	"minisky/pkg/shims/serverless"
	"minisky/pkg/version"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type API struct {
	svcMgr        *orchestrator.ServiceManager
	bqBackend     *bigquery.DuckDBBackend
	gkeBackend    *gke.KindBackend
	servBackend   *serverless.BuildpacksBackend
	logAPI        *logging.API
	monAPI        *monitoring.API
	appEngineAPI  *appengine.API
	memoAPI       *memorystore.API
}

func NewAPIHandler(
	svcMgr *orchestrator.ServiceManager,
	bqBackend *bigquery.DuckDBBackend,
	gkeBackend *gke.KindBackend,
	servBackend *serverless.BuildpacksBackend,
	logAPI *logging.API,
	monAPI *monitoring.API,
	appEngineAPI *appengine.API,
	memoAPI *memorystore.API,
) http.Handler {
	api := &API{
		svcMgr:       svcMgr,
		bqBackend:    bqBackend,
		gkeBackend:   gkeBackend,
		servBackend:  servBackend,
		logAPI:       logAPI,
		monAPI:       monAPI,
		appEngineAPI: appEngineAPI,
		memoAPI:      memoAPI,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/services", api.handleServices)
	mux.HandleFunc("/api/services/", api.handleServiceAction) // /api/services/{id}/start
	mux.HandleFunc("/api/settings", api.handleSettings)
	mux.HandleFunc("/api/config/images", api.handleConfigImages)
	mux.HandleFunc("/api/manage/compute/terminal", api.handleTerminal)
	mux.HandleFunc("/api/manage/system/install-dependency/", api.handleInstallDependency)
	mux.HandleFunc("/api/manage/system/reset-logs", api.handleResetLogs)
	mux.HandleFunc("/api/manage/system/prune-containers", api.handlePruneContainers)
	mux.HandleFunc("/api/system/info", api.handleSystemInfo)

	// Add reverse proxy for management APIs
	mux.Handle("/api/manage/storage/", api.handleManageStorage())
	mux.Handle("/api/manage/iam/", api.handleManageIam())
	mux.Handle("/api/manage/compute/", api.handleManageCompute())
	mux.Handle("/api/manage/dns/", api.handleManageDns())
	mux.Handle("/api/manage/network/", api.handleManageNetwork())
	mux.Handle("/api/manage/firestore/", api.handleManageFirestore())
	mux.Handle("/api/manage/pubsub/", api.handleManagePubSub())
	mux.Handle("/api/manage/bigquery/", api.handleManageBigQuery())
	mux.Handle("/api/manage/cloudsql/", api.handleManageCloudSql())
	mux.Handle("/api/manage/dataproc/", api.handleManageDataproc())
	mux.Handle("/api/manage/bigtable/", api.handleManageBigtable())
	mux.Handle("/api/manage/spanner/", api.handleManageSpanner())
	mux.Handle("/api/manage/gke/", api.handleManageGke())
	mux.Handle("/api/manage/serverless/", api.handleManageServerless())
	mux.HandleFunc("/api/manage/logging/entries", api.handleLoggingEntries)
	mux.HandleFunc("/api/manage/logging/container", api.handleContainerLogs)
	mux.HandleFunc("/api/manage/monitoring/stats", api.handleMonitoringStats)
	mux.Handle("/api/manage/firebase/", api.handleManageFirebase())
	mux.Handle("/api/manage/appengine/", api.handleManageAppEngine())
	mux.Handle("/api/manage/memorystore/", api.handleManageMemorystore())
	return mux
}

// ServiceStatus matches the UI's expected schema
type ServiceStatus struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Status      string   `json:"status"` // RUNNING, SLEEPING
	Port        *int     `json:"port"`
	Description string   `json:"description"`
	MissingDeps []string `json:"missingDeps,omitempty"`
}

func (api *API) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 1. Check Docker Containers (Lazy-loaded)
	gcsStatus, port := api.checkDockerStatus("minisky-gcs", 4443)
	pubsubStatus, psPort := api.checkDockerStatus("minisky-pubsub", 8085)
	fsStatus, fsPort := api.checkDockerStatus("minisky-firestore", 8082)
	btStatus, btPort := api.checkDockerStatus("minisky-bigtable", 8086)
	dsStatus, dsPort := api.checkDockerStatus("minisky-datastore", 8081)
	spStatus, spPort := api.checkDockerStatus("minisky-spanner", 9020)
	authStatus, authPort := api.checkDockerStatus("minisky-firebase-auth", 9099)
	rtdbStatus, rtdbPort := api.checkDockerStatus("minisky-firebase-rtdb", 9000)
	hostingStatus, hostingPort := api.checkDockerStatus("minisky-firebase-hosting", 5000)

	var firebaseDeps []string
	fbImage := "andreysenov/firebase-tools:latest"
	if exists, _ := api.svcMgr.ImageExistsPublic(fbImage); !exists {
		firebaseDeps = []string{"docker-image:" + fbImage}
	}

	// 2. Check Deep Integrations (Native Shims)
	bqStatus := "SLEEPING"
	if api.bqBackend.Enabled() {
		bqStatus = "RUNNING"
	}

	gkeStatus := "SLEEPING"
	if api.gkeBackend.Enabled() {
		gkeStatus = "RUNNING"
	}

	servStatus := "SLEEPING"
	if api.servBackend.Enabled() {
		servStatus = "RUNNING"
	}

	var gkeDeps []string
	if !api.gkeBackend.Enabled() {
		localKind := filepath.Join(".minisky", "bin", "kind")
		if _, err := os.Stat(localKind); err != nil {
			if _, err := exec.LookPath("kind"); err != nil {
				gkeDeps = []string{"kind"}
			}
		}
	}

	var servDeps []string
	if !api.servBackend.Enabled() {
		binName := orchestrator.GetPackBinaryName()
		localPack := filepath.Join(".minisky", "bin", binName)
		if _, err := os.Stat(localPack); err != nil {
			if _, err := exec.LookPath(binName); err != nil {
				servDeps = []string{"pack"}
			}
		}
	}

	// Always running proxy routes
	computeStatus := "RUNNING"
	sqlStatus := "RUNNING"
	dnsStatus := "RUNNING"
	iamStatus := "RUNNING"
	dpStatus := "RUNNING"

	services := []ServiceStatus{
		{ID: "storage", Name: "fake-gcs-server", Label: "Cloud Storage", Status: gcsStatus, Port: port, Description: "Intercepting and persisting JSON data to storage.googleapis.com"},
		{ID: "pubsub", Name: "gcloud-pubsub", Label: "Cloud Pub/Sub", Status: pubsubStatus, Port: psPort, Description: "Official GCP emulator handling topic subscriptions"},
		{ID: "firestore", Name: "gcloud-firestore", Label: "Cloud Firestore", Status: fsStatus, Port: fsPort, Description: "Official GCP emulator managing NoSQL document routing"},
		{ID: "compute", Name: "minisky-gce", Label: "Compute Engine", Status: computeStatus, Port: nil, Description: "Docker VM orchestration & Armor Load Balancing"},
		{ID: "gke", Name: "minisky-gke", Label: "Google Kubernetes Engine (GKE)", Status: gkeStatus, Port: nil, Description: "Native kind cluster provisioning", MissingDeps: gkeDeps},
		{ID: "bigquery", Name: "bq-shim", Label: "BigQuery (DuckDB)", Status: bqStatus, Port: nil, Description: "Instant, serverless local analytical SQL parallel execution"},
		{ID: "sqladmin", Name: "cloud-sql", Label: "Cloud SQL", Status: sqlStatus, Port: nil, Description: "Postgres/MySQL docker container mapping"},
		{ID: "serverless", Name: "cloud-functions", Label: "Cloud Functions & Run", Status: servStatus, Port: nil, Description: "Source to Image using GCP Buildpacks", MissingDeps: servDeps},
		{ID: "dns", Name: "cloud-dns", Label: "Cloud DNS", Status: dnsStatus, Port: nil, Description: "Internal managed zone resolution"},
		{ID: "iam", Name: "cloud-iam", Label: "Cloud IAM", Status: iamStatus, Port: nil, Description: "Role binding & policy engine evaluation"},
		{ID: "dataproc", Name: "cloud-dataproc", Label: "Cloud Dataproc", Status: dpStatus, Port: nil, Description: "Spark cluster emulation & LRO tracking"},
		{ID: "bigtable", Name: "cloud-bigtable", Label: "Cloud Bigtable", Status: btStatus, Port: btPort, Description: "REST-to-gRPC Admin Bridge for high-performance NoSQL"},
		{ID: "datastore", Name: "cloud-datastore", Label: "Cloud Datastore", Status: dsStatus, Port: dsPort, Description: "Official GCP emulator for legacy Datastore mode storage"},
		{ID: "spanner", Name: "cloud-spanner", Label: "Cloud Spanner", Status: spStatus, Port: spPort, Description: "High-performance relational database with global scale"},
		{ID: "firebase-auth", Name: "firebase-auth", Label: "Firebase Authentication", Status: authStatus, Port: authPort, Description: "Local identity toolkit for user management and auth tokens", MissingDeps: firebaseDeps},
		{ID: "firebase-rtdb", Name: "firebase-rtdb", Label: "Firebase Realtime Database", Status: rtdbStatus, Port: rtdbPort, Description: "NoSQL cloud database that synchronizes data in real-time", MissingDeps: firebaseDeps},
		{ID: "firebase-hosting", Name: "firebase-hosting", Label: "Firebase Hosting", Status: hostingStatus, Port: hostingPort, Description: "Local hosting of web assets and content with SSL", MissingDeps: firebaseDeps},
		{ID: "appengine", Name: "app-engine", Label: "App Engine", Status: "RUNNING", Port: nil, Description: "Deploy and version serverless applications with zero infrastructure management"},
		{ID: "memorystore", Name: "cloud-memorystore", Label: "Memorystore", Status: "RUNNING", Port: nil, Description: "In-memory data store for Redis and Memcached"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (api *API) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	id := pathParts[3]
	action := pathParts[4]

	// Map ID to docker domains.
	domainMap := map[string]string{
		"storage":   "storage.googleapis.com",
		"pubsub":    "pubsub.googleapis.com",
		"firestore": "firestore.googleapis.com",
		"bigtable":  "bigtable.googleapis.com",
		"datastore": "datastore.googleapis.com",
		"spanner":   "spanner.googleapis.com",
		"firebase-auth": "identitytoolkit.googleapis.com",
		"firebase-rtdb": "firebaseio.com",
		"firebase-hosting": "firebasehosting.googleapis.com",
		"memorystore": "redis.googleapis.com",
	}

	if action == "start" {
		if domain, ok := domainMap[id]; ok {
			project := r.URL.Query().Get("project")
			go func() {
				// We don't block the UI
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				
				var env []string
				if project != "" {
					env = append(env, "CLOUDSDK_CORE_PROJECT="+project)
					env = append(env, "GCP_PROJECT="+project)
					env = append(env, "GOOGLE_CLOUD_PROJECT="+project)
				}
				
				_, err := api.svcMgr.EnsureServiceRunning(ctx, domain, env...)
				if err != nil {
					log.Printf("[UI/API] Failed to EnsureServiceRunning for %s (project: %s): %v", domain, project, err)
				}
			}()
		}
	} else if action == "stop" {
		if domain, ok := domainMap[id]; ok {
			go func() {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if err := api.svcMgr.StopServiceContainer(ctx, domain); err != nil {
					log.Printf("[UI/API] Failed to stop container for %s: %v", domain, err)
				}
			}()
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (api *API) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings := map[string]bool{
			"bq_duckdb":       api.bqBackend.Enabled(),
			"gke_kind":        api.gkeBackend.Enabled(),
			"serverless_pack": api.servBackend.Enabled(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)
		return
	}

	if r.Method == http.MethodPost {
		var req map[string]bool
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if val, exists := req["bq_duckdb"]; exists {
			if err := api.bqBackend.SetEnabled(val); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if val, exists := req["gke_kind"]; exists {
			if err := api.gkeBackend.SetEnabled(val); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if val, exists := req["serverless_pack"]; exists {
			if err := api.servBackend.SetEnabled(val); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (api *API) checkDockerStatus(name string, defaultPort int) (string, *int) {
	status, err := api.svcMgr.CheckStatusPublic(name)
	if err != nil || status != "running" {
		return "SLEEPING", nil
	}
	return "RUNNING", &defaultPort
}

func (api *API) handleManageStorage() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/storage")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		
		if req.Method == "POST" && strings.HasSuffix(path, "/o") {
			req.URL.Path = "/upload/storage/v1" + path
			q := req.URL.Query()
			if q.Get("uploadType") == "" {
				q.Set("uploadType", "media")
				req.URL.RawQuery = q.Encode()
			}
		} else {
			req.URL.Path = "/storage/v1" + path
		}
		
		req.Host = "storage.googleapis.com"
		log.Printf("[UI/API Proxy] Translated to %s", req.URL.Path)
	}
	
	return proxy
}

func (api *API) handleManageIam() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/iam")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/v1" + path
		req.Host = "iam.googleapis.com"
		log.Printf("[UI/API Proxy] Translated to %s for IAM", req.URL.Path)
	}
	
	return proxy
}

func (api *API) handleManageCompute() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		path := strings.TrimPrefix(req.URL.Path, "/api/manage/compute")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/compute/v1" + path
		req.Host = "compute.googleapis.com"
		log.Printf("[UI/API Proxy] Translated to %s for Compute", req.URL.Path)
	}

	return proxy
}

func (api *API) handleManageDns() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/dns")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/dns/v1" + path
		req.Host = "dns.googleapis.com"
		log.Printf("[UI/API Proxy] DNS \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageNetwork() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/network")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/compute/v1" + path
		req.Host = "compute.googleapis.com"
		log.Printf("[UI/API Proxy] VPC Network \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageFirestore() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/firestore")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/v1" + path
		req.Host = "firestore.googleapis.com"
		log.Printf("[UI/API Proxy] Firestore \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManagePubSub() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/pubsub")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/v1" + path
		req.Host = "pubsub.googleapis.com"
		log.Printf("[UI/API Proxy] PubSub \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageBigQuery() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/bigquery")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/bigquery/v2" + path
		req.Host = "bigquery.googleapis.com"
		log.Printf("[UI/API Proxy] BigQuery \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageCloudSql() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/cloudsql")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/v1" + path
		req.Host = "sqladmin.googleapis.com"
		log.Printf("[UI/API Proxy] Cloud SQL \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageDataproc() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/dataproc")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = "/v1" + path
		req.Host = "dataproc.googleapis.com"
		log.Printf("[UI/API Proxy] Dataproc \u2192 %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageBigtable() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/bigtable")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		
		req.URL.Path = "/v2" + path
		
		// Switch between Admin and Data APIs
		if strings.Contains(path, ":") {
			req.Host = "bigtable.googleapis.com"
		} else {
			req.Host = "bigtableadmin.googleapis.com"
		}
		
		log.Printf("[UI/API Proxy] Bigtable (%s) \u2192 %s", req.Host, req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageSpanner() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/spanner")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		// Spanner Admin API is under /v1
		req.URL.Path = "/v1" + path
		req.Host = "spanner.googleapis.com"
		log.Printf("[UI/API Proxy] Spanner Admin → %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageServerless() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/serverless")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		
		if strings.Contains(path, "/functions") {
			req.Host = "cloudfunctions.googleapis.com"
			req.URL.Path = "/v2" + path
		} else {
			req.Host = "run.googleapis.com"
			req.URL.Path = "/v2" + path
		}
		log.Printf("[UI/API Proxy] Serverless \u2192 %s (Host: %s)", req.URL.Path, req.Host)
	}
	return proxy
}

func (api *API) handleTerminal(w http.ResponseWriter, r *http.Request) {
	container := r.URL.Query().Get("container")
	if container == "" {
		http.Error(w, "missing container", http.StatusBadRequest)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Terminal] Upgrade error: %v", err)
		return
	}
	defer ws.Close()

	conn, err := api.svcMgr.StreamContainerExec(container)
	if err != nil {
		log.Printf("[Terminal] Failed to connect to container %s: %v", container, err)
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n[Error] Failed to connect to container: "+err.Error()+"\r\n"))
		return
	}
	defer conn.Close()

	log.Printf("[Terminal] Connected to container: %s", container)

	errChan := make(chan error, 2)

	go func() {
		buf := make(byteSlice, 1024*4)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					errChan <- err
					return
				}
			}
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		timeout := 30 * time.Minute
		for {
			ws.SetReadDeadline(time.Now().Add(timeout))
			_, p, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if _, err := conn.Write(p); err != nil {
				errChan <- err
				return
			}
		}
	}()

	err = <-errChan
	log.Printf("[Terminal] Session for %s closed: %v", container, err)
}

type byteSlice []byte

func (api *API) handleConfigImages(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(config.GetImageRegistry())
}

func (api *API) handleInstallDependency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "missing dependency id", http.StatusBadRequest)
		return
	}
	id := parts[5]

	log.Printf("[UI/API] Request to install dependency: %s", id)
	if err := api.svcMgr.InstallDependency(id); err != nil {
		log.Printf("[UI/API] Installation failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (api *API) handleManageGke() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/manage/gke/clusters")
		
		if r.Method == http.MethodGet && (path == "" || path == "/") {
			clusters, err := api.gkeBackend.ListClusters()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(clusters)
			return
		}

		if r.Method == http.MethodDelete {
			name := strings.TrimPrefix(path, "/")
			if name == "" {
				http.Error(w, "missing cluster name", http.StatusBadRequest)
				return
			}
			if err := api.gkeBackend.DeleteCluster(name); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && (path == "" || path == "/") {
			var req struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Name == "" {
				http.Error(w, "missing cluster name", http.StatusBadRequest)
				return
			}
			
			log.Printf("[UI/API] Request to provision GKE cluster: %s", req.Name)
			go func() {
				// Creation is slow, run in background
				if _, err := api.gkeBackend.CreateCluster(req.Name); err != nil {
					log.Printf("[UI/API] Cluster provisioning failed: %v", err)
				}
			}()
			
			w.WriteHeader(http.StatusAccepted) // 202 Accepted
			return
		}

		if r.Method == http.MethodGet && strings.HasSuffix(path, "/config") {
			name := strings.TrimPrefix(strings.TrimSuffix(path, "/config"), "/")
			if name == "" {
				http.Error(w, "missing cluster name", http.StatusBadRequest)
				return
			}
			
			configPath := fmt.Sprintf("/tmp/minisky-kubeconfig-%s.yaml", name)
			data, err := os.ReadFile(configPath)
			if err != nil {
				http.Error(w, "kubeconfig not found", http.StatusNotFound)
				return
			}
			
			w.Header().Set("Content-Type", "application/x-yaml")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-kubeconfig.yaml\"", name))
			w.Write(data)
			return
		}

	})
}

// handleLoggingEntries returns all centralized log entries.
func (api *API) handleLoggingEntries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	entries := api.logAPI.GetEntries()
	type response struct {
		Entries interface{} `json:"entries"`
	}
	json.NewEncoder(w).Encode(response{Entries: entries})
}

// handleContainerLogs returns the stdout/stderr of a named container.
func (api *API) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	name := r.URL.Query().Get("name")
	if name == "" {
		// List all serverless containers
		containers := api.svcMgr.ListManagedContainers()
		json.NewEncoder(w).Encode(containers)
		return
	}
	// Fetch logs for a specific container
	containerName := name
	if !strings.HasPrefix(name, "minisky-serverless-") {
		containerName = "minisky-serverless-" + name
	}
	logs, _ := api.svcMgr.GetContainerLogs(containerName, 200)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logs))
}

// handleMonitoringStats returns CPU/Memory stats for all managed containers.
func (api *API) handleMonitoringStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	containers := api.svcMgr.ListManagedContainers()
	type ContainerMetrics struct {
		Name   string  `json:"name"`
		Status string  `json:"status"`
		CPU    float64 `json:"cpu"`
		MemMB  float64 `json:"memMB"`
	}
	metrics := make([]ContainerMetrics, 0)
	for _, c := range containers {
		m := ContainerMetrics{Name: c.Name, Status: c.Status}
		if stats, err := api.svcMgr.GetContainerStats(c.Name); err == nil {
			m.CPU = stats.CPUPercentage
			m.MemMB = stats.MemoryUsageMB
		}
		metrics = append(metrics, m)
	}
	json.NewEncoder(w).Encode(metrics)
}
func (api *API) handleResetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	log.Printf("[UI/API] Request to reset all logs")
	api.logAPI.Reset()
	w.WriteHeader(http.StatusOK)
}

func (api *API) handlePruneContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	log.Printf("[UI/API] Request to prune exited containers and unused images")
	ctx := context.Background()
	if err := api.svcMgr.PruneExitedContainers(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := api.svcMgr.PruneUnusedImages(ctx); err != nil {
		log.Printf("[UI/API] Image pruning failed: %v", err)
	}
	w.WriteHeader(http.StatusOK)
}

func (api *API) handleManageFirebase() http.Handler {
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDir := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDir(req)
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/firebase")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		
		// Map back to specific domains if needed, or just forward as is for project logic
		// Most management calls will be to firebaseio.com or identitytoolkit
		req.URL.Path = path
		log.Printf("[UI/API Proxy] Firebase → %s", req.URL.Path)
	}
	return proxy
}

func (api *API) handleManageAppEngine() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/manage/appengine")
		if path == "" {
			path = "/"
		}

		// Direct deploy: POST /api/manage/appengine/deploy
		if r.Method == http.MethodPost && path == "/deploy" {
			// Forward to the appengine shim's direct deploy endpoint
			r.URL.Path = "/v1/projects/local-dev-project/apps/local-dev-project/deploy"
			r.Host = "appengine.googleapis.com"
			api.appEngineAPI.ServeHTTP(w, r)
			return
		}

		// List services, versions, etc: /api/manage/appengine/services?project=...
		// Forward as-is to the app engine shim
		r.URL.Path = "/v1/projects/local-dev-project/apps/local-dev-project" + path
		r.Host = "appengine.googleapis.com"
		api.appEngineAPI.ServeHTTP(w, r)
	})
}

func (api *API) handleManageMemorystore() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/manage/memorystore")
		if path == "" {
			path = "/"
		}

		// Determine if it's Redis or Memcached based on path or query
		host := "redis.googleapis.com"
		if strings.Contains(path, "memcache") {
			host = "memcache.googleapis.com"
		}

		// Map /api/manage/memorystore/instances -> /v1/projects/local-dev-project/locations/us-central1/instances
		r.URL.Path = "/v1/projects/local-dev-project/locations/us-central1" + path
		r.Host = host
		api.memoAPI.ServeHTTP(w, r)
	})
}


func (api *API) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	info := map[string]string{
		"version": version.Version,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
