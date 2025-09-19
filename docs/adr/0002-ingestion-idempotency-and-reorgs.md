# ADR 0002: Ingestion Idempotency and Reorg Handling

Status: Accepted

Date: 2025-09-10

## Context

We ingest Ethereum events (transactions, logs, traces) into ClickHouse using append-only writes. Providers may return duplicates and out-of-order items. Ethereum occasionally reorgs, replacing blocks within a small depth (typically < 12). ClickHouse lacks strong PK constraints; the ReplacingMergeTree engine deduplicates rows that share the same sorting key, keeping the row with the greatest `version` value.

We need consistent, idempotent ingestion that:

- Survives duplicates and retries without double-counting,
- Handles reorgs by letting canonical rows supersede earlier rows,
- Avoids deletes, using upsert-by-replacement semantics,
- Keeps hot-path queries fast without always relying on `FINAL`.

## Decision

1) Confirmations and cursoring

- Maintain `addresses.last_synced_block` as the last fully confirmed block height for each address.
- Use N confirmations (default 12). Only advance `last_synced_block` when `current_head - N >= block`.
- Always re-process the rolling reorg window (last N blocks) on each delta cycle; re-insert rows idempotently.

2) Logical keys with versioned replacement

- Deduplicate using the natural composite keys surfaced by Ethereum data:
  - `(tx_hash, log_index, batch_ordinal)` for `token_transfers` to retain ERC-1155 batch ordinals.
  - `(tx_hash, log_index)` across `logs` and `approvals`.
  - `(tx_hash, trace_id)` for `traces`.
  - `transactions` continue to rely on `(tx_hash, is_internal, ifNull(trace_id, ''))` so external rows coexist with normalized internal traces.
- Keep `ingested_at` (`DateTime64(3) UTC`) as the ReplacingMergeTree `version` column so the latest retry/reorg write wins without deletes.
- Retain `event_uid` / `trace_uid` columns for debugging and parity with historical exports, but they no longer drive primary sorting keys.

3) Insert semantics

- Never delete; always append. On reorgs/retries, insert the same logical key with a higher `ingested_at`; ReplacingMergeTree keeps the latest row.
- Accept duplicates inside a single fetch batch. ClickHouse performs replacement during merges, which keeps ingestion code simple and resilient to provider quirks.

4) Query semantics

- For correctness-critical queries (e.g., materialized views, nightly reconciliations), use `FINAL` or pre-aggregated projections that already apply replacement semantics.
- For hot-path API queries, prefer materialized views/pre-aggregates over base tables with `FINAL` to avoid performance penalties.

## Rationale

- Natural keys derived from on-chain identifiers keep replacements reliable even across reorgs while staying human-auditable.
- `ingested_at` provides strict monotonicity across retries and reorg updates (“last write wins”).
- Append-only and replacement semantics fit ClickHouse strengths while keeping operational complexity low.

## Consequences

Positive:

- Idempotent ingestion without deletions; reorg-safe updates naturally override prior rows.
- Simple ingester logic: compute UIDs for traceability, append writes, and rely on ClickHouse merges for deduplication.

Negative / Mitigations:

- `FINAL` can be expensive on large tables. Mitigate by using materialized views for API-facing aggregates and scheduling periodic `OPTIMIZE ... FINAL` during off-peak.
- Replacements take effect after background merges. Provide explicit maintenance queries (`sql/queries/maintenance/replacing_merge_tree_cleanup.sql`) for operators that need deterministic cleanup windows.

## Implementation Notes

- Ensure canonical tables include `ingested_at` (DateTime64(3) UTC) as the ReplacingMergeTree version column and natural-key ORDER BY clauses.
- Keep `event_uid` / `trace_uid` populated in normalization helpers for traceability, but do not rely on them for deduplication.
- Provide operators with `sql/queries/maintenance/replacing_merge_tree_cleanup.sql` showing the `OPTIMIZE ... FINAL` cadence per table, including duplicate checks on the new batch ordinal.
- Keep N configurable via `SYNC_CONFIRMATIONS` (default 12).

## References

- PRD: `docs/PRD_MVP_Wallet_Intelligence.md`
- ADR-0001 (DB Choice): `docs/adr/0001-db-choice.md`
