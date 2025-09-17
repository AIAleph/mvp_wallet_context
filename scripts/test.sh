#!/usr/bin/env bash
set -euo pipefail

status=0

run_step() {
  local label="$1"
  shift
  echo "${label}"
  if ! ( set -euo pipefail; "$@" ); then
    local rc=$?
    echo "${label} failed (exit ${rc})" >&2
    status=1
  else
    echo "${label} ok"
  fi
}

require_tool() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "${bin} not found in PATH" >&2
    return 1
  fi
}

go_tests() {
  require_tool go
  require_tool python3
  local go_cache_root="$(pwd)/.gocache"
  GOCACHE="${go_cache_root}" \
    GOMODCACHE="${go_cache_root}/mod" \
    GOPATH="${go_cache_root}/gopath" \
    go test -race -covermode=atomic -coverprofile=coverage.out ./...
  python3 tools/check_go_coverage.py coverage.out
}

ts_tests() {
  require_tool npm
  pushd api >/dev/null
  if [[ ! -d node_modules ]]; then
    echo "Installing API dependencies (npm ci)..."
    npm ci
  fi
  npm run test
  popd >/dev/null
}

python_tests() {
  require_tool python3
  require_tool pytest
  pushd tools >/dev/null
  pytest --cov=apply_priority --cov=create_github_issues --cov=check_go_coverage --cov-fail-under=100 -q
  popd >/dev/null
}

run_step "[check] Go tests + coverage" go_tests
run_step "[check] API tests (Vitest)" ts_tests
run_step "[check] Python tools tests" python_tests

if (( status != 0 )); then
  echo "Some test suites failed." >&2
else
  echo "All tests passed."
fi

exit ${status}
