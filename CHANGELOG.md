# Changelog

All notable changes to the MiniSky project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.4.0] - 2026-08-29

A fidelity release aimed at one question: can you run a real analytics engineering pipeline —
Terraform, BigQuery, dbt, Airflow — entirely against MiniSky? Building that pipeline surfaced
several defects that made it impossible, and this release closes them. See
`examples/analytics-pipeline` for the pipeline and its fidelity notes.

### Added
- **Official BigQuery query surface**: MiniSky now serves `POST /projects/{project}/queries`
  (`jobs.query`, executed synchronously) and `GET /projects/{project}/queries/{jobId}`
  (`jobs.getQueryResults`). Results were previously only reachable at
  `/jobs/{jobId}/results`, a path no Google client library uses — so `google-cloud-bigquery`,
  and therefore dbt, `bq` and every SDK, got a 404 when reading query results. The original
  path still works.
- **Typed query results**: Query responses now report real BigQuery types (`INT64`, `FLOAT64`,
  `NUMERIC`, `DATE`, `TIMESTAMP`, `BOOL`, `JSON`, …) mapped from the DuckDB column types, instead
  of declaring every column `STRING`. Values are rendered in the shapes the REST API uses — `DATE`
  as `2026-07-01`, `TIMESTAMP` as epoch seconds, and SQL `NULL` as JSON `null` rather than the
  string `<nil>`.
- **Query-created tables are visible to the metadata API**: After a query job succeeds, MiniSky
  reconciles DuckDB's `information_schema` into its table registry, so a table created by
  `CREATE TABLE AS SELECT` — which is how every dbt model materialises — can then be read back
  through `tables.get` and `tables.list` with its schema.
- **Boot images honour `initializeParams.sourceImage`**: `google_compute_instance`'s
  `boot_disk.initialize_params.image` is now resolved through the `os_images` registry, so
  `ubuntu-2204-lts`, `debian-12`, a full `projects/…/global/images/family/…` path, or a raw
  Docker reference such as `python:3.12-slim` all select the image you asked for. Previously the
  field was not decoded at all and every VM silently booted the default image — which also meant
  there was no way to boot a VM from an image already present on a machine that could not reach
  Docker Hub.

### Fixed
- **BigQuery SQL translation no longer corrupts literals**: The BigQuery→DuckDB translator
  rewrote `dataset.table` into `dataset__table` with two regexes applied to the whole statement,
  which also rewrote every other dot. Numeric literals were destroyed (`24.99` became `24__99`),
  string contents were silently altered (`'ada.okonkwo@example.com'` became
  `'ada__okonkwo@example__com'`) and qualified column references broke (`o.order_id` became
  `o__order_id`) — enough to stop any pipeline that handles money or email addresses. The
  translator now scans the statement instead: string literals, quoted identifiers, comments and
  numbers pass through untouched, and a two-part reference is only collapsed when its first
  segment names a known dataset, so table aliases survive.
- **`terraform apply` converges**: Applying the same configuration twice produced the same
  in-place update forever. `friendlyName` was not modelled on datasets or tables, the dataset
  `PATCH` handler echoed the stored resource back without applying the request, tables answered
  `405` to `PATCH` (completing the support started in 1.2.1), and `google_compute_network` did
  not report `networkFirewallPolicyEnforcementOrder`. All four are fixed, and every apply now
  reaches a clean plan.
- **`tables.delete` deletes the data**: Deleting a table removed it from the metadata registry
  but left the DuckDB table in place, so queries kept returning rows from a table that
  `tables.get` reported as gone. The underlying table is now dropped too.
- **Deterministic result column order**: Query result schemas were built by ranging over a Go
  map, so the column order of an identical query changed between runs. Column order now follows
  the `SELECT` list.

## [1.3.1] - 2026-08-16

