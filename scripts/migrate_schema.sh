#!/usr/bin/env bash
set -euo pipefail

# Simple migration tool to apply dev or canonical schema and record schema_version.
# Usage: scripts/migrate_schema.sh TO=dev|canonical [DB=name]

for arg in "$@"; do
  case "$arg" in
    TO=*) TO="${arg#*=}" ;;
    DB=*) CH_DB="${arg#*=}" ;;
    DRY_RUN=*) DRY_RUN="${arg#*=}" ;;
  esac
done

TO="${TO:-canonical}"
CH_DB="${CH_DB:-${CLICKHOUSE_DB:-wallets}}"
DRY_RUN="${DRY_RUN:-false}"
CH_USER="${CLICKHOUSE_USER:-${CH_USER:-}}"
CH_PASS="${CLICKHOUSE_PASS:-${CLICKHOUSE_PASSWORD:-${CH_PASS:-}}}"
CH_CLIENT_FLAGS=()
if [[ -n "${CH_USER}" ]]; then
  CH_CLIENT_FLAGS+=(--user "${CH_USER}")
fi

run_clickhouse() {
  local cmd=(clickhouse-client)
  if ((${#CH_CLIENT_FLAGS[@]})); then
    cmd+=("${CH_CLIENT_FLAGS[@]}")
  fi
  cmd+=("$@")
  local password="${CH_PASS:-${CLICKHOUSE_PASSWORD:-}}"
  if [[ -n "${password}" ]]; then
    CLICKHOUSE_PASSWORD="${password}" "${cmd[@]}"
  else
    "${cmd[@]}"
  fi
}

is_truthy() {
  case "$1" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

if [[ "${TO}" != "dev" && "${TO}" != "canonical" ]]; then
  echo "TO must be 'dev' or 'canonical'" >&2
  exit 2
fi

SCHEMA_FILE="sql/schema.sql"
DESC="canonical baseline"
VERSION=2
if [[ "${TO}" == "dev" ]]; then
  SCHEMA_FILE="sql/schema_dev.sql"
  DESC="dev baseline"
  VERSION=1
fi

if ! is_truthy "${DRY_RUN}" && ! command -v clickhouse-client >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then
  echo "clickhouse-client or docker is required" >&2
  exit 1
fi

if is_truthy "${DRY_RUN}"; then
  echo "[dry-run] Would apply ${TO} schema to DB=${CH_DB} (file=${SCHEMA_FILE}) via scripts/schema.sh"
else
  echo "Applying ${TO} schema to DB=${CH_DB} (file=${SCHEMA_FILE})"
  export CH_DB
  export SCHEMA_FILE
  export CH_USER
  export CH_PASS
  if [[ -n "${CH_PASS}" && -z "${CLICKHOUSE_PASSWORD:-}" ]]; then
    export CLICKHOUSE_PASSWORD="${CH_PASS}"
  fi
  "$(dirname "$0")/schema.sh"
fi

echo "Recording schema_version entry (${VERSION}: ${DESC})"
SQL="CREATE TABLE IF NOT EXISTS schema_version (version UInt32, applied_at DateTime64(3, 'UTC') DEFAULT now64(3), description String) ENGINE = ReplacingMergeTree(applied_at) ORDER BY (version); INSERT INTO schema_version (version, description) VALUES ({version:UInt32}, {description:String})"

if is_truthy "${DRY_RUN}"; then
  echo "[dry-run] Would execute schema_version DDL/DML against DB=${CH_DB}"
elif command -v clickhouse-client >/dev/null 2>&1; then
  run_clickhouse \
    --database "${CH_DB}" \
    --param_version="${VERSION}" \
    --param_description="${DESC}" \
    -q "$SQL"
elif command -v docker >/dev/null 2>&1 && docker compose ps --status=running >/dev/null 2>&1; then
  # Support overriding docker compose command via DOCKER_COMPOSE when running inside the container.
  read -r -a DOCKER_COMPOSE_CMD <<< "${DOCKER_COMPOSE:-docker compose}"
  docker_clickhouse_client() {
    local password="${CH_PASS:-${CLICKHOUSE_PASSWORD:-}}"
    local -a args=("${CH_CLIENT_FLAGS[@]}" "$@")
    if [[ -n "${password}" ]]; then
      "${DOCKER_COMPOSE_CMD[@]}" exec -T clickhouse \
        sh -c 'IFS= read -r CLICKHOUSE_PASSWORD; export CLICKHOUSE_PASSWORD; exec clickhouse-client "$@"' \
        clickhouse-client "${args[@]}" <<<"${password}"
      return
    fi
    local cmd=("${DOCKER_COMPOSE_CMD[@]}" exec -T clickhouse clickhouse-client)
    if ((${#CH_CLIENT_FLAGS[@]})); then
      cmd+=("${CH_CLIENT_FLAGS[@]}")
    fi
    cmd+=("$@")
    "${cmd[@]}"
  }

  docker_clickhouse_client \
    --database "${CH_DB}" \
    --param_version="${VERSION}" \
    --param_description="${DESC}" \
    --query "$SQL"
else
  echo "Warning: unable to record schema_version (no client available)" >&2
fi

if is_truthy "${DRY_RUN}"; then
  echo "Dry-run complete."
else
  echo "Migration complete."
fi
