# MiniSky: Deep Research & Service Mapping

This document details the research findings for emulating complex GCP services like Compute Engine, GKE, and Networking. It provides a roadmap for how MiniSky will handle services that lack official emulators.

## 1. The "Infrastructure Gap" Analysis

| GCP Service | Official Emulator? | Community Emulator? | MiniSky Strategy |
| :--- | :---: | :---: | :--- |
| **Compute Engine (GCE)** | ❌ | ❌ | **API Shim + Docker:** Mimic the GCE API. When a VM is created, start a persistent Docker container with a custom SSH/Metadata server. |
| **Kubernetes Engine (GKE)**| ❌ | ❌ | **API Shim + Kind/Minikube:** Mimic the GKE API. When a cluster is requested, spawn a `kind` cluster locally. |
| **VPC Networking** | ❌ | ❌ | **Docker Networks:** Map VPCs and Subnets to Docker bridge networks. Simulate firewall rules using local `iptables` or proxy-level ACLs. |
| **Cloud SQL** | ❌ | ✅ (Indirect) | **Native Containers:** Automatically spin up the requested DB engine (Postgres/MySQL) via Docker. |
| **Cloud Armor** | ❌ | ❌ | **ModSecurity/Nginx:** Map WAF rules to an Nginx/ModSecurity proxy layer that MiniSky manages. |
| **Cloud Load Balancing** | ❌ | ❌ | **HAProxy/Envoy:** Use a dynamic load balancer container to simulate Global/Regional LBs. |
| **Cloud DNS** | ❌ | ✅ (`coredns`) | **CoreDNS Integration:** Use CoreDNS to manage local `.internal` domains and simulate GCP DNS zones. |

---

## 2. Emulating Complex Services (Deep Dive)

### A. Compute Engine (GCE)
**The Challenge:** GCE involves VMs, disks, and metadata, but critically it relies on **Operations** and **Instance States**.
**MiniSky Approach:**
1. **API Shim:** Implement the `compute.v1` REST API using Discovery Doc validation.
2. **Instance Mapping:** When `instances.insert` is called, MiniSky returns an **Operation (LRO)** immediately.
3. **State Transitions:** In the background, MiniSky transitions the instance through `PROVISIONING` -> `STAGING` -> `RUNNING`. This allows Terraform to correctly wait for completion.
4. **Metadata Service:** MiniSky runs a tiny internal web server at `169.254.169.254` within the container's network to provide VM-specific metadata.

### B. Google Kubernetes Engine (GKE)
**The Challenge:** Building a local GKE requires a Kubernetes control plane and realistic **Cluster Status** reporting.
**MiniSky Approach:**
1. **Cluster API:** Implement the `container.v1` API.
2. **Orchestration:** When a user calls `clusters.create`, MiniSky registers an **Operation** and initiates a background `kind create cluster`.
3. **Endpoint Mapping:** The Cluster object only moves to `RUNNING` once the `kind` control plane is reachable. MiniSky returns a local `kubeconfig` compatible with the newly created cluster.

### C. Networking & Cloud Armor
**The Challenge:** Simulated firewalling and WAF.
**MiniSky Approach:**
1. **VPC Simulation:** Each VPC created via Terraform/CLI creates a dedicated Docker Network.
2. **Cloud Armor:** Map the JSON-based Cloud Armor rules to **Nginx/ModSecurity** configuration. This allows developers to test their "XSS" or "SQLi" block rules locally.
3. **Global Load Balancer:** Use an **Envoy** proxy as the entry point for all "Load Balanced" services, allowing for header-based routing and health check simulation.

---

## 3. The "Persistence" Strategy
For services like Cloud SQL, GCS, and GCE, MiniSky will use **Docker Volumes** mapped to a local `.minisky/data` directory.
- **Snapshotting:** The `minisky state save` command will `docker commit` running containers and tarball the data volumes to ensure a perfect state capture.

## 4. Summary of Gaps & Work-in-Progress
- **High-Fidelity IAM:** MiniSky implements a lightweight **IAM Policy Engine** that validates `Principal` and `Permission` strings against a local registry for every request.
- **Contract Enforcement:** All API shims are wired to a central **Discovery Doc Validator** to ensure schema parity.
- **Cloud KMS:** A simple AES-based stub for encryption/decryption keys, supporting standard `cryptoKey` resource schemas.
- **Logging/Monitoring:** Aggregate logs from all managed containers into a single stream, exposed via a local Log Viewer in the Dashboard.
