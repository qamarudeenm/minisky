# Image & Emulator Management

MiniSky uses a centralized image registry to manage all Docker containers used for emulators, Compute Engine VMs, and Cloud SQL instances. This allows for easy updates and version control without modifying backend source code.

## Configuration File

The registry is defined in a single JSON file:
`configs/images.json`

### Structure Overview

The file is divided into four main categories:

1. **`emulators`**: Mappings for GCP API domains (Storage, Pub/Sub, Firestore) to their respective Docker containers.
2. **`compute`**: Lists available OS images for GCE VM provisioning.
3. **`sql`**: Version mapping and image references for PostgreSQL and MySQL.
4. **`serverless`**: Global configuration for builders (e.g., Google Cloud Buildpacks).

---

## How to Add a New OS Image

To add a new operating system option to the Compute Engine dashboard:

1. Open `configs/images.json`.
2. Locate the `compute.os_images` array.
3. Add a new entry:
   ```json
   { 
     "id": "debian-13", 
     "label": "Debian 13 Trixie", 
     "image": "debian:13" 
   }
   ```
4. Save the file.
5. The "OS Image" dropdown in the MiniSky UI will update automatically on the next refresh.

---

## How to Add a New Database Version

To add a new version for Cloud SQL (e.g., Postgres 19):

1. Open `configs/images.json`.
2. Locate `sql.postgres.versions`.
3. Add a new version object:
   ```json
   { 
     "version": "19", 
     "label": "PostgreSQL 19 (Beta)", 
     "image": "postgres:19-alpine" 
   }
   ```
4. If you want this to be the default for new instances, update the `default_image` field as well.

---

## Technical Details

### Backend Loader (`pkg/config/config_loader.go`)
The backend uses a singleton loader that lazily parses the JSON file on first access. It includes a `fallbackRegistry()` to ensure the server can still start if the configuration file is missing or corrupted.

### Dashboard API (`/api/config/images`)
This endpoint exposes the entire registry to the frontend.
- **URL**: `GET http://localhost:8081/api/config/images`
- **Purpose**: Syncing UI dropdowns and default settings.

### Version Mapping in Orchestrator
When a specific version like `POSTGRES_18` is requested via the Cloud SQL API, the `ServiceManager` in `pkg/orchestrator/manager.go` performs the following lookup:
1. Splits the version string (e.g., `18`).
2. Scans the registered versions in `sql.postgres.versions`.
3. Selects the corresponding Docker `image` (e.g., `postgres:18.3-alpine`).
