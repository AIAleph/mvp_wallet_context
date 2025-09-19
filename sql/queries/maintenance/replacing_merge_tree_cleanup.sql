-- Maintenance helpers for ReplacingMergeTree tables.
-- Force background merges so the latest `ingested_at`/`updated_at` wins on
-- logical keys. Run during low-traffic windows.

OPTIMIZE TABLE logs FINAL;
OPTIMIZE TABLE traces FINAL;
OPTIMIZE TABLE token_transfers FINAL;
OPTIMIZE TABLE approvals FINAL;
OPTIMIZE TABLE transactions FINAL;
OPTIMIZE TABLE addresses FINAL;
OPTIMIZE TABLE contracts FINAL;
OPTIMIZE TABLE labels FINAL;
OPTIMIZE TABLE embeddings FINAL;

-- Optional verification: check for duplicate logical keys that would indicate
-- outstanding merges or ingestion regressions. Run after the OPTIMIZE passes.

SELECT tx_hash, log_index, count() AS rows
FROM logs FINAL
GROUP BY tx_hash, log_index
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT tx_hash, trace_id, count() AS rows
FROM traces FINAL
GROUP BY tx_hash, trace_id
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT tx_hash, log_index, count() AS rows
FROM approvals FINAL
GROUP BY tx_hash, log_index
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT tx_hash, log_index, token_id, batch_ordinal, count() AS rows
FROM token_transfers FINAL
GROUP BY tx_hash, log_index, token_id, batch_ordinal
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT tx_hash, is_internal, ifNull(trace_id, '') AS trace_key, count() AS rows
FROM transactions FINAL
GROUP BY tx_hash, is_internal, trace_key
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT address, count() AS rows
FROM addresses FINAL
GROUP BY address
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT address, count() AS rows
FROM contracts FINAL
GROUP BY address
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT address, label, count() AS rows
FROM labels FINAL
GROUP BY address, label
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;

SELECT entity_kind, entity_id, count() AS rows
FROM embeddings FINAL
GROUP BY entity_kind, entity_id
HAVING rows > 1
ORDER BY rows DESC
LIMIT 50;
