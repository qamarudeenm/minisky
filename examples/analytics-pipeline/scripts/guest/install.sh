#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Guest installer — runs INSIDE the VM (the container MiniSky provisioned).
#
# Installs, in isolated environments so their dependency trees cannot collide:
#   • base OS packages
#   • uv + a pinned CPython
#   • Apache Airflow           → $PIPELINE_HOME/venv
#   • dbt-core + dbt-bigquery  → $PIPELINE_HOME/dbt-venv (+ MiniSky auth shim)
#   • JDK + Trino server & CLI → /opt/trino  (optional)
#
# Every stage is guarded by a marker in $PIPELINE_HOME/.state so re-running is
# cheap and safe.
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

PIPELINE_HOME="${PIPELINE_HOME:-/opt/pipeline}"
STATE_DIR="${PIPELINE_HOME}/.state"
LOG_DIR="${PIPELINE_HOME}/logs"
CACHE_DIR="${PIPELINE_HOME}/cache"
PYTHON_VERSION="${PYTHON_VERSION:-3.12}"
AIRFLOW_VERSION="${AIRFLOW_VERSION:-2.10.5}"
DBT_BIGQUERY_SPEC="${DBT_BIGQUERY_SPEC:->=1.9,<2}"
INSTALL_TRINO="${INSTALL_TRINO:-true}"
TRINO_PORT="${TRINO_PORT:-8090}"
UV_BIN="/root/.local/bin/uv"

mkdir -p "$STATE_DIR" "$LOG_DIR" "$CACHE_DIR"

say()  { printf '  [guest] %s\n' "$*"; }
done_marker() { [[ -f "${STATE_DIR}/$1.done" ]]; }
mark_done()   { date -u +%FT%TZ > "${STATE_DIR}/$1.done"; }

retry() {
  local attempts="$1"; shift
  local n=1
  until "$@"; do
    if (( n >= attempts )); then
      echo "  [guest] command failed after ${n} attempts: $*" >&2
      return 1
    fi
    say "retry ${n}/${attempts}: $*"
    n=$(( n + 1 )); sleep $(( n * 3 ))
  done
}

# ── 1. Base OS packages ──────────────────────────────────────────────────────
# python3 is needed by Trino's launcher; procps/curl/git by Airflow and dbt.
if done_marker base; then
  say "base packages already installed"
else
  say "installing base OS packages"
  export DEBIAN_FRONTEND=noninteractive
  retry 3 apt-get update -qq
  retry 3 apt-get install -y --no-install-recommends \
    ca-certificates curl git unzip xz-utils procps tzdata locales \
    python3 python3-venv python3-pip libpq-dev build-essential jq less nano iproute2 >/dev/null
  ln -sf /usr/share/zoneinfo/UTC /etc/localtime
  mark_done base
  say "base packages installed"
fi

# ── 2. Python interpreter ────────────────────────────────────────────────────
# Prefer the image's own Python when Airflow supports it — ubuntu:24.04 ships
# 3.12, which is exactly what we want, and using it avoids downloading an
# interpreter at all. Only when the distro Python is too new or too old (the
# distro tracks ahead of what Airflow supports) do we fetch one with uv.
SUPPORTED_PYTHONS="3.9 3.10 3.11 3.12"
SYSTEM_PYTHON=""
if command -v python3 >/dev/null 2>&1; then
  system_version="$(python3 -c 'import sys; print("%d.%d" % sys.version_info[:2])' 2>/dev/null || true)"
  for candidate in $SUPPORTED_PYTHONS; do
    if [[ "$system_version" == "$candidate" ]]; then
      SYSTEM_PYTHON="$(command -v python3)"
      PYTHON_VERSION="$system_version"
      say "using the image's Python ${system_version} (${SYSTEM_PYTHON})"
      break
    fi
  done
  [[ -n "$SYSTEM_PYTHON" ]] || say "image Python is ${system_version:-unknown}; Airflow needs one of: ${SUPPORTED_PYTHONS}"
fi

