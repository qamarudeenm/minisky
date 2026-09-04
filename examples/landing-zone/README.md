# Landing zone on a local emulator

Builds a GCP resource hierarchy — organization, folders, projects and the IAM
bound to them — entirely against MiniSky. No cloud account, no billing, no spend.

```
organizations/{id}
├── Innovation
│   ├── Initiative-A ── initiative-a-dev / -test / -prod
│   └── Initiative-B
├── Shared-Services
└── Platform
```

## Run it

```bash
MINISKY_BQ_BACKEND=duckdb minisky start          # in another terminal

terraform init
terraform apply -var org_id=100000000000
```

MiniSky seeds one organization at startup, because a Google Cloud organization
comes from Cloud Identity and cannot be created through the API. Its id is
printed in the log and overridable:

```bash
MINISKY_ORGANIZATION_ID=123456789012 MINISKY_ORGANIZATION_DOMAIN=example.com minisky start
```

## The endpoint wiring, and why it looks inconsistent

Four overrides are needed, and they do not take the same form. Whether the
version belongs in the URL depends on how each client appends its path — get it
wrong and you see a doubled prefix such as `/v3/v3/folders`, which the gateway
reports rather than silently mishandling.

| Override | Value | Serves |
|---|---|---|
| `resource_manager_custom_endpoint` | `http://localhost:8080/` | `google_project` (v1) |
| `resource_manager_v3_custom_endpoint` | `http://localhost:8080/` | `google_folder` (v3) |
| `billing_custom_endpoint` | `http://localhost:8080/v1/` | the generated billing client |
| `cloud_billing_custom_endpoint` | `http://localhost:8080/` | `google_project`'s billing read |

The last one matters more than it looks. `google_project` reads its billing
account on **every** refresh, through a different client from the one
`billing_custom_endpoint` configures. Without `cloud_billing_custom_endpoint`
that single call escapes to the real `cloudbilling.googleapis.com` and fails
authentication, so the project is created and then unusable.

## What is faithful, and what is not

Faithful: folder ids are assigned by the server, display names must be unique
among siblings, nesting is capped at ten levels, a folder cannot be deleted
while it holds anything, deletes are recoverable rather than destructive, and
projects carry both the id you chose and the number the server assigned.

Not: IAM is recorded, never enforced. MiniSky accepts every caller, so a binding
captures intent for the configuration under test — it will not tell you whether
a principal really has access.
