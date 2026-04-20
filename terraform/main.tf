provider "google" {
  project      = "local-dev-project"
  region       = "us-central1"
  access_token = "minisky-token"

  # Redirect APIs to MiniSky Gateway
  storage_custom_endpoint         = "http://localhost:8080/storage/v1/"
  cloud_functions_custom_endpoint = "http://localhost:8080/v2/"
  pubsub_custom_endpoint          = "http://localhost:8080/"
  compute_custom_endpoint         = "http://localhost:8080/compute/v1/"
}

resource "google_storage_bucket" "test_bucket" {
  name     = "terraform-managed-bucket"
  location = "US"
}
