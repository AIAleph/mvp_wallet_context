-- Canonical ClickHouse schema for MVP Wallet Intelligence
-- Use UTC DateTime64(3) and ReplacingMergeTree for idempotent upserts.

-- Logs (events)
CREATE TABLE IF NOT EXISTS logs (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  address String,
  topics Array(String),
  data_hex String,
  block_number UInt64,
  ts DateTime64(3, 'UTC'),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, log_index);

-- Internal traces
CREATE TABLE IF NOT EXISTS traces (
  trace_uid String,
  tx_hash String,
  trace_id String,
  from_addr String,
  to_addr String,
  value_raw String,
  block_number UInt64,
  ts DateTime64(3, 'UTC'),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, trace_id);

-- Token transfers (ERC-20/721/1155)
CREATE TABLE IF NOT EXISTS token_transfers (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  token String,
  from_addr String,
  to_addr String,
  amount_raw String,
  token_id String,
  standard LowCardinality(String),
  block_number UInt64,
  ts DateTime64(3, 'UTC'),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, log_index, token_id);

-- Approvals (ERC-20/721/1155)
CREATE TABLE IF NOT EXISTS approvals (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  token String,
  owner String,
  spender String,
  amount_raw String,
  token_id String,
  is_approval_for_all UInt8,
  standard LowCardinality(String),
  block_number UInt64,
  ts DateTime64(3, 'UTC'),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, log_index);

-- Addresses sync checkpoints
CREATE TABLE IF NOT EXISTS addresses (
  address String,
  last_synced_block UInt64,
  last_backfill_at DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3, 'UTC'),
  last_delta_at DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3, 'UTC'),
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (address);

-- Contracts registry and metadata
CREATE TABLE IF NOT EXISTS contracts (
  address String,
  is_contract UInt8,
  name String,
  symbol String,
  decimals UInt16,
  created_at_tx String,
  first_seen_block UInt64,
  probed_at DateTime64(3, 'UTC') DEFAULT now64(3),
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (address);

-- Label registry (curated + imported)
CREATE TABLE IF NOT EXISTS labels (
  address String,
  label String,
  source LowCardinality(String),
  confidence Decimal(5, 4) DEFAULT 1.0000,
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (address, label);

-- Embeddings for semantic search (address/token/contract/label)
CREATE TABLE IF NOT EXISTS embeddings (
  entity_kind LowCardinality(String), -- address|token|contract|label
  entity_id String,                   -- e.g., address or symbol
  model String,
  dim UInt16,
  embedding Array(Float32),
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (entity_kind, entity_id);

