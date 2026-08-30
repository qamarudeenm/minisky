package router

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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

// isGoogleServiceHost reports whether a Host header names a Google API domain,
// in which case the request is routed by domain rather than by URL prefix.
func isGoogleServiceHost(host string) bool {
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return strings.HasSuffix(host, "googleapis.com") ||
		strings.HasSuffix(host, "google.com") ||
		strings.HasSuffix(host, "firebaseio.com") ||
		strings.HasSuffix(host, "google.internal")
}

// pathMappedDomain maps a request path onto the service that owns it, or ""
// when no prefix matches.
func pathMappedDomain(path string) string {
	switch {
	case strings.HasPrefix(path, "/storage/") || strings.HasPrefix(path, "/upload/storage/"):
		return "storage.googleapis.com"
	case strings.HasPrefix(path, "/bigquery/"):
		return "bigquery.googleapis.com"
	case (strings.HasPrefix(path, "/v1/projects/") || strings.HasPrefix(path, "/projects/")) &&
		(strings.Contains(path, "/topics") || strings.Contains(path, "/subscriptions")):
		return "pubsub.googleapis.com"
	case strings.HasPrefix(path, "/v2/") ||
		(strings.HasPrefix(path, "/v1/projects/") && strings.Contains(path, "/locations/")):
		return "cloudfunctions.googleapis.com"
	case strings.HasPrefix(path, "/compute/"):
		return "compute.googleapis.com"
	default:
		return ""
	}
}

func (p *ProxyRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetDomain := r.Host

	// 1. Path-based routing.
	//
	// Clients that target the emulator by address rather than by Google's
	// hostnames — Terraform's custom_endpoint, the CLI, and anything running
	// inside an emulated VM calling http://host.docker.internal:8080 — carry a
	// Host header that names no GCP service. Route those by URL prefix.
	//
	// Requests that already name a Google domain keep domain-based routing, so
	// an SDK pointed at bigquery.googleapis.com still reaches its shim.
	if !isGoogleServiceHost(targetDomain) {
		if mapped := pathMappedDomain(r.URL.Path); mapped != "" {
			targetDomain = mapped
			log.Printf("[Router] Path-mapped request from %s: %s -> %s", r.Host, r.URL.Path, targetDomain)
		}
	}

	// 2. Subdomain Flattening (e.g. project-id.firebaseio.com -> firebaseio.com)
	if strings.HasSuffix(targetDomain, ".firebaseio.com") {
		targetDomain = "firebaseio.com"
	} else if strings.HasSuffix(targetDomain, ".googleapis.com") {
		// Some services use [service].googleapis.com, we want the base domain if needed
		// But usually we register the full domain. For now, keep as is unless it's a known multi-subdomain service.
	}

	log.Printf("[Router] %s %s%s", r.Method, targetDomain, r.URL.Path)

	// 2. Schema Validation
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
