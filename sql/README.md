This folder will contain ClickHouse DDL/DML and common query files.

Initial scope:
- schema.sql: base tables (addresses, transactions, token_transfers, approvals, contracts, labels, embeddings)
- queries/: summary and list queries, semantic search helpers
- queries/maintenance/replacing_merge_tree_cleanup.sql: `OPTIMIZE ... FINAL`
  cadence plus duplicate-check queries for the ReplacingMergeTree tables

See PRD and ADR-0001/0002 for schema and idempotency guidance.
