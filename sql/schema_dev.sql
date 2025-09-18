-- Minimal dev tables for normalized ingestion previews
CREATE TABLE IF NOT EXISTS dev_logs (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  address String,
  topics Array(String),
  data_hex String,
  block_number UInt64,
  ts_millis Int64,
  INDEX idx_dev_logs_addr address TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_logs_block block_number TYPE minmax GRANULARITY 1
) ENGINE = MergeTree ORDER BY (event_uid);

CREATE TABLE IF NOT EXISTS dev_traces (
  trace_uid String,
  tx_hash String,
  trace_id String,
  from_addr String,
  to_addr String,
  value_raw String,
  block_number UInt64,
  ts_millis Int64,
  INDEX idx_dev_traces_from from_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_traces_to to_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_traces_block block_number TYPE minmax GRANULARITY 1
) ENGINE = MergeTree ORDER BY (trace_uid);

CREATE TABLE IF NOT EXISTS dev_transactions (
  tx_hash String,
  block_number UInt64,
  ts_millis Int64,
  from_addr String,
  to_addr String,
  value_raw String,
  gas_used UInt64,
  status UInt8,
  input_method String,
  is_internal UInt8,
  trace_id String,
  INDEX idx_dev_tx_from from_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_tx_to to_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_tx_block block_number TYPE minmax GRANULARITY 1
) ENGINE = MergeTree ORDER BY (tx_hash, is_internal, trace_id);

CREATE TABLE IF NOT EXISTS dev_token_transfers (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  token String,
  from_addr String,
  to_addr String,
  amount_raw String,
  token_id String,
  standard String,
  block_number UInt64,
  ts_millis Int64,
  INDEX idx_dev_xfer_token token TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_xfer_from from_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_xfer_to to_addr TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_xfer_block block_number TYPE minmax GRANULARITY 1
) ENGINE = MergeTree ORDER BY (event_uid);

CREATE TABLE IF NOT EXISTS dev_approvals (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  token String,
  owner String,
  spender String,
  amount_raw String,
  token_id String,
  is_approval_for_all UInt8,
  standard String,
  block_number UInt64,
  ts_millis Int64,
  INDEX idx_dev_appr_token token TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_appr_owner owner TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_appr_spender spender TYPE bloom_filter GRANULARITY 2,
  INDEX idx_dev_appr_block block_number TYPE minmax GRANULARITY 1
) ENGINE = MergeTree ORDER BY (event_uid);

-- Schema version tracking (dev)
CREATE TABLE IF NOT EXISTS schema_version (
  version UInt32,
  applied_at DateTime64(3, 'UTC') DEFAULT now64(3),
  description String
) ENGINE = ReplacingMergeTree(applied_at)
ORDER BY (version);
