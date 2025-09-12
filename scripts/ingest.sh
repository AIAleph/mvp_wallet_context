#!/usr/bin/env bash
set -euo pipefail

# Helper to run the Go ingester for a single address.
# Usage: scripts/ingest.sh ADDRESS=0x... [MODE=backfill|delta] [FROM=0] [TO=0] [BATCH=5000] [SCHEMA=dev|canonical]

# Read key params from environment-style args or env vars
for arg in "$@"; do
  case "$arg" in
    ADDRESS=*) ADDRESS="${arg#*=}" ;;
    MODE=*) MODE="${arg#*=}" ;;
    FROM=*) FROM="${arg#*=}" ;;
    TO=*) TO="${arg#*=}" ;;
    BATCH=*) BATCH="${arg#*=}" ;;
    SCHEMA=*) SCHEMA="${arg#*=}" ;;
  esac
done

ADDRESS="${ADDRESS:-${1:-${ADDRESS:-}}}"
MODE="${MODE:-backfill}"
FROM="${FROM:-0}"
TO="${TO:-0}"
BATCH="${BATCH:-5000}"
SCHEMA="${SCHEMA:-canonical}"

if [[ -z "${ADDRESS}" ]]; then
  echo "ADDRESS is required (0x...)" >&2
  exit 2
fi
if ! [[ "${ADDRESS}" =~ ^0x[a-fA-F0-9]{40}$ ]]; then
  echo "Invalid ADDRESS; expected 0x-prefixed 40 hex chars" >&2
  exit 2
fi
if [[ "${MODE}" != "backfill" && "${MODE}" != "delta" ]]; then
  echo "MODE must be backfill or delta" >&2
  exit 2
fi

echo "Running ingester for ${ADDRESS} mode=${MODE} range=${FROM}..${TO} batch=${BATCH} schema=${SCHEMA}"
GOCACHE="$(pwd)/.gocache" GOMODCACHE="$(pwd)/.gocache/mod" GOPATH="$(pwd)/.gocache/gopath" \
  go run ./cmd/ingester --address "${ADDRESS}" --mode "${MODE}" --from-block "${FROM}" --to-block "${TO}" --batch "${BATCH}" --schema "${SCHEMA}"
