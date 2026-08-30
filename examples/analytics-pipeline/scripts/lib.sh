#!/usr/bin/env bash
# Shared helpers for the analytics-pipeline provisioning scripts.
# Sourced by every script in this directory; never executed directly.

set -euo pipefail

if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BLUE=$'\033[34m'
else
  C_RESET=""; C_BOLD=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""
fi

log()  { printf '%s[pipeline]%s %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf '%s  ✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s  !%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()  { printf '%s  ✗ %s%s\n' "$C_RED" "$*" "$C_RESET" >&2; exit 1; }

require_env() {
  local missing=()
  for v in "$@"; do
    [[ -n "${!v:-}" ]] || missing+=("$v")
  done
  (( ${#missing[@]} == 0 )) || die "missing required environment: ${missing[*]}"
}

require_cmd() {
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || die "'$c' is required but not on PATH"
  done
}

# container_state <name> -> running | exited | missing
container_state() {
  docker inspect -f '{{.State.Status}}' "$1" 2>/dev/null || echo missing
}

# vm_exec <container> <command...> — run a command inside the VM as root.
vm_exec() {
  local container="$1"; shift
  docker exec -i "$container" bash -lc "$*"
}

# vm_exec_env <container> <envfile> <command...> — run with the pipeline env loaded.
vm_exec_env() {
  local container="$1"; local envfile="$2"; shift 2
  docker exec -i "$container" bash -lc "set -a; . '$envfile'; set +a; $*"
}

# guest_endpoint <container> [port] — the URL the VM should use to reach MiniSky.
#
# A container cannot reach a host service as 'localhost', and which address does
# work depends on the setup: on Docker Desktop the daemon runs in its own VM, so
# the bridge gateway is not the host and only host.docker.internal resolves;
# on plain Docker Engine the network gateway is the host. Rather than guess, try
# each candidate from inside the container and keep the first that answers.
guest_endpoint() {
  local container="$1" port="${2:-8080}" candidate gw
  gw="$(docker inspect -f \
    '{{range $k, $v := .NetworkSettings.Networks}}{{if $v.Gateway}}{{$v.Gateway}} {{end}}{{end}}' \
    "$container" 2>/dev/null | awk '{print $1}')"

  for candidate in host.docker.internal "$gw" 172.17.0.1; do
    [[ -n "$candidate" ]] || continue
    if docker exec "$container" \
         curl -sf -m 3 -o /dev/null "http://${candidate}:${port}/compute/v1/projects/probe/global/networks" 2>/dev/null; then
      printf 'http://%s:%s' "$candidate" "$port"
      return 0
    fi
  done

  # Nothing answered — fall back to the gateway so the caller still gets a URL
  # and the failure surfaces where it is diagnosable.
  printf 'http://%s:%s' "${gw:-172.17.0.1}" "$port"
}

# published_port <container> <container_port> — host port Docker bound, or "".
# `docker port` exits non-zero when nothing is bound, which would abort a caller
# running under `set -e`; an unbound port is a normal state here, not an error.
published_port() {
  docker port "$1" "$2/tcp" 2>/dev/null | head -n1 | awk -F: '{print $NF}' || true
}

# minisky_api <method> <path> [body] — call the MiniSky gateway from the host.
minisky_api() {
  local method="$1" path="$2" body="${3:-}"
  local url="${MINISKY_ENDPOINT:-http://localhost:8080}${path}"
  if [[ -n "$body" ]]; then
    curl -sS -X "$method" -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${MINISKY_ACCESS_TOKEN:-minisky-local-token}" \
      -d "$body" "$url"
  else
    curl -sS -X "$method" \
      -H "Authorization: Bearer ${MINISKY_ACCESS_TOKEN:-minisky-local-token}" \
      "$url"
  fi
}

require_minisky_up() {
  local url="${MINISKY_ENDPOINT:-http://localhost:8080}"
  curl -sS -o /dev/null -m 5 "${url}/compute/v1/projects/${PROJECT_ID:-local}/global/networks" \
    || die "MiniSky is not answering on ${url} — run 'minisky start' first"
}

# Values written by deploy_pipeline.sh so the operator-facing scripts
# (pipeline_status.sh, verify_infra.sh, run_pipeline.sh) work standalone.
# Anything already in the environment — e.g. exported by terraform — wins.
_load_pipeline_env() {
  local file="${PIPELINE_ENV_FILE:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.pipeline.env}"
  [[ -f "$file" ]] || return 0
  local key value
  while IFS='=' read -r key value; do
    [[ "$key" =~ ^[A-Z][A-Z0-9_]*$ ]] || continue
    [[ -n "${!key:-}" ]] && continue
    export "$key=$value"
  done < "$file"
}
_load_pipeline_env
