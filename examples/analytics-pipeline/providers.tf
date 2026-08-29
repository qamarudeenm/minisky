# MiniSky serves the Google Cloud REST APIs on a single local gateway and routes by
# URL prefix (pkg/router/proxy.go): /storage/, /bigquery/, /compute/, and
# /v1/projects/*/topics|subscriptions. Only those prefixes are wired for local
# path-based routing, which is exactly the surface this pipeline uses.
provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone

  # MiniSky accepts any well-formed bearer token. Using access_token (rather than a
  # mock credentials JSON) avoids local ASN.1 key-parsing errors in the provider.
  access_token = var.minisky_access_token

  storage_custom_endpoint   = "${local.minisky_endpoint}/storage/v1/"
  compute_custom_endpoint   = "${local.minisky_endpoint}/compute/v1/"
  big_query_custom_endpoint = "${local.minisky_endpoint}/bigquery/v2/"
  pubsub_custom_endpoint    = "${local.minisky_endpoint}/"
}
