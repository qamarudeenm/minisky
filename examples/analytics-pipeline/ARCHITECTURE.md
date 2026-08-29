# Architecture — Local Analytics Engineering Pipeline on MiniSky

Every box below runs on the developer's laptop. No GCP project, no billing account, no network
egress to Google. MiniSky serves the Google Cloud REST APIs on `localhost:8080`; Terraform, dbt,
Airflow and the Python clients all believe they are talking to Google.

## 1. System context

```mermaid
flowchart LR
    subgraph DEV["👩‍💻 Developer laptop"]
        direction LR
        TF["Terraform 1.16<br/>google provider 5.45"]
        MS["MiniSky v1.3.1<br/>API Gateway :8080<br/>Dashboard :8081"]
        DK["Docker Engine<br/>data plane"]
    end

    TF -->|"HTTP · custom_endpoint<br/>compute · storage · bigquery · pubsub"| MS
    MS -->|"containers · networks · port bindings"| DK
    MS -->|"embedded DuckDB<br/>~/.minisky/data/bigquery.duckdb"| BQ[("BigQuery engine")]
    MS -->|"fake-gcs-server"| GCS[("Cloud Storage")]
    MS -->|"gcloud pubsub emulator"| PS[("Pub/Sub")]
    DK --> VM["minisky-vm-analytics-orchestrator<br/>Ubuntu · Airflow · dbt · Trino"]
```

## 2. Provisioned infrastructure (what Terraform owns)

```mermaid
flowchart TB
    subgraph VPC["VPC · retail-analytics-vpc"]
        FW1["🔥 allow-airflow-ui<br/>INGRESS tcp:8080"]
        FW2["🔥 allow-trino-ui<br/>INGRESS tcp:8090"]
        VM["🖥️ analytics-orchestrator<br/>n1-standard-4 · Ubuntu 26.04<br/>container: minisky-vm-analytics-orchestrator"]
    end

    subgraph LAKE["☁️ Cloud Storage — data lake"]
        B1["retail-analytics-raw<br/>landing zone + seeds"]
        B2["retail-analytics-curated<br/>mart exports"]
        B3["retail-analytics-artifacts<br/>dbt docs, run logs"]
    end

    subgraph WH["📊 BigQuery — warehouse"]
        D1["retail_raw<br/>orders · customers"]
        D2["retail_staging<br/>dbt views/tables"]
        D3["retail_marts<br/>dbt facts + dims"]
    end

    subgraph EVT["📨 Pub/Sub"]
        T1["retail-ingestion-events"]
        S1["retail-ingestion-events-sub"]
    end

    FW1 --> VM
    FW2 --> VM
    VM --> LAKE
    VM --> WH
    VM --> EVT
    T1 --> S1
```

Ordering constraint enforced in `compute.tf`: **firewall rules are created before the VM**.
MiniSky binds container ports at container-create time, so a firewall rule added after the VM
exists forces a tear-down/re-provision of that VM — which would wipe the software installed on it.
`google_compute_instance.orchestrator` therefore carries an explicit `depends_on` for both rules.

## 3. Software installed on the VM OS (what Terraform provisions)

```mermaid
flowchart TB
    subgraph VMOS["minisky-vm-analytics-orchestrator · Ubuntu"]
        BASE["apt: curl · ca-certificates · git · unzip · procps · tzdata"]
        PY["Python: the image's own if Airflow supports it<br/>(ubuntu:24.04 ships 3.12), else uv fetches one"]
        VENV["/opt/pipeline/venv"]
        AF["apache-airflow 2.10.5<br/>+ constraints file · SQLite meta-db · LocalExecutor"]
        DBT["dbt-core + dbt-bigquery"]
        SHIM["sitecustomize.py<br/>anonymous creds + BigQuery endpoint → MiniSky"]
        JAVA["openjdk (24 → 23 → 21 fallback)<br/><i>optional</i>"]
        TRINO["Trino server + CLI<br/>catalogs: memory, tpch<br/><i>opt-in: install_trino = true</i>"]
    end

    BASE --> PY --> VENV
    VENV --> AF
    VENV --> DBT
    VENV --> SHIM
    BASE --> JAVA --> TRINO
```

`terraform_data.install_stack` runs `scripts/install_stack.sh`, which drives the install with
`docker exec` against the container MiniSky created for the VM. Re-running `terraform apply` only
re-installs when the script or its inputs change (hash-based `triggers_replace`).

