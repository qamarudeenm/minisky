# ─────────────────────────────────────────────────────────────────────────────
# Software provisioning
#
# Terraform owns the whole lifecycle: infrastructure, the OS packages on the VM,
# the pipeline code, and the running services. Each stage is a separate
# terraform_data node so a change to (say) a dbt model redeploys the code without
# reinstalling Airflow.
#
# The transport is `docker exec` against the container MiniSky created for the
# instance. MiniSky v1.3.1 boots VM containers with `tail -f /dev/null` and runs
# no guest agent, so neither metadata.startup-script nor remote-exec/SSH can
# deliver software. FIDELITY-NOTES.md tracks what it would take to change that.
# ─────────────────────────────────────────────────────────────────────────────

locals {
  provision_env = {
    VM_CONTAINER         = local.vm_container_name
    VM_NAME              = var.vm_name
    ZONE                 = var.zone
    MINISKY_HOST         = var.minisky_host
    MINISKY_PORT         = tostring(var.minisky_port)
    MINISKY_ENDPOINT     = local.minisky_endpoint
    MINISKY_ACCESS_TOKEN = var.minisky_access_token
    PROJECT_ID           = var.project_id
    NAME_PREFIX          = var.name_prefix
    DATASET_RAW          = local.datasets.raw
    DATASET_STAGING      = local.datasets.staging
    DATASET_MARTS        = local.datasets.marts
    BUCKET_RAW           = local.buckets.raw
    BUCKET_CURATED       = local.buckets.curated
    BUCKET_ARTIFACTS     = local.buckets.artifacts
    PUBSUB_TOPIC         = google_pubsub_topic.ingestion_events.name
    AIRFLOW_PORT         = tostring(var.airflow_port)
    TRINO_PORT           = tostring(var.trino_port)
    PYTHON_VERSION       = var.python_version
    AIRFLOW_VERSION      = var.airflow_version
    DBT_BIGQUERY_SPEC    = var.dbt_bigquery_version
    INSTALL_TRINO        = var.install_trino ? "true" : "false"
    TRINO_VERSION        = var.trino_version
    PROVISION_TIMEOUT    = var.provision_timeout
    HOST_CACHE_DIR       = pathexpand("~/.minisky/cache/analytics-pipeline")
  }

  pipeline_fingerprint = sha256(join(",", [
    for f in sort(fileset("${path.module}/pipeline", "**")) :
    "${f}:${filesha256("${path.module}/pipeline/${f}")}"
  ]))

  guest_fingerprint = sha256(join(",", [
    for f in sort(fileset("${path.module}/scripts/guest", "**")) :
    "${f}:${filesha256("${path.module}/scripts/guest/${f}")}"
  ]))
}

# 1. Wait for MiniSky's LRO to finish and the backing container to report healthy.
resource "terraform_data" "vm_ready" {
  triggers_replace = [google_compute_instance.orchestrator.instance_id]

  provisioner "local-exec" {
    command     = "${path.module}/scripts/wait_for_vm.sh"
    environment = local.provision_env
  }
}

# 2. Install the OS packages and the Python/Java toolchain onto the VM.
resource "terraform_data" "install_stack" {
  triggers_replace = [
    terraform_data.vm_ready.id,
    filesha256("${path.module}/scripts/install_stack.sh"),
    local.guest_fingerprint,
    var.python_version,
    var.airflow_version,
    var.dbt_bigquery_version,
    var.install_trino,
    var.trino_version,
  ]

  provisioner "local-exec" {
    command     = "${path.module}/scripts/install_stack.sh"
    environment = local.provision_env
  }
}

# 3. Deploy the DAGs, the dbt project and the rendered configuration.
resource "terraform_data" "deploy_pipeline" {
  triggers_replace = [
    terraform_data.install_stack.id,
    local.pipeline_fingerprint,
    filesha256("${path.module}/scripts/deploy_pipeline.sh"),
    google_bigquery_dataset.raw.dataset_id,
    google_bigquery_dataset.staging.dataset_id,
    google_bigquery_dataset.marts.dataset_id,
    google_storage_bucket.raw.name,
    google_storage_bucket.curated.name,
    google_pubsub_topic.ingestion_events.name,
  ]

  provisioner "local-exec" {
    command     = "${path.module}/scripts/deploy_pipeline.sh"
    environment = local.provision_env
  }

  depends_on = [
    google_bigquery_table.raw_orders,
    google_bigquery_table.raw_customers,
    google_storage_bucket_object.seed_orders,
    google_storage_bucket_object.seed_customers,
    google_pubsub_subscription.ingestion_events,
  ]
}

# 4. Start Airflow (and Trino) and report the host ports Docker published.
resource "terraform_data" "start_services" {
  count = var.start_services ? 1 : 0

  triggers_replace = [terraform_data.deploy_pipeline.id]

  provisioner "local-exec" {
    command     = "${path.module}/scripts/start_services.sh"
    environment = local.provision_env
  }
}
