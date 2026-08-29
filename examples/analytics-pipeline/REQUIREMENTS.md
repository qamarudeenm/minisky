# Source Requirement — Community Support Thread (WhatsApp)

Verbatim transcript of the conversation that this example implements, extracted from the
screenshot supplied on 2026-08-29. Chat: `+234 806 045 37…`, referred from the
**DataEngineeringCommunity** group. Times are as shown on the device (17:56 screenshot time).

---

> **You · DataEngineeringCommunity** *(quoted message)*
> Do you have any questions or support.
> I am happy to render support, or you can rais…

**Them — 14:58**
> Hello Chief,
>
> Good afternoon trust you're doing great. I am yet to start using the emulator. However the
> problem I am trying to solve with it is simple ( Build an Analytics Engineering pipeline )
> without any cloud costs

**Them — 14:58**
> Would that be possible?

**Them — 14:59**
> Tools
>
> dbt
> Terraform
> Big Query
> Apache Airflow
> Presta *(sic — Presto/Trino)*

**You — 16:12**
> Yes,
> This is possible

**You — 16:13**
> Good afternoon

**You — 16:13** *(message truncated in screenshot)*
> If you have your data pipeline architecture,
>
> You can first have your terraform deployed vm, where you can install both dbt and Apache
> airflow, then connect to…

---

## Requirements this example has to satisfy

| # | Requirement | Source | Where it is met |
|---|-------------|--------|-----------------|
| R1 | End-to-end **Analytics Engineering pipeline** | "Build an Analytics Engineering pipeline" | [ARCHITECTURE.md](ARCHITECTURE.md) |
| R2 | **Zero cloud cost** — everything local | "without any cloud costs" | All APIs served by MiniSky on `localhost:8080` |
| R3 | **Terraform** provisions the infrastructure | tool list | `network.tf`, `storage.tf`, `bigquery.tf`, `pubsub.tf`, `compute.tf` |
| R4 | **BigQuery** as the warehouse | tool list | MiniSky BigQuery shim (DuckDB engine) — `bigquery.tf` |
| R5 | **dbt** for transformations | tool list | `pipeline/dbt/analytics`, installed by `provision.tf` |
| R6 | **Apache Airflow** for orchestration | tool list | `pipeline/dags`, installed by `provision.tf` |
| R7 | **Presto/Trino** as the interactive query layer | tool list ("Presta") | `scripts/install_stack.sh` (`install_trino = true`) |
| R8 | Terraform-deployed **VM** hosting dbt + Airflow | "terraform deployed vm, where you can install both dbt and Apache airflow" | `compute.tf` + `provision.tf` |
| R9 | Software installed **onto the VM OS** by Terraform | same | `provision.tf` → `scripts/install_stack.sh` |
| R10 | Every provisioned resource verifiably working | benchmark requirement | `scripts/verify_infra.sh` |
