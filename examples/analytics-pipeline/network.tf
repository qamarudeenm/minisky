# ─────────────────────────────────────────────────────────────────────────────
# VPC
#
# MiniSky maps a VPC 1:1 onto a Docker bridge network (minisky-vpc-<name>) and
# gives every VM attached to it L2 isolation from other VPCs.
#
# auto_create_subnetworks is true on purpose: MiniSky's compute shim routes
# /global/networks and /global/firewalls but has no /regions/*/subnetworks
# handler, so a google_compute_subnetwork resource would 404. Auto-mode keeps the
# configuration honest about what the emulator actually implements.
# ─────────────────────────────────────────────────────────────────────────────
resource "google_compute_network" "pipeline" {
  name                    = "${var.name_prefix}-analytics-vpc"
  description             = "Network for the ${var.name_prefix} analytics engineering pipeline"
  auto_create_subnetworks = true
}

# ─────────────────────────────────────────────────────────────────────────────
# Firewall
#
# MiniSky turns an INGRESS allow rule into a real Docker port binding
# (127.0.0.1:<random>-><port>) by re-provisioning every VM on the network. Both
# rules are therefore created *before* the VM — see compute.tf.
# ─────────────────────────────────────────────────────────────────────────────
resource "google_compute_firewall" "airflow_ui" {
  name        = "${var.name_prefix}-allow-airflow-ui"
  network     = google_compute_network.pipeline.name
  description = "Expose the Airflow web UI from the orchestrator VM"
  direction   = "INGRESS"
  priority    = 1000

  allow {
    protocol = "tcp"
    ports    = [tostring(var.airflow_port)]
  }

  source_ranges = ["0.0.0.0/0"]
}

resource "google_compute_firewall" "trino_ui" {
  name        = "${var.name_prefix}-allow-trino-ui"
  network     = google_compute_network.pipeline.name
  description = "Expose the Trino coordinator UI/API from the orchestrator VM"
  direction   = "INGRESS"
  priority    = 1000

  allow {
    protocol = "tcp"
    ports    = [tostring(var.trino_port)]
  }

  source_ranges = ["0.0.0.0/0"]
}
