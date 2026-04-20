package storage

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
	"minisky/pkg/shims/serverless"
)

func init() {
	registry.Register("storage.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.SvcMgr)
	})
}

// EventObserver is implemented by shims that want to receive GCS events (like Serverless).
type EventObserver interface {
	OnStorageEvent(bucket, object, eventType string)
}

type API struct {
	svcMgr   *orchestrator.ServiceManager
	observer EventObserver
}

func (api *API) OnPostBoot(ctx *registry.Context) {
	if slsShim, ok := ctx.GetShim("cloudfunctions.googleapis.com").(*serverless.API); ok {
		api.SetObserver(slsShim)
	}
}

func NewAPI(sm *orchestrator.ServiceManager) *API {
	return &API{svcMgr: sm}
}

func (api *API) SetObserver(o EventObserver) {
	api.observer = o
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Ensure the GCS emulator is running
	targetURL, err := api.svcMgr.EnsureServiceRunning(r.Context(), "storage.googleapis.com")
	if err != nil {
		http.Error(w, "GCS Emulator cold-start failed", http.StatusServiceUnavailable)
		return
	}

	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Intercept the response to trigger events
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			api.handlePotentialEvent(r, resp)
		}
		return nil
	}

	proxy.ServeHTTP(w, r)
}

func (api *API) handlePotentialEvent(req *http.Request, resp *http.Response) {
	if api.observer == nil {
		return
	}

	path := req.URL.Path
	// Detect uploads: POST /b/{bucket}/o or POST /upload/storage/v1/b/{bucket}/o
	if req.Method == "POST" && (strings.Contains(path, "/b/") && strings.HasSuffix(path, "/o")) {
		bucket := extractSegmentAfter(path, "b")
		object := req.URL.Query().Get("name")
		
		if object != "" {
			log.Printf("[Storage Event] File finalized: gs://%s/%s", bucket, object)
			go api.observer.OnStorageEvent(bucket, object, "google.storage.object.finalize")
		}
	}

	// Detect deletions: DELETE /b/{bucket}/o/{object}
	if req.Method == "DELETE" && strings.Contains(path, "/o/") {
		bucket := extractSegmentAfter(path, "b")
		object := extractSegmentAfter(path, "o")
		log.Printf("[Storage Event] File deleted: gs://%s/%s", bucket, object)
		go api.observer.OnStorageEvent(bucket, object, "google.storage.object.delete")
	}
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
