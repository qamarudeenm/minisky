#!/usr/bin/env bash
# Trigger the analytics DAG and follow it to completion.
#
#   ./run_pipeline.sh              # trigger and wait
#   ./run_pipeline.sh --no-wait    # trigger and return
#   ./run_pipeline.sh --dbt-only   # run dbt directly, bypassing Airflow

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=lib.sh
source ./lib.sh

require_cmd docker
require_env VM_CONTAINER

DAG_ID="${DAG_ID:-retail_analytics_pipeline}"
WAIT=true
MODE=airflow

for arg in "$@"; do
  case "$arg" in
    --no-wait)  WAIT=false ;;
    --dbt-only) MODE=dbt ;;
    -h|--help)  sed -n '2,7p' "$0"; exit 0 ;;
    *) die "unknown argument: $arg" ;;
  esac
done

if [[ "$MODE" == "dbt" ]]; then
  log "running dbt build directly inside ${VM_CONTAINER}"
  vm_exec_env "$VM_CONTAINER" /opt/pipeline/env.sh \
    'cd $DBT_PROJECT_DIR && $DBT_BIN build --profiles-dir $DBT_PROFILES_DIR'
  exit $?
fi

RUN_ID="manual__$(date -u +%Y%m%dT%H%M%S)"
log "triggering ${DAG_ID} (run id ${RUN_ID})"
vm_exec_env "$VM_CONTAINER" /opt/pipeline/env.sh \
  "/opt/pipeline/venv/bin/airflow dags trigger ${DAG_ID} --run-id ${RUN_ID}"

if [[ "$WAIT" != "true" ]]; then
  ok "triggered — follow it in the Airflow UI"
  exit 0
fi

log "waiting for the run to finish"
for _ in $(seq 1 120); do
  state="$(vm_exec_env "$VM_CONTAINER" /opt/pipeline/env.sh \
    "/opt/pipeline/venv/bin/airflow dags list-runs -d ${DAG_ID} -o plain" 2>/dev/null \
    | awk -v id="$RUN_ID" '$0 ~ id {print $3}' | head -n1)"
  case "$state" in
    success) ok "DAG run ${RUN_ID} succeeded"; exit 0 ;;
    failed)  die "DAG run ${RUN_ID} failed — check the Airflow UI for the failing task" ;;
  esac
  sleep 5
done
die "DAG run ${RUN_ID} did not finish within 10 minutes"
