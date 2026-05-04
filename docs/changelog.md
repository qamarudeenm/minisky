# Changelog

All notable changes to this project will be documented in this file.
## [v1.2.1] - 2026-05-04
### Added
- **UI "Winding Down" Deletion State**: The Dashboard now accurately reflects asynchronous deletion operations for heavy resources (Compute instances, Cloud SQL, and GKE clusters) by transitioning them into a red `DELETING` or `STOPPING` state before they are removed from the view.
- **Granular LRO Visibility**: Adjusted backend `RunAsync` operation delays to ensure state transitions (like `STAGING` -> `PROVISIONING` -> `RUNNING`) are clearly captured by Terraform and UI polling intervals.

### Fixed
- **PubSub Emulator Conflicts (409 Error)**: Fixed a path routing issue in the `minisky-pubsub` shim that occasionally left orphaned topics during `terraform destroy`, preventing `409 Conflict` errors on subsequent applies.
- **BigQuery 405 Errors**: Implemented `PATCH` and `PUT` methods in the BigQuery shim to prevent method-not-allowed errors during Terraform attribute updates (such as modifying deletion protection).
- **Dashboard Polling Rate**: Increased UI polling frequency for compute and monitoring resources to 1s to provide a more responsive, real-time feel.

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
