#!/usr/bin/env bash
set -euo pipefail

echo "Go lint (golangci-lint)..."
if command -v golangci-lint >/dev/null 2>&1; then
  GO_CACHE_ROOT="${GOCACHE:-${PWD}/.gocache}"
  GO_MOD_CACHE="${GOMODCACHE:-${GO_CACHE_ROOT}/mod}"
  GO_PATH="${GOPATH:-${GO_CACHE_ROOT}/gopath}"
  mkdir -p "${GO_CACHE_ROOT}" "${GO_MOD_CACHE}" "${GO_PATH}"
  GOCACHE="${GO_CACHE_ROOT}" \
    GOMODCACHE="${GO_MOD_CACHE}" \
    GOPATH="${GO_PATH}" \
    golangci-lint run
else
  echo "golangci-lint not installed; skipping"
fi

echo "Python lint (ruff, black)..."
pushd tools >/dev/null
ruff check .
black --check .
popd >/dev/null

echo "TypeScript type-check (tsc build)..."
pushd api >/dev/null
if [[ ! -d node_modules ]]; then
  echo "Installing API dependencies (npm ci)..."
  npm ci
fi
npm run --silent build
popd >/dev/null

# Shell scripts lint (shellcheck)...
SHELLCHECK_BIN=""
if command -v shellcheck >/dev/null 2>&1; then
  SHELLCHECK_BIN="$(command -v shellcheck)"
elif [[ -n "${VIRTUAL_ENV:-}" && -x "${VIRTUAL_ENV}/bin/shellcheck" ]]; then
  SHELLCHECK_BIN="${VIRTUAL_ENV}/bin/shellcheck"
elif [[ -x ".venv/bin/shellcheck" ]]; then
  SHELLCHECK_BIN=".venv/bin/shellcheck"
fi

if [[ -n "${SHELLCHECK_BIN}" ]]; then
  echo "Shell lint (shellcheck)..."
  "${SHELLCHECK_BIN}" scripts/*.sh
else
  echo "shellcheck not installed; install via 'brew install shellcheck' or 'pip install shellcheck-py'"
fi

echo "Lint completed."
