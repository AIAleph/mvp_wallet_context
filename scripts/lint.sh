#!/usr/bin/env bash
# shellcheck disable=SC2317
set -euo pipefail

status=0

run_step() {
  local label="$1"
  shift
  echo "${label}..."
  if ! ( set -euo pipefail; "$@" ); then
    local rc=$?
    echo "${label} failed (exit ${rc})" >&2
    status=1
  else
    echo "${label} ok"
  fi
}

go_lint() {
  if command -v golangci-lint >/dev/null 2>&1; then
    local go_cache_root="${GOCACHE:-${PWD}/.gocache}"
    local go_mod_cache="${GOMODCACHE:-${go_cache_root}/mod}"
    local go_path="${GOPATH:-${go_cache_root}/gopath}"
    mkdir -p "${go_cache_root}" "${go_mod_cache}" "${go_path}"
    GOCACHE="${go_cache_root}" \
      GOMODCACHE="${go_mod_cache}" \
      GOPATH="${go_path}" \
      golangci-lint run
  else
    echo "golangci-lint not installed; skipping" >&2
  fi
}

python_lint() {
  pushd tools >/dev/null
  ruff check .
  black --check .
  popd >/dev/null
}

ts_build() {
  pushd api >/dev/null
  if [[ ! -d node_modules ]]; then
    echo "Installing API dependencies (npm ci)..."
    npm ci
  fi
  npm run --silent build
  popd >/dev/null
}

shell_lint() {
  local shellcheck_bin=""
  if command -v shellcheck >/dev/null 2>&1; then
    shellcheck_bin="$(command -v shellcheck)"
  elif [[ -n "${VIRTUAL_ENV:-}" && -x "${VIRTUAL_ENV}/bin/shellcheck" ]]; then
    shellcheck_bin="${VIRTUAL_ENV}/bin/shellcheck"
  elif [[ -x ".venv/bin/shellcheck" ]]; then
    shellcheck_bin=".venv/bin/shellcheck"
  fi

  if [[ -n "${shellcheck_bin}" ]]; then
    "${shellcheck_bin}" scripts/*.sh
  else
    echo "shellcheck not installed; install via 'brew install shellcheck' or 'pip install shellcheck-py'" >&2
  fi
}

run_step "Go lint (golangci-lint)" go_lint
run_step "Python lint (ruff, black)" python_lint
run_step "TypeScript type-check (npm run build)" ts_build
run_step "Shell lint (shellcheck)" shell_lint

if (( status != 0 )); then
  echo "Lint completed with failures." >&2
else
  echo "Lint completed successfully."
fi

exit ${status}
