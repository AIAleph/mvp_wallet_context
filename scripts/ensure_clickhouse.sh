#!/usr/bin/env bash
set -euo pipefail

# Ensures a ClickHouse instance is reachable for local development.
# 1. Try local clickhouse-client against configured host/port.
# 2. If unreachable, spin up the docker compose ClickHouse service and wait for it.

CH_HOST="${CLICKHOUSE_HOST:-localhost}"
CH_TCP_PORT="${CLICKHOUSE_PORT:-9000}"
WAIT_SECS="${CLICKHOUSE_WAIT_SECS:-30}"
DOCKER_COMPOSE_CMD="${DOCKER_COMPOSE:-docker compose}"

has_clickhouse_client() {
  command -v clickhouse-client >/dev/null 2>&1
}

clickhouse_ping() {
  local client_cmd=(clickhouse-client --host "${CH_HOST}" --port "${CH_TCP_PORT}" -q "SELECT 1")
  if [[ -n "${CLICKHOUSE_USER:-}" ]]; then
    client_cmd+=(--user "${CLICKHOUSE_USER}")
  fi
  if [[ -n "${CLICKHOUSE_PASS:-}" ]]; then
    client_cmd+=(--password "${CLICKHOUSE_PASS}")
  fi
  "${client_cmd[@]}" >/dev/null 2>&1
}

if has_clickhouse_client && clickhouse_ping; then
  echo "clickhouse-client reachable at ${CH_HOST}:${CH_TCP_PORT}"
  exit 0
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "clickhouse-client unavailable and docker not installed; cannot ensure ClickHouse" >&2
  exit 1
fi

# Start ClickHouse via docker compose if needed.
if ! ${DOCKER_COMPOSE_CMD} ps --status=running --services 2>/dev/null | grep -q '^clickhouse$'; then
  echo "Starting ClickHouse container via docker compose..."
  ${DOCKER_COMPOSE_CMD} up -d clickhouse
else
  echo "ClickHouse container already running; waiting for readiness..."
fi

# Wait for readiness by execing clickhouse-client inside the container.
for ((i=0; i<WAIT_SECS; i++)); do
  if ${DOCKER_COMPOSE_CMD} exec -T clickhouse clickhouse-client -q "SELECT 1" >/dev/null 2>&1; then
    echo "ClickHouse container ready."
    exit 0
  fi
  sleep 1
done

echo "Timed out waiting for ClickHouse container after ${WAIT_SECS}s" >&2
exit 1
