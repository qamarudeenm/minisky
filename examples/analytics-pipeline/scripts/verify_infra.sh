#!/usr/bin/env bash
# Prove every provisioned resource actually works — not just that terraform
# recorded it in state. Each check hits the live MiniSky API or the VM itself.
#
#   ./verify_infra.sh            # infrastructure + toolchain
#   ./verify_infra.sh --data     # also check that the pipeline produced data
#
# Exit code is the number of failed checks (0 = everything passed).

set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=lib.sh
source ./lib.sh

require_cmd docker curl
require_env VM_CONTAINER PROJECT_ID DATASET_RAW DATASET_STAGING DATASET_MARTS \
            BUCKET_RAW BUCKET_CURATED BUCKET_ARTIFACTS PUBSUB_TOPIC

CHECK_DATA=false
[[ "${1:-}" == "--data" ]] && CHECK_DATA=true

ZONE="${ZONE:-us-central1-a}"
VM_NAME="${VM_NAME:-analytics-orchestrator}"
NAME_PREFIX="${NAME_PREFIX:-retail}"
VPC_NAME="${NAME_PREFIX}-analytics-vpc"

PASS=0; FAIL=0
declare -a FAILURES=()

check() {  # check <description> <command...>
  local desc="$1"; shift
  local output
  if output="$("$@" 2>&1)"; then
    printf '  %s✓%s %s\n' "$C_GREEN" "$C_RESET" "$desc"
    PASS=$(( PASS + 1 ))
  else
    printf '  %s✗%s %s\n' "$C_RED" "$C_RESET" "$desc"
    printf '      %s\n' "${output:0:300}" | head -n3
    FAIL=$(( FAIL + 1 ))
    FAILURES+=("$desc")
  fi
}

# ── helpers used as check targets ────────────────────────────────────────────
api_has() {  # api_has <path> <needle>
  minisky_api GET "$1" | grep -q "$2"
}

bq_query() {  # bq_query <sql> -> prints the first row's values
  local sql job
  sql="$(printf '%s' "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')"
  job="$(minisky_api POST "/bigquery/v2/projects/${PROJECT_ID}/jobs" \
    "{\"configuration\":{\"query\":{\"query\":${sql},\"useLegacySql\":false}},\"jobReference\":{\"projectId\":\"${PROJECT_ID}\",\"location\":\"US\"}}" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["jobReference"]["jobId"])')" || return 1
  for _ in $(seq 1 60); do
    local status
    status="$(minisky_api GET "/bigquery/v2/projects/${PROJECT_ID}/jobs/${job}")"
    if grep -q '"state":"DONE"' <<<"$status"; then
      if grep -q '"errorResult"' <<<"$status"; then
        printf '%s\n' "$status" >&2
        return 1
      fi
      minisky_api GET "/bigquery/v2/projects/${PROJECT_ID}/jobs/${job}/results"
      return 0
    fi
    sleep 1
  done
  return 1
}

# bq_count_positive <dataset.table> — succeeds only when the table has rows.
bq_count_positive() {
  bq_query "SELECT COUNT(*) AS row_count FROM $1" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
rows = payload.get("rows") or []
count = int(rows[0]["f"][0]["v"]) if rows else 0
print(f"{count} rows")
sys.exit(0 if count > 0 else 1)
'
}

vm_has_binary() { docker exec "$VM_CONTAINER" test -x "$1"; }

# `check` runs some targets through `bash -c`, which is a child shell: these have
# to be exported to survive the fork.
export -f bq_query bq_count_positive api_has minisky_api guest_endpoint container_state published_port \
          vm_exec vm_exec_env log ok warn die 2>/dev/null || true
export MINISKY_ENDPOINT PROJECT_ID MINISKY_ACCESS_TOKEN VM_CONTAINER

printf '\n%s═══ MiniSky gateway ═══%s\n' "$C_BOLD" "$C_RESET"
check "API gateway answers on ${MINISKY_ENDPOINT:-http://localhost:8080}" \
  curl -sSf -m 5 -o /dev/null "${MINISKY_ENDPOINT:-http://localhost:8080}/compute/v1/projects/${PROJECT_ID}/global/networks"
check "BigQuery SQL engine (DuckDB backend) executes queries" \
  bash -c 'bq_query "SELECT 1 AS ok" | grep -q "\"jobComplete\":true"'

