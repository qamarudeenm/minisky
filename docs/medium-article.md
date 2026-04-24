# Why We Built MiniSky: The Open-Source GCP Emulator That Treats Your Localhost Like a Real Cloud

## Local Cloud Emulation Is Broken. Here's How We're Fixing It for Google Cloud Platform.

---

If you've ever developed against Google Cloud Platform, you know the drill. You spin up a Pub/Sub emulator here, a Firestore emulator there, manually wire fake-gcs-server for storage, and then spend an afternoon figuring out why your Terraform plan works locally but fails in staging.

The problem isn't GCP. The problem is that the local development experience for GCP has always been fragmented — a patchwork of official emulators (that only cover a handful of services), community hacks, and shallow mocks that don't actually behave like the real cloud.

AWS developers had LocalStack. Then, when LocalStack moved core features behind a paid tier, they got MiniStack. Azure developers got MiniBlue.

GCP developers got... nothing.

Until now.

**MiniSky** is a high-fidelity, open-source GCP emulator that runs 16+ Google Cloud services on your laptop in a single binary. It's MIT Licensed, has zero feature gates, and is designed to make your local environment behave exactly like production.

This article explains what MiniSky is, why we built it the way we did, and how it can transform your development workflow.

![MiniSky System Diagnostics Dashboard](images/Screenshot%20from%202026-04-20%2013-17-02.png)

---

## The Problem: Why Existing GCP Emulators Fall Short

Google provides official emulators for a few services — Pub/Sub, Firestore, Bigtable, and Spanner. These work well for what they cover, but they have three fundamental limitations:

### 1. Coverage Gaps

There is no official emulator for Compute Engine, GKE, BigQuery, Dataproc, Cloud Functions, Cloud Run, VPC Networking, IAM, or Cloud SQL. These aren't edge-case services. They're the backbone of most GCP architectures.

Developers resort to mocking these services in tests, which means they're not actually testing their integration logic. Or they maintain a dedicated "dev" GCP project, which introduces cost, latency, and flakiness into every CI run.

### 2. No Unified Interface

Each official emulator runs independently on its own port, with its own startup command, its own configuration mechanism, and no shared state. If your application talks to both Pub/Sub and Cloud Storage, you need two separate emulators running, configured independently, with no coordination between them.

Compare this to the real GCP experience: one project, one IAM policy, one API gateway. The local experience should mirror that.

### 3. Low Fidelity

This is the most subtle and the most dangerous problem.

Most emulators return instant success responses. Call `instances.insert`? Here's a 200 OK with the instance. Call `clusters.create`? Done immediately.

But the real GCP API doesn't work that way. These are asynchronous operations. GCP returns a `google.longrunning.Operation` object, and your code must poll until the operation completes. The resource transitions through intermediate states — `PROVISIONING`, `STAGING`, `RUNNING` — and Terraform, the Google Cloud SDK, and well-written application code all depend on this behavior.

When your emulator skips these mechanics, you're not testing your code. You're testing a simplified fantasy that will break the moment it touches a real cloud.

---

## The MiniSky Approach: High-Fidelity by Default

MiniSky was designed around a single principle: **your local environment should behave like production, not like a shortcut.**

This means every architectural decision we made prioritized fidelity over convenience.

### Discovery Document Validation

Every API request that hits MiniSky is validated against the official GCP Discovery Documents — the same JSON schemas that define the real Google Cloud APIs.

If your request contains a field that doesn't exist in the API spec, MiniSky rejects it with the exact same `google.rpc.Status` error format that GCP would return. If a type is wrong — a string where an integer should be — MiniSky catches it.

This is critical for Terraform users. HCL-to-API translation bugs are one of the most common sources of deployment failures, and they're nearly impossible to catch with shallow mocks. With MiniSky, you catch them on `terraform plan`, on your laptop, before you push.

### Long-Running Operations Engine

MiniSky implements a global LRO Manager that handles asynchronous operations the same way GCP does.

When you create a Compute Engine instance, MiniSky returns an `Operation` object immediately. In the background, it transitions the instance through `PROVISIONING` -> `STAGING` -> `RUNNING`. Your code polls the operation endpoint. Terraform waits correctly. Your retry logic gets exercised.

This isn't just about correctness. It's about catching race conditions. If your application assumes a resource is ready the moment it's requested, you'll discover that bug in staging — or worse, in production at 2 AM. MiniSky surfaces that bug on your laptop.

### Mock IAM Policy Engine

Every request to MiniSky carries a local identity, and every action is checked against a mock RBAC policy registry.

If the local service account doesn't have `storage.buckets.create`, the request fails with a `403 PERMISSION_DENIED`. The error message matches GCP's format exactly. This allows teams to:

- Validate that their service accounts have the minimum required permissions
- Test their error handling for permission denied scenarios
- Enforce least-privilege policies from the very first line of code

