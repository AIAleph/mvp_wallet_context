# Repository Guidelines

## Project Structure & Module Organization
- Go-first layout: binaries in `cmd/`, internal libs in `internal/`, reusable packages in `pkg/` (e.g., `cmd/ingester`, `internal/eth`, `pkg/ch`).
- Public API in `api/` (TypeScript), one-off/data tooling in `tools/` (Python), ClickHouse DDL/DML in `sql/`.
- Tests live beside code; shared fixtures in `testdata/` and `fixtures/abi/`.

## Build, Test, and Development Commands
- Go: `go build ./cmd/...` (build), `go test -race ./...` (tests), `golangci-lint run` (linters).
- TypeScript API: `cd api && npm ci && npm run test` (Node 20+, strict TS).
- Python tools: `cd tools && ruff check . && black --check . && pytest -q`.
- SQL: `clickhouse-client --queries-file sql/schema.sql` to apply schema.

## Coding Style & Naming Conventions
- English comments required (files, functions, tricky blocks). Reference EIPs/EVM when relevant.
- No floats for on-chain values: Go `*big.Int`, Python `int/Decimal`, TS `bigint`.
- Go: JSON structured logs (slog/zerolog); wrap errors with `%w`; use `errgroup` + shared `http.Client` with timeouts.
- TS: Fastify + Zod; avoid mixing `ethers.BigNumber` with `bigint`.
- Python: type hints + pydantic; asyncio/aiohttp; ruff + black.
- SQL: ReplacingMergeTree, UTC `DateTime64(3)`, dedup on `(tx_hash, log_index)`.

## Testing Guidelines
- Unit + integration with recorded fixtures; forbid live-chain calls in CI.
- Names: Go `*_test.go`, Python `test_*.py`, TS `*.spec.ts`.
- Targets: race-safe tests, table-driven/go golden files for ABI/decoders; coverage 100% for decoders and ClickHouse writes.

## Commit & Pull Request Guidelines
- Commits: imperative mood, explain why; small, focused changes.
- PRs include: language choice + 1–2 sentence justification, module goal & data contracts, tests, config/env notes, and relevant logs/screenshots.
- Checklist: idempotent ingestion (dedup `(tx_hash, log_index)`), retries with backoff + timeouts, structured JSON logs (include `address`, `from_block`, `to_block`, `cursor`, `provider`), secrets not logged, SQL engines/partitions reviewed, provider versions pinned.

## Security & Configuration Tips
- 12‑factor config via env; validate addresses and block ranges; confirm reorg safety with N=12 blocks; rate-limit RPC, use exponential backoff + circuit breaker.

## MVP Wallet Intelligence Notes
- Scope (MVP): Ethereum mainnet only; ingest external transactions, internal traces, ERC‑20/721/1155 transfers, approvals, and contract creation; classify counterparties (EOA vs contract; token/NFT/dApp) and persist to ClickHouse.
- DB Choice: ClickHouse as primary OLAP store with native Vector Index (HNSW) for semantic search; append‑only writes with idempotent upserts and deduplication on `(tx_hash, log_index)`.
- Reorg Safety & Delta: Maintain `addresses.last_synced_block`; process in block ranges with ≥12 confirmations; resume from checkpoints per data type; backfill first, then delta updates.
- Ingestion (Go): provider‑agnostic RPC layer (Alchemy/Infura/QuickNode/Covalent adapters), shared `http.Client` with timeouts, retries with backoff, structured JSON logs. No live‑chain calls in CI; rely on recorded fixtures.
- Normalization: unify tx/logs/traces into common schema columns exactly as in `sql/schema.sql` (ReplacingMergeTree, UTC `DateTime64(3)`), avoid floats (`*big.Int`/`bigint`/`Decimal`).
- Enrichment: `eth_getCode` for EOA/contract, ERC‑165 probe, name/symbol/decimals, proxy detection (EIP‑1967/UUPS), curated label registry with confidence scoring; persist to `contracts` and `labels`.
- API (TS): Fastify + Zod, Node 20+, ClickHouse HTTP client; endpoints for sync, summary, filtered lists (token/NFT/approvals/dApps), and semantic search over embeddings.

References: see `docs/PRD_MVP_Wallet_Intelligence.md`, `docs/adr/0001-db-choice.md`, and `docs/adr/0002-ingestion-idempotency-and-reorgs.md`.
