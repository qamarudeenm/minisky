# MiniSky Use Cases & Case Studies

This document outlines real-world scenarios where MiniSky provides a massive advantage over standard GCP development workflows.

## 1. The "Terraform CI/CD" Efficiency Gain
**Scenario:** A DevOps team runs 50+ integration tests per day using Terraform. Each run spins up real GCP resources, taking 15 minutes and costing $2.00 per run.
**The MiniSky Solution:**
- Move integration tests to GitHub Actions using the MiniSky binary.
- **Result:** Tests run in 2 minutes (no cloud provisioning wait). Cost drops to $0.00. Catch race conditions locally before they ever hit the cloud.

## 2. The "Data Engineer" Offline Workspace
**Scenario:** A Data Engineer is building a PySpark pipeline for Dataproc. They are traveling and have limited/intermittent internet access.
**The MiniSky Solution:**
- Use the MiniSky Dataproc shim + DuckDB-backed BigQuery.
- **Result:** Continue developing SQL transforms and Spark logic entirely offline. Reference `gs://` buckets that live on the laptop. Full productivity without a wifi connection.

## 3. The "Security Auditor" Playground
**Scenario:** A Security Engineer needs to validate a complex IAM policy with multiple conditions and custom roles. Testing this in a real project is slow and risks accidental exposure.
**The MiniSky Solution:**
- Deploy the IAM policy to MiniSky. Run a suite of "Identity Tests" using the MiniSky CLI to verify `403 FORBIDDEN` vs `200 OK` for different service accounts.
- **Result:** Validate least-privilege security posture in seconds without touching a production project.

## 4. The "Serverless" Event-Driven Loop
**Scenario:** A developer is building a Cloud Function that triggers when a specific file type is uploaded to a GCS bucket. Testing the "Upload -> Trigger -> Process -> Write" loop in GCP takes 5 minutes per iteration.
**The MiniSky Solution:**
- Deploy the function to MiniSky's local Cloud Run/Functions environment.
- Use `minisky storage cp` to trigger the event.
- **Result:** 10-second iteration loop. View unified logs from the storage event and the function execution in one terminal window.

---

## Want to share your use case?
We're always looking for "MiniSky in the Wild" stories. Open a PR to add your scenario here!
