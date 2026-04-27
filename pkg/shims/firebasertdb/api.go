package firebasertdb

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("firebaseio.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.SvcMgr)
	})
}

type API struct {
	svcMgr *orchestrator.ServiceManager
}

func NewAPI(sm *orchestrator.ServiceManager) *API {
	return &API{svcMgr: sm}
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Ensure the Firebase RTDB emulator is running
	targetURL, err := api.svcMgr.EnsureServiceRunning(r.Context(), "firebaseio.com")
	if err != nil {
		http.Error(w, "Firebase RTDB Emulator cold-start failed", http.StatusServiceUnavailable)
		return
	}

	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}
