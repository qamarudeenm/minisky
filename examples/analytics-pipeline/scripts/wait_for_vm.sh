#!/usr/bin/env bash
# Block until MiniSky's compute LRO has produced a running container for the VM.
# MiniSky returns the instance as RUNNING only after the Docker data plane is up,
# but terraform's create completes on the LRO, so we confirm the container too.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=lib.sh
source ./lib.sh

require_cmd docker curl
require_env VM_CONTAINER VM_NAME PROJECT_ID MINISKY_ENDPOINT

TIMEOUT="${VM_READY_TIMEOUT:-240}"
log "waiting for VM '${VM_NAME}' (container ${VM_CONTAINER}) to report RUNNING"

deadline=$(( $(date +%s) + TIMEOUT ))
state=""
while (( $(date +%s) < deadline )); do
  state="$(container_state "$VM_CONTAINER")"
  if [[ "$state" == "running" ]]; then
    # Container is up; make sure a shell is actually usable before handing it work.
    if docker exec "$VM_CONTAINER" true 2>/dev/null; then
      ok "VM container is running"
      api_status="$(minisky_api GET \
        "/compute/v1/projects/${PROJECT_ID}/zones/${ZONE:-us-central1-a}/instances/${VM_NAME}" \
        | grep -o '"status":"[A-Z_]*"' | head -n1 | cut -d'"' -f4 || true)"
      [[ -n "$api_status" ]] && log "MiniSky reports instance status: ${api_status}"
      exit 0
    fi
  fi
  sleep 2
done

die "VM container '${VM_CONTAINER}' was '${state:-missing}' after ${TIMEOUT}s"
