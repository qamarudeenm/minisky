# MiniSky Developer & Contributing Guide

MiniSky is built to be extensible. If a GCP service you need is missing, you can add it through our plugin architecture or by building a custom "Shim."

## 1. Project Structure
- **/cmd/minisky:** CLI entry points.
- **/pkg/router:** Reverse proxy and protocol detection.
- **/pkg/orchestrator:** Docker lifecycle and container management.
- **/pkg/shims:** API translation layers (GCE, GKE, BigQuery, Dataproc).
- **/plugins:** YAML-based service definitions.

---

## 2. Adding a Simple Service (Plugin)
If a service already has a high-quality Docker image (e.g., official Google emulators or community tools), you can add it via a YAML manifest in the `plugins/` directory.

```yaml
id: custom-service
name: My Custom Service
image: gcr.io/google-samples/custom-emulator:latest
default_port: 9000
routing:
  - host: custom.googleapis.com
    path: /
```

MiniSky will automatically:
1. Register the routing rule.
2. Manage the container lifecycle.
3. Handle persistence for any volumes defined in the image.

---

## 3. Developing a Service Shim
When no emulator exists (e.g., for Compute Engine), you must build a "Shim." 

### Step 1: Define the API Surface
Identify the REST/gRPC endpoints that need to be emulated. Implement these handlers in a new package under `pkg/shims/<service-name>`.

### Step 2: Register with the Validator
Enable **High-Fidelity** validation by linking your shim to the corresponding GCP Discovery Document. MiniSky will then automatically validate incoming JSON against the official GCP schema.

### Step 3: Implement Async Operations (LRO)
If the action is asynchronous in GCP (e.g., creating a cluster), your shim must register a task with the **LRO Manager**. The shim should return an `Operation` object immediately and update the operation status once the local Docker container is ready.

### Step 4: Bridge to Local Resources
Your shim should map GCP concepts to Docker or local OS primitives.
- **Example (GCE):** `instances.insert` -> `docker run --name <vm-name>`.
- **Example (BigQuery):** `jobs.query` -> `duckdb.Query(...)`.

### Step 3: Register the Shim
Add your shim to the `ServiceRegistry` in `pkg/orchestrator/registry.go`.

---

## 4. Key Implementation Principles
1. **API Fidelity:** Always match the GCP response schema exactly, including headers like `x-goog-generation`.
2. **Behavioral Fidelity:** Don't just return 200 OK. If GCP uses LROs, return an Operation. If GCP has eventual consistency, simulate it.
3. **IAM Checking:** Use the internal `AuthMiddleware` to verify that the local service account has the required permission strings.
4. **Lazy Loading:** All services must be "Lazy" by default. Use the Router to trigger initialization.
5. **State Persistence:** Always use volumes mapped to `.minisky/data/`.

---

## 5. Local Testing
To test your new service integration:
1. Rebuild the MiniSky binary: `go build ./cmd/minisky`.
2. Run your service using Terraform (point `custom_endpoint` to `localhost:8080`).
3. Verify the resources appear in the **MiniSky Dashboard**.
