Ingester (Go)

Overview
- Binary: `cmd/ingester` (Go 1.21+).
- Modes: `backfill` (historical) and `delta` (recent with confirmations).
- Writes to ClickHouse in canonical schema by default.

Usage

go run ./cmd/ingester --address 0x... --mode backfill [flags]

Key flags
- `--address` 0x-prefixed 40-hex address (required)
- `--mode` backfill | delta (default: backfill)
- `--from-block` start block (default 0 = auto)
- `--to-block` end block (default 0 = head)
- `--confirmations` confirmations for delta (default 12)
- `--batch` block batch size (default 5000)
- `--schema` dev | canonical (default: canonical)
- `--clickhouse` DSN (uses env if omitted; see below)
- `--provider` Ethereum RPC URL (optional)

Environment
- Preferred: `CLICKHOUSE_DSN` (e.g., `http://user:pass@localhost:8123/wallets`).
- Or parts: `CLICKHOUSE_URL`, `CLICKHOUSE_DB`, optional `CLICKHOUSE_USER`, `CLICKHOUSE_PASS`.
- Other: `ETH_PROVIDER_URL`, `SYNC_CONFIRMATIONS`, `BATCH_BLOCKS`, `RATE_LIMIT`, `HTTP_RETRIES`, `HTTP_BACKOFF_BASE`.

Schema targets
- canonical (default): tables `logs`, `traces`, `token_transfers`, `approvals` as defined in `sql/schema.sql` (ReplacingMergeTree, UTC DateTime64(3), dedup keys).
- dev: lightweight preview tables `dev_logs`, `dev_traces`, `dev_token_transfers`, `dev_approvals` from `sql/schema_dev.sql`.

Examples
- Backfill full history (canonical schema):
  `go run ./cmd/ingester --address 0xabc... --mode backfill --schema canonical`
- Delta update last N blocks (canonical schema):
  `go run ./cmd/ingester --address 0xabc... --mode delta --confirmations 12`
- Dev preview tables:
  `go run ./cmd/ingester --address 0xabc... --schema dev`

Make targets
- `make ingest ADDRESS=0x... [MODE=backfill|delta] [FROM=0] [TO=0] [BATCH=5000] [SCHEMA=canonical|dev]`
- `make schema` applies `sql/schema.sql`. To apply dev schema: `SCHEMA_FILE=sql/schema_dev.sql make schema`.
