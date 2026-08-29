# ─────────────────────────────────────────────────────────────────────────────
# Orchestrator VM — hosts Airflow, dbt and Trino.
#
# depends_on is load-bearing, not decorative: MiniSky binds container ports at
# create time and re-provisions every VM on a network when a firewall rule
# changes (docs/network-firewall.md, "Level 2"). Creating the VM first and the
# rules second would destroy and recreate the container *after* provisioning,
# silently discarding every package installed on it.
# ─────────────────────────────────────────────────────────────────────────────
resource "google_compute_instance" "orchestrator" {
  name         = var.vm_name
  machine_type = var.machine_type
  zone         = var.zone
  description  = "Airflow + dbt + Trino host for the ${var.name_prefix} analytics pipeline"

  boot_disk {
    initialize_params {
      image = var.vm_image
      size  = 50
      type  = "pd-balanced"
    }
  }

  network_interface {
    network = google_compute_network.pipeline.name

    access_config {
      # Ephemeral external IP. MiniSky reports the container's address on the
      # VPC's Docker network here.
    }
  }

  labels = merge(local.common_labels, { role = "orchestrator" })

  metadata = {
    role            = "analytics-orchestrator"
    airflow-port    = tostring(var.airflow_port)
    trino-port      = tostring(var.trino_port)
    minisky-project = var.project_id

    # Recorded for parity with a real GCE deployment. MiniSky v1.3.1 stores
    # metadata but does not execute startup-script, so the same steps are driven
    # from provision.tf via docker exec. See FIDELITY-NOTES.md.
    startup-script = file("${path.module}/scripts/startup-script.sh")
  }

  allow_stopping_for_update = true

  depends_on = [
    google_compute_firewall.airflow_ui,
    google_compute_firewall.trino_ui,
  ]
}
