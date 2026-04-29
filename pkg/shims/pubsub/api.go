package pubsub

import (
	"bytes"
	"io"
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
	registry.Register("pubsub.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.SvcMgr)
	})
}

type EventObserver interface {
	HandleEvent(eventType, resource, payload string)
}

type API struct {
	svcMgr   *orchestrator.ServiceManager
	proxy    *httputil.ReverseProxy
	observer EventObserver
}

func (api *API) OnPostBoot(ctx *registry.Context) {
	if slsShim, ok := ctx.GetShim("cloudfunctions.googleapis.com").(*serverless.API); ok {
		api.SetObserver(slsShim)
	}
}

func NewAPI(sm *orchestrator.ServiceManager) *API {
	return &API{
		svcMgr: sm,
	}
}

func (api *API) SetObserver(obs EventObserver) {
	api.observer = obs
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Ensure Pub/Sub emulator is running
	targetURL, err := api.svcMgr.EnsureServiceRunning(r.Context(), "pubsub.googleapis.com")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	// 2. Intercept Publish
	if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":publish") {
		api.handlePublish(w, r, targetURL)
		return
	}

	// 3. Normal Proxy
	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Ensure /v1 prefix for emulator compatibility
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		r.URL.Path = "/v1" + r.URL.Path
	}

	proxy.ServeHTTP(w, r)
}

func (api *API) handlePublish(w http.ResponseWriter, r *http.Request, targetURL string) {
	// Reconstruct the topic name from URL
	// /v1/projects/{project}/topics/{topic}:publish
	parts := strings.Split(r.URL.Path, "/")
	topic := ""
	for i, p := range parts {
		if p == "topics" && i+1 < len(parts) {
			topic = strings.Split(parts[i+1], ":")[0]
			break
		}
	}

	// Read body to notify observer
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body)) // reset for proxy

	// Proxy the request first so we don't block
	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	// Create a response recorder to see if publish succeeded
	// Actually for simplicity in shim-to-shim, we just notify if we intercepted it and it looks valid.
	if api.observer != nil && topic != "" {
		log.Printf("[PubSub Shim] 📢 Intercepted publish to topic: %s", topic)
		// We pass the raw payload (usually contains "messages")
		api.observer.HandleEvent("google.cloud.pubsub.topic.v1.messagePublished", topic, string(body))
	}

	proxy.ServeHTTP(w, r)
}
