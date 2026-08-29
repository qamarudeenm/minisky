# ─────────────────────────────────────────────────────────────────────────────
# Emulator / project context
# ─────────────────────────────────────────────────────────────────────────────

variable "project_id" {
  description = "Project ID used for every emulated GCP resource."
  type        = string
  default     = "retail-analytics-local"
}

variable "region" {
  description = "Region reported to the API. Purely cosmetic under MiniSky."
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "Zone the orchestrator VM is created in."
  type        = string
  default     = "us-central1-a"
}

variable "minisky_host" {
  description = "Host the MiniSky API gateway is reachable on from this machine."
  type        = string
  default     = "localhost"
}

variable "minisky_port" {
  description = "Port of the MiniSky API gateway (minisky start --port)."
  type        = number
  default     = 8080
}

variable "minisky_access_token" {
  description = "Bearer token sent to MiniSky. Any non-empty string is accepted."
  type        = string
  default     = "minisky-local-token"
}

variable "name_prefix" {
  description = "Prefix applied to every resource name. Keep it DNS-safe."
  type        = string
  default     = "retail"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,18}$", var.name_prefix))
    error_message = "name_prefix must be lowercase alphanumeric/hyphen, 2-19 chars, starting with a letter."
  }
}

variable "environment" {
  description = "Environment label attached to every resource."
  type        = string
  default     = "local"
}

# ─────────────────────────────────────────────────────────────────────────────
# Compute / orchestrator VM
# ─────────────────────────────────────────────────────────────────────────────

variable "vm_name" {
  description = "Name of the GCE instance that hosts Airflow, dbt and Trino."
  type        = string
  default     = "analytics-orchestrator"
}

variable "vm_image" {
  description = <<-EOT
    Boot image for the orchestrator VM. Accepts any os_images id from MiniSky's
    registry (ubuntu-2604-lts, ubuntu-2404-lts, ubuntu-2204-lts, debian-12,
    rocky-9) or a raw Docker reference such as "python:3.12-slim" — useful when
    the machine already holds an image and cannot reach Docker Hub.
    Defaults to 24.04 rather than the registry default because it is the newest
    tag that is reliably published.
  EOT
  type        = string
  default     = "ubuntu-2404-lts"
}

variable "machine_type" {
  description = "GCE machine type. Advisory only under MiniSky (containers are not resource-capped)."
  type        = string
  default     = "n1-standard-4"
}

variable "airflow_port" {
  description = "Port Airflow's web UI listens on inside the VM."
  type        = number
  default     = 8080
}

variable "trino_port" {
  description = "Port the Trino coordinator listens on inside the VM."
  type        = number
  default     = 8090
}

# ─────────────────────────────────────────────────────────────────────────────
# Software versions installed onto the VM OS
# ─────────────────────────────────────────────────────────────────────────────

variable "python_version" {
  description = "CPython version provisioned by uv for the pipeline virtualenv."
  type        = string
  default     = "3.12"
}

variable "airflow_version" {
  description = "Apache Airflow version. 2.10.5 is pinned because its published constraints file for Python 3.12 resolves reproducibly offline-ish."
  type        = string
  default     = "2.10.5"
}

variable "dbt_bigquery_version" {
  description = "Version specifier for the dbt-bigquery adapter (pulls a matching dbt-core)."
  type        = string
  default     = ">=1.9,<2"
}

variable "install_trino" {
  description = <<-EOT
    Install a Trino coordinator on the VM. OFF by default and not needed by this
    pipeline: dbt talks to BigQuery directly through MiniSky's endpoint, Airflow
    calls the same REST APIs, and analysts query BigQuery itself — so Trino's
    memory catalog would only hold a second copy of the marts. Turn it on when
    you want to rehearse federating BigQuery with other sources behind one SQL
    endpoint. Costs a JDK plus a ~1GB download (cached on the host afterwards).
  EOT
  type        = bool
  default     = false
}

variable "trino_version" {
  description = "Trino server version. Leave empty to let the installer pick one matching the JDK it finds."
  type        = string
  default     = ""
}

variable "start_services" {
  description = "Start Airflow and Trino at the end of apply. Set false to provision only."
  type        = bool
  default     = true
}

variable "provision_timeout" {
  description = "Timeout for the software install step."
  type        = string
  default     = "45m"
}
