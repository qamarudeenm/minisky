# ─────────────────────────────────────────────────────────────────────────────
# Warehouse — medallion layout.
#
# MiniSky executes BigQuery SQL on an embedded DuckDB database
# (~/.minisky/data/bigquery.duckdb) when MINISKY_BQ_BACKEND=duckdb is set. Tables
# created here through tables.insert are materialised in DuckDB as
# "<dataset>__<table>", which is the same name the shim's SQL translator rewrites
# `project.dataset.table` references to at query time.
#
# Only the raw layer is declared in Terraform: staging and marts tables are owned
# by dbt, so Terraform provisions their datasets and stays out of dbt's way.
# ─────────────────────────────────────────────────────────────────────────────

resource "google_bigquery_dataset" "raw" {
  dataset_id    = local.datasets.raw
  friendly_name = "${var.name_prefix} raw"
  description   = "Landing layer — untransformed source extracts"
  location      = "US"
  labels        = merge(local.common_labels, { layer = "bronze" })
}

resource "google_bigquery_dataset" "staging" {
  dataset_id    = local.datasets.staging
  friendly_name = "${var.name_prefix} staging"
  description   = "dbt staging models — typed, cleaned, deduplicated"
  location      = "US"
  labels        = merge(local.common_labels, { layer = "silver" })
}

resource "google_bigquery_dataset" "marts" {
  dataset_id    = local.datasets.marts
  friendly_name = "${var.name_prefix} marts"
  description   = "dbt marts — facts and dimensions consumed by the serving layer"
  location      = "US"
  labels        = merge(local.common_labels, { layer = "gold" })
}

resource "google_bigquery_table" "raw_orders" {
  dataset_id          = google_bigquery_dataset.raw.dataset_id
  table_id            = "orders"
  description         = "Raw order lines loaded from gs://${local.buckets.raw}/seeds/orders"
  deletion_protection = false
  labels              = merge(local.common_labels, { layer = "bronze" })

  schema = jsonencode([
    { name = "order_id", type = "INT64", mode = "REQUIRED", description = "Source order identifier" },
    { name = "customer_id", type = "INT64", mode = "REQUIRED", description = "FK to raw.customers" },
    { name = "order_date", type = "DATE", mode = "REQUIRED", description = "Date the order was placed" },
    { name = "status", type = "STRING", mode = "REQUIRED", description = "placed | shipped | delivered | returned | cancelled" },
    { name = "payment_method", type = "STRING", mode = "NULLABLE", description = "card | transfer | wallet" },
    { name = "quantity", type = "INT64", mode = "REQUIRED", description = "Units ordered" },
    { name = "unit_price", type = "FLOAT64", mode = "REQUIRED", description = "Price per unit in USD" },
    { name = "discount", type = "FLOAT64", mode = "NULLABLE", description = "Fractional discount, 0.0-1.0" },
    { name = "loaded_at", type = "TIMESTAMP", mode = "NULLABLE", description = "Ingestion watermark" },
  ])
}

resource "google_bigquery_table" "raw_customers" {
  dataset_id          = google_bigquery_dataset.raw.dataset_id
  table_id            = "customers"
  description         = "Raw customer records loaded from gs://${local.buckets.raw}/seeds/customers"
  deletion_protection = false
  labels              = merge(local.common_labels, { layer = "bronze" })

  schema = jsonencode([
    { name = "customer_id", type = "INT64", mode = "REQUIRED", description = "Source customer identifier" },
    { name = "full_name", type = "STRING", mode = "REQUIRED", description = "Customer display name" },
    { name = "email", type = "STRING", mode = "NULLABLE", description = "Contact email" },
    { name = "country", type = "STRING", mode = "NULLABLE", description = "ISO country name" },
    { name = "signup_date", type = "DATE", mode = "NULLABLE", description = "Account creation date" },
    { name = "segment", type = "STRING", mode = "NULLABLE", description = "consumer | smb | enterprise" },
    { name = "loaded_at", type = "TIMESTAMP", mode = "NULLABLE", description = "Ingestion watermark" },
  ])
}