### Added
- **Multi-VPC compute networking (#4)**: Compute VMs with multiple `networkInterfaces` now attach to a distinct Docker network per interface instead of collapsing every interface onto nic0's network. Firewall rules follow every VPC a VM is attached to (not just its primary NIC), each interface reports its real per-network IP, and network attach / container-start failures roll the container back atomically (matching GCE's "every requested NIC or none" semantics). Contributed by @michaelcatalano17.

### Fixed
- **Duplicate-VPC interfaces rejected at insert**: `instances.insert` now returns `INVALID_ARGUMENT` when two interfaces reference the same VPC network, matching real GCE validation instead of silently attaching only one.
- **Firewall rule keying**: Firewall rules are registered under the short VPC name rather than the full network URL, fixing port re-application being silently dropped on VM recreate.

## [1.3.0] - 2026-08-16

### Fixed
- **Cloud Functions / Cloud Run builds on modern Docker (#5)**: MiniSky now negotiates the Docker Engine API version with the running daemon instead of relying on a hardcoded or implicit one. It queries the daemon's `/version`, clamps a preferred version into the daemon-advertised `[MinAPIVersion, ApiVersion]` range, and exports it as `DOCKER_API_VERSION` so the `pack` (Buildpacks) and `kind` subprocesses inherit a version the engine accepts. This resolves `client version 1.38 is too old` on Docker Engine v25+/v26+, and also prevents the inverse "client version too new" failure on older daemons. An explicit user-set `DOCKER_API_VERSION` is honored.
- **Architecture-aware `pack` installer**: The Buildpacks CLI download URL now includes the CPU architecture, so arm64 hosts (including Apple Silicon) fetch the correct binary instead of the amd64 one. Added a startup warning when the installed `pack` is not the pinned release (e.g. it reports `0.0.0`).

## [1.2.2] - 2026-05-04

### Added
- **macOS arm64 Build**: Added `darwin/arm64` target to the release pipeline, producing a native Apple Silicon binary (`minisky_darwin_arm64.tar.gz`) for the first time. Installer script now validates the asset exists in the release before downloading, preventing silent 404 failures.
- **Improved Installer Robustness**: Switched `curl` to `--fail` mode (`-fsSL`) so HTTP errors abort cleanly. Asset URL is now resolved from the GitHub Releases API manifest instead of being blindly constructed.
- **Docker Socket Detection (macOS)**: Added `~/.docker/run/docker.sock` as a candidate path in the Docker socket resolver, matching Docker Desktop ≥ 4.13 on macOS.
- **Platform Roadmap**: Added a `Platform Roadmap — DuckDB / CGO` section to the README documenting the planned path to full BigQuery DuckDB emulation on macOS arm64 and Windows native.

### Changed
- **macOS BigQuery**: The `darwin/arm64` binary is built with `CGO_ENABLED=0` for this release. BigQuery SQL execution falls back to the in-memory mock (same behaviour as Windows). All other GCP services are fully functional. Full DuckDB support on macOS is tracked on the roadmap.

## [1.2.1] - 2026-05-04

### Added
- **Global Uninstall Command**: Added an `uninstall` CLI command (`minisky uninstall`) to gracefully stop the daemon, prune all `minisky-*` Docker containers/networks, and delete the data directory.
- **Centralized Data Storage**: Moved the `.minisky` state directory from the local working directory to the global user home directory (e.g., `~/.minisky` or `C:\Users\Username\.minisky`). Includes an automatic, zero-data-loss migration for legacy local directories on startup.

### Fixed
- **Missing Dropdown Options**: Embedded `images.json` configuration directly into the compiled Go binary using `//go:embed` to resolve an issue where Compute and Dataproc dropdown menus were empty on Windows deployments.
- **Container Volume Failures on Windows**: Refactored Docker volume binding logic in the Orchestrator to correctly parse absolute Windows host paths (e.g., `C:\path`), preventing container initialization failures.
- **BigQuery CSV Uploads on Windows**: Sanitized local file paths before SQL injection (converting backslashes to forward slashes) to prevent DuckDB from evaluating Windows paths as invalid SQL escape sequences.

## [1.0.2] - 2026-04-28

### Added
- **Artifact Registry**: New native Go shim and management drawer.
  - Support for repository creation and listing.
  - Integrated with local `registry:2` Docker container.
  - Multi-project isolation support.
- **Cloud Build**: Enhanced GitHub source support with workspace volume mapping.
- **Secret Manager**: Native Go-based implementation with multi-versioning.
- **Cloud Tasks**: Native Go-based implementation with queue management.

## [1.0.1] - 2026-04-24

### Added
- **Cloud KMS Shim**: Fully native Go-based implementation using AES-256-GCM. Supports Key Ring and Crypto Key management, key version creation, key rotation, and version destruction. Full encrypt/decrypt operations via the REST API and UI Dashboard.
- **Cloud Build Shim**: Native implementation supporting the `cloudbuild.googleapis.com` API. Features include asynchronous build execution, multi-step pipeline orchestration using transient Docker containers, and a specialized UI drawer for build submission and history tracking.

### Fixed
- **Memorystore Container Provisioning**: Fixed a critical bug where Memorystore instances were failing to provision due to an invalid JSON payload sent to the Docker API. 
- **Memorystore Dynamic Ports**: Updated the Orchestrator to support dynamic port bindings, allowing multiple Redis/Memcached instances to run without host port conflicts. The correctly assigned port is now reflected in the dashboard UI.

- **Native Windows Support**: Implemented cross-platform Docker socket resolution to support Windows Named Pipes (`//./pipe/docker_engine`).
- **New Visual Identity**: Integrated the official MiniSky favicon across the web landing page and embedded dashboard.
- **Improved Documentation**: 
    - Added a Prerequisites section (Docker, Git) to README and website.
    - Added detailed Windows installation instructions for Scoop.
    - Updated website with authentic high-fidelity dashboard screenshots.
- **Enhanced Release Pipeline**: Upgraded `release.sh` to automatically clean up remote tags/releases and push local commits before deployment.

### Fixed
- Resolved `dial unix /var/run/docker.sock` error on Windows machines.
- Fixed UI asset embedding to ensure the new favicon is included in the single binary release.

## [1.0.0] - 2026-04-20

### Added
- **Initial Release**: Core MiniSky emulator with support for 16+ GCP service shims.
- **Embedded Console**: Premium React-based dashboard for observability and resource management.
- **Terraform Integration**: Custom endpoint routing support for the official Google Cloud provider.
- **Lazy Loading**: Sub-100ms service startup times via Go-based lazy initialization.
- **Single Binary**: Fully self-contained architecture for maximum portability.

---
[1.2.2]: https://github.com/qamarudeenm/minisky/compare/v1.2.1...v1.2.2
[1.2.1]: https://github.com/qamarudeenm/minisky/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/qamarudeenm/minisky/compare/v1.0.3...v1.2.0
[1.0.3]: https://github.com/qamarudeenm/minisky/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/qamarudeenm/minisky/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/qamarudeenm/minisky/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/qamarudeenm/minisky/releases/tag/v1.0.0
