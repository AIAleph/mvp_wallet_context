-- Add batch ordinal column to token transfers tables and update sorting keys
-- to preserve ERC-1155 TransferBatch rows that share token_id values.

ALTER TABLE token_transfers
    ADD COLUMN IF NOT EXISTS batch_ordinal UInt16 DEFAULT 0 AFTER token_id;

ALTER TABLE token_transfers
    MODIFY ORDER BY (tx_hash, log_index, token_id, batch_ordinal);

ALTER TABLE dev_token_transfers
    ADD COLUMN IF NOT EXISTS batch_ordinal UInt16 DEFAULT 0 AFTER token_id;

ALTER TABLE dev_token_transfers
    MODIFY ORDER BY (tx_hash, log_index, token_id, batch_ordinal);
