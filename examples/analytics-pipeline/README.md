# Analytics Engineering Pipeline — zero cloud cost, all local

An end-to-end analytics engineering stack built entirely on [MiniSky](../../README.md): Terraform
provisions the infrastructure, installs the toolchain onto a VM, and deploys an Airflow DAG that
loads a data lake into BigQuery and transforms it with dbt. No GCP project, no billing account, no
network calls to Google.

It answers the community question transcribed in [REQUIREMENTS.md](REQUIREMENTS.md) — *"Build an
Analytics Engineering pipeline without any cloud costs"* using dbt, Terraform, BigQuery, Airflow
(and optionally Presto/Trino).

* **[ARCHITECTURE.md](ARCHITECTURE.md)** — diagrams: system context, provisioned infrastructure,
  installed software, runtime data flow, and why Trino is optional.
* **[FIDELITY-NOTES.md](FIDELITY-NOTES.md)** — the emulator gaps this build uncovered, with fixes.

---

## What gets created

| Layer | Resources |
|---|---|
| Network | 1 VPC (`retail-analytics-vpc`) + 2 firewall rules |
| Compute | 1 GCE VM (`analytics-orchestrator`) running Airflow + dbt |
| Lake | 3 GCS buckets (raw / curated / artifacts) + 2 seed objects |
| Warehouse | 3 BigQuery datasets (raw / staging / marts) + 2 raw tables |
| Eventing | 1 Pub/Sub topic + 1 subscription |
| Software | uv + CPython 3.12, Apache Airflow 2.10.5, dbt-core + dbt-bigquery, *(optional)* JDK + Trino |
| Code | 1 Airflow DAG, 5 dbt models, 14 dbt tests |

## Prerequisites

```bash
docker info >/dev/null && terraform version    # Docker running, Terraform ≥ 1.5
```

MiniSky must be running **with the DuckDB backend enabled** — without it BigQuery accepts SQL but
returns empty results:

```bash
export MINISKY_BQ_BACKEND=duckdb
minisky start                      # API :8080, dashboard :8081
```

## Run it

```bash
cd examples/analytics-pipeline
cp terraform.tfvars.example terraform.tfvars     # optional

terraform init
terraform plan                                   # review before applying
terraform apply

./scripts/verify_infra.sh                        # prove every resource works
python3 scripts/verify_warehouse.py              # prove the warehouse handles real SQL
./scripts/run_pipeline.sh                        # trigger the DAG and wait
./scripts/verify_infra.sh --data                 # prove the pipeline produced data
```

`terraform apply` does four things in order: creates the infrastructure, waits for the VM's
container, installs the toolchain onto the VM OS, deploys the DAG and dbt project, then starts
Airflow. First apply downloads Airflow and dbt into the VM, so budget 5–10 minutes; subsequent
applies only redo the stages whose inputs changed.

## Reaching the pipeline

Docker assigns the host port when MiniSky binds the VM's firewall ports, so it is not fixed:

```bash
./scripts/pipeline_status.sh
```

```
  VM container      : minisky-vm-analytics-orchestrator (running)
  Airflow           : up           http://127.0.0.1:49187  (admin / admin)
  Trino             : not installed
  MiniSky (from VM) : http://172.19.0.1:8080
  Dashboard         : http://localhost:8081
```

## Working on it

```bash
./scripts/run_pipeline.sh --dbt-only     # run dbt build without Airflow
./scripts/run_pipeline.sh --no-wait      # trigger and return
docker exec -it minisky-vm-analytics-orchestrator bash -lc \
  'set -a; . /opt/pipeline/env.sh; set +a; exec bash'   # shell inside the VM, env loaded
```

Editing a dbt model or the DAG and re-running `terraform apply` redeploys the code — the
`terraform_data.deploy_pipeline` node is keyed on a hash of `pipeline/`.

## Layout

```
├── versions.tf providers.tf variables.tf locals.tf   provider + configuration
├── network.tf storage.tf bigquery.tf pubsub.tf       the emulated GCP resources
├── compute.tf                                        the orchestrator VM
├── provision.tf                                      install → deploy → start, in the TF graph
├── outputs.tf
├── seeds/                                            source CSVs, uploaded to the raw bucket
├── scripts/
│   ├── install_stack.sh deploy_pipeline.sh           driven by terraform_data
│   ├── start_services.sh wait_for_vm.sh
│   ├── verify_infra.sh pipeline_status.sh            operator tools
│   ├── run_pipeline.sh startup-script.sh
│   └── guest/install.sh guest/sitecustomize.py       run inside the VM
└── pipeline/
    ├── dags/retail_analytics_pipeline.py
    ├── lib/minisky_client.py                         REST client for the emulated APIs
    └── dbt/analytics/                                dbt project (staging + marts)
```

## How the pieces connect to MiniSky

| Component | Mechanism |
|---|---|
| Terraform | `*_custom_endpoint` on the `google` provider → `localhost:8080` |
| Airflow tasks | `pipeline/lib/minisky_client.py` — plain REST, no `google-cloud-*` dependency |
| dbt | stock `dbt-bigquery` adapter + `sitecustomize.py` (anonymous credentials + endpoint override) |
| VM → MiniSky | the Docker network gateway address, resolved at provision time (a container's `localhost` is not the host) |

Point `MINISKY_ENDPOINT` and the provider endpoints at real GCP and the DAG, the dbt project and
the Terraform all run unchanged.

## Verified end to end

Against MiniSky built from this repository (`MINISKY_BQ_BACKEND=duckdb`), on Docker Desktop:

```
scripts/verify_infra.sh --data     41 passed, 0 failed
scripts/run_pipeline.sh            DAG run succeeded
```

The DAG loads the seed CSVs from the emulated GCS bucket into BigQuery, dbt builds two staging
models and three marts and runs 14 tests against them, the headline mart is exported back to the
curated bucket, and each stage announces itself on Pub/Sub. `fct_daily_revenue` ends up holding 30
days of revenue:

| order_date | orders_total | net_revenue |
|---|---|---|
| 2026-07-01 | 7 | 2893.11 |
| 2026-07-02 | 10 | 4654.69 |
| 2026-07-03 | 6 | 1427.59 |

Getting there required nine fixes to MiniSky itself — see [FIDELITY-NOTES.md](FIDELITY-NOTES.md).
On a MiniSky older than those fixes this example will not complete.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `terraform apply` fails on the first BigQuery resource | MiniSky not running, or started without `MINISKY_BQ_BACKEND=duckdb` |
| No host port for Airflow | the firewall rule was created after the VM — `terraform apply` again, the `depends_on` in `compute.tf` fixes the order |
| dbt errors on decimal values | you are on a MiniSky older than the SQL-translator fix — see [FIDELITY-NOTES.md](FIDELITY-NOTES.md) item 1 |
| VM never leaves PROVISIONING | the boot image could not be pulled. Set `vm_image` to something already local (`docker images`), e.g. `vm_image = "python:3.12-slim"` |
| Buckets hang on create | MiniSky is cold-starting `fsouza/fake-gcs-server` and cannot pull it. Pull it once by hand, then re-apply |
| Install stage stalls | Airflow and dbt come from PyPI; on a slow link the first apply is long. `terraform apply` is resumable — re-run it |
| Airflow unhealthy after apply | `docker exec minisky-vm-analytics-orchestrator tail -100 /opt/pipeline/logs/airflow.log` |
| Everything gone after editing a firewall rule | expected — MiniSky re-provisions VMs on a firewall change; re-run `terraform apply` to reinstall |
