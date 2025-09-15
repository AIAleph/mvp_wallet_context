#!/usr/bin/env bash
set -euo pipefail

# Applies ClickHouse schema using local clickhouse-client if available,
# otherwise attempts docker compose exec into the clickhouse service.

CH_DB="${CLICKHOUSE_DB:-${CH_DB:-wallets}}"

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
  clickhouse-client -q "CREATE DATABASE IF NOT EXISTS ${CH_DB}"
  if [[ "${SCHEMA_FILE}" == "/dev/null" ]]; then
    echo "Database ensured; skipping schema apply (SCHEMA_FILE=/dev/null)"
    exit 0
  fi
  echo "Applying schema file: ${SCHEMA_FILE} to DB=${CH_DB}"
  clickhouse-client --database "${CH_DB}" --queries-file "${SCHEMA_FILE}"
  exit 0
fi

echo "clickhouse-client not found on host; trying docker compose exec..."
if command -v docker >/dev/null 2>&1; then
  # shellcheck disable=SC2090
  if docker compose ps --status=running >/dev/null 2>&1; then
    if [[ "${SCHEMA_FILE}" == "/dev/null" ]]; then
      docker compose exec -T clickhouse bash -lc "clickhouse-client -q 'CREATE DATABASE IF NOT EXISTS ${CH_DB}'"
    else
      docker compose exec -T clickhouse bash -lc "clickhouse-client -q 'CREATE DATABASE IF NOT EXISTS ${CH_DB}' && clickhouse-client --database '${CH_DB}' -n" < "${SCHEMA_FILE}"
    fi
    exit 0
  fi
fi

echo "Error: no clickhouse-client on host and no running docker compose stack." >&2
echo "Start the dev stack (make dev-up) or install clickhouse-client." >&2
exit 1
