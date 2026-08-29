provider "google" {
  project     = "minisky-demo-project"
  region      = "us-central1"
  zone        = "us-central1-a"
  access_token = "minisky-local-token" # Mock token for MiniSky

  # MiniSky Custom Endpoints
  storage_custom_endpoint   = "http://localhost:8080/storage/v1/"
  compute_custom_endpoint   = "http://localhost:8080/compute/v1/"
  big_query_custom_endpoint = "http://localhost:8080/bigquery/v2/"
  pubsub_custom_endpoint    = "http://localhost:8080/"
  firestore_custom_endpoint = "http://localhost:8080/"
}

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}
