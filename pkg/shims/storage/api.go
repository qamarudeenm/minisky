package storage

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
	"minisky/pkg/shims/serverless"
)

func init() {
	registry.Register("storage.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.SvcMgr)
	})

	registry.RegisterRoutes("storage.googleapis.com",
		"/storage/v1",
		"/upload/storage/v1",
		"/batch/storage/v1",
	)
}

// EventObserver is implemented by shims that want to receive GCS events (like Serverless).
type EventObserver interface {
	OnStorageEvent(bucket, object, eventType string)
}

type API struct {
	svcMgr   *orchestrator.ServiceManager
	observer EventObserver

	buckets *bucketRegistry
}

func (api *API) OnPostBoot(ctx *registry.Context) {
	if slsShim, ok := ctx.GetShim("cloudfunctions.googleapis.com").(*serverless.API); ok {
		api.SetObserver(slsShim)
	}
}

func NewAPI(sm *orchestrator.ServiceManager) *API {
	return &API{svcMgr: sm, buckets: newBucketRegistry()}
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

	// Capture bucket attributes the emulator does not model, before the body is
	// consumed by the proxy.
	api.captureBucketRequest(r)

	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Intercept the response to trigger events
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			api.handlePotentialEvent(r, resp)
			if err := api.restoreBucketAttrs(r, resp); err != nil {
				log.Printf("[Storage] bucket metadata overlay skipped: %v", err)
			}
		}
		return nil
	}

	proxy.ServeHTTP(w, r)
}

// captureBucketRequest records the location, storage class and labels from a
// bucket insert or patch, and forgets a bucket on delete.
func (api *API) captureBucketRequest(r *http.Request) {
	if !isBucketRequest(r.URL.Path) {
		return
	}

	name := bucketNameFromPath(r.URL.Path)

	switch r.Method {
	case http.MethodPost, http.MethodPatch, http.MethodPut:
		if r.Body == nil {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		// The proxy still has to forward the body, so put it back.
		r.Body = io.NopCloser(bytes.NewReader(body))
		api.buckets.record(name, body)
	case http.MethodDelete:
		if name != "" {
			api.buckets.forget(name)
		}
	}
}

// restoreBucketAttrs overlays the recorded attributes onto a bucket response.
//
// Without this, `location` comes back as the emulator's default and terraform —
// for which location is ForceNew — destroys and recreates every bucket on each
// apply, taking its objects with it.
func (api *API) restoreBucketAttrs(r *http.Request, resp *http.Response) error {
	if !isBucketRequest(r.URL.Path) || r.Method == http.MethodDelete {
		return nil
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "json") {
		return nil
	}
	if resp.Header.Get("Content-Encoding") != "" {
		return nil // never rewrite an encoded body
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return err
	}

	rewritten := api.buckets.overlay(body)
	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	return nil
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
