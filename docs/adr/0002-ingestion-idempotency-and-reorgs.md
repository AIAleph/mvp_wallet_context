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

2) Stable logical IDs for deduplication

- Introduce surrogate IDs computed from logical keys:
  - `event_uid`: `String` composed as `<tx_hash>:<log_index>` for logs (token transfers, approvals).
  - `trace_uid`: `String` composed as `<tx_hash>:<trace_id>` for internal traces.
  - External transactions use `tx_hash` as the natural key; set `is_internal=0`.
- Store `ingested_at` (`DateTime64(3) UTC`) as the ReplacingMergeTree `version` column.
- Adjust ClickHouse ORDER BY to start with the surrogate key to enable replacement independent of timestamps, e.g.:
  - `token_transfers`: `ORDER BY (event_uid)` (optionally followed by secondary sort columns for locality),
  - `approvals`: `ORDER BY (event_uid)`,
  - `transactions`: `ORDER BY (tx_hash, is_internal, trace_uid)`.

3) Insert semantics

- Never delete; always append. On reorgs/retries, insert the same surrogate key with a higher `ingested_at` value; ReplacingMergeTree keeps the latest.
- Within-batch dedup: the ingester ensures uniqueness per batch on the surrogate keys to avoid needless duplicates.

4) Query semantics

- For correctness-critical queries (e.g., materialized views, nightly reconciliations), use `FINAL` or pre-aggregated projections that already apply replacement semantics.
- For hot-path API queries, prefer materialized views/pre-aggregates over base tables with `FINAL` to avoid performance penalties.

## Rationale

- Surrogate IDs decouple dedup from variable columns (like timestamps) so replacements are reliable even across reorgs.
- `ingested_at` provides strict monotonicity across retries and reorg updates (“last write wins”).
- Append-only and replacement semantics fit ClickHouse strengths while keeping operational complexity low.

## Consequences

Positive:

- Idempotent ingestion without deletions; reorg-safe updates naturally override prior rows.
- Simple ingester logic: compute UIDs, batch-dedup, append writes.

Negative / Mitigations:

- `FINAL` can be expensive on large tables. Mitigate by using materialized views for API-facing aggregates and scheduling periodic `OPTIMIZE ... FINAL` during off-peak.
- Surrogate IDs add storage cost; acceptable for MVP. Hash-based UIDs can be introduced later if needed.

## Implementation Notes

- Add columns: `event_uid` (String), `trace_uid` (String), `ingested_at` (DateTime64(3) UTC) to relevant tables.
- Update `sql/schema.sql` to place surrogate IDs first in `ORDER BY`, with `ReplacingMergeTree(ingested_at)`.
- Update the ingester to compute UIDs and perform within-batch dedup.
- Keep N configurable via `SYNC_CONFIRMATIONS` (default 12).

## References

- PRD: `docs/PRD_MVP_Wallet_Intelligence.md`
- ADR-0001 (DB Choice): `docs/adr/0001-db-choice.md`