if [[ -z "$SYSTEM_PYTHON" ]]; then
  if done_marker uv && [[ -x "$UV_BIN" ]]; then
    say "uv already installed"
  else
    say "installing uv"
    # Without pipefail a failed download still exits 0: sh reads an empty script
    # and succeeds, so the retry never fires and the failure only surfaces later
    # as "uv not on PATH".
    retry 3 bash -c 'set -o pipefail; curl -LsSf https://astral.sh/uv/install.sh | sh' >/dev/null
    mark_done uv
  fi
  export PATH="/root/.local/bin:${PATH}"
  command -v uv >/dev/null || { echo "uv not on PATH after install" >&2; exit 1; }

  if done_marker python; then
    say "CPython ${PYTHON_VERSION} already provisioned"
  else
    say "provisioning CPython ${PYTHON_VERSION}"
    retry 3 uv python install "${PYTHON_VERSION}"
    mark_done python
  fi
fi

# make_venv <path> — build a virtualenv with whichever interpreter we settled on.
make_venv() {
  if [[ -n "$SYSTEM_PYTHON" ]]; then
    "$SYSTEM_PYTHON" -m venv "$1"
    "$1/bin/python" -m pip install --quiet --upgrade pip
  else
    uv venv --python "${PYTHON_VERSION}" "$1" >/dev/null
  fi
}

# venv_install <venv> <args...> — install into a virtualenv with uv when it is
# available (much faster) and pip otherwise.
venv_install() {
  local venv="$1"; shift
  if [[ -z "$SYSTEM_PYTHON" ]] || command -v uv >/dev/null 2>&1; then
    VIRTUAL_ENV="$venv" uv pip install "$@" >/dev/null
  else
    "$venv/bin/pip" install --quiet "$@"
  fi
}

# ── 3. Airflow (its own virtualenv) ──────────────────────────────────────────
AIRFLOW_VENV="${PIPELINE_HOME}/venv"
CONSTRAINTS="https://raw.githubusercontent.com/apache/airflow/constraints-${AIRFLOW_VERSION}/constraints-${PYTHON_VERSION}.txt"

if done_marker airflow && [[ -x "${AIRFLOW_VENV}/bin/airflow" ]]; then
  say "Airflow ${AIRFLOW_VERSION} already installed"
else
  say "installing Apache Airflow ${AIRFLOW_VERSION} (python ${PYTHON_VERSION})"
  make_venv "${AIRFLOW_VENV}"
  retry 3 venv_install "${AIRFLOW_VENV}" \
    "apache-airflow==${AIRFLOW_VERSION}" --constraint "${CONSTRAINTS}"
  # requests is the only extra the DAGs need: they speak MiniSky's REST APIs
  # directly rather than depending on google-cloud-* inside the Airflow venv.
  retry 3 venv_install "${AIRFLOW_VENV}" requests --constraint "${CONSTRAINTS}"
  mark_done airflow
  say "Airflow installed"
fi

# ── 4. dbt (separate virtualenv) ─────────────────────────────────────────────
# dbt-bigquery pins google-cloud-* versions that fight with Airflow's provider
# constraints. Keeping them apart is the standard production layout and lets the
# DAG call dbt as a subprocess.
DBT_VENV="${PIPELINE_HOME}/dbt-venv"

if done_marker dbt && [[ -x "${DBT_VENV}/bin/dbt" ]]; then
  say "dbt already installed"
else
  say "installing dbt-core + dbt-bigquery (${DBT_BIGQUERY_SPEC})"
  make_venv "${DBT_VENV}"
  retry 3 venv_install "${DBT_VENV}" "dbt-bigquery${DBT_BIGQUERY_SPEC}"
  mark_done dbt
  say "dbt installed"
fi

# The auth shim must land in the dbt venv's site-packages on every run: a dbt
# upgrade can replace the directory.
DBT_SITE="$(${DBT_VENV}/bin/python -c 'import site; print(site.getsitepackages()[0])')"
install -m 0644 "${PIPELINE_HOME}/bootstrap/sitecustomize.py" "${DBT_SITE}/sitecustomize.py"
say "MiniSky auth shim installed into ${DBT_SITE}"

# ── 5. Trino (optional) ──────────────────────────────────────────────────────
if [[ "$INSTALL_TRINO" != "true" ]]; then
  say "skipping Trino (install_trino = false)"
elif done_marker trino && [[ -x /opt/trino/bin/launcher ]]; then
  say "Trino already installed"
