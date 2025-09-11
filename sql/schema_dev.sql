-- Minimal dev tables for normalized ingestion previews
CREATE TABLE IF NOT EXISTS dev_logs (
  event_uid String,
  tx_hash String,
  log_index UInt32,
  address String,
  topics Array(String),
  data_hex String,
  block_number UInt64,
  ts_millis Int64
) ENGINE = MergeTree ORDER BY (event_uid);

CREATE TABLE IF NOT EXISTS dev_traces (
  trace_uid String,
  tx_hash String,
  trace_id String,
  from_addr String,
  to_addr String,
  value_raw String,
  block_number UInt64,
  ts_millis Int64
) ENGINE = MergeTree ORDER BY (trace_uid);

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
  ts_millis Int64
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
  ts_millis Int64
) ENGINE = MergeTree ORDER BY (event_uid);

