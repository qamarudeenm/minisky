# MiniSky Comparison Battlecard: MiniSky vs. The Competition

This document is for the core team to understand exactly where MiniSky wins and how to address competitor strengths.

## 1. Feature Breakdown

| Feature | Official GCP Emulators | LocalStack (for GCP) | **MiniSky** |
| :--- | :--- | :--- | :--- |
| **Philosophy** | "Good enough" mocks | Cloud-agnostic (secondary focus) | **High-Fidelity native Go shim** |
| **Unified Binary** | No (multiple processes) | Yes (Docker) | **Yes (Single Go Binary)** |
| **LRO Support** | Minimal/None | Basic | **Native Async Manager (First-class)** |
| **Discovery Doc Validation**| No | No | **Yes (Strict Schema Enforcement)** |
| **BigQuery Support** | Sandbox/Cloud-only | N/A | **Native DuckDB Integration (Offline)**|
| **IAM Mocking** | No | Paid Tier Only | **Free/Included (RBAC Engine)** |
| **Dashboard** | None | Paid Tier Only | **Free/Included (GCP-Style Console)**|
| **Resource Usage** | Static (Java-heavy) | Static (Python/Java) | **Dynamic (Go Lazy-Loading)** |

## 2. Competitive Narratives (How to Win)

### VS. Official Emulators
- **Weakness:** Fragmented, missing critical services (Compute, GKE, BigQuery), hard to wire together.
- **The MiniSky Play:** "Why manage 5 containers when you can run one? MiniSky provides the services Google forgot, with the fidelity they skipped."

### VS. LocalStack (GCP Support)
- **Weakness:** LocalStack is AWS-first. Their GCP support is often secondary or gated behind a $35/user/month license.
- **The MiniSky Play:** "GCP isn't a second-class citizen here. MiniSky was built *by* GCP developers *for* GCP developers. And it's 100% free."

## 3. Handling Objections

**Objection:** *"Why should I trust a community emulator over Google's official one?"*
- **Response:** "We use Google's own Discovery Documents to validate every API call. We don't just mock the response; we enforce the contract. Plus, we fill the gaps for services Google doesn't emulate at all."

**Objection:** *"Is it hard to migrate my existing code?"*
- **Response:** "Zero code changes required. MiniSky acts as a drop-in replacement. You only change your `*_EMULATOR_HOST` variables or Terraform provider endpoints."

**Objection:** *"Does it support [Niche Service X]?"*
- **Response:** "If it's on our roadmap, we're building it with high fidelity. If not, you can use our Plugin system to wrap any Docker-based emulator in minutes."