printf '\n%s═══ Network ═══%s\n' "$C_BOLD" "$C_RESET"
check "VPC ${VPC_NAME} exists" \
  api_has "/compute/v1/projects/${PROJECT_ID}/global/networks/${VPC_NAME}" "\"name\":\"${VPC_NAME}\""
check "Docker network minisky-vpc-${VPC_NAME} created" \
  docker network inspect "minisky-vpc-${VPC_NAME}"
check "firewall ${NAME_PREFIX}-allow-airflow-ui exists" \
  api_has "/compute/v1/projects/${PROJECT_ID}/global/firewalls/${NAME_PREFIX}-allow-airflow-ui" '"IPProtocol":"tcp"'
check "firewall ${NAME_PREFIX}-allow-trino-ui exists" \
  api_has "/compute/v1/projects/${PROJECT_ID}/global/firewalls/${NAME_PREFIX}-allow-trino-ui" '"IPProtocol":"tcp"'

printf '\n%s═══ Compute ═══%s\n' "$C_BOLD" "$C_RESET"
check "instance ${VM_NAME} reports RUNNING" \
  api_has "/compute/v1/projects/${PROJECT_ID}/zones/${ZONE}/instances/${VM_NAME}" '"status":"RUNNING"'
check "container ${VM_CONTAINER} is running" \
  bash -c "[[ \"\$(docker inspect -f '{{.State.Status}}' '${VM_CONTAINER}')\" == running ]]"
check "VM is attached to the pipeline VPC network" \
  bash -c "docker inspect -f '{{range \$k,\$v := .NetworkSettings.Networks}}{{\$k}} {{end}}' '${VM_CONTAINER}' | grep -q 'minisky-vpc-${VPC_NAME}'"
check "host port published for Airflow (container ${AIRFLOW_PORT:-8080})" \
  bash -c "[[ -n \"\$(docker port '${VM_CONTAINER}' '${AIRFLOW_PORT:-8080}/tcp' 2>/dev/null)\" ]]"
check "VM can reach the MiniSky gateway" \
  bash -c "docker exec '${VM_CONTAINER}' curl -sSf -m 5 -o /dev/null \"\${MINISKY_GUEST_ENDPOINT:-$(guest_endpoint "$VM_CONTAINER" "${MINISKY_PORT:-8080}")}/compute/v1/projects/${PROJECT_ID}/global/networks\""

printf '\n%s═══ Cloud Storage ═══%s\n' "$C_BOLD" "$C_RESET"
for bucket in "$BUCKET_RAW" "$BUCKET_CURATED" "$BUCKET_ARTIFACTS"; do
  check "bucket ${bucket} exists" api_has "/storage/v1/b/${bucket}" "\"name\":\"${bucket}\""
done
check "seed object seeds/orders/raw_orders.csv is readable" \
  bash -c "curl -sSf -m 10 '${MINISKY_ENDPOINT:-http://localhost:8080}/storage/v1/b/${BUCKET_RAW}/o/seeds%2Forders%2Fraw_orders.csv?alt=media' | head -n1 | grep -q order_id"
check "seed object seeds/customers/raw_customers.csv is readable" \
  bash -c "curl -sSf -m 10 '${MINISKY_ENDPOINT:-http://localhost:8080}/storage/v1/b/${BUCKET_RAW}/o/seeds%2Fcustomers%2Fraw_customers.csv?alt=media' | head -n1 | grep -q customer_id"

printf '\n%s═══ BigQuery ═══%s\n' "$C_BOLD" "$C_RESET"
for dataset in "$DATASET_RAW" "$DATASET_STAGING" "$DATASET_MARTS"; do
  check "dataset ${dataset} exists" \
    api_has "/bigquery/v2/projects/${PROJECT_ID}/datasets/${dataset}" "\"datasetId\":\"${dataset}\""
done
for table in orders customers; do
  check "table ${DATASET_RAW}.${table} exists with a schema" \
    api_has "/bigquery/v2/projects/${PROJECT_ID}/datasets/${DATASET_RAW}/tables/${table}" '"fields"'
  check "table ${DATASET_RAW}.${table} is queryable" \
    bash -c "bq_query 'SELECT COUNT(*) AS n FROM ${DATASET_RAW}.${table}' | grep -q '\"jobComplete\":true'"