---

## What's Inside the Box: The Service Catalog

MiniSky ships with support for the following GCP services:

| Service | Emulation Strategy | Key Feature |
| :--- | :--- | :--- |
| **Cloud Storage** | `fake-gcs-server` integration | Full JSON API + event triggers |
| **Pub/Sub** | Official emulator + MiniSky IAM layer | Push & Pull subscriptions |
| **Firestore** | Official emulator + MiniSky IAM layer | Standard document operations |
| **BigQuery** | DuckDB-powered REST API shim | Fast analytical queries, no Java overhead |
| **Compute Engine** | Docker container lifecycle mapping | Full state transitions + metadata service |
| **GKE** | Kind cluster orchestration | Real kubeconfig, real kubectl |
| **Dataproc** | Spark-in-Docker with GCS connector | Reads `gs://` URIs from local storage |
| **Cloud SQL** | Native PostgreSQL/MySQL containers | Standard database access |
| **Cloud Functions v2** | Google Cloud Buildpacks | Production-identical containers |
| **Cloud Run** | Google Cloud Buildpacks | Same as Functions, service mode |
| **VPC Networking** | Docker bridge networks | Real isolation between VPCs |
| **Firewall Rules** | Container-level port binding | Enforced, not just recorded |
| **IAM** | In-memory RBAC engine | Permission checks on every request |
| **Bigtable** | Emulator integration | Standard operations |
| **Spanner** | Emulator integration | Standard operations |
| **Cloud Logging** | Container log aggregation | Unified log stream |
| **Cloud Monitoring** | Container metrics collection | CPU/Memory dashboards |

![Cloud SQL Instance Management in MiniSky](images/Screenshot%20from%202026-04-20%2013-53-46.png)

All of these services are accessible through a single API gateway at `http://localhost:8080`. No separate ports. No separate configurations. One endpoint, just like the real cloud.

---

## The Architecture: How It Works

MiniSky is a Go binary that runs three components:

### 1. The Router (Port 8080)

All incoming HTTP and gRPC traffic enters through the Router. It performs protocol detection, validates the request against Discovery Documents, checks IAM permissions, and then routes to the appropriate service handler.

### 2. The Service Orchestrator

Services in MiniSky are lazy-loaded. When the Router receives a request for a service that isn't running, the Orchestrator pulls the necessary Docker image (if missing), starts the container, and proxies the request — all transparently.

If no requests arrive for a configurable period, the service is put to sleep. This means MiniSky uses only the resources your workflow actually needs.

### 3. The Dashboard (Port 8081)

A React-based single page application that mirrors the GCP Console experience. From here you can:

- Toggle services on and off with a single click
- Browse Cloud Storage buckets, Pub/Sub topics, and BigQuery tables
- Watch Long-Running Operations transition in real time
- View aggregated logs from all running services
- Inspect and modify IAM policies
- Deploy serverless functions with a visual editor
- Monitor CPU and memory usage across all emulated services

The dashboard isn't an afterthought. It's a first-class interface designed to give you the same operational visibility locally that the GCP Console gives you in the cloud.

![Cloud Monitoring Dashboard showing real-time resource usage](images/Screenshot%20from%202026-04-20%2016-43-19.png)

![Cloud Logging Interface for unified log aggregation](images/Screenshot%20from%202026-04-20%2016-43-24.png)

---

## Real-World Use Case: Terraform

One of the most impactful use cases for MiniSky is Terraform development.

Here's how you point the Google Terraform provider at MiniSky:

```hcl
provider "google" {
  project      = "local-dev-project"
  region       = "us-central1"
  access_token = "minisky-local-token"

  storage_custom_endpoint   = "http://localhost:8080/storage/v1/"
  compute_custom_endpoint   = "http://localhost:8080/compute/v1/"
  bigquery_custom_endpoint  = "http://localhost:8080/bigquery/v2/"
  pubsub_custom_endpoint    = "http://localhost:8080/"
  firestore_custom_endpoint = "http://localhost:8080/"
}
```

Then run `terraform apply` as usual. MiniSky handles the rest:

1. **Validates the API contract** against Discovery Documents — catching schema errors before they reach the cloud.
2. **Checks IAM permissions** — ensuring your service account config is correct.
3. **Returns Long-Running Operations** for async resources — so Terraform's polling logic works correctly.
4. **Persists state locally** — so `terraform plan` on the next run produces accurate diffs.

No modified providers. No special "local" mode. The same Terraform configuration works against MiniSky and against real GCP. Just change one variable.

---

## Real-World Use Case: Data Engineering with Dataproc

Dataproc is one of MiniSky's most interesting integrations. Running a full Hadoop/Spark cluster locally is resource-intensive, so MiniSky takes a smarter approach.

