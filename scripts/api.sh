#!/usr/bin/env bash
set -euo pipefail

# API helper. Subcommands:
#   dev   - start Fastify dev server (watch)
#   test  - run Vitest with coverage
#   build - type-check and build to dist/

cmd="${1:-test}"
pushd api >/dev/null
# Ensure dependencies on first run
if [[ ! -d node_modules ]]; then
  echo "Installing API dependencies (npm ci)..."
  npm ci
fi
case "${cmd}" in
  dev)
    npm run dev
    ;;
  test)
    npm run test
    ;;
  build)
    npm run build
    ;;
  *)
    echo "Unknown subcommand: ${cmd} (use dev|test|build)" >&2
    exit 2
    ;;
esac
popd >/dev/null
