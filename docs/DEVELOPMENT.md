Development Guide

Overview
- Make targets delegate to scripts in `scripts/` to keep logic in one place.
- CI (GitHub Actions) uses these same scripts for linting and tests.

Core commands
- `make dev-up` / `make dev-down` / `make dev-nuke` – bring up/down local ClickHouse + Redis via docker-compose.
- `make schema` – apply canonical schema (`sql/schema.sql`).
  - To apply the dev schema instead, run: `SCHEMA_FILE=sql/schema_dev.sql make schema`.
- `make api` – start API in dev mode (Fastify watch; installs deps if needed).
- `make ingest ADDRESS=0x... [MODE=backfill|delta] [FROM=0] [TO=0] [BATCH=5000] [SCHEMA=canonical|dev]` – run Go ingester. Defaults to canonical schema.
- `make test` – Go tests with coverage + API tests + Python tools tests with coverage.
- `make lint` – golangci-lint (if installed) + ruff + black + TypeScript type-check.

Scripts
- `scripts/schema.sh` – idempotently creates DB and applies schema; prefers local `clickhouse-client`, falls back to `docker compose exec`.
  - Honors `CLICKHOUSE_DB` and optional `SCHEMA_FILE` (e.g., `sql/schema.sql` or `sql/schema_dev.sql`). Set to `/dev/null` to only create DB.
- `scripts/ingest.sh` – wraps `go run ./cmd/ingester` and validates inputs; defaults to `SCHEMA=canonical`.
- `scripts/api.sh` – `dev|test|build`; installs Node deps when needed.
- `scripts/test.sh` – enforces 100% Go coverage (when code present), runs API and tools tests with coverage.
- `scripts/lint.sh` – unified lint across Go/TS/Python.

Environment
- Prefer `CLICKHOUSE_DSN` (e.g., `http://user:pass@localhost:8123/wallets`). Otherwise set `CLICKHOUSE_URL`, `CLICKHOUSE_DB`, and optionally `CLICKHOUSE_USER`, `CLICKHOUSE_PASS`.
- API health details: set `HEALTH_DEBUG=1` to enable `GET /healthz` details.

Notes
- Canonical schema lives at `sql/schema.sql` (ReplacingMergeTree, UTC `DateTime64(3)`); dev preview tables in `sql/schema_dev.sql`.
- Ingester writes canonical tables by default; pass `SCHEMA=dev` to write `dev_*` tables.
