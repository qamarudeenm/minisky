# Changelog

All notable changes to the MiniSky project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
[1.0.1]: https://github.com/qamarudeenm/minisky/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/qamarudeenm/minisky/releases/tag/v1.0.0
