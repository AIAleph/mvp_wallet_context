#!/usr/bin/env bash
set -euo pipefail

echo "Go lint (golangci-lint)..."
if command -v golangci-lint >/dev/null 2>&1; then
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
if command -v shellcheck >/dev/null 2>&1; then
  echo "Shell lint (shellcheck)..."
  shellcheck scripts/*.sh
else
  echo "shellcheck not installed; skipping"
fi

echo "Lint completed."
