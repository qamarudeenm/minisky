locals {
  minisky_endpoint = "http://${var.minisky_host}:${var.minisky_port}"

  # MiniSky names the Docker container backing a GCE instance
  # "minisky-vm-<instance name>" (pkg/shims/compute/api.go:455).
  vm_container_name = "minisky-vm-${var.vm_name}"

  datasets = {
    raw     = "${replace(var.name_prefix, "-", "_")}_raw"
    staging = "${replace(var.name_prefix, "-", "_")}_staging"
    marts   = "${replace(var.name_prefix, "-", "_")}_marts"
  }

  buckets = {
    raw       = "${var.name_prefix}-analytics-raw"
    curated   = "${var.name_prefix}-analytics-curated"
    artifacts = "${var.name_prefix}-analytics-artifacts"
  }

  common_labels = {
    env       = var.environment
    stack     = "analytics-engineering"
    managedby = "terraform"
  }
}
