Project Configuration (12-factor)

Environment variables configure all services (no hardcoded secrets). Defaults are safe for local dev; production should set explicit values.

Core (shared)
- ETH_PROVIDER_URL: Ethereum RPC endpoint (e.g., https://mainnet.infura.io/v3/...).
- SYNC_CONFIRMATIONS: Required confirmations for delta safety. Default: 12.
- BATCH_BLOCKS: Block batch size for range fetchers. Default: 5000.
- RATE_LIMIT: Provider rate limit in requests/second. Default: 0 (unlimited).

ClickHouse (preferred separate fields; DSN supported for compatibility)
- CLICKHOUSE_URL: Base URL, e.g., http://localhost:8123
- CLICKHOUSE_DB: Database name
- CLICKHOUSE_USER: Username (optional)
- CLICKHOUSE_PASS: Password (optional)
- CLICKHOUSE_DSN: If set, overrides the above (e.g., http://user:pass@host:8123/db)

Optional integrations
- REDIS_URL: Redis connection URL for caching/job state (optional)
- EMBEDDING_MODEL: Embedding model identifier for semantic search (optional)

Go ingester flags map to env with sensible defaults. Example:
  ETH_PROVIDER_URL=https://... \
  CLICKHOUSE_URL=http://localhost:8123 \
  CLICKHOUSE_DB=wallets \
  CLICKHOUSE_USER=default \
  CLICKHOUSE_PASS=secret \
  SYNC_CONFIRMATIONS=12 \
  BATCH_BLOCKS=5000 \
  RATE_LIMIT=20 \
  INGEST_TIMEOUT=45s \
  ingester --address 0xabc... --mode backfill

Security note: tooling and CLI avoid logging secrets; DSNs are redacted in dry-run output.

