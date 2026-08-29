output "project_id" {
  description = "Project every emulated resource was created under."
  value       = var.project_id
}

output "vpc_network" {
  description = "VPC name and the Docker network MiniSky created for it."
  value = {
    name           = google_compute_network.pipeline.name
    self_link      = google_compute_network.pipeline.self_link
    docker_network = "minisky-vpc-${google_compute_network.pipeline.name}"
  }
}

output "orchestrator_vm" {
  description = "The VM hosting Airflow, dbt and Trino."
  value = {
    name         = google_compute_instance.orchestrator.name
    zone         = google_compute_instance.orchestrator.zone
    machine_type = google_compute_instance.orchestrator.machine_type
    instance_id  = google_compute_instance.orchestrator.instance_id
    container    = local.vm_container_name
  }
}

output "buckets" {
  description = "Data lake buckets by zone."
  value = {
    raw       = google_storage_bucket.raw.name
    curated   = google_storage_bucket.curated.name
    artifacts = google_storage_bucket.artifacts.name
  }
}

output "bigquery_datasets" {
  description = "Warehouse datasets by medallion layer."
  value = {
    bronze_raw     = google_bigquery_dataset.raw.dataset_id
    silver_staging = google_bigquery_dataset.staging.dataset_id
    gold_marts     = google_bigquery_dataset.marts.dataset_id
  }
}

output "raw_tables" {
  description = "Raw tables Terraform owns (dbt owns everything downstream)."
  value = [
    "${google_bigquery_dataset.raw.dataset_id}.${google_bigquery_table.raw_orders.table_id}",
    "${google_bigquery_dataset.raw.dataset_id}.${google_bigquery_table.raw_customers.table_id}",
  ]
}

output "pubsub" {
  description = "Eventing resources."
  value = {
    topic        = google_pubsub_topic.ingestion_events.id
    subscription = google_pubsub_subscription.ingestion_events.id
  }
}

output "next_steps" {
  description = "How to reach the pipeline once apply completes."
  value       = <<-EOT
    Airflow / Trino host ports are assigned by Docker at container-create time.
    Print the live URLs and service health with:

      ./scripts/pipeline_status.sh

    Verify every provisioned resource end to end with:

      ./scripts/verify_infra.sh

    Trigger the pipeline:

      ./scripts/run_pipeline.sh
  EOT
}
