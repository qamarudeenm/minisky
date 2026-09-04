terraform {
  required_providers {
    google = { source = "hashicorp/google", version = "~> 5.45" }
  }
}

provider "google" {
  access_token = "minisky-local-token"

  resource_manager_custom_endpoint    = "http://localhost:8080/"
  resource_manager_v3_custom_endpoint = "http://localhost:8080/"
  billing_custom_endpoint             = "http://localhost:8080/v1/"
  cloud_billing_custom_endpoint       = "http://localhost:8080/"
}

variable "org_id" {
  description = "The organization MiniSky seeded. See the startup log."
  type        = string
  default     = "100000000000"
}

# Organisation
# ├── Innovation
# │   ├── Initiative-A ── Dev / Test / Prod
# │   └── Initiative-B
# ├── Shared-Services
# └── Platform
resource "google_folder" "innovation" {
  display_name = "Innovation"
  parent       = "organizations/${var.org_id}"
}

resource "google_folder" "initiative_a" {
  display_name = "Initiative-A"
  parent       = google_folder.innovation.name
}

resource "google_folder" "initiative_b" {
  display_name = "Initiative-B"
  parent       = google_folder.innovation.name
}

resource "google_folder" "shared_services" {
  display_name = "Shared-Services"
  parent       = "organizations/${var.org_id}"
}

resource "google_folder" "platform" {
  display_name = "Platform"
  parent       = "organizations/${var.org_id}"
}

resource "google_project" "initiative_a" {
  for_each   = toset(["dev", "test", "prod"])
  name       = "Initiative-A ${each.key}"
  project_id = "initiative-a-${each.key}"
  folder_id  = google_folder.initiative_a.folder_id
}

resource "google_folder_iam_member" "innovation_viewer" {
  folder = google_folder.innovation.name
  role   = "roles/viewer"
  member = "user:ada@example.com"
}

resource "google_project_iam_member" "dev_editor" {
  project = google_project.initiative_a["dev"].project_id
  role    = "roles/editor"
  member  = "serviceAccount:ci@example.iam.gserviceaccount.com"
}
