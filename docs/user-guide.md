# MiniSky: The Complete User Guide

MiniSky is a high-fidelity GCP emulator designed for local development, testing, and CI/CD. It allows you to run a full GCP cloud environment locally using Docker and a custom API Gateway.

## Table of Contents
1. [Core Configuration](#core-configuration)
2. [Serverless (Functions & Cloud Run)](#serverless)
3. [Cloud Storage (GCS)](#cloud-storage)
4. [Pub/Sub](#pubsub)
5. [Databases (Firestore, SQL, Bigtable)](#databases)
6. [Observability (Logging & Monitoring)](#observability)
7. [BigQuery](#bigquery)
8. [Lazy Loading & Snapshots](#lazy-loading--snapshots)

---

## Core Configuration

To use MiniSky, you must point your Google Cloud SDKs to the local gateway.

- **API Gateway**: `http://localhost:8080`
- **Dashboard**: `http://localhost:8081`
- **Project ID**: `local-dev-project` (Default)

### Environment Variables
Set these in your terminal or `.env` file:
```bash
export STORAGE_EMULATOR_HOST=http://localhost:8080
export PUBSUB_EMULATOR_HOST=http://localhost:8080
export FIRESTORE_EMULATOR_HOST=localhost:8080
export BIGQUERY_EMULATOR_HOST=http://localhost:8080
```

---

## Serverless

MiniSky uses **Google Cloud Buildpacks** to build production-ready containers from your source code.

### Deploying via Dashboard
1. Go to **Compute Engine Instances** -> **Serverless Console**.
2. Click **Deploy New**.
3. Select **Cloud Functions v2** or **Cloud Run**.
4. Enter your code and click **Deploy**.

### Event Triggers (GCS)
You can trigger functions automatically on GCS uploads.
```python
def handler(event, context):
    print(f"Triggered by file: {event['name']}")
```

---

## Cloud Storage

Emulated via `fake-gcs-server`.

### Python Example
```python
from google.cloud import storage

# MiniSky automatically handles the emulator host if env var is set
client = storage.Client()
bucket = client.bucket("my-bucket")
blob = bucket.blob("test.txt")
blob.upload_from_string("Hello MiniSky!")
```

---

## Pub/Sub

Full support for Topics, Subscriptions (Push & Pull).

### Node.js Example (Publisher)
```javascript
const {PubSub} = require('@google-cloud/pubsub');
const pubsub = new PubSub();

async function publishMessage() {
  const dataBuffer = Buffer.from('Hello World');
  await pubsub.topic('my-topic').publishMessage({data: dataBuffer});
}
```

---

## Databases

### Firestore
Standard Firestore emulator integration.
```python
from google.cloud import firestore
db = firestore.Client()
doc_ref = db.collection("users").document("alace")
doc_ref.set({"name": "Alice", "active": True})
```

### Cloud SQL
MiniSky spins up high-performance Docker containers for MySQL/PostgreSQL.
- **Access**: Via the local port shown in the **Database Topology** dashboard.

---

## Observability

### Cloud Logging
All container logs (Serverless, Compute, etc.) are automatically harvested into the **Cloud Logging** dashboard.
- **Search**: Filter by resource name or text content.
- **Live Stream**: Real-time log tailing.

### Cloud Monitoring
Real-time CPU and Memory metrics are collected from all managed containers.
- View charts in the **Cloud Monitoring** tab.

---

## BigQuery

Powered by **DuckDB** for lightning-fast local analytical queries without the heavy overhead of the official BQ emulator.

```python
from google.cloud import bigquery
client = bigquery.Client()
query = "SELECT count(*) FROM `my-project.my-dataset.my-table`"
results = client.query(query)
```

---

## Lazy Loading & Snapshots

### Lazy Loading
MiniSky is "Lazy" by default. If you run a command like `gcloud pubsub topics create ...`, MiniSky will detect that Pub/Sub is not running, pull the image (if missing), start the container, and then execute your command.

### Collaboration with Snapshots
MiniSky allows you to share your environment state with your team.
- **Save your state**: `minisky state save --name=testing-feature-x`
- **Share/Load**: Share the `.minisky` file. Teammates can load it with `minisky state load testing-feature-x.minisky`.

---

## System Diagnostics

Use the **System Diagnostics** page to check if required tools like `docker`, `pack`, and `kind` are installed. MiniSky can automatically fix many missing dependencies for you.
