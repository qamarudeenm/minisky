package dashboard

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"minisky/pkg/orchestrator"
	"minisky/pkg/shims/bigquery"
	"minisky/pkg/shims/gke"
	"minisky/pkg/shims/serverless"
)

type API struct {
	svcMgr      *orchestrator.ServiceManager
	bqBackend   *bigquery.DuckDBBackend
	gkeBackend  *gke.KindBackend
	servBackend *serverless.BuildpacksBackend
}

func NewAPIHandler(
	svcMgr *orchestrator.ServiceManager,
	bqBackend *bigquery.DuckDBBackend,
	gkeBackend *gke.KindBackend,
	servBackend *serverless.BuildpacksBackend,
) http.Handler {
	api := &API{
		svcMgr:      svcMgr,
		bqBackend:   bqBackend,
		gkeBackend:  gkeBackend,
		servBackend: servBackend,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/services", api.handleServices)
	mux.HandleFunc("/api/services/", api.handleServiceAction) // /api/services/{id}/start
	mux.HandleFunc("/api/settings", api.handleSettings)
	
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
	return mux
}

// ServiceStatus matches the UI's expected schema
type ServiceStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Status      string `json:"status"` // RUNNING, SLEEPING
	Port        *int   `json:"port"`
	Description string `json:"description"`
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
		{ID: "gke", Name: "minisky-gke", Label: "Kubernetes Engine", Status: gkeStatus, Port: nil, Description: "Native kind cluster provisioning"},
		{ID: "bigquery", Name: "bq-shim", Label: "BigQuery (DuckDB)", Status: bqStatus, Port: nil, Description: "Instant, serverless local analytical SQL parallel execution"},
		{ID: "sqladmin", Name: "cloud-sql", Label: "Cloud SQL", Status: sqlStatus, Port: nil, Description: "Postgres/MySQL docker container mapping"},
		{ID: "serverless", Name: "cloud-functions", Label: "Cloud Functions & Run", Status: servStatus, Port: nil, Description: "Source to Image using GCP Buildpacks"},
		{ID: "dns", Name: "cloud-dns", Label: "Cloud DNS", Status: dnsStatus, Port: nil, Description: "Internal managed zone resolution"},
		{ID: "iam", Name: "cloud-iam", Label: "Cloud IAM", Status: iamStatus, Port: nil, Description: "Role binding & policy engine evaluation"},
		{ID: "dataproc", Name: "cloud-dataproc", Label: "Cloud Dataproc", Status: dpStatus, Port: nil, Description: "Spark cluster emulation & LRO tracking"},
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
	}

	if action == "start" {
		if domain, ok := domainMap[id]; ok {
			go func() {
				// We don't block the UI
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				_, err := api.svcMgr.EnsureServiceRunning(ctx, domain)
				if err != nil {
					log.Printf("[UI/API] Failed to EnsureServiceRunning for %s: %v", domain, err)
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
			api.bqBackend.SetEnabled(val)
		}
		if val, exists := req["gke_kind"]; exists {
			api.gkeBackend.SetEnabled(val)
		}
		if val, exists := req["serverless_pack"]; exists {
			api.servBackend.SetEnabled(val)
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (api *API) checkDockerStatus(name string, defaultPort int) (string, *int) {
	// Let's use a dummy GetStatus call, we know svcMgr has a private checkStatus function.
	// Since checkStatus is private in ServiceManager, let's expose a helper or read it via shell.
	// Wait! I will add `CheckStatusPublic(name string) (string, error)` to ServiceManager.
	status, err := api.svcMgr.CheckStatusPublic(name)
	if err != nil || status != "running" {
		return "SLEEPING", nil
	}
	return "RUNNING", &defaultPort
}

func (api *API) handleManageStorage() http.Handler {
	// The frontend will call /api/manage/storage/b
	// We want to proxy to http://localhost:8080/storage/v1/b
	target, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		// Strip /api/manage/storage and prepend /storage/v1
		path := strings.TrimPrefix(req.URL.Path, "/api/manage/storage")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		
		// If uploading an object, use /upload/storage/v1
		if req.Method == "POST" && strings.HasSuffix(path, "/o") {
			req.URL.Path = "/upload/storage/v1" + path
			
			// Optional: fake-gcs-server needs uploadType=media for simple uploads
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
		
		// Map /api/manage/iam/* -> /v1/*
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

		// /api/manage/compute/* -> /compute/v1/*
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
		log.Printf("[UI/API Proxy] DNS → %s", req.URL.Path)
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
		log.Printf("[UI/API Proxy] VPC Network → %s", req.URL.Path)
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
		log.Printf("[UI/API Proxy] Firestore → %s", req.URL.Path)
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
		log.Printf("[UI/API Proxy] PubSub → %s", req.URL.Path)
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
		// BigQuery uses /bigquery/v2
		req.URL.Path = "/bigquery/v2" + path
		req.Host = "bigquery.googleapis.com"
		log.Printf("[UI/API Proxy] BigQuery → %s", req.URL.Path)
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
		log.Printf("[UI/API Proxy] Cloud SQL → %s", req.URL.Path)
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
		log.Printf("[UI/API Proxy] Dataproc → %s", req.URL.Path)
	}
	return proxy
}
