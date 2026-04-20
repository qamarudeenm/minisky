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
)

// Register maps a domain to a shim factory.
func Register(domain string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories[domain] = factory
	log.Printf("[Registry] Registered Shim Factory for %s", domain)
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
