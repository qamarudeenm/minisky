# MiniSky Service Catalog

This document lists the services supported by MiniSky and the underlying technology used for each.

| GCP Service | Emulator Engine | Fidelity Tier | Notes |
| :--- | :--- | :--- | :--- |
| **Cloud Storage** | `fake-gcs-server` | High (Shim) | Full JSON API validation + LRO support. |
| **Pub/Sub** | `google/cloud-sdk` | Full (GCP) | Uses official emulator; MiniSky adds IAM layers. |
| **Firestore** | `google/cloud-sdk` | Full (GCP) | Uses official emulator; MiniSky adds IAM layers. |
| **BigQuery** | `bq-shim` + **DuckDB** | High (Shim) | Strict Schema validation + LRO Job tracking. |
| **Compute Engine** | `gce-shim` + **Docker** | High (Shim) | Realistic state lifecycle + Operation polling. |
| **GKE** | `gke-shim` + **Kind** | High (Shim) | Full `clusters.get` Operation flow support. |
| **Networking** | `vpc-shim` + **Docker** | Medium | VPC/Subnet mapping with security rule simulation. |
| **IAM** | `iam-mock` | High (Logic) | Dynamic RBAC checks for all incoming API calls. |
| **Dataproc** | `dp-shim` + **Spark** | High (Shim) | Job-to-Container mapping with LRO polling. |

## 1. Cloud Storage (GCS)
The GCS emulator implements most of the JSON API. It is accessible via `http://localhost:8080/storage/v1/`.
- **Data Location:** `./data/storage`
- **Configuration:** You can pre-seed buckets by placing files in `./init/storage/`.

## 2. BigQuery (Experimental)
A high-priority feature. The shim provides a REST interface for:
- Querying tables (translating to DuckDB).
- Creating/Deleting datasets and tables.
- Inserting data (streaming inserts supported).
- **Limitations:** Some complex legacy SQL functions may not be fully mapped yet.

## 3. Cloud Functions / Cloud Run
MiniSky uses Google Cloud Buildpacks to build local Docker images of your code and run them.
- When you run `terraform apply` for a Cloud Function, MiniSky builds the image and starts a container on-the-fly.

## 4. Feature: Adding Custom Services
MiniSky is extensible. You can add a new service by creating a file in `plugins/service-name.yaml`:

```yaml
id: custom-api
name: My Custom API
image: my-org/custom-api:latest
ports:
  - 9000:9000
routing:
  host: custom.googleapis.com
  path: /v1/
```
Injected configurations allow MiniSky to manage the lifecycle of this container like any other GCP service.
