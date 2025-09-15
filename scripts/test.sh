#!/usr/bin/env bash
set -euo pipefail

command -v go >/dev/null 2>&1 || { echo "go not found in PATH" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "python3 not found in PATH" >&2; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "npm not found in PATH" >&2; exit 1; }

echo "[1/3] Go tests + coverage..."
GOCACHE="$(pwd)/.gocache" GOMODCACHE="$(pwd)/.gocache/mod" GOPATH="$(pwd)/.gocache/gopath" \
  go test -race -covermode=atomic -coverprofile=coverage.out ./...
python3 tools/check_go_coverage.py coverage.out

echo "[2/3] API tests (Vitest)..."
pushd api >/dev/null
if [[ ! -d node_modules ]]; then
  echo "Installing API dependencies (npm ci)..."
  npm ci
fi
npm run test
popd >/dev/null

echo "[3/3] Python tools tests..."
pushd tools >/dev/null
pytest --cov=apply_priority --cov=create_github_issues --cov=check_go_coverage --cov-fail-under=100 -q
popd >/dev/null

echo "All tests passed."
