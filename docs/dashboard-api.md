# MiniSky Internal API Design (Dashboard <-> Daemon)

To power the high-fidelity MiniSky UI, the Go Daemon must expose an internal REST API (served on port `8081`). This API provides real-time visibility into the emulated environment, running services, and in-flight operations.

---

## 1. Service Management API

Controls the Lazy-Loading lifecycle and checks which shims/emulators are active.

### `GET /api/v1/services`
Returns the status of all available MiniSky services.

**Response:**
```json
{
  "services": [
    {
      "id": "compute",
      "name": "Compute Engine",
      "status": "RUNNING",
      "is_lazy": true,
      "metrics": {
        "cpu_percent": 2.4,
        "memory_mb": 128
      }
    },
    {
      "id": "bigquery",
      "name": "BigQuery (DuckDB)",
      "status": "SLEEPING",
      "is_lazy": true,
      "metrics": null
    }
  ]
}
```

### `POST /api/v1/services/{id}/toggle`
Manually starts ("Cold Start") or stops a service via the dashboard toggle.

**Request:** `{"action": "start"}`
**Response:** `{"status": "RUNNING"}`

---

## 2. High-Fidelity Components

### `GET /api/v1/operations` (LRO Visualizer)
Tracks Long-Running Operations spawned by terraform or manual API calls.

**Response:**
```json
{
  "operations": [
    {
      "id": "operation-12345-abcde",
      "target_resource": "projects/local-dev/zones/us-central1-a/instances/test-vm",
      "action": "compute.instances.insert",
      "status": "PROVISIONING",
      "progress_percent": 45,
      "started_at": "2026-04-16T15:00:00Z"
    }
  ]
}
```

### `GET /api/v1/iam/policies`
Allows the UI to visualize currently active mock IAM permission grants.

**Response:**
```json
{
  "bindings": [
    {
      "role": "roles/storage.admin",
      "members": ["serviceAccount:local-dev@local-project.iam.gserviceaccount.com"]
    }
  ]
}
```

---

## 3. Global State & Diagnostics

### `GET /api/v1/health`
System health check for the Daemon.
**Response:** `{"status": "healthy", "uptime_seconds": 3600, "active_containers": 4}`

### `GET /api/v1/logs/stream` (WebSocket)
Streams aggregated stdout/stderr logs from all running service containers securely to the Dashboard's terminal component.

### `POST /api/v1/state/snapshot`
Trigger a local persistence snapshot command (`minisky state save`) from the UI.
