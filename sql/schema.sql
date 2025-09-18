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
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
  -- Data skipping indexes for common filters (ClickHouse requires these inside column list)
  INDEX idx_logs_address address TYPE bloom_filter GRANULARITY 2,
  INDEX idx_logs_block block_number TYPE minmax GRANULARITY 1,
  CONSTRAINT logs_address_chk CHECK match(address, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, log_index)
SETTINGS index_granularity = 8192;

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
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_traces_from from_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_traces_to to_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_traces_block block_number TYPE minmax GRANULARITY 1,
  CONSTRAINT traces_from_chk CHECK match(from_addr, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT traces_to_chk CHECK match(to_addr, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, trace_id)
SETTINGS index_granularity = 8192;

-- Transactions (external + internal traces normalization)
CREATE TABLE IF NOT EXISTS transactions (
  tx_hash String,
  block_number UInt64,
  ts DateTime64(3, 'UTC'),
  from_addr String,
  to_addr String,
  value_raw String,
  gas_used UInt64,
  status UInt8,
  input_method Nullable(FixedString(10)),
  is_internal UInt8,
  trace_id Nullable(String),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_tx_from from_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_tx_to to_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_tx_block block_number TYPE minmax GRANULARITY 1,
  CONSTRAINT tx_from_chk CHECK match(from_addr, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT tx_to_chk CHECK to_addr = '' OR match(to_addr, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, is_internal, ifNull(trace_id, ''))
SETTINGS index_granularity = 4096;

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
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_tok_xfer_token token TYPE bloom_filter GRANULARITY 2,
  INDEX idx_tok_xfer_from from_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_tok_xfer_to to_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_tok_xfer_block block_number TYPE minmax GRANULARITY 1,
  CONSTRAINT token_transfers_token_chk CHECK match(token, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT token_transfers_from_chk CHECK match(from_addr, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT token_transfers_to_chk CHECK match(to_addr, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, log_index, token_id)
SETTINGS index_granularity = 4096;

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
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_approvals_token token TYPE bloom_filter GRANULARITY 2,
  INDEX idx_approvals_owner owner TYPE bloom_filter GRANULARITY 2,
  INDEX idx_approvals_spender spender TYPE bloom_filter GRANULARITY 2,
  INDEX idx_approvals_block block_number TYPE minmax GRANULARITY 1,
  CONSTRAINT approvals_token_chk CHECK match(token, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT approvals_owner_chk CHECK match(owner, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT approvals_spender_chk CHECK match(spender, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (tx_hash, log_index)
SETTINGS index_granularity = 4096;

-- Addresses sync checkpoints
CREATE TABLE IF NOT EXISTS addresses (
  address String,
  last_synced_block UInt64,
  last_backfill_at DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3, 'UTC'),
  last_delta_at DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3, 'UTC'),
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_addresses_addr address TYPE bloom_filter GRANULARITY 2,
  INDEX idx_addresses_block last_synced_block TYPE minmax GRANULARITY 1,
  CONSTRAINT addresses_addr_chk CHECK match(address, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (address)
SETTINGS index_granularity = 2048;

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
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_contracts_addr address TYPE bloom_filter GRANULARITY 2,
  CONSTRAINT contracts_addr_chk CHECK match(address, '^0x[0-9a-fA-F]{40}$')
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (address)
SETTINGS index_granularity = 2048;

-- Label registry (curated + imported)
CREATE TABLE IF NOT EXISTS labels (
  address String,
  label String,
  source LowCardinality(String),
  confidence Decimal(5, 4) DEFAULT 1.0000,
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3),
  INDEX idx_labels_addr address TYPE bloom_filter GRANULARITY 2,
  CONSTRAINT labels_addr_chk CHECK match(address, '^0x[0-9a-fA-F]{40}$'),
  CONSTRAINT labels_confidence_chk CHECK confidence >= 0 AND confidence <= 1
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (address, label)
SETTINGS index_granularity = 2048;

-- Embeddings for semantic search (address/token/contract/label)
CREATE TABLE IF NOT EXISTS embeddings (
  entity_kind LowCardinality(String), -- address|token|contract|label
  entity_id String,                   -- e.g., address or symbol
  model String,
  dim UInt16,
  embedding Array(Float32),
  updated_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (entity_kind, entity_id)
SETTINGS index_granularity = 2048;

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
  version UInt32,
  applied_at DateTime64(3, 'UTC') DEFAULT now64(3),
  description String
) ENGINE = ReplacingMergeTree(applied_at)
ORDER BY (version);
