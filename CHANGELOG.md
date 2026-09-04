# Changelog

All notable changes to the MiniSky project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2026-09-04

Two things made this release. A user asked whether Cloud Resource Manager was available (issue #7),
which turned into an audit of what the gateway could actually reach: only 5 of 23 finished shims
were reachable by URL, the rest answering `501` to every client that was not addressing them by
hostname. And probing that work turned up a restart bug that destroyed data — a bucket came back in
the emulator's default location, and since `location` is ForceNew, the next `terraform apply`
deleted the bucket with every object in it.

### Added
- **Cloud Resource Manager (v1 and v3)**: organizations, folders and projects, with nesting up to
  ten levels, move, delete, undelete and search, plus IAM bindings at every level. Both API
  generations are served at once, since they differ in field names and parent shapes and clients
  pick either. The hierarchy is persisted, so a landing zone applied from Terraform survives a
  restart. Cloud Billing is emulated alongside it because `google_project` reads billing on every
  refresh.
- **`minisky iam explain`**: answers whether a principal has a permission on a resource and says
  why — which binding granted it, and at which level of the hierarchy it was inherited from. Backed
  by a curated catalogue of 52 roles and 139 permissions, extensible with `~/.minisky/roles.json`.
  IAM is analysed, not enforced: the emulator will not stop a call, but it will tell you what a
  policy actually grants.
- **Resource hierarchy in the dashboard**: create, move and delete folders and projects, and
  inspect access, without leaving the UI.
- **Cloud Tasks queue settings**: rate limits, retry policy, App Engine routing override and
  logging config are modelled, with `queues.patch`, `pause`, `resume` and `purge`. A paused queue
  holds its tasks rather than only reporting a state.

### Fixed
- **A restart destroyed Cloud Storage buckets and their objects**: MiniSky removes its emulator
  containers on shutdown, and fake-gcs-server had no volume, so everything written to it was lost.
  Storage now keeps its data in a named Docker volume. The bucket metadata registry added in 1.4.0
  is persisted too — without it a bucket reverted to `US-CENTRAL1` on restart and Terraform replaced
  it, which is the same data loss by a different route.
- **Resource state did not survive a restart**: eighteen shims held their resources for the lifetime
  of the process, so a restart made the emulator contradict what it had just said, and
  `terraform plan` reads a contradiction as drift. All of them now reload on start. Three lose more
  than a record: a Cloud KMS key ring takes every ciphertext written under it, a secret's payload
  exists nowhere else, and a scheduler job restored without its cron entry reads back as `ENABLED`
  while never firing again — so KMS persists its key material, Secret Manager its payloads, and the
  scheduler re-arms the cron on load. Compute reconciles instance status against Docker rather than
  replaying it, and re-registers firewall rules with the enforcement registry, which starts empty.
- **Only 5 of 23 services were reachable through the gateway**: the router matched on prefixes,
  which cannot work when the segment identifying a service sits after a variable one — thirteen
  services share `/v1/projects/`. Each shim now declares its own URL patterns and the router orders
  them by specificity, so `/v1/projects/*/topics` and `/v1/projects/*/secrets` reach different
  services. Operation polls are attributed by the operation's recorded kind rather than by pattern,
  because `/v1/projects/*/locations/*/operations/*` belongs to five services at once and real GCP
  separates them by hostname.
- **App Engine could not be read or created**: `GET /v1/apps/{id}` fell through to a blanket 404
  because the dispatch matched only a path ending in `/apps`, and `apps.create` answered 200 with an
  empty body while creating nothing.
- **The Vertex AI generative endpoint was unroutable**: the shim translates `generateContent` to a
  local LLM provider — Ollama or any OpenAI-compatible endpoint, set from the dashboard — but the
  path carries a `publishers` segment that no route matched.
- **Cloud Scheduler jobs could not be read back**: a job created with the full resource name in the
  body, which is what `google_cloud_scheduler_job` sends, was stored under that name while every
  lookup built one from the URL, complete with leading slash and version segment. Both shapes are
  now reduced to the canonical name.
- **Cloud Tasks queues outside us-central1 were unreachable**: `queues.get` did not exist, and every
  other lookup built the queue's name with the location hardcoded, so a queue in any other region
  could be created and then never read, listed or deleted.
- **A service was declared ready before it could serve**: readiness was a TCP connect, but Docker
  binds a published port before the process inside is listening, so the first request after a cold
  start could fail. Readiness now requires an HTTP response, with a TCP fallback for emulators that
  speak no HTTP.
- **The version was a literal in three files**: it is now taken from the git tag at build time, so
  `minisky version`, the dashboard and the release artefacts cannot disagree.

### Note for macOS users
The darwin build still ships without CGO, so BigQuery falls back to a mock that returns zero rows
for every query rather than erroring. Everything else on darwin is unaffected. Building from source
with `CGO_ENABLED=1 go build ./cmd/minisky` gives full DuckDB-backed BigQuery on macOS today.

## [1.4.2] - 2026-08-30

### Fixed
- **macOS install: no longer prompts for password when run via `curl | sh`** — the installer
  unconditionally used `sudo` to copy the binary to `/usr/local/bin`, but when a script is piped
  through `curl | sh` there is no real TTY attached to stdin, so `sudo` cannot read the password
  and loops indefinitely (issue #6). The installer now tries a direct write first (Homebrew-managed
  macOS typically makes `/usr/local/bin` user-writable), falls back to `sudo` only when a real
  terminal is present, and as a last resort installs to `~/.local/bin` with clear PATH instructions.

## [1.4.1] - 2026-08-30

1.4.0 made dbt able to *talk* to MiniSky. Actually running the pipeline in
`examples/analytics-pipeline` end to end surfaced nine more defects, and this release closes all
of them. Most were silent: `terraform apply` reported success every time while never converging,
one was quietly destroying data on every run, and three of them together meant nothing running
inside an emulated VM could reach the emulator at all.

### Fixed
- **Buckets were destroyed and recreated on every `terraform apply`**: the GCS emulator does not
  model a bucket's location, labels or storage class — it accepts them on create and then reports
  its own defaults. Since `location` is a ForceNew attribute in the Terraform provider, a bucket
  created with `location = "US"` came back as `US-CENTRAL1` and was replaced on the next apply,
  taking every object in it. The shim now records what the caller asked for and overlays it on the
  emulator's responses, for both a single bucket and a bucket listing; buckets it has not seen pass
  through untouched.
- **Firewall rules never published a port**: two code paths decided whether a rule permits traffic
  by reading `rule.Action == "allow"`, but a GCE firewall has no `action` field — allow versus deny
  is expressed by which of `allowed` / `denied` is populated, and the ports live inside those
  entries. The field was always empty, so no rule ever matched, every VM was provisioned with
  `ports: 0`, and nothing running inside a VM was reachable from the host. This is the "Level 2
  Firewall Port Binding" behaviour described in `docs/network-firewall.md`, which had never
  actually worked. Port ranges are now expanded too, capped so a rule like `0-65535` cannot try to
  bind every port on the host.
- **VMs had no network route to the emulator**: compute containers were created without any
  `ExtraHosts`, so from inside a VM the gateway was unreachable and `host.docker.internal` did not
  resolve — on Docker Desktop the daemon runs in its own VM, so a container's bridge gateway is not
  the host. Every VM is now created with `host.docker.internal:host-gateway`.
- **Requests from anywhere but localhost were rejected**: path-based routing only applied when the
  `Host` header contained `localhost` or `127.0.0.1`. A client inside a VM connects to
  `host.docker.internal:8080`, matched neither, and got
  `501 'host.docker.internal' is not yet implemented` for every call. Routing is now inverted: a
  request whose Host names a Google API domain routes by domain, and everything else — every client
  addressing the emulator directly — routes by URL prefix.
- **`instances.insert` adopted a pre-existing container**: a `409 Conflict` from Docker was treated
  as success and the existing container was started instead. Docker fixes a container's image and
  port bindings at create time, so the VM came back with the wrong image and no published ports
  while the API reported a fresh create. The conflicting container is now replaced.
- **dbt could not materialise a single model**: every dbt materialisation emits BigQuery's
  `OPTIONS(...)` clause, which DuckDB cannot parse — `dbt build` failed with
  `Parser Error: syntax error at or near "OPTIONS"`. The clause is metadata only and is now
  stripped from DDL before execution, with the parenthesis group matched by depth so nested parens
  and parens inside string literals do not end it early.
- **`terraform apply` never converged on BigQuery metadata**: `friendlyName` was not modelled on
  datasets or tables, the dataset `PATCH` handler echoed the stored resource back without applying
  the request, and tables answered `405` to `PATCH` (completing the support started in 1.2.1).
- **`google_compute_network` never converged either**: the network resource did not report
  `networkFirewallPolicyEnforcementOrder`, which GCE always sets, so every plan proposed the same
  in-place update forever.
- **Compute instances could not be updated, and lost their description**: the shim wrote its
  internal container mapping into the instance's `description`, overwriting whatever the caller
  set, and then answered `405` to the update Terraform issued to reconcile the drift it had itself
  created. The mapping moved to a `minisky-container` label, a description is only synthesised when
  the caller supplied none, and `instances.update` / `instances.patch` now apply description,
  labels and metadata.

### Verified
- `examples/analytics-pipeline` now runs end to end against MiniSky: Terraform provisions the VPC,
  firewall rules, three GCS buckets, three BigQuery datasets, a Pub/Sub topic and subscription and
  the orchestrator VM, then installs Airflow and dbt onto the VM's OS. The DAG loads seed CSVs from
  the emulated lake into BigQuery, dbt builds five models and runs 14 tests, the headline mart is
  exported back to the curated bucket, and every stage announces itself on Pub/Sub.
  `scripts/verify_infra.sh --data` reports **41 passed, 0 failed**.

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
