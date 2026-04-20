# MiniSky CLI Reference Manual

The `minisky` binary is a unified command-line tool designed to manage your local Google Cloud environment. It mirrors many common `gcloud` patterns while providing direct control over the emulator's lifecycle.

## Table of Contents
1. [Core Control](#core-control)
2. [Compute & Serverless](#compute--serverless)
3. [Data Storage](#data-storage)
4. [Messaging & Events](#messaging--events)
5. [Infrastructure & Databases](#infrastructure--databases)
6. [Observability](#observability)

---

## Core Control

Commands to manage the MiniSky daemon.

### `minisky start`
Starts the API Gateway (port 8080) and the Dashboard UI (port 8081).
- **Flags**:
  - `--port`: API Gateway port (default: 8080)
  - `--ui-port`: Dashboard port (default: 8081)

### `minisky stop`
Safely shuts down all emulators, stops managed Docker containers, and tears down the isolated network.

### `minisky restart`
A shorthand for `stop` followed by `start`. Useful for clearing in-memory state.

---

## Compute & Serverless

### `minisky deploy`
Deploys a Cloud Function or Cloud Run service.
- **Example**: `./minisky deploy --name my-func --runtime python312 --source main.py`
- **Flags**:
  - `--name`: Name of the resource (Required)
  - `--source`: Path to code file (Required)
  - `--runtime`: `python312`, `nodejs22`, `go122`
  - `--entry-point`: Function name (default: `handler`)
  - `--type`: `function` or `service`

### `minisky list`
Lists all active serverless resources.
```bash
./minisky list
```

### `minisky compute instances list`
Lists all emulated GCE instances (Docker VMs).

---

## Data Storage

### `minisky storage buckets list`
Lists all buckets in the Storage emulator.

### `minisky storage buckets create`
*Coming soon*: Create a bucket via CLI. (Currently use Dashboard or `gsutil`).

---

## Messaging & Events

### `minisky pubsub topics list`
Lists all Pub/Sub topics.

### `minisky pubsub topics create`
*Coming soon*: Create a topic via CLI.

---

## Infrastructure & Databases

### `minisky sql instances list`
Lists all Cloud SQL (MySQL/PostgreSQL) instances.

### `minisky gke clusters list`
Lists all Kubernetes clusters (managed via `kind`).

### `minisky bigtable instances list`
Lists all Bigtable instances.

### `minisky spanner instances list`
Lists all Spanner instances.

### `minisky spanner instances create [id]`
Creates a new Spanner instance in the local emulator.

### `minisky dataproc clusters list`
Lists all Spark/Hadoop clusters.

---

## Observability

### `minisky logs tail`
Streams all MiniSky logs in real-time to your terminal.
- **Features**:
  - Color-coded severity levels.
  - Automatic resource labeling.
  - Streams logs from Serverless, Compute, and Database containers simultaneously.

---

## Environment Variables
MiniSky respects the following environment variables if you want to avoid passing flags:
- `MINISKY_PORT`: Sets the API gateway port.
- `MINISKY_UI_PORT`: Sets the Dashboard port.
- `STORAGE_EMULATOR_HOST`: Point `gsutil` to `http://localhost:8080`.
- `PUBSUB_EMULATOR_HOST`: Point SDKs to `http://localhost:8080`.
