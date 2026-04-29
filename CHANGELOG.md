# Changelog

All notable changes to the MiniSky project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.3] - 2026-04-29

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
[1.0.3]: https://github.com/qamarudeenm/minisky/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/qamarudeenm/minisky/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/qamarudeenm/minisky/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/qamarudeenm/minisky/releases/tag/v1.0.0