When you submit a Dataproc job that references a `gs://` URI, MiniSky:

1. Fetches the job code from its own internal Cloud Storage emulator
2. Launches a lightweight Spark container in local mode
3. Injects the Google Cloud Storage Connector JAR, pre-configured to point back at MiniSky's storage endpoint
4. Runs the job and reports status through the LRO system

The result: your PySpark job reads from and writes to `gs://` buckets that exist entirely on your machine. No cloud. No credentials. No cost.

![BigQuery Analytical Workspace powered by DuckDB](images/Screenshot%20from%202026-04-20%2014-22-36.png)

---

## Real-World Use Case: Event-Driven Architectures

MiniSky supports Cloud Storage event triggers out of the box. Deploy a Cloud Function, configure it to trigger on `google.storage.object.finalize`, upload a file to a bucket, and watch the function execute — all locally.

This is particularly valuable for teams building data pipelines where files landing in GCS trigger downstream processing. Testing this flow end-to-end previously required a real cloud environment. With MiniSky, it runs on your laptop in seconds.

![Serverless Console for Cloud Functions and Cloud Run](images/Screenshot%20from%202026-04-20%2013-43-22.png)

---

## Networking That Actually Works

Most emulators ignore networking entirely. MiniSky doesn't.

When you create a VPC through the Compute Engine API, MiniSky provisions a real Docker bridge network. VMs on different VPCs are genuinely isolated — you cannot ping across them. When you create a firewall rule, MiniSky tears down affected containers and re-provisions them with the correct port bindings.

This isn't theater. It's real network isolation backed by Docker's networking stack, mapped to GCP's VPC and firewall concepts.

![Networking and VPC Configuration in MiniSky](images/Screenshot%20from%202026-04-20%2016-43-31.png)

---

## The "Mini" Movement

MiniSky doesn't exist in isolation. It's part of a broader shift in the developer tools ecosystem.

When LocalStack moved toward a Business Source License and gated more features behind paid tiers, the open-source community responded. **MiniStack** emerged as the free AWS alternative. **MiniBlue** did the same for Azure.

MiniSky completes the trifecta for Google Cloud.

The philosophy is simple: local development tools should be free, open, and high-fidelity. Paying for the privilege of running code on your own machine was never the right model.

---

## Getting Started

MiniSky is designed to be running in under a minute:

```bash
# Start MiniSky
./minisky start

# Set environment variables
export STORAGE_EMULATOR_HOST=http://localhost:8080
export PUBSUB_EMULATOR_HOST=http://localhost:8080
export FIRESTORE_EMULATOR_HOST=localhost:8080
export BIGQUERY_EMULATOR_HOST=http://localhost:8080

# Use standard Google Cloud SDKs — they just work
python -c "
from google.cloud import storage
client = storage.Client()
bucket = client.bucket('my-bucket')
blob = bucket.blob('hello.txt')
blob.upload_from_string('Hello MiniSky!')
print('Upload successful')
"
```

Open `http://localhost:8081` in your browser to access the dashboard. That's it.

---

## Extending MiniSky

MiniSky is built to be extensible. If you need a GCP service that isn't supported yet, you have two options:

**Option 1: Plugin a Docker image.** Drop a YAML file in the `plugins/` directory:

```yaml
id: custom-service
name: My Custom Service
image: gcr.io/my-org/my-emulator:latest
ports:
  - 9000:9000
routing:
  host: custom.googleapis.com
  path: /v1/
```

MiniSky will manage the container lifecycle, register routing rules, and handle persistence.

**Option 2: Build a Shim.** For services without existing emulators, implement a Go handler under `pkg/shims/` that translates GCP API calls into local operations. Register it with the Discovery Doc Validator for schema enforcement and the LRO Manager for async operations.

---

## What's Next

MiniSky is under active development. Key items on the roadmap include:

- **Cloud KMS** — AES-based local key management
- **Cloud Armor** — WAF rule simulation via ModSecurity/Nginx
- **Cloud Load Balancing** — Envoy-based local load balancer
- **Cloud DNS** — CoreDNS integration for `.internal` domain simulation
- **Snapshot sharing** — Save and share your full environment state with `minisky state save`

---

## Try It. Break It. Build With Us.

MiniSky is MIT Licensed. No "Pro" tier. No feature gates. No telemetry.

If you're building on Google Cloud, whether it's a startup running Terraform for the first time or an enterprise team running thousands of integration tests per day, MiniSky gives you a local environment that actually behaves like production.

Star the repo, file an issue, or submit a PR. Let's build the future of local GCP development together.

**GitHub:** https://github.com/qamarudeenm/minisky

---

*MiniSky is built with Go, React, Docker, and DuckDB. It runs on Linux, macOS, and Windows (via WSL). Contributions welcome.*
