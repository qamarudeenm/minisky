# MiniSky Deep Integration Task List

This document acts as a living project tracker for our MiniSky deep UI integrations. Each cloud service shim needs an extensive, native management pane built directly into the UI dashboard allowing users to manage state identical to the real GCP console.

## Completed
- [x] **Cloud DNS, VPC Networking & Firewall Rules** *(Dedicated Networking Page)*
  - Added **Networking** sidebar page with 3 tabs: VPC Networks | Firewall Rules | Cloud DNS.
  - VPC: Create/delete VPC networks with auto or custom subnet modes.
  - **Firewall**: Full CRUD — create allow/deny rules for INGRESS/EGRESS traffic with protocol, port, and source/destination range configuration.
  - DNS: Create managed zones and manage DNS resource records inline.
- [x] **Compute Engine** *(True Data Plane Docker Emulation)*
  - VM provisioning boots real Docker containers (`ubuntu:latest`, `debian`, `centos`, etc.).
  - VM state persists until explicitly deleted from Dashboard or `minisky` CLI.
  - SSH access: `docker exec -it minisky-vm-<name> /bin/bash` (1-click copy from UI).
  - **GKE Integration**: Cluster nodes automatically appear as Compute VMs. Nodes are protected from manual deletion and clamped to a max of 3 for local stability.
- [x] **Cloud IAM** *(Project Scoping & Cryptography Management Drawer)*
  - Integrated with active context Project ID Selector.
  - Deep integration accomplished: SA Creation, SA Deletion, JSON RSA 2048 Key Generation & automated browser download.
- [x] **Cloud Storage** *(Storage Drawer & Reverse API Proxy)*
  - `fake-gcs-server` connected on port `4443`
  - Deep integration accomplished: Bucket Creation, Bucket Listing, Object Upload, Object Deletion.
  - **Lifecycle Management**: Added "Stop Container" support to free up system resources.
- [x] **BigQuery (DuckDB Backend)** *(Analytical SQL Workspace)*
  - Fully integrated DuckDB for local analytical SQL execution.
  - **UI Workspace**: Create Datasets, Create Tables with custom schemas, and a multi-tab Query Editor.
  - **SQL Engine**: Advanced translation layer mapping BigQuery multi-segment identifiers to local DuckDB structures.
- [x] **Kubernetes Engine (Kind Integration)**
  - Local `kind` cluster provisioning with node synchronization to Compute Engine.
  - **Network Weaving**: Automatic bridging of Kind nodes to the unified `minisky-net` bridge for cross-shim communication.
- [x] **Cloud SQL** *(Relational Database Provisioning)*
  - **Docker Engine Connectivity**: Built dynamic orchestration hook binding standard Docker images (`postgres:15`, `mysql:8.0`) to the host.
  - **Persistence**: Deploys localized Docker Volumes to retain instances across physical Daemon restarts safely.
  - **UI Workspace**: Built the `CloudSqlManagerDrawer` facilitating live DB instantiation, real-time connection string extraction, and IP / port publication viewing natively from the dashboard.
- [x] **Cloud Firestore** *(Database Document Explorer)*
  - **UI Management**: Specialized drawer for Document CRUD operations and Collection management.
  - **Live SDK Sync**: Dynamically injects the Firebase Web SDK to enable real-time WebSockets to the emulator.
- [x] **Cloud Pub/Sub** *(Event Stream Sandbox)*
  - **Management Console**: CRUD for Topics and Subscriptions.
  - **Sandbox**: Feature-rich publish/pull interface with automatic Base64 encoding/decoding and message acknowledgment support.

## Pending Roadmap (In Priority Order)

- [ ] **Serverless (Cloud Functions/Run)**
  - **Goal**: Buildpacks visibility and execution interface.
  - **Capabilities Required**: List deployed serverless functions, manually invoke endpoints, view build logs.
- [ ] **Cloud Dataproc**
  - **Goal**: Tracking Spark LRO (Long Running Operations).
  - **Capabilities Required**: Monitor active jobs.
- [ ] **Logging and Monitoring (Cloud Logging / Operations Suite)**
  - **Goal**: Centralized log aggregator GUI.
  - **Capabilities Required**: Capture logs from all other MiniSky services via Docker log streams. Display them with timestamps, log producer source, and visually separate severity levels (Error, Info, Debug) with a legend.