else
  # Pick the newest JDK the distro offers, then a Trino release that supports it.
  JAVA_MAJOR=""
  for candidate in 25 24 23 22 21; do
    if apt-cache show "openjdk-${candidate}-jre-headless" >/dev/null 2>&1; then
      say "installing openjdk-${candidate}-jre-headless"
      if retry 2 apt-get install -y --no-install-recommends "openjdk-${candidate}-jre-headless" >/dev/null; then
        JAVA_MAJOR="$candidate"
        break
      fi
    fi
  done

  if [[ -z "$JAVA_MAJOR" ]]; then
    say "WARNING: no supported JDK available — skipping Trino"
  else
    if [[ -n "${TRINO_VERSION:-}" ]]; then
      TRINO_V="$TRINO_VERSION"
    elif (( JAVA_MAJOR >= 24 )); then TRINO_V="476"
    elif (( JAVA_MAJOR == 23 )); then TRINO_V="465"
    elif (( JAVA_MAJOR == 22 )); then TRINO_V="448"
    else TRINO_V="440"
    fi
    say "installing Trino ${TRINO_V} on JDK ${JAVA_MAJOR}"

    SERVER_TGZ="${CACHE_DIR}/trino-server-${TRINO_V}.tar.gz"
    CLI_JAR="${CACHE_DIR}/trino-cli-${TRINO_V}.jar"
    BASE_URL="https://repo1.maven.org/maven2/io/trino"

    [[ -s "$SERVER_TGZ" ]] || retry 3 curl -fsSL -o "$SERVER_TGZ" \
      "${BASE_URL}/trino-server/${TRINO_V}/trino-server-${TRINO_V}.tar.gz"
    [[ -s "$CLI_JAR" ]] || retry 3 curl -fsSL -o "$CLI_JAR" \
      "${BASE_URL}/trino-cli/${TRINO_V}/trino-cli-${TRINO_V}-executable.jar"

    rm -rf /opt/trino "/opt/trino-server-${TRINO_V}"
    tar -xzf "$SERVER_TGZ" -C /opt
    ln -sfn "/opt/trino-server-${TRINO_V}" /opt/trino
    install -m 0755 "$CLI_JAR" /usr/local/bin/trino

    mkdir -p /opt/trino/etc/catalog /var/lib/trino/data
    cat > /opt/trino/etc/node.properties <<EOF
node.environment=minisky
node.id=analytics-orchestrator
node.data-dir=/var/lib/trino/data
EOF
    cat > /opt/trino/etc/jvm.config <<'EOF'
-server
-Xmx2G
-XX:InitialRAMPercentage=40
-XX:MaxRAMPercentage=40
-XX:+UseG1GC
-XX:G1HeapRegionSize=32M
-XX:+ExplicitGCInvokesConcurrent
-XX:+ExitOnOutOfMemoryError
-XX:+HeapDumpOnOutOfMemoryError
-XX:-OmitStackTraceInFastThrow
-XX:ReservedCodeCacheSize=512M
-XX:PerMethodRecompilationCutoff=10000
-XX:PerBytecodeRecompilationCutoff=10000
-Djdk.attach.allowAttachSelf=true
-Dfile.encoding=UTF-8
EOF
    cat > /opt/trino/etc/config.properties <<EOF
coordinator=true
node-scheduler.include-coordinator=true
http-server.http.port=${TRINO_PORT}
discovery.uri=http://localhost:${TRINO_PORT}
query.max-memory=1GB
query.max-memory-per-node=512MB
EOF
    # memory: holds the published marts for interactive serving.
    # tpch:   a always-available catalog to sanity-check the engine itself.
    printf 'connector.name=memory\nmemory.max-data-per-node=512MB\n' > /opt/trino/etc/catalog/memory.properties
    printf 'connector.name=tpch\n' > /opt/trino/etc/catalog/tpch.properties

    mark_done trino
    say "Trino ${TRINO_V} installed at /opt/trino"
  fi
fi

# ── 6. Record what got installed ─────────────────────────────────────────────
{
  echo "installed_at=$(date -u +%FT%TZ)"
  echo "os=$(. /etc/os-release; echo "$PRETTY_NAME")"
  echo "python=$(${AIRFLOW_VENV}/bin/python --version 2>&1)"
  echo "airflow=$(${AIRFLOW_VENV}/bin/airflow version 2>/dev/null || echo 'n/a')"
  echo "dbt=$(${DBT_VENV}/bin/dbt --version 2>/dev/null | head -n2 | tr '\n' ' ' || echo 'n/a')"
  if [[ -x /opt/trino/bin/launcher ]]; then
    echo "trino=$(basename "$(readlink -f /opt/trino)")"
    echo "java=$(java -version 2>&1 | head -n1)"
  else
    echo "trino=not installed"
  fi
} > "${STATE_DIR}/versions.txt"

say "install complete"
