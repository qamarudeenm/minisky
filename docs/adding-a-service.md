# 🛠️ Guide: Adding a New Service to MiniSky

This guide outlines the standardized process for adding a new Google Cloud Platform (GCP) service emulator to MiniSky. Following these steps ensures consistency and allows your service to benefit from MiniSky's "Lazy Loading" and service orchestration.

## 🏗️ Architectural Overview

MiniSky acts as a **Service Orchestrator and API Proxy**. Most services follow this pattern:
1. **The Shim**: A Go package that intercepts GCP API requests.
2. **The Backend**: A Docker container or local binary (emulator) that actually handles the logic.
3. **The Proxy**: The shim translates or forwards requests to the backend.

---

## 🚀 Step-by-Step Implementation

### 1. Identify the Service API
Find the official GCP discovery document or API reference for the service you want to add (e.g., `pubsub.googleapis.com`).

### 2. Choose the Implementation Type

#### Option A: Custom Go Shim (Recommended for complex services)
Use this if you need to intercept requests, trigger cross-service events (like GCS -> Cloud Functions), or handle custom authentication.
- **Path**: `pkg/shims/<service_name>/`
- **Logic**: Use `httputil.NewSingleHostReverseProxy` to forward traffic.

#### Option B: Lazy Docker (Simple Proxy)
Use this if the service has an official Docker emulator and you don't need any custom Go logic.
- **Registration**: Register directly in `pkg/shims/registry_init.go`.

---

### 3. Implementing a Custom Shim

#### Create the package
Create `pkg/shims/<service_name>/api.go`.

#### Standard Template:
```go
package <service_name>

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("<service>.googleapis.com", func(ctx *registry.Context) http.Handler {
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
	// 1. Ensure the backend emulator is running (Lazy Loading)
	targetURL, err := api.svcMgr.EnsureServiceRunning(r.Context(), "<service>.googleapis.com")
	if err != nil {
		http.Error(w, "Service cold-start failed", http.StatusServiceUnavailable)
		return
	}

	// 2. Proxy the request
	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}
```

### 4. Register the Service

Open `pkg/shims/registry_init.go` and add your service:

```go
import (
    // ...
    _ "minisky/pkg/shims/<service_name>"
)

func init() {
    // If using Option B (Lazy Docker):
    registry.RegisterLazyDocker("<service>.googleapis.com")
}
```

### 5. Declare Your URL Patterns

A shim is only reachable by hostname until it declares the paths it owns. Terraform's
`custom_endpoint`, gcloud's `api_endpoint_overrides` and any client library pointed at the gateway
all send `Host: localhost:8080`, so the router has to identify your service from the URL alone.

Add `RegisterRoutes` beside your `Register` call:

```go
func init() {
    registry.Register("<service>.googleapis.com", func(ctx *registry.Context) http.Handler {
        return NewAPI(ctx.OpMgr)
    })

    registry.RegisterRoutes("<service>.googleapis.com",
        "/v1/projects/*/locations/*/<yourCollection>",
    )
}
```

**Rules for a pattern:**

- `*` matches exactly one path segment; a pattern matches on a prefix, so
  `/v1/projects/*/secrets` also owns `/v1/projects/p/secrets/my-secret/versions/1`.
- Be specific enough to distinguish your service from its neighbours. Thirteen services share
  `/v1/projects/`, so the collection segment is what identifies you.
  `/v1/projects/*/secrets` is a route; `/v1/projects/*` is a land grab and is rejected by
  `TestNoDeclarationIsDangerouslyBroad`.
- When two patterns match, the one with more literal segments wins, so
  `/v1/projects/*/locations/*/jobs` beats `/v1/projects/*/jobs` regardless of registration order.
- A Docker-backed service has no shim `init()` to hold the declaration — put it next to its
  `RegisterLazyDocker` call in `registry_init.go` instead.

Add your paths to `TestDeclaredRoutesResolveProbedPaths` in `pkg/router/routes_declared_test.go` so
a future declaration cannot quietly capture them.

### 6. Define the Backend Configuration
MiniSky needs to know which Docker image to pull for your service. Add the service definition to the internal orchestrator config (usually `configs/services.yaml` or similar).

### 7. Update Documentation
Add your service to the matrix in `docs/service_tracking.md` and set its status to ✅.

---

## 🔗 Cross-Service Interactions (Post-Boot)

If your shim needs to talk to another shim (e.g., Pub/Sub needs to notify Cloud Functions), implement the `PostBoot` interface:

```go
func (api *API) OnPostBoot(ctx *registry.Context) {
	if otherShim, ok := ctx.GetShim("other.googleapis.com").(*other.API); ok {
		api.SetDependency(otherShim)
	}
}
```

---

## ✅ Checklist for Contributors
- [ ] Service registered in `registry_init.go`.
- [ ] Docker image/emulator identified and added to config.
- [ ] `ServeHTTP` handles proxying and lazy-loading.
- [ ] API parity tested using `gcloud` or Terraform.
- [ ] Documentation updated in `service_tracking.md`.
