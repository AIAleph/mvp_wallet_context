# MVP Wallet Intelligence (Ethereum)

Address-first analytics pipeline and API that ingest Ethereum mainnet activity, normalize it into a ClickHouse warehouse, enrich counterparties, and expose wallet summaries plus semantic search primitives.

## Features

- **Incremental ingestion** – backfill and delta sync per address with configurable confirmations, retries, and rate limiting.
- **Canonical data model** – ReplacingMergeTree tables for logs, traces, token transfers, approvals, checkpoints, contracts, labels, and embeddings.
- **Normalization & decoding** – Stable surrogate IDs, bigint-safe decoding for ERC‑20/721/1155 transfers and approvals, UTC timestamps.
- **Provider abstraction** – JSON-RPC client with shared HTTP timeout/retry policy and pluggable rate limiter.
- **API scaffold** – Fastify server with `/health`, `/healthz`, `/metrics`, and stubbed `/v1/address/:address/sync`.
- **Tooling & docs** – Make targets delegated to scripts, detailed PRD and ADRs, recorded-fixture test harnesses, Docker Compose ClickHouse stack.

## Repository Layout

```
cmd/ingester          Go CLI orchestrating backfill/delta per address
internal/             Shared Go packages (config, eth provider, ingest, normalize, enrich stubs)
pkg/ch                Minimal ClickHouse HTTP client wrapper
api/                  Fastify + Zod API (Node 20) with health endpoints and tests
tools/                Python automation + GitHub issue utilities
sql/                  Canonical and dev ClickHouse schemas, migrations, helper scripts
docs/                 PRD, ADRs, runbooks, development & testing guides
fixtures/abi          ABI and log fixtures exercised by decoder tests
testdata/             Recorded-fixture guidance plus (eventual) provider captures; CI forbids live-chain calls
```

## Prerequisites

- Go 1.21 (module target in `go.mod`)
- Node.js 20.x + npm
- Python 3.11+ with `venv`
- Docker + Docker Compose (for local ClickHouse/Redis)
- ClickHouse client (`clickhouse-client`) if you want direct host access

## Getting Started

1. **Clone & install tooling**

   ```bash
   git clone git@github.com:AIAleph/mvp_wallet_context.git
   cd mvp_wallet_context
   ```

2. **Start local data stack**

   ```bash
   make dev-up                                # ClickHouse + Redis via docker-compose
   make schema                               # Apply canonical schema (sql/schema.sql)
   SCHEMA_FILE=sql/schema_dev.sql make schema  # Apply dev schema when experimenting
   ```

3. **Configure environment**

   Copy `.env.example` (from `docs/config.md`) or export variables manually. **⚠️ Never commit credentials, API keys, or RPC URLs to version control.**

   - `ETH_PROVIDER_URL` – Ethereum RPC endpoint (Alchemy/Infura/QuickNode/etc.)
   - `CLICKHOUSE_DSN` or `CLICKHOUSE_URL`/`CLICKHOUSE_DB`/`CLICKHOUSE_USER`/`CLICKHOUSE_PASS`
   - `SYNC_CONFIRMATIONS`, `BATCH_BLOCKS`, `RATE_LIMIT`, `HTTP_RETRIES`, `HTTP_BACKOFF_BASE`
   - Optional: `REDIS_URL`, `EMBEDDING_MODEL`, `HEALTH_DEBUG`, `HEALTH_RATE_LIMIT_RPS`

4. **Run the ingester**

   ```bash
   # Backfill in canonical schema
   make ingest ADDRESS=0xabc... MODE=backfill

   # Delta sync with explicit range and confirmations
   make ingest ADDRESS=0xabc... MODE=delta FROM=17500000 TO=17510000
   ```

   Flags map to `cmd/ingester` options; pass `SCHEMA=dev` for the dev tables or `DRY_RUN=1` to print the plan.

5. **Run the API**

   ```bash
   make api           # wraps ./scripts/api.sh dev
   ```

   Endpoints:
   - `GET /health` – lightweight ClickHouse availability check (cached, rate-limited)
   - `GET /healthz` – detailed probe (requires `HEALTH_DEBUG=1`)
   - `GET /metrics` – Prometheus-style counters + process stats
   - `POST /v1/address/:address/sync` – stub that will enqueue backfill/delta work

6. **Run tests & linters**

   ```bash
   make test          # go test -race, Node tests, Python tests
   make lint          # golangci-lint, ruff/black, npm lint hooks
   make go-test       # Go-only fast iteration
   make api-test      # Fastify/Vitest suite (threads mode)
   make tools-test    # Python tooling checks (ruff, black, pytest --cov=100)
   ```

   Recorded fixtures belong under `testdata/`; avoid live-chain calls in CI.

## Configuration Notes

- Go config loader (`internal/config`) clamps ranges and builds sanitized DSNs; it refuses inline credentials passed via `--clickhouse` flag to avoid accidental leaks.
- Provider wiring (`internal/eth/factory.go`) applies rate limiting and retry/backoff defaults; extend with host-specific adapters as needed.
- ClickHouse schema uses ReplacingMergeTree with `ingested_at` or `updated_at` version columns; surrogate keys (`event_uid`, `trace_uid`) should lead ORDER BY clauses per ADR-0002 (pending schema update).

## Additional Documentation

- `docs/PRD_MVP_Wallet_Intelligence.md` – MVP scope, user journeys, and data requirements
- `docs/TESTING_AND_COVERAGE.md` – language-specific testing and coverage expectations
- `docs/adr/` – architectural decisions (database choice, idempotency & reorg handling)
- `AGENTS.md` – coding conventions, ingestion guarantees, PR checklist, and operational guidance

## Roadmap Snapshot

- Implement canonical `transactions` pipeline (external tx + normalized traces).
- Persist checkpoints in `addresses` table and enforce rolling reorg window.
- Flesh out enrichment (EOA/contract probing, metadata, proxy detection) and labeling registry.
- Add embeddings job + semantic search endpoints (`/v1/search`).
- Record RPC fixtures for all fetchers and integrate CI harness.

## Troubleshooting

- `make schema` falls back to docker-compose exec when `clickhouse-client` is absent locally; ensure `make dev-up` is running.
- Health endpoints cache responses—tune via `HEALTH_CACHE_CAPACITY`, `HEALTH_CACHE_TTL_MS`, `HEALTH_CIRCUIT_BREAKER_*` env vars.
- `--dry-run` on the ingester prints the computed plan (blocks, rate limit, redacted DSN) without touching providers or ClickHouse.

---

Questions or contributions? See `docs/DEVELOPMENT.md`, `docs/INGESTER.md`, and the ADRs for deeper context, or open an issue in GitHub.