> **Why `docker exec` and not a `metadata.startup-script`?**
> MiniSky v1.3.1 stores instance metadata faithfully but its compute shim boots the VM container
> with `tail -f /dev/null` and never executes `startup-script` (`pkg/shims/compute/api.go:508`).
> There is also no SSH daemon in the VM image, so `remote-exec` has nothing to connect to.
> `docker exec` is the honest local equivalent of the GCE guest agent, and it keeps the whole
> install inside the Terraform graph. See [FIDELITY-NOTES.md](FIDELITY-NOTES.md).

## 4. Runtime data flow (the Airflow DAG)

```mermaid
sequenceDiagram
    autonumber
    participant AF as Airflow (VM)
    participant GCS as Cloud Storage (MiniSky)
    participant BQ as BigQuery / DuckDB (MiniSky)
    participant DBT as dbt-bigquery (VM)
    participant PS as Pub/Sub (MiniSky)

    AF->>GCS: download raw_orders.csv / raw_customers.csv
    AF->>BQ: INSERT INTO retail_raw.* (batched query jobs)
    AF->>PS: publish "raw.loaded" event
    AF->>DBT: dbt run  (staging → marts)
    DBT->>BQ: CREATE OR REPLACE TABLE retail_staging.* / retail_marts.*
    AF->>DBT: dbt test (not_null / unique / accepted_values)
    AF->>BQ: SELECT * FROM retail_marts.fct_daily_revenue
    AF->>GCS: upload mart extract to retail-analytics-curated
    AF->>PS: publish "marts.published" event
```

## 5. Layered model (medallion)

```mermaid
flowchart LR
    SRC["seeds/*.csv<br/>(uploaded by Terraform)"] --> RAW
    RAW["🥉 retail_raw<br/>orders · customers<br/>schema from Terraform"] --> STG
    STG["🥈 retail_staging<br/>stg_orders · stg_customers<br/>typed, cleaned, deduped"] --> MRT
    MRT["🥇 retail_marts<br/>dim_customers<br/>fct_orders<br/>fct_daily_revenue"] --> SERVE
    SERVE["🔎 Serving<br/>BigQuery REST · MiniSky dashboard<br/>curated GCS extracts"]
```

## 6. Component ownership

| Layer | Tool | Provisioned by | Runs where |
|-------|------|----------------|-----------|
| Infrastructure | Terraform | you | laptop → MiniSky API |
| Emulated GCP | MiniSky v1.3.1 | `minisky start` | laptop (Go binary + Docker) |
| Compute host | GCE VM | `compute.tf` | Docker container |
| Orchestration | Airflow 2.10.5 | `provision.tf` | VM |
| Transformation | dbt-core + dbt-bigquery | `provision.tf` | VM |
| Warehouse | BigQuery (DuckDB) | `bigquery.tf` | MiniSky process |
| Lake | Cloud Storage (fake-gcs-server) | `storage.tf` | Docker container |
| Eventing | Pub/Sub (gcloud emulator) | `pubsub.tf` | Docker container |
| Interactive query | BigQuery REST / dashboard | — | MiniSky process |
| Federation (opt-in) | Trino | `provision.tf`, `install_trino = true` | VM |

## 7. Why there is no Trino in the default path

The original tool list included Presto/Trino, so the installer still supports it
(`install_trino = true`). It is off by default because nothing in this pipeline needs it:

```mermaid
flowchart LR
    subgraph DIRECT["Default — everything connects to BigQuery directly"]
        DBT2["dbt-core<br/>dbt-bigquery adapter"] -->|"REST /bigquery/v2"| BQ2[("BigQuery<br/>MiniSky · DuckDB")]
        AF2["Airflow tasks"] -->|"REST /bigquery/v2"| BQ2
        AN2["Analyst / BI tool"] -->|"REST · dashboard"| BQ2
    end

    subgraph FED["Opt-in — Trino only pays for itself when federating"]
        TR2["Trino"] --> BQ3[("BigQuery")]
        TR2 --> PG3[("Postgres")]
        TR2 --> OBJ3[("Object store")]
        TR2 --> KFK3[("Kafka")]
    end
```

* **dbt** speaks to BigQuery through the adapter plus MiniSky's endpoint override — a Trino
  hop would add latency and a second SQL dialect to translate through.
* **Airflow** tasks call the same REST APIs the adapter does.
* **Analysts** query BigQuery directly, or browse it in the MiniSky dashboard on `:8081`.
* Trino's `memory` catalog would hold a **duplicate** of the marts that has to be reloaded on
  every restart, and drags in a JDK plus a ~1GB server download.

Turn it on when the exercise is federation — one SQL endpoint over BigQuery *and* an operational
Postgres *and* files — which is the problem Trino actually solves.
