# MiniSky User Guide

Welcome to MiniSky, the **high-fidelity** open-source GCP emulator designed for local development, testing, and collaboration. MiniSky goes beyond simple mocking by providing strict API validation and realistic cloud behavior.

## 1. Quick Start

### Installation
- **Linux/Mac:**
  ```bash
  curl -sSL https://minisky.io/install.sh | sh
  ```
- **Windows (PowerShell):**
  ```powershell
  iwr https://minisky.io/install.ps1 | iex
  ```
- **Prerequisites:** Docker must be installed and running.

### Starting the Daemon
```bash
minisky start
```
By default, the daemon starts on port `8080` (API) and `8081` (Dashboard).

---

## 2. Using the User Dashboard
The dashboard is accessible at `http://localhost:8081`. It is designed to look and feel like the GCP Console.

### Features:
1. **Service Control Center:** A grid of cards for each supported service. Toggle a switch to "Activate" or "Deactivate" (which starts/stops the underlying Docker containers).
2. **Project Setup:** Define local Project IDs and global environment variables.
3. **Resource Explorer:**
   - **Storage:** Browse buckets, upload files, and view metadata.
   - **Pub/Sub:** View message counts, push/pull messages manually, and manage subscriptions.
   - **BigQuery:** A built-in SQL editor connected to the local DuckDB instance.
4. **Health Monitor:** Real-time CPU and Memory usage for all enabled emulators.
5. **Logs:** A unified terminal view tailing logs from all active service containers.

---

## 3. Lazy Loading & Automation
MiniSky is "Lazy" by default. If you run a command like:
```bash
gcloud pubsub topics create my-topic --endpoint-url=http://localhost:8080
```
MiniSky will detect that Pub/Sub is not running, pull the image (if missing), start the container, and then execute your command. This ensures your machine resources are only used when needed.

---

## 4. Collaboration with Snapshots
MiniSky allows you to share your environment state with your team.

- **Save your state:**
  ```bash
  minisky state save --name=testing-feature-x
  ```
  This creates a compressed bundle of all database files and storage buckets.
- **Share/Load:**
  Share the `.minisky` file with a teammate. They can load it with:
  ```bash
  minisky state load testing-feature-x.minisky
  ```
  All services will restart with the exact data and **resource states** you had.

---

## 5. High-Fidelity Features
MiniSky is built to handle complex production scenarios locally:
- **Terraform Readiness:** Supports Long-Running Operations (LRO), ensuring `terraform apply` works without modification.
- **Contract Validation:** Catches malformed API requests locally by validating against official GCP Discovery Docs.
- **IAM Simulation:** Test your service account permissions before they hit production.

---

## 6. Development Workflow
1. **Define Infrastructure:** Use Terraform with `custom_endpoint` overrides (see [Terraform Guide](terraform-integration.md)).
2. **Develop:** Point your application SDKs (Go, Python, Java, etc.) to `http://localhost:8080`.
3. **Debug:** Use the Dashboard at `http://localhost:8081` to inspect resources.
4. **Test:** Run your CI/CD pipelines against a headless instance of MiniSky.
