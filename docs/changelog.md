# Changelog

All notable changes to this project will be documented in this file.
## [v1.1.0] - 2026-04-28
### Added
- **Cloud Tasks**: Native Go-based shim for background job management and UI dashboard.
- **Memorystore**: Multi-tenant support for Redis, Memcached, and Valkey.
- **Cloud Scheduler**: Native scheduling engine for HTTP and Pub/Sub targets.
- **Secret Manager**: Secure, project-scoped secret storage with native Go implementation.

## [v1.2.0] - 2026-04-29
### Added
- **Project Discovery**: Automatic detection of project contexts across shims (Compute, Logging) to populate the Dashboard selector.
- **Compute Engine Stability**: Implemented thread-safe `DeepCopy` state management to prevent race conditions during concurrent Terraform reconciliations.
- **GCS API Compatibility**: Standardized bucket listing and creation paths in the dashboard to match official GCP REST patterns.

### Fixed
- **Terraform Panics**: Resolved persistent gRPC/HTTP panics in the Google provider by populating mandatory schema fields (`Fingerprint`, `Scheduling`).
- **Pub/Sub Routing**: Fixed path-based routing for subscriptions and ensured correct `/v1` prefixing for the underlying emulator.
- **BigQuery Provider**: Corrected argument mapping for `big_query_custom_endpoint` in example configurations.
