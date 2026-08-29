terraform {
  required_version = ">= 1.5.0"

  required_providers {
    # Pinned to the 5.x line: this is the provider generation verified against
    # MiniSky's compute/storage/bigquery/pubsub shims (see examples/demo-launch).
    # Newer provider majors send request shapes the shims have not been validated
    # against yet — bump deliberately, not incidentally.
    google = {
      source  = "hashicorp/google"
      version = "~> 5.45"
    }
  }
}
