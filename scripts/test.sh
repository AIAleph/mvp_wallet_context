#!/usr/bin/env bash
set -euo pipefail

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