done

printf '\n%s═══ Pub/Sub ═══%s\n' "$C_BOLD" "$C_RESET"
check "topic ${PUBSUB_TOPIC} exists" \
  api_has "/v1/projects/${PROJECT_ID}/topics/${PUBSUB_TOPIC}" "${PUBSUB_TOPIC}"
check "subscription ${PUBSUB_TOPIC}-sub exists" \
  api_has "/v1/projects/${PROJECT_ID}/subscriptions/${PUBSUB_TOPIC}-sub" "${PUBSUB_TOPIC}"
check "publishing to ${PUBSUB_TOPIC} succeeds" \
  bash -c "minisky_api POST '/v1/projects/${PROJECT_ID}/topics/${PUBSUB_TOPIC}:publish' '{\"messages\":[{\"data\":\"dmVyaWZ5\"}]}' | grep -q messageIds"

printf '\n%s═══ Toolchain on the VM ═══%s\n' "$C_BOLD" "$C_RESET"
check "Airflow is installed"  vm_has_binary /opt/pipeline/venv/bin/airflow
check "dbt is installed"      vm_has_binary /opt/pipeline/dbt-venv/bin/dbt
check "runtime config /opt/pipeline/env.sh is present" \
  docker exec "$VM_CONTAINER" test -f /opt/pipeline/env.sh
check "DAG file deployed" \
  docker exec "$VM_CONTAINER" test -f /opt/pipeline/dags/retail_analytics_pipeline.py
check "dbt project parses against MiniSky" \
  vm_exec_env "$VM_CONTAINER" /opt/pipeline/env.sh \
    'cd $DBT_PROJECT_DIR && $DBT_BIN parse --profiles-dir $DBT_PROFILES_DIR'
check "dbt can connect to the emulated warehouse" \
  vm_exec_env "$VM_CONTAINER" /opt/pipeline/env.sh \
    'cd $DBT_PROJECT_DIR && $DBT_BIN debug --profiles-dir $DBT_PROFILES_DIR'
check "Airflow parses the DAG with no import errors" \
  vm_exec_env "$VM_CONTAINER" /opt/pipeline/env.sh \
    '/opt/pipeline/venv/bin/airflow dags list-import-errors -o plain | grep -qv retail_analytics || true; \
     /opt/pipeline/venv/bin/airflow dags list -o plain | grep -q retail_analytics_pipeline'

if docker exec "$VM_CONTAINER" test -x /opt/trino/bin/launcher 2>/dev/null; then
  check "Trino coordinator responds" \
    vm_exec "$VM_CONTAINER" "curl -sf -m 5 http://localhost:${TRINO_PORT:-8090}/v1/info"
fi

if [[ "$CHECK_DATA" == "true" ]]; then
  printf '\n%s═══ Pipeline output ═══%s\n' "$C_BOLD" "$C_RESET"
  for table in orders customers; do
    check "raw table ${DATASET_RAW}.${table} holds rows" bq_count_positive "${DATASET_RAW}.${table}"
  done
  for model in stg_orders stg_customers; do
    check "staging model ${model} built and populated" bq_count_positive "${DATASET_STAGING}.${model}"
  done
  for model in dim_customers fct_orders fct_daily_revenue; do
    check "mart ${model} built and populated" bq_count_positive "${DATASET_MARTS}.${model}"
  done
  check "curated extract written to gs://${BUCKET_CURATED}" \
    bash -c "minisky_api GET '/storage/v1/b/${BUCKET_CURATED}/o?prefix=marts%2F' | grep -q fct_daily_revenue"
fi

printf '\n%s─────────────────────────────────────────────%s\n' "$C_BOLD" "$C_RESET"
printf '  %s%d passed%s, %s%d failed%s\n' \
  "$C_GREEN" "$PASS" "$C_RESET" "$([[ $FAIL -gt 0 ]] && printf '%s' "$C_RED" || printf '')" "$FAIL" "$C_RESET"
if (( FAIL > 0 )); then
  printf '\n  Failed checks:\n'
  printf '    • %s\n' "${FAILURES[@]}"
fi
printf '\n'
exit "$FAIL"
