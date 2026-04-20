# Contributing to MiniSky

Thank you for your interest in contributing to MiniSky! This project aims to provide the most accurate, high-fidelity local emulator for Google Cloud Platform. 

## 🏗️ Architecture Overview

MiniSky consists of three main layers:
1.  **The API Gateway (Go)**: Intercepts requests to `*.googleapis.com`.
2.  **Service Shims (Go)**: Logic layers that handle metadata, LROs (Long Running Operations), and state management.
3.  **Docker Backends**: Pure emulators (like `fake-gcs-server` or `pubsub-emulator`) that handle the heavy lifting of data storage.

---

## 🛠️ Adding a New Service Shim

MiniSky uses a **Dynamic Registry** system. To add a new GCP service (e.g., `translation.googleapis.com`), follow these steps:

### 1. Create the Package
Create a new directory in `pkg/shims/<service_name>/`.

### 2. Implement the API Handler
Your shim must implement the `http.Handler` interface.

```go
package myservice

import (
    "net/http"
    "minisky/pkg/registry"
)

type API struct {}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte(`{"message": "Hello from MiniSky!"}`))
}
```

### 3. Register the Shim
Add an `init()` function to your package that registers the service domain with the registry.

```go
func init() {
    registry.Register("myservice.googleapis.com", func(ctx *registry.Context) http.Handler {
        return &API{}
    })
}
```

### 4. Enable the Shim
Add a blank import to `pkg/shims/registry_init.go` to ensure your shim is compiled and its `init()` function is triggered.

```go
import (
    _ "minisky/pkg/shims/myservice"
)
```

---

## 🔗 Cross-Service Wiring (Post-Boot)

If your shim needs to interact with another shim (e.g., Pub/Sub sending events to Serverless), implement the `PostBoot` interface.

```go
func (api *API) OnPostBoot(ctx *registry.Context) {
    // Resolve another shim by its domain
    otherShim := ctx.GetShim("other.googleapis.com")
    // ... wire up observers or shared state ...
}
```

---

## 🎨 Dashboard Development

The UI is built with **React + Vite + Tailwind CSS**.
1.  Navigate to `ui/`.
2.  Add a new manager component in `src/components/`.
3.  Register your tab in `App.tsx`.
4.  The UI communicates with the backend via the Management API at `:8081/api/manage/`.

---

## ✅ Pull Request Process

1.  **Format**: Ensure your code is formatted with `go fmt`.
2.  **Tests**: If you add logic, add a unit test in your shim directory.
3.  **Documentation**: Update the `CLI Reference` or `User Guide` if you added user-facing flags or commands.
4.  **Screenshot**: If you modified the UI, include a screenshot in your PR description.

We are excited to see what you build!
