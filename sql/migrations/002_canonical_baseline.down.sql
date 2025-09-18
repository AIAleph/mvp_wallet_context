-- v2 down: drop canonical tables (leaves dev_* tables intact)
DROP TABLE IF EXISTS logs;
DROP TABLE IF EXISTS traces;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS token_transfers;
DROP TABLE IF EXISTS approvals;
DROP TABLE IF EXISTS addresses;
DROP TABLE IF EXISTS contracts;
DROP TABLE IF EXISTS labels;
DROP TABLE IF EXISTS embeddings;
