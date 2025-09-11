Testing and Coverage

This repo enforces 100% test coverage across Go, TypeScript (API), and Python (tools).

Commands

- Go
  - Run: `GOCACHE=$PWD/.gocache GOTMPDIR=$PWD/.gotmp go test -race ./... -coverprofile=coverage.out -covermode=atomic`
  - Summary: `GOCACHE=$PWD/.gocache go tool cover -func=coverage.out`
  - Make: `make go-test` or `make go-cover` (prints per‑func coverage)

- API (TypeScript)
  - Prereqs: Node 20+, `npm ci` in `api/`
  - Run: `cd api && npm test`
  - Coverage thresholds (100%) are configured in `api/vitest.config.ts`.
  - Make: `make api-cover`

- Tools (Python)
  - Prereqs: Python 3.11+, `pip install -r tools/requirements.txt`
  - Run: `cd tools && ruff check . && black --check . && pytest --cov=apply_priority --cov=create_github_issues --cov=check_go_coverage --cov-fail-under=100 -q`
  - Make: `make tools-test`

CI

- `.github/workflows/ci.yml` runs all three suites and enforces 100% coverage.
- Go coverage check uses `tools/check_go_coverage.py`, which fails the job unless total statements equal 100%.

Small test seams (intentional)

- `pkg/ch/client.go` defines `httpNewRequest` (defaults to `http.NewRequestWithContext`).
  - Purpose: allow tests to simulate request‑creation failures to cover rarely hit branches.
  - Behavior: no production change; tests can temporarily replace this var.

- `cmd/ingester/main.go` exposes `defaultNewIngest`, `defaultNewIngestWithProvider`, and `defaultNewProvider`, and uses `wireDefaults()` to assign function variables.
  - Purpose: cleanly exercise wiring paths and permit overrides in tests without side effects.
  - Behavior: production wiring is identical to the original, just more testable.

Notes

- The Go test commands set `GOCACHE`/`GOTMPDIR` to local folders to avoid sandbox issues on some systems.
- Coverage for EVM handling intentionally avoids floats; tests ensure integer/hex safety and branch behavior across decoders and ClickHouse writes.

