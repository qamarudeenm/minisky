output "bucket_url" {
  value = google_storage_bucket.demo_assets.url
}

output "pubsub_topic" {
  value = google_pubsub_topic.demo_topic.id
}

output "bigquery_table" {
  value = "${google_bigquery_dataset.demo_dataset.dataset_id}.${google_bigquery_table.demo_table.table_id}"
}

output "compute_instance_status" {
  value = google_compute_instance.demo_vm.instance_id
}
