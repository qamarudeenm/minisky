package router

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"minisky/pkg/orchestrator"
	"minisky/pkg/validator"
)

// ProxyRouter intercepts and routes all incoming GCP API requests.
type ProxyRouter struct {
	mu          sync.RWMutex
	routes      map[string]http.Handler
	lazyDomains map[string]bool // domains that should trigger Docker orchestration
	validator   *validator.Validator
	serviceMgr  *orchestrator.ServiceManager
}

// NewProxyRouterWithManager creates the router with a pre-initialized ServiceManager injected.
func NewProxyRouterWithManager(sm *orchestrator.ServiceManager) *ProxyRouter {
	return &ProxyRouter{
		routes:      make(map[string]http.Handler),
		lazyDomains: make(map[string]bool),
		validator:   validator.NewValidator(),
		serviceMgr:  sm,
	}
}

// NewProxyRouter creates a standalone router (for backward compatibility).
func NewProxyRouter() *ProxyRouter {
	sm, err := orchestrator.NewServiceManager()
	if err != nil {
		log.Printf("[WARN] Failed to initialize Docker ServiceManager: %v", err)
	}
	return NewProxyRouterWithManager(sm)
}

// RegisterProxy maps a domain to a fixed external backend URL.
func (p *ProxyRouter) RegisterProxy(domain string, targetURL string) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	p.mu.Lock()
	p.routes[domain] = proxy
	p.mu.Unlock()
	log.Printf("[Router] Registered External Proxy: %s -> %s", domain, targetURL)
	return nil
}

// RegisterShim maps a domain to an internal Go handler (no Docker required).
func (p *ProxyRouter) RegisterShim(domain string, handler http.Handler) {
	p.mu.Lock()
	p.routes[domain] = handler
	p.lazyDomains[domain] = false
	p.mu.Unlock()
	log.Printf("[Router] Registered Internal Shim: %s", domain)
}

// RegisterLazyDocker marks a domain for lazy Docker-backed orchestration.
// On first request, the orchestrator boots the container and dynamically wires the proxy.
func (p *ProxyRouter) RegisterLazyDocker(domain string) {
	p.mu.Lock()
	p.lazyDomains[domain] = true
	p.mu.Unlock()
	log.Printf("[Router] Registered Lazy Docker Backend: %s (boots on first request)", domain)
}

func (p *ProxyRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetDomain := r.Host
	log.Printf("[Router] %s %s%s", r.Method, targetDomain, r.URL.Path)

	// 1. Schema Validation
	if !p.validator.ValidateRequest(w, r) {
		return
	}

	// 2. Check if this is a lazy-loaded Docker backend
	p.mu.RLock()
	isLazy := p.lazyDomains[targetDomain]
	p.mu.RUnlock()

	if isLazy && p.serviceMgr != nil {
		internalURL, err := p.serviceMgr.EnsureServiceRunning(r.Context(), targetDomain)
		if err != nil {
			log.Printf("[Router ERROR] Orchestrator failed for '%s': %v", targetDomain, err)
			// Clear the stale wired route so the next request will re-attempt the cold start
			p.mu.Lock()
			delete(p.routes, targetDomain)
			p.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"code":503,"message":"MiniSky: Cold-start failed for ` + targetDomain + `"}}`))
			return
		}
		if internalURL != "" {
			// Dynamically wire (or re-wire if container moved IP) the discovered internal URL
			target, _ := url.Parse(internalURL)
			proxy := httputil.NewSingleHostReverseProxy(target)
			p.mu.Lock()
			p.routes[targetDomain] = proxy
			p.mu.Unlock()
			log.Printf("[Router] Dynamically wired: %s -> %s", targetDomain, internalURL)
		}
	}

	// 3. Dispatch to handler
	p.mu.RLock()
	handler, exists := p.routes[targetDomain]
	p.mu.RUnlock()

	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(`{"error":{"code":501,"message":"MiniSky: '` + targetDomain + `' is not yet implemented","status":"UNIMPLEMENTED"}}`))
		return
	}

	handler.ServeHTTP(w, r)
}
