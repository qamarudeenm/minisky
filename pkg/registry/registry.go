package registry

import (
	"log"
	"net/http"
	"sync"

	"minisky/pkg/orchestrator"
)

// Context provides shared resources to shims during initialization.
type Context struct {
	OpMgr  *orchestrator.OperationManager
	SvcMgr *orchestrator.ServiceManager
	shims  map[string]http.Handler
	mu     sync.RWMutex
}

// GetShim allows one shim to find another for cross-service events (e.g. Pub/Sub -> Serverless).
func (c *Context) GetShim(domain string) http.Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.shims[domain]
}

// Factory is a function that creates a shim instance.
type Factory func(ctx *Context) http.Handler

var (
	registryMu sync.Mutex
	factories  = make(map[string]Factory)
	lazyDocker = make(map[string]bool)
	routes     = make(map[string][]string)
	opKinds    = make(map[string]string)
)

// Register maps a domain to a shim factory.
func Register(domain string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories[domain] = factory
	log.Printf("[Registry] Registered Shim Factory for %s", domain)
}

// RegisterRoutes declares the URL patterns a domain owns, so the router can
// dispatch a request that arrives without a Google hostname — which is every
// request from Terraform's custom_endpoint, gcloud's api_endpoint_overrides and
// any client library pointed at the gateway.
//
// A pattern uses the same "*"-per-segment glob vocabulary as the validator's
// PathGlob, and matches on a prefix: "/v1/projects/*/secrets" claims
// "/v1/projects/p/secrets/my-secret/versions/1" too.
//
// Patterns must be specific enough to distinguish the service from its
// neighbours on a shared prefix. "/v1/projects/*/topics" identifies Pub/Sub;
// "/v1/projects/*" identifies nothing.
func RegisterRoutes(domain string, globs ...string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	routes[domain] = append(routes[domain], globs...)
}

// Routes returns a copy of the declared URL patterns, keyed by domain.
func Routes() map[string][]string {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make(map[string][]string, len(routes))
	for domain, globs := range routes {
		out[domain] = append([]string(nil), globs...)
	}
	return out
}

// RegisterOperationKinds declares the operation "kind" values a service mints,
// so a poll of a long-running operation can be routed back to the service that
// created it.
//
// Several regional APIs expose the identical path for this —
// /v1/projects/*/locations/*/operations belongs to GKE, Cloud Functions,
// Artifact Registry, Memorystore and others at once. Real GCP tells them apart
// by hostname; a client pointed at the gateway has no hostname to offer. The
// operation id in the URL does identify the owner, because whichever service
// minted it recorded its kind, so that is what the router resolves against
// rather than picking a winner.
//
// The kind cannot be derived from the domain: sqladmin.googleapis.com mints
// "sql#operation" and redis.googleapis.com mints "memorystore#operation".
func RegisterOperationKinds(domain string, kinds ...string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, kind := range kinds {
		opKinds[kind] = domain
	}
}

// OperationKinds returns a copy of the kind-to-domain mapping.
func OperationKinds() map[string]string {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make(map[string]string, len(opKinds))
	for kind, domain := range opKinds {
		out[kind] = domain
	}
	return out
}

// RegisterLazyDocker marks a domain as a pure Docker-backed service.
func RegisterLazyDocker(domain string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	lazyDocker[domain] = true
	log.Printf("[Registry] Registered Lazy Docker Factory for %s", domain)
}

// PostBoot is implemented by shims that need to wire themselves to other services
// after all shims have been instantiated (e.g. Pub/Sub observer setup).
type PostBoot interface {
	OnPostBoot(ctx *Context)
}

// ProjectDiscoverer is implemented by shims that track resources by project ID.
type ProjectDiscoverer interface {
	ListProjects() []string
}

// BootAll initializes all registered shims and returns the mapping.
func BootAll(opMgr *orchestrator.OperationManager, svcMgr *orchestrator.ServiceManager) (map[string]http.Handler, []string) {
	ctx := &Context{
		OpMgr:  opMgr,
		SvcMgr: svcMgr,
		shims:  make(map[string]http.Handler),
	}

	// First pass: Instantiate all shims
	registryMu.Lock()
	for domain, factory := range factories {
		shim := factory(ctx)
		ctx.shims[domain] = shim
	}
	registryMu.Unlock()

	// Second pass: Wire dependencies (PostBoot)
	for _, shim := range ctx.shims {
		if pb, ok := shim.(PostBoot); ok {
			pb.OnPostBoot(ctx)
		}
	}

	// Return the initialized shims and the list of lazy domains
	lazyList := []string{}
	for domain := range lazyDocker {
		lazyList = append(lazyList, domain)
	}

	return ctx.shims, lazyList
}
