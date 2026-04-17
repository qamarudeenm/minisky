# Terraform Integration with MiniSky

MiniSky is designed to be a **high-fidelity** drop-in replacement for the Google Cloud Platform API. Unlike other emulators, MiniSky correctly emulates **Long-Running Operations (LRO)** and **IAM permissions**, ensuring that your existing Terraform configurations work without modification or specialized "local" workarounds.

## 1. Provider Configuration

To point Terraform to your local MiniSky instance, use the `custom_endpoint` arguments in the `google` provider block. We recommend using a variable to toggle between `local` and `cloud` environments.

```hcl
variable "gcp_environment" {
  description = "Set to 'local' to use MiniSky"
  default     = "local"
}

provider "google" {
  project     = "local-project"
  region      = "us-central1"
  credentials = file("dummy-creds.json") # MiniSky ignores actual credentials

  # Custom Endpoints for Local Development
  access_approval_custom_endpoint      = var.gcp_environment == "local" ? "http://localhost:8080/accessapproval/" : null
  bigquery_custom_endpoint             = var.gcp_environment == "local" ? "http://localhost:8080/bigquery/" : null
  cloud_functions_custom_endpoint     = var.gcp_environment == "local" ? "http://localhost:8080/cloudfunctions/" : null
  pubsub_custom_endpoint               = var.gcp_environment == "local" ? "http://localhost:8080/pubsub/" : null
  storage_custom_endpoint              = var.gcp_environment == "local" ? "http://localhost:8080/storage/" : null
  # Add other services as needed...
}
```

## 2. Using the MiniSky Helper
The MiniSky CLI provides a convenient way to generate this block for your specific version and enabled services:

```bash
minisky terraform-config --output=hcl
```

## 3. Local Authentication
Since Terraform usually checks for a valid Google credential file, you can provide a "dummy" JSON file. MiniSky's internal proxy will accept any well-formed token but will not validate it against Google's servers.

**dummy-creds.json:**
```json
{
  "type": "service_account",
  "project_id": "local-project",
  "private_key_id": "dummy",
  "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
  "client_email": "local-dev@local-project.iam.gserviceaccount.com",
  "client_id": "12345",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/local-dev%40local-project.iam.gserviceaccount.com"
}
```

## 4. Resource Compatibility
Most standard resources are supported. For example:

```hcl
resource "google_storage_bucket" "local_bucket" {
  name     = "my-test-bucket"
  location = "US"
}

resource "google_pubsub_topic" "local_topic" {
  name = "test-topic"
}
```

When you run `terraform apply`, MiniSky will:
1. **Validate API Contract:** Check the incoming HCL request against official Discovery Docs to catch schema errors.
2. **Handle Identity:** Verify that the provided dummy credentials have the required Mock IAM permissions.
3. **Trigger Asynchronous Work:** If the resource creation is slow (e.g., a GKE cluster), MiniSky returns an **Operation** token.
4. **Poll for Completion:** Terraform will poll MiniSky until the internal LRO Manager marks the task as `DONE`.
5. **Persist State:** Ensure the new resource is saved to the local database for subsequent plans.
