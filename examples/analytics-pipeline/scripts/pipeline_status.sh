#!/usr/bin/env bash
# Print where the pipeline is reachable and whether each component is alive.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=lib.sh
source ./lib.sh

require_cmd docker
require_env VM_CONTAINER

AIRFLOW_PORT="${AIRFLOW_PORT:-8080}"
TRINO_PORT="${TRINO_PORT:-8090}"

state="$(container_state "$VM_CONTAINER")"
airflow_host_port="$(published_port "$VM_CONTAINER" "$AIRFLOW_PORT")"
trino_host_port="$(published_port "$VM_CONTAINER" "$TRINO_PORT")"

health() {  # <container-port> <path> <match>
  vm_exec "$VM_CONTAINER" "curl -sf -m 3 http://localhost:$1$2" 2>/dev/null | grep -q "$3" \
    && printf 'up' || printf 'down'
}

printf '\n%s╭─ Analytics pipeline ─────────────────────────────────────────╮%s\n' "$C_BOLD" "$C_RESET"
printf '  VM container      : %s (%s)\n' "$VM_CONTAINER" "$state"

if [[ "$state" != "running" ]]; then
  printf '  %sVM is not running — nothing else to report%s\n\n' "$C_YELLOW" "$C_RESET"
  exit 0
fi

af_state="$(health "$AIRFLOW_PORT" /health healthy)"
tr_state="down"
docker exec "$VM_CONTAINER" test -x /opt/trino/bin/launcher 2>/dev/null \
  && tr_state="$(health "$TRINO_PORT" /v1/info nodeVersion)" \
  || tr_state="not installed"

printf '  Airflow           : %-12s %s\n' "$af_state" \
  "${airflow_host_port:+http://127.0.0.1:${airflow_host_port}  (admin / admin)}"
# A port is published for Trino whenever the firewall rule exists, but printing
# a URL for a service that was never installed only invites a confusing click.
trino_url=""
[[ "$tr_state" != "not installed" && -n "$trino_host_port" ]] &&
  trino_url="http://127.0.0.1:${trino_host_port}"
printf '  Trino             : %-12s %s\n' "$tr_state" "$trino_url"
printf '  MiniSky (from VM) : %s\n' "${MINISKY_GUEST_ENDPOINT:-unknown}"
printf '  Dashboard         : http://localhost:8081\n'
printf '%s╰──────────────────────────────────────────────────────────────╯%s\n' "$C_BOLD" "$C_RESET"

if [[ -z "$airflow_host_port" ]]; then
  warn "no host port published for container port ${AIRFLOW_PORT}."
  warn "MiniSky binds ports from INGRESS firewall rules at VM-create time —"
  warn "check that the allow-airflow-ui rule existed before the VM was created."
fi

printf '\n%sInstalled toolchain%s\n' "$C_BOLD" "$C_RESET"
docker exec "$VM_CONTAINER" sed 's/^/  /' /opt/pipeline/.state/versions.txt 2>/dev/null \
  || warn "the toolchain has not been installed yet"
printf '\n'
