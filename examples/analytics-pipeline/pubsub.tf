# ─────────────────────────────────────────────────────────────────────────────
# Pipeline eventing. The DAG publishes a message when raw data lands and again
# when the marts are refreshed, so downstream consumers can be event-driven
# instead of polling. Backed by the real gcloud Pub/Sub emulator.
# ─────────────────────────────────────────────────────────────────────────────
resource "google_pubsub_topic" "ingestion_events" {
  name   = "${var.name_prefix}-ingestion-events"
  labels = local.common_labels
}

resource "google_pubsub_subscription" "ingestion_events" {
  name  = "${var.name_prefix}-ingestion-events-sub"
  topic = google_pubsub_topic.ingestion_events.name

  ack_deadline_seconds       = 20
  message_retention_duration = "600s"
  labels                     = local.common_labels
}
