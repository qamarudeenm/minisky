#!/usr/bin/env bash
# Install the analytics toolchain onto the VM's operating system.
#
# Runs from `terraform apply` (terraform_data.install_stack). Everything it does
# is idempotent: re-running only redoes stages whose marker file is missing.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=lib.sh
source ./lib.sh

require_cmd docker
require_env VM_CONTAINER PYTHON_VERSION AIRFLOW_VERSION DBT_BIGQUERY_SPEC INSTALL_TRINO

[[ "$(container_state "$VM_CONTAINER")" == "running" ]] \
  || die "VM container '${VM_CONTAINER}' is not running"

GUEST_ENDPOINT="$(guest_endpoint "$VM_CONTAINER" "${MINISKY_PORT:-8080}")"
CACHE_DIR="${HOST_CACHE_DIR:-$HOME/.minisky/cache/analytics-pipeline}"
mkdir -p "$CACHE_DIR"

log "installing the pipeline stack onto ${VM_CONTAINER}"
log "  VM reaches MiniSky at ${GUEST_ENDPOINT}"

docker exec "$VM_CONTAINER" mkdir -p /opt/pipeline/bootstrap /opt/pipeline/cache /opt/pipeline/logs /opt/pipeline/.state

# Ship the guest-side installer.
docker cp ./guest/. "${VM_CONTAINER}:/opt/pipeline/bootstrap/"

# Warm the Trino tarball from the host cache so repeat applies skip the download.
if [[ "$INSTALL_TRINO" == "true" ]]; then
  shopt -s nullglob
  for tarball in "$CACHE_DIR"/trino-server-*.tar.gz "$CACHE_DIR"/trino-cli-*.jar; do
    log "  reusing cached $(basename "$tarball")"
    docker cp "$tarball" "${VM_CONTAINER}:/opt/pipeline/cache/"
  done
  shopt -u nullglob
fi

timeout_cmd=()
if command -v timeout >/dev/null 2>&1 && [[ -n "${PROVISION_TIMEOUT:-}" ]]; then
  timeout_cmd=(timeout "$PROVISION_TIMEOUT")
fi

"${timeout_cmd[@]}" docker exec \
  -e "PIPELINE_HOME=/opt/pipeline" \
  -e "MINISKY_ENDPOINT=${GUEST_ENDPOINT}" \
  -e "PYTHON_VERSION=${PYTHON_VERSION}" \
  -e "AIRFLOW_VERSION=${AIRFLOW_VERSION}" \
  -e "DBT_BIGQUERY_SPEC=${DBT_BIGQUERY_SPEC}" \
  -e "INSTALL_TRINO=${INSTALL_TRINO}" \
  -e "TRINO_VERSION=${TRINO_VERSION:-}" \
  -e "TRINO_PORT=${TRINO_PORT:-8090}" \
  -e "AIRFLOW_PORT=${AIRFLOW_PORT:-8080}" \
  -e "DEBIAN_FRONTEND=noninteractive" \
  "$VM_CONTAINER" bash /opt/pipeline/bootstrap/install.sh

# Pull newly downloaded artefacts back into the host cache for the next apply.
if [[ "$INSTALL_TRINO" == "true" ]]; then
  while read -r artefact; do
    [[ -n "$artefact" ]] || continue
    base="$(basename "$artefact")"
    [[ -f "${CACHE_DIR}/${base}" ]] && continue
    docker cp "${VM_CONTAINER}:${artefact}" "${CACHE_DIR}/${base}" 2>/dev/null \
      && log "  cached ${base} on the host" || true
  done < <(docker exec "$VM_CONTAINER" sh -c 'ls -1 /opt/pipeline/cache/trino-* 2>/dev/null' || true)
fi

ok "toolchain installed"
docker exec "$VM_CONTAINER" bash -lc 'cat /opt/pipeline/.state/versions.txt 2>/dev/null || true'
