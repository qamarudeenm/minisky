"""Retail analytics pipeline — extract → load → transform → publish.

Runs on the Terraform-provisioned VM against MiniSky's emulated Google Cloud
APIs. Nothing in this DAG is emulator-specific except the endpoint it reads from
the environment: point MINISKY_ENDPOINT at real GCP and the same tasks run
against BigQuery proper.

    seeds in GCS ──▶ raw tables (BigQuery) ──▶ dbt staging ──▶ dbt marts
                          │                                      │
                          └── Pub/Sub "raw.loaded"               ├── CSV extract → curated bucket
                                                                 └── Pub/Sub "marts.published"
"""

from __future__ import annotations

import csv
import io
import os
from datetime import datetime, timedelta, timezone

import pendulum
from airflow.decorators import dag, task
from airflow.operators.bash import BashOperator

from minisky_client import MiniSky, parse_csv, rows_to_insert

# ── Configuration (written by terraform into /opt/pipeline/env.sh) ──────────
DATASET_RAW = os.environ.get("BQ_DATASET_RAW", "retail_raw")
DATASET_STAGING = os.environ.get("BQ_DATASET_STAGING", "retail_staging")
DATASET_MARTS = os.environ.get("BQ_DATASET_MARTS", "retail_marts")
BUCKET_RAW = os.environ.get("BUCKET_RAW", "retail-analytics-raw")
BUCKET_CURATED = os.environ.get("BUCKET_CURATED", "retail-analytics-curated")
PUBSUB_TOPIC = os.environ.get("PUBSUB_TOPIC", "retail-ingestion-events")
DBT_BIN = os.environ.get("DBT_BIN", "/opt/pipeline/dbt-venv/bin/dbt")
DBT_PROJECT_DIR = os.environ.get("DBT_PROJECT_DIR", "/opt/pipeline/dbt/analytics")
DBT_PROFILES_DIR = os.environ.get("DBT_PROFILES_DIR", "/opt/pipeline/dbt")

# Source definitions: object in the raw bucket → raw table + column types.
SOURCES = {
    "orders": {
        "object": "seeds/orders/raw_orders.csv",
        "table": "orders",
        "types": {
            "order_id": "int",
            "customer_id": "int",
            "quantity": "int",
            "unit_price": "float",
            "discount": "float",
        },
    },
    "customers": {
        "object": "seeds/customers/raw_customers.csv",
        "table": "customers",
        "types": {"customer_id": "int"},
    },
}


def _coerce_row(header: list[str], row: list[str], types: dict[str, str]) -> list:
    coerced = []
    for column, value in zip(header, row):
        kind = types.get(column, "string")
        if value == "":
            coerced.append(None)
        elif kind == "int":
            coerced.append(int(value))
        elif kind == "float":
            coerced.append(float(value))
        else:
            coerced.append(value)
    return coerced


@dag(
    dag_id="retail_analytics_pipeline",
    description="End-to-end analytics engineering pipeline on MiniSky",
    schedule="0 6 * * *",
    start_date=pendulum.datetime(2026, 1, 1, tz="UTC"),
    catchup=False,
    max_active_runs=1,
    default_args={
        "owner": "analytics-engineering",
        "retries": 2,
        "retry_delay": timedelta(seconds=30),
    },
    tags=["minisky", "analytics-engineering", "dbt", "bigquery"],
)
def retail_analytics_pipeline() -> None:
    @task
    def preflight() -> dict:
        """Fail fast if the infrastructure Terraform created is not reachable."""
        client = MiniSky.from_env()
        for dataset, table in (
            (DATASET_RAW, "orders"),
            (DATASET_RAW, "customers"),
        ):
            if not client.bq_table_exists(dataset, table):
                raise RuntimeError(f"missing raw table {dataset}.{table} — run terraform apply")

        objects = {o.get("name") for o in client.gcs_list(BUCKET_RAW, prefix="seeds/")}
        expected = {source["object"] for source in SOURCES.values()}
        missing = expected - objects
        if missing:
            raise RuntimeError(f"missing seed objects in gs://{BUCKET_RAW}: {sorted(missing)}")

        return {"endpoint": client.endpoint, "objects": sorted(objects)}

    @task
    def load_source(source_name: str) -> dict:
        """Land one source object from the lake into its raw BigQuery table."""
        client = MiniSky.from_env()
        source = SOURCES[source_name]
        table = source["table"]

        raw_bytes = client.gcs_download(BUCKET_RAW, source["object"])
        header, rows = parse_csv(raw_bytes)
        loaded_at = datetime.now(timezone.utc).replace(microsecond=0).isoformat()

        columns = header + ["loaded_at"]
        typed_rows = [_coerce_row(header, row, source["types"]) + [loaded_at] for row in rows]

        # Full refresh: the raw layer mirrors the current source extract.
        client.bq_execute(f"DELETE FROM {DATASET_RAW}.{table} WHERE TRUE")
        for statement in rows_to_insert(DATASET_RAW, table, columns, typed_rows):
            client.bq_execute(statement)

        count = client.bq_query(f"SELECT COUNT(*) AS row_count FROM {DATASET_RAW}.{table}")
        loaded = int(count[0]["row_count"]) if count else 0
        if loaded != len(typed_rows):
            raise RuntimeError(
                f"{DATASET_RAW}.{table}: loaded {loaded} rows, expected {len(typed_rows)}"
            )
        return {"table": table, "rows": loaded, "loaded_at": loaded_at}

    @task
    def announce_raw_loaded(results: list[dict]) -> None:
        client = MiniSky.from_env()
        # A mapped task's collected XComs arrive as a lazy sequence, which the
        # JSON encoder cannot serialise — materialise it before it goes into a
        # message body.
        client.publish(
            PUBSUB_TOPIC,
            {"event": "raw.loaded", "tables": list(results)},
            attributes={"stage": "raw", "pipeline": "retail_analytics"},
        )

    dbt_build = BashOperator(
        task_id="dbt_build",
        bash_command=(
            f"cd {DBT_PROJECT_DIR} && "
            f"{DBT_BIN} build --profiles-dir {DBT_PROFILES_DIR} --target local"
        ),
        doc_md="Runs every staging and mart model, then every dbt test, in dependency order.",
    )

    @task
    def export_marts() -> dict:
        """Publish the headline mart to the curated zone as a CSV extract."""
        client = MiniSky.from_env()
        rows = client.bq_query(
            f"SELECT * FROM {DATASET_MARTS}.fct_daily_revenue ORDER BY order_date"
        )
        if not rows:
            raise RuntimeError(f"{DATASET_MARTS}.fct_daily_revenue returned no rows")

        buffer = io.StringIO()
        writer = csv.DictWriter(buffer, fieldnames=list(rows[0].keys()))
        writer.writeheader()
        writer.writerows(rows)

        object_name = (
            f"marts/fct_daily_revenue/"
            f"{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}.csv"
        )
        client.gcs_upload(BUCKET_CURATED, object_name, buffer.getvalue().encode())
        return {"object": f"gs://{BUCKET_CURATED}/{object_name}", "rows": len(rows)}

    @task
    def announce_marts_published(extract: dict) -> None:
        client = MiniSky.from_env()
        client.publish(
            PUBSUB_TOPIC,
            {"event": "marts.published", "extract": extract},
            attributes={"stage": "marts", "pipeline": "retail_analytics"},
        )

    checked = preflight()
    loaded = load_source.expand(source_name=list(SOURCES))
    raw_event = announce_raw_loaded(loaded)
    extract = export_marts()

    checked >> loaded >> raw_event >> dbt_build >> extract >> announce_marts_published(extract)


retail_analytics_pipeline()
