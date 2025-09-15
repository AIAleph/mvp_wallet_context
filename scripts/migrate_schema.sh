#!/usr/bin/env bash
set -euo pipefail

# Simple migration tool to apply dev or canonical schema and record schema_version.
# Usage: scripts/migrate_schema.sh TO=dev|canonical [DB=name]

for arg in "$@"; do
  case "$arg" in
    TO=*) TO="${arg#*=}" ;;
    DB=*) CH_DB="${arg#*=}" ;;
  esac
done

TO="${TO:-canonical}"
CH_DB="${CH_DB:-${CLICKHOUSE_DB:-wallets}}"

if [[ "${TO}" != "dev" && "${TO}" != "canonical" ]]; then
  echo "TO must be 'dev' or 'canonical'" >&2
  exit 2
fi

if ! command -v clickhouse-client >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then
  echo "clickhouse-client or docker is required" >&2
  exit 1
fi

SCHEMA_FILE="sql/schema.sql"
DESC="canonical baseline"
VERSION=2
if [[ "${TO}" == "dev" ]]; then
  SCHEMA_FILE="sql/schema_dev.sql"
  DESC="dev baseline"
  VERSION=1
fi

echo "Applying ${TO} schema to DB=${CH_DB} (file=${SCHEMA_FILE})"
export CH_DB
export SCHEMA_FILE
"$(dirname "$0")/schema.sh"

echo "Recording schema_version entry (${VERSION}: ${DESC})"
SQL="CREATE TABLE IF NOT EXISTS schema_version (version UInt32, applied_at DateTime64(3, 'UTC') DEFAULT now64(3), description String) ENGINE = ReplacingMergeTree(applied_at) ORDER BY (version); INSERT INTO schema_version (version, description) VALUES (${VERSION}, '${DESC}')"

if command -v clickhouse-client >/dev/null 2>&1; then
  clickhouse-client --database "${CH_DB}" -q "$SQL"
elif command -v docker >/dev/null 2>&1 && docker compose ps --status=running >/dev/null 2>&1; then
  docker compose exec -T clickhouse bash -lc "clickhouse-client --database '${CH_DB}' -q \"$SQL\""
else
  echo "Warning: unable to record schema_version (no client available)" >&2
fi

echo "Migration complete."

