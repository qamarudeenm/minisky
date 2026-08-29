#!/usr/bin/env bash
# GCE startup-script, attached to the instance metadata in compute.tf.
#
# On real GCE the guest agent executes this at boot. MiniSky v1.3.1 stores the
# metadata faithfully but boots VM containers with `tail -f /dev/null` and runs
# no guest agent, so Terraform delivers the same work over `docker exec`
# (provision.tf → scripts/install_stack.sh). Keeping this script here means the
# configuration is portable: point the provider at real GCP and the VM
# bootstraps itself with no changes to the Terraform.
set -euo pipefail

PIPELINE_HOME=/opt/pipeline
LOG=/var/log/minisky-pipeline-startup.log
exec > >(tee -a "$LOG") 2>&1

echo "[startup] $(date -u +%FT%TZ) bootstrapping analytics orchestrator"

if [[ -x "${PIPELINE_HOME}/bootstrap/install.sh" ]]; then
  exec bash "${PIPELINE_HOME}/bootstrap/install.sh"
fi

echo "[startup] ${PIPELINE_HOME}/bootstrap/install.sh not present yet."
echo "[startup] Terraform pushes it during terraform_data.install_stack."
