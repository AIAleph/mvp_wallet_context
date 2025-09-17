#!/usr/bin/env bash
set -euo pipefail

# Migration orchestrator for ClickHouse schemas.
# Supports sequential migrations stored in sql/migrations where files follow the
# pattern NN_description.up.sql / NN_description.down.sql.

TO="canonical"
DRY_RUN="false"
ROLLBACK="false"
CH_DB="${CLICKHOUSE_DB:-${CH_DB:-wallets}}"
CH_USER="${CLICKHOUSE_USER:-${CH_USER:-}}"
CH_PASS="${CLICKHOUSE_PASS:-${CLICKHOUSE_PASSWORD:-${CH_PASS:-}}}"
MIGRATIONS_DIR="sql/migrations"

for arg in "$@"; do
  case "$arg" in
    TO=*) TO="${arg#*=}" ;;
    DB=*) CH_DB="${arg#*=}" ;;
    DRY_RUN=*) DRY_RUN="${arg#*=}" ;;
    ROLLBACK=*) ROLLBACK="${arg#*=}" ;;
    *)
      echo "Unknown argument: ${arg}" >&2
      exit 2
      ;;
  esac
done

truthy() {
  case "$1" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
  echo "Migrations directory not found: ${MIGRATIONS_DIR}" >&2
  exit 1
fi

# Build ClickHouse client flag list (user/password handled separately)
CH_CLIENT_FLAGS=()
if [[ -n "${CH_USER}" ]]; then
  CH_CLIENT_FLAGS+=(--user "${CH_USER}")
fi

have_local_client=false
if command -v clickhouse-client >/dev/null 2>&1; then
  have_local_client=true
fi

have_docker_client=false
if command -v docker >/dev/null 2>&1 && docker compose ps --status=running >/dev/null 2>&1; then
  have_docker_client=true
  read -r -a DOCKER_COMPOSE_CMD <<< "${DOCKER_COMPOSE:-docker compose}"
fi

if ! $have_local_client && ! $have_docker_client; then
  echo "clickhouse-client not found and docker compose stack not running." >&2
  echo "Start ClickHouse (make dev-up) or install clickhouse-client." >&2
  exit 1
fi

run_clickhouse() {
  local password="${CH_PASS:-${CLICKHOUSE_PASSWORD:-}}"
  local args=("$@")
  if $have_local_client; then
    local cmd=(clickhouse-client)
    if ((${#CH_CLIENT_FLAGS[@]})); then
      cmd+=("${CH_CLIENT_FLAGS[@]}")
    fi
    cmd+=("${args[@]}")
    if [[ -n "${password}" ]]; then
      CLICKHOUSE_PASSWORD="${password}" "${cmd[@]}"
    else
      "${cmd[@]}"
    fi
  else
    local docker_cmd=("${DOCKER_COMPOSE_CMD[@]}" exec -T clickhouse clickhouse-client)
    if ((${#CH_CLIENT_FLAGS[@]})); then
      docker_cmd+=("${CH_CLIENT_FLAGS[@]}")
    fi
    docker_cmd+=("${args[@]}")
    if [[ -n "${password}" ]]; then
      "${DOCKER_COMPOSE_CMD[@]}" exec -T clickhouse \
        sh -c 'IFS= read -r CLICKHOUSE_PASSWORD; export CLICKHOUSE_PASSWORD; exec clickhouse-client "$@"' \
        clickhouse-client "${docker_cmd[@]:2}" <<<"${password}"
    else
      "${docker_cmd[@]}"
    fi
  fi
}

declare -A MIGRATION_UP
declare -A MIGRATION_DOWN
declare -A MIGRATION_DESC

while IFS= read -r file; do
  base="$(basename "$file")"
  version="${base%%_*}"
  desc="${base#*_}"
  desc="${desc%.up.sql}"
  MIGRATION_UP["${version}"]="${file}"
  MIGRATION_DESC["${version}"]="${desc//_/ }"
done < <(find "${MIGRATIONS_DIR}" -type f -name '*.up.sql' | sort)

while IFS= read -r file; do
  base="$(basename "$file")"
  version="${base%%_*}"
  MIGRATION_DOWN["${version}"]="${file}"
done < <(find "${MIGRATIONS_DIR}" -type f -name '*.down.sql' | sort)

if ((${#MIGRATION_UP[@]} == 0)); then
  echo "No migrations found under ${MIGRATIONS_DIR}" >&2
  exit 1
fi

mapfile -t ALL_VERSIONS < <(printf '%s\n' "${!MIGRATION_UP[@]}" | sort -n)
LATEST_VERSION="${ALL_VERSIONS[-1]}"

resolve_target_version() {
  local target="${1}"
  case "${target}" in
    dev|DEV) echo 1 ;;
    canonical|CANONICAL) echo "${LATEST_VERSION}" ;;
    version:*|VERSION:*)
      local num="${target#*:}"
      if [[ ! "${num}" =~ ^[0-9]+$ ]]; then
        echo ""; return 1
      fi
      echo "${num}"
      ;;
    "") echo "${LATEST_VERSION}" ;;
    *)
      if [[ "${target}" =~ ^[0-9]+$ ]]; then
        echo "${target}"
      else
        echo ""; return 1
      fi
      ;;
  esac
}

TARGET_VERSION=""
if ! TARGET_VERSION="$(resolve_target_version "${TO}")"; then
  echo "Unable to resolve target version from TO=${TO}" >&2
  exit 2
fi

if [[ -z "${TARGET_VERSION}" ]]; then
  echo "Target version not determined." >&2
  exit 2
fi

if [[ -z "${MIGRATION_UP[${TARGET_VERSION}]}" && "${TARGET_VERSION}" != "0" ]]; then
  echo "No migration file registered for version ${TARGET_VERSION}." >&2
  exit 2
fi

ensure_database() {
  if truthy "${DRY_RUN}"; then
    echo "[dry-run] Would ensure database ${CH_DB} exists"
    return
  fi
  run_clickhouse --query "CREATE DATABASE IF NOT EXISTS ${CH_DB}"
}

table_exists() {
  local table="$1"
  local query
  query=$(printf 'EXISTS TABLE %s.%s' "${CH_DB}" "${table}")
  run_clickhouse --database "${CH_DB}" --query "${query}" --format=TabSeparated
}

current_version() {
  if [[ "$(table_exists schema_version 2>/dev/null || true)" != "1" ]]; then
    echo 0
    return
  fi
  local out
  out=$(run_clickhouse --database "${CH_DB}" --query "SELECT toUInt32OrZero(maxOrNull(version)) FROM schema_version" --format=TabSeparated 2>/dev/null || echo 0)
  echo "${out##*$'\n'}"
}

apply_schema_version_insert() {
  local version="$1"
  local description="$2"
  if truthy "${DRY_RUN}"; then
    echo "[dry-run] Would record schema_version=${version} (${description})"
    return
  fi
  run_clickhouse \
    --database "${CH_DB}" \
    --param_version="${version}" \
    --param_description="${description}" \
    --query "INSERT INTO schema_version (version, description) VALUES ({version:UInt32}, {description:String})"
}

remove_schema_version_entry() {
  local version="$1"
  if truthy "${DRY_RUN}"; then
    echo "[dry-run] Would delete schema_version entry for version ${version}"
    return
  fi
  run_clickhouse \
    --database "${CH_DB}" \
    --param_version="${version}" \
    --query "ALTER TABLE schema_version DELETE WHERE version = {version:UInt32}"
}

apply_migration_up() {
  local version="$1"
  local file="${MIGRATION_UP[${version}]}"
  local description="${MIGRATION_DESC[${version}]}"
  if truthy "${DRY_RUN}"; then
    echo "[dry-run] Would apply up migration v${version} (${description}) using ${file}"
  else
    echo "Applying up migration v${version} (${description})"
    run_clickhouse --database "${CH_DB}" --queries-file "${file}"
  fi
  apply_schema_version_insert "${version}" "${description}"
}

apply_migration_down() {
  local version="$1"
  local file="${MIGRATION_DOWN[${version}]}"
  local description="${MIGRATION_DESC[${version}]}"
  if [[ -z "${file}" ]]; then
    echo "No down migration available for version ${version}" >&2
    exit 1
  fi
  if truthy "${DRY_RUN}"; then
    echo "[dry-run] Would apply down migration v${version} (${description}) using ${file}"
  else
    echo "Applying down migration v${version} (${description})"
    run_clickhouse --database "${CH_DB}" --queries-file "${file}"
  fi
  remove_schema_version_entry "${version}"
}

ensure_database

CURRENT_VERSION="$(current_version)"
if [[ -z "${CURRENT_VERSION}" ]]; then
  CURRENT_VERSION=0
fi

echo "Current schema version: ${CURRENT_VERSION}"
echo "Target schema version: ${TARGET_VERSION}"

if truthy "${ROLLBACK}"; then
  if (( TARGET_VERSION >= CURRENT_VERSION )); then
    echo "Rollback requested but target (${TARGET_VERSION}) is not lower than current (${CURRENT_VERSION}). Nothing to do."
    exit 0
  fi
  for (( ver=CURRENT_VERSION; ver>TARGET_VERSION; ver-- )); do
    apply_migration_down "${ver}" || exit 1
  done
else
  if (( TARGET_VERSION == CURRENT_VERSION )); then
    echo "Schema already at target version ${TARGET_VERSION}."
    exit 0
  fi
  if (( TARGET_VERSION < CURRENT_VERSION )); then
    echo "Current version (${CURRENT_VERSION}) is ahead of target (${TARGET_VERSION}). Use ROLLBACK=true to downgrade." >&2
    exit 2
  fi
  for ver in "${ALL_VERSIONS[@]}"; do
    if (( ver > CURRENT_VERSION && ver <= TARGET_VERSION )); then
      apply_migration_up "${ver}" || exit 1
    fi
  done
fi

if truthy "${DRY_RUN}"; then
  echo "Dry-run complete."
else
  echo "Migration complete."
fi
