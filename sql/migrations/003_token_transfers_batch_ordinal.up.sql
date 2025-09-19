-- Add batch ordinal column to token transfers tables and update sorting keys
-- to preserve ERC-1155 TransferBatch rows that share token_id values.
-- NOTE: The UPDATE statements issue synchronous mutations; run during a
-- low-traffic window or batch via smaller partitions for very large tables.

ALTER TABLE token_transfers
    ADD COLUMN IF NOT EXISTS batch_ordinal UInt16 DEFAULT 0 AFTER token_id;

ALTER TABLE token_transfers
    MODIFY ORDER BY (tx_hash, log_index, token_id, batch_ordinal);

-- Backfill existing rows using the `event_uid` suffix when present.
ALTER TABLE token_transfers
    UPDATE batch_ordinal = toUInt16(
        multiIf(
            length(splitByChar(':', event_uid)) = 3,
            arrayElement(splitByChar(':', event_uid), 3),
            '0'
        )
    )
WHERE batch_ordinal = 0
  AND length(splitByChar(':', event_uid)) = 3
SETTINGS mutations_sync = 1;

ALTER TABLE dev_token_transfers
    ADD COLUMN IF NOT EXISTS batch_ordinal UInt16 DEFAULT 0 AFTER token_id;

ALTER TABLE dev_token_transfers
    MODIFY ORDER BY (tx_hash, log_index, token_id, batch_ordinal);

ALTER TABLE dev_token_transfers
    UPDATE batch_ordinal = toUInt16(
        multiIf(
            length(splitByChar(':', event_uid)) = 3,
            arrayElement(splitByChar(':', event_uid), 3),
            '0'
        )
    )
WHERE batch_ordinal = 0
  AND length(splitByChar(':', event_uid)) = 3
SETTINGS mutations_sync = 1;
