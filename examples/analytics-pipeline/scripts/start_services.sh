#!/usr/bin/env bash
# Start Airflow (and Trino, when installed) inside the VM and wait for both to
# answer. Idempotent: an already-healthy service is left alone.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=lib.sh
source ./lib.sh

require_cmd docker
require_env VM_CONTAINER

AIRFLOW_PORT="${AIRFLOW_PORT:-8080}"
TRINO_PORT="${TRINO_PORT:-8090}"

start_airflow() {
  if vm_exec "$VM_CONTAINER" "curl -sf -m 3 http://localhost:${AIRFLOW_PORT}/health >/dev/null"; then
    ok "Airflow already running"
    return 0
  fi
  log "starting Airflow (standalone: webserver + scheduler + triggerer)"
  vm_exec "$VM_CONTAINER" \
    "set -a; . /opt/pipeline/env.sh; set +a; \
     nohup /opt/pipeline/venv/bin/airflow standalone \
       > /opt/pipeline/logs/airflow.log 2>&1 & echo \$! > /opt/pipeline/airflow.pid"

  for _ in $(seq 1 60); do
    if vm_exec "$VM_CONTAINER" "curl -sf -m 3 http://localhost:${AIRFLOW_PORT}/health >/dev/null"; then
      ok "Airflow is healthy on container port ${AIRFLOW_PORT}"
      return 0
    fi
    sleep 5
  done
  warn "Airflow did not become healthy in 5 minutes — see /opt/pipeline/logs/airflow.log"
  return 1
}

start_trino() {
  if ! docker exec "$VM_CONTAINER" test -x /opt/trino/bin/launcher 2>/dev/null; then
    log "Trino not installed — skipping"
    return 0
  fi
  if vm_exec "$VM_CONTAINER" "curl -sf -m 3 http://localhost:${TRINO_PORT}/v1/info >/dev/null"; then
    ok "Trino already running"
    return 0
  fi
  log "starting the Trino coordinator"
  vm_exec "$VM_CONTAINER" "/opt/trino/bin/launcher start" || true

  for _ in $(seq 1 36); do
    if vm_exec "$VM_CONTAINER" \
        "curl -sf -m 3 http://localhost:${TRINO_PORT}/v1/info | grep -q '\"starting\":false'"; then
      ok "Trino is serving on container port ${TRINO_PORT}"
      return 0
    fi
    sleep 5
  done
  warn "Trino did not finish starting in 3 minutes — see /var/lib/trino/data/var/log/server.log"
  return 1
}

airflow_rc=0; trino_rc=0
start_airflow || airflow_rc=$?
start_trino   || trino_rc=$?

printf '\n'
./pipeline_status.sh || true

# A slow Trino start should not fail the apply; a dead Airflow should.
exit "$airflow_rc"
