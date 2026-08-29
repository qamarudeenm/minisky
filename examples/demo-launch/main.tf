# 📦 Cloud Storage: High-Fidelity Bucket Creation
resource "google_storage_bucket" "demo_assets" {
  name     = "minisky-demo-assets"
  location = "US"
  force_destroy = true

  labels = {
    env = "demo"
  }
}

# 📨 Pub/Sub: Real-time Message Bus
resource "google_pubsub_topic" "demo_topic" {
  name = "minisky-demo-events"
}

resource "google_pubsub_subscription" "demo_sub" {
  name  = "minisky-demo-sub"
  topic = google_pubsub_topic.demo_topic.name

  ack_deadline_seconds = 20
}

# 📊 BigQuery: DuckDB-powered Analytics
resource "google_bigquery_dataset" "demo_dataset" {
  dataset_id                  = "minisky_analytics"
  friendly_name               = "Demo Dataset"
  description                 = "Analytics dataset powered by MiniSky"
  location                    = "US"
}

resource "google_bigquery_table" "demo_table" {
  dataset_id          = google_bigquery_dataset.demo_dataset.dataset_id
  table_id            = "event_logs"
  deletion_protection = false

  schema = <<EOF
[
  {
    "name": "event_time",
    "type": "TIMESTAMP",
    "mode": "REQUIRED"
  },
  {
    "name": "event_type",
    "type": "STRING",
    "mode": "REQUIRED"
  },
  {
    "name": "payload",
    "type": "JSON",
    "mode": "NULLABLE"
  }
]
EOF
}

# 🖥️ Compute Engine: Demonstrates LROs (Long Running Operations)
resource "google_compute_instance" "demo_vm" {
  name         = "minisky-demo-instance"
  machine_type = "n1-standard-1"
  zone         = "us-central1-a"

  boot_disk {
    initialize_params {
      image = "ubuntu-2604-lts"
    }
  }

  network_interface {
    network = "default"
  }

  metadata = {
    demo = "true"
  }
}
