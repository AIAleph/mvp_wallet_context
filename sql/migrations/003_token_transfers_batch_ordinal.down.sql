-- Revert batch ordinal column and sorting changes on token transfer tables.

ALTER TABLE dev_token_transfers
    MODIFY ORDER BY (tx_hash, log_index, token_id);

ALTER TABLE dev_token_transfers
    DROP COLUMN IF EXISTS batch_ordinal;

ALTER TABLE token_transfers
    MODIFY ORDER BY (tx_hash, log_index, token_id);

ALTER TABLE token_transfers
    DROP COLUMN IF EXISTS batch_ordinal;
