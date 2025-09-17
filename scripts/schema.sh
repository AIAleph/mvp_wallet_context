#!/usr/bin/env bash
set -euo pipefail

# Applies ClickHouse schema using local clickhouse-client if available,
# otherwise attempts docker compose exec into the clickhouse service.

CH_DB="${CLICKHOUSE_DB:-${CH_DB:-wallets}}"
CH_USER="${CLICKHOUSE_USER:-${CH_USER:-}}"
# Prefer CLICKHOUSE_PASS but fall back to CLICKHOUSE_PASSWORD for compatibility.
CH_PASS="${CLICKHOUSE_PASS:-${CLICKHOUSE_PASSWORD:-${CH_PASS:-}}}"
# Normalize password handling: prefer env var and avoid passing via argv/stdin.
if [[ -n "${CH_PASS}" ]]; then
  export CLICKHOUSE_PASSWORD="${CH_PASS}"
fi
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

# Prefer sql/schema.sql if present, else fallback to sql/schema_dev.sql
SCHEMA_FILE="${SCHEMA_FILE:-}"
if [[ -z "${SCHEMA_FILE}" ]]; then
  if [[ -f "sql/schema.sql" ]]; then
    SCHEMA_FILE="sql/schema.sql"
  else
    SCHEMA_FILE="sql/schema_dev.sql"
  fi
fi

# Validate schema file exists unless explicitly disabled
if [[ "${SCHEMA_FILE}" != "/dev/null" && ! -f "${SCHEMA_FILE}" ]]; then
  echo "Error: schema file not found: ${SCHEMA_FILE}" >&2
  exit 1
fi

echo "Ensuring database exists: ${CH_DB}"
if command -v clickhouse-client >/dev/null 2>&1; then
  run_clickhouse -q "CREATE DATABASE IF NOT EXISTS ${CH_DB}"
  if [[ "${SCHEMA_FILE}" == "/dev/null" ]]; then
    echo "Database ensured; skipping schema apply (SCHEMA_FILE=/dev/null)"
    exit 0
  fi
  echo "Applying schema file: ${SCHEMA_FILE} to DB=${CH_DB}"
  run_clickhouse --database "${CH_DB}" --queries-file "${SCHEMA_FILE}"
  exit 0
fi

echo "clickhouse-client not found on host; trying docker compose exec..."
if command -v docker >/dev/null 2>&1; then
  if docker compose ps --status=running >/dev/null 2>&1; then
    # Support overriding docker compose command via DOCKER_COMPOSE.
    read -r -a DOCKER_COMPOSE_CMD <<< "${DOCKER_COMPOSE:-docker compose}"
    docker_clickhouse_client() {
      local -a cmd=("${DOCKER_COMPOSE_CMD[@]}" exec -T clickhouse clickhouse-client)
      if ((${#CH_CLIENT_FLAGS[@]})); then
        cmd+=("${CH_CLIENT_FLAGS[@]}")
      fi
      cmd+=("$@")
      "${cmd[@]}"
    }

    docker_clickhouse_client --query "CREATE DATABASE IF NOT EXISTS ${CH_DB}"
    if [[ "${SCHEMA_FILE}" == "/dev/null" ]]; then
      echo "Database ensured; skipping schema apply (SCHEMA_FILE=/dev/null)"
      exit 0
    fi
    docker_clickhouse_client --database "${CH_DB}" -n < "${SCHEMA_FILE}"
    exit 0
  fi
fi

echo "Error: no clickhouse-client on host and no running docker compose stack." >&2
echo "Start the dev stack (make dev-up) or install clickhouse-client." >&2
exit 1
