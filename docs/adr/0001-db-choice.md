# ADR 0001: Database Choice for MVP

Status: Accepted

Date: 2025-09-10

## Context

The MVP needs a primary data store for high-volume, append-only, event-centric data (transactions, logs, traces) and a secondary capability for semantic similarity search over labels, contracts, and entities. We evaluated:

- ClickHouse (MergeTree family) with native Vector Index (HNSW)
- Postgres with `pgvector`
- Vector-only engines (Weaviate/Milvus)
- Graph databases (Neo4j/Arango) for relationships (post-MVP scope)

Constraints and goals:

- Fast analytical queries over large, partitioned event tables
- Idempotent ingestion and dedup on logical keys
- Integrated vector similarity for semantic search
- Minimal operational surface for MVP

## Decision

Adopt ClickHouse as the primary database for the MVP and use its native HNSW vector index for semantic search.

## Rationale

- Single engine covers both OLAP on events and ANN for embeddings, reducing ops and latency.
- Excellent performance and compression on append-only, time-partitioned data.
- Mature ecosystem and HTTP interface simplifies service integration.

## Consequences

Positive:

- Unified storage for structured events and vectors; simpler infra and deployment.
- Strong performance on range scans and aggregations; scalable partitioning.

Negative / Mitigations:

- No strong PK constraints: use ReplacingMergeTree with version columns and logical keys (e.g., `(tx_hash, log_index)`), and dedup on insert.
- Less suitable for OLTP: design append-only ingestion and build pre-aggregates/materialized views for API latency.

## Alternatives Considered

- Postgres + pgvector: simpler ACID semantics, but weaker for large-scale event analytics; vector search is adequate but operationally separate indexes grow large.
- Weaviate/Milvus: strong vector capabilities, but introduces a second engine and higher complexity; overkill for MVP.
- Graph DB: valuable for relationship queries but outside MVP scope; can be introduced later for path/cluster analysis.

## References

- PRD – MVP Wallet Intelligence: `docs/PRD_MVP_Wallet_Intelligence.md`
- ClickHouse Vector Index docs (HNSW)
