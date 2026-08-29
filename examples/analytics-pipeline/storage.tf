# ─────────────────────────────────────────────────────────────────────────────
# Data lake — three buckets, one per zone of responsibility.
# Backed by fake-gcs-server inside MiniSky, so uploads are real objects on disk.
# ─────────────────────────────────────────────────────────────────────────────
resource "google_storage_bucket" "raw" {
  name          = local.buckets.raw
  location      = "US"
  force_destroy = true
  labels        = merge(local.common_labels, { zone = "raw" })
}

resource "google_storage_bucket" "curated" {
  name          = local.buckets.curated
  location      = "US"
  force_destroy = true
  labels        = merge(local.common_labels, { zone = "curated" })
}

resource "google_storage_bucket" "artifacts" {
  name          = local.buckets.artifacts
  location      = "US"
  force_destroy = true
  labels        = merge(local.common_labels, { zone = "artifacts" })
}

# ─────────────────────────────────────────────────────────────────────────────
# Seed data lands in the raw bucket so the DAG's first task reads from the lake,
# exactly as it would against real GCS.
# ─────────────────────────────────────────────────────────────────────────────
resource "google_storage_bucket_object" "seed_orders" {
  name         = "seeds/orders/raw_orders.csv"
  bucket       = google_storage_bucket.raw.name
  source       = "${path.module}/seeds/raw_orders.csv"
  content_type = "text/csv"
}

resource "google_storage_bucket_object" "seed_customers" {
  name         = "seeds/customers/raw_customers.csv"
  bucket       = google_storage_bucket.raw.name
  source       = "${path.module}/seeds/raw_customers.csv"
  content_type = "text/csv"
}
