package spanner

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	f := func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.SvcMgr, ctx)
	}
	registry.Register("spanner.googleapis.com", f)
}

type API struct {
	svcMgr *orchestrator.ServiceManager
	ctx    *registry.Context
}

func NewAPI(svcMgr *orchestrator.ServiceManager, ctx *registry.Context) *API {
	return &API{svcMgr: svcMgr, ctx: ctx}
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Spanner] %s %s", r.Method, r.URL.Path)
	
	// Proxy to the actual Spanner emulator container
	internalURL, err := api.svcMgr.EnsureServiceRunning(r.Context(), "spanner.googleapis.com")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	target, _ := url.Parse(internalURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}
