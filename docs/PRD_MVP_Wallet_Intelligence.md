# PRD – MVP Wallet Intelligence (Ethereum)

## 0) Executive Summary

Goal: Given an Ethereum address, extract and normalize all relevant activity (external transactions, internal traces, ERC‑20 token transfers, ERC‑721/1155 NFT transfers, approvals, contract creation/upgrades), label counterparties (dApps, tokens, NFT collections, CEX, etc.), persist the data, and expose a concise, queryable view.

Two key answers:

- Vector DB: not efficient as a primary store for transactional data. It is valuable as a secondary semantic index (e.g., “similar wallets”, free text over ABI/labels). For MVP we will use ClickHouse as the primary analytic store and its native Vector Index (HNSW) for semantic search.
- Ingestion: Feasible via an incremental, block/addr-based pipeline with provider APIs (Alchemy/Infura/QuickNode/Covalent). MVP: backfill per address, then delta updates; handle rate limits and chain reorgs via checkpoints.

## 1) Scope (In/Out)

In scope (MVP):

- Input: single Ethereum (EVM mainnet) address.
- Collection: external tx (from/to), internal traces, ERC‑20/721/1155 Transfer events, approvals, contract creation; basic token/contract metadata.
- Enrichment: counterparty classification (EOA vs contract; if contract → token? NFT? known dApp?) + labels with confidence.
- Persistence: canonical event store, vector index, optional cache.
- Query: cache check (“already fetched?”); delta update; address summary view.

Out of scope (MVP):

- Multi-chain (L2/sidechains), full historical price/time series, advanced AML, multi-heuristic clustering, complex front-end.

## 2) Personas & Use Cases

- Analyst: wants a wallet profile and protocols used.
- Builder: maps test-user interactions and dApp dependencies.
- Power user: quickly inspects token/NFT activity and risky approvals.

Primary use cases:

- Enter an address → get overview + labeled counterparties.
- Re-enter address → hit cache, only delta update, refresh counters.
- Free text search (beta): “Uniswap” → find relevant transactions via embeddings.

## 3) User Flow (MVP)

- Input address → checksum/0x validation.
- Cache check: `addresses.last_synced_block`.
- Backfill or Delta: batched fetch by block range (e.g., 1k–5k) and data type.
- Normalize: unify tx, logs, traces → common schema.
- Enrich: for each counterparty: `eth_getCode` (EOA/contract), ERC‑165/ABI probing, name/symbol/decimals, label registry, heuristics (CEX hot wallet, bridge, router).
- Persist: idempotent upsert; update indices and embeddings.
- Serve: response sections (overview, tokens, NFT, dApp, approvals, timeline).

## 4) Functional Requirements

- F1. Address input and validation.
- F2. Fetch external tx, internal traces, ERC‑20/721/1155 transfers, approvals, contract creation.
- F3. Counterparty labeling: {EOA | Contract[Token20/721/1155 | dApp(other)]} + confidence.
- F4. Idempotent persistence; deduplicate on (tx_hash, log_index).
- F5. Delta updates based on last_synced_block with N confirmations (e.g., 12).
- F6. Summary view + filterable lists (tokens, NFT, dApps, approvals).
- F7. Text/semantic search (beta) over labels/ABI/token/collection names.

## 5) Non-Functional Requirements (MVP targets)

- Latency: p95 < 1.5s if address already indexed; < 8s for first backfill up to 5k events.
- Throughput: ≥ 10 addresses/min under light backfill (provider-dependent).
- Reliability: retries with backoff; idempotency; per-type checkpoints.
- Cost: provider API budget under configurable threshold.
- Security: rate limiting, input sanitization, audit log.

## 6) Proposed Architecture (MVP)

- Primary store: ClickHouse (MergeTree family) + native Vector Index (HNSW).
- Pipeline: Ingestion Service → Normalizer → Enricher → ClickHouse → API.
- Providers: Alchemy/Infura/QuickNode/Covalent for tx/logs/traces. NFT metadata optional post-MVP.
- Cache: optional Redis for faster summary responses.
- Key jobs: backfill_address, delta_update, enrich_counterparty, embed_entities.

Pros:

- Unifies analytic events + vector search in one engine.
- Efficient partitioning/compression for large log volumes.
- Integrated ANN (cosine/L2) with HNSW.

Cons/Mitigations:

- Less suited for OLTP/mutative workloads → append-only schema + idempotency via logical keys and dedup.
- No strong PK constraints → use ReplacingMergeTree with version and deduplication on insert.

## 7) Data Schema (ClickHouse)

Primary tables (simplified DDL):

- addresses (sync state/cache)
  - address String, first_seen_ts DateTime64(3), last_synced_block UInt64, last_synced_at DateTime64(3), counters…
  - Engine: ReplacingMergeTree(last_synced_at) PARTITION BY toYYYYMM(first_seen_ts) ORDER BY (address)

- transactions (external + normalized internal traces)
  - tx_hash String, block_number UInt64, ts DateTime64(3), from_addr String, to_addr String, value_wei Decimal(38,0), gas_used UInt64, status UInt8, input_method FixedString(10) NULL, is_internal UInt8, trace_id String NULL
  - Engine: ReplacingMergeTree(ts) PARTITION BY toYYYYMM(ts) ORDER BY (to_addr, from_addr, ts, tx_hash)

- token_transfers
  - tx_hash String, log_index UInt32, ts DateTime64(3), token_address String, from_addr String, to_addr String, amount_raw Decimal(76,0), decimals UInt8, standard LowCardinality(String), token_id Nullable(UInt256)
  - Engine: ReplacingMergeTree(ts) PARTITION BY toYYYYMM(ts) ORDER BY (token_address, ts, log_index, tx_hash)

- approvals
  - tx_hash String, log_index UInt32, ts DateTime64(3), owner String, spender String, token_address String, token_id Nullable(UInt256), allowance_raw Nullable(Decimal(76,0)), standard LowCardinality(String)
  - Engine: ReplacingMergeTree(ts) PARTITION BY toYYYYMM(ts) ORDER BY (owner, token_address, spender, ts, log_index)

- contracts
  - contract_address String, created_at_tx String, verified UInt8, name Nullable(String), symbol Nullable(String), decimals Nullable(UInt8), standard Nullable(String), is_token UInt8, is_nft UInt8, supports_erc165 UInt8, abi_json String NULL
  - Engine: ReplacingMergeTree(created_at_tx) ORDER BY (contract_address)

- labels
  - entity_address String, label String, source LowCardinality(String), confidence Float32, updated_at DateTime64(3)
  - Engine: ReplacingMergeTree(updated_at) ORDER BY (entity_address, source)

- embeddings
  - entity_id String, kind LowCardinality(String), embedding Vector(Float32, 1536), model LowCardinality(String), created_at DateTime64(3)
  - Engine: MergeTree ORDER BY (kind, entity_id)

Vector index (ANN):

CREATE INDEX emb_hnsw ON embeddings (embedding)
TYPE hnsw('metric_type=cosine','M=16','ef_construction=200')
GRANULARITY 1;

Notes:

- Idempotency: dedup on (tx_hash, log_index) during ingest or via ReplacingMergeTree versioning.
- Text search: LowCardinality(String) columns; derive tables for full-text (optional OpenSearch post-MVP).
- Joins: prefer Materialized Views for pre-aggregates (e.g., address counters).

## 8) Enrichment & Classification

Per counterparty:

- `eth_getCode` → EOA vs contract.
- ERC‑165 probe (if available) and standard functions: `supportsInterface`, `name()`, `symbol()`, `decimals()`.
- Token/NFT heuristics: presence of Transfer events (ERC‑20/721/1155 topics); check `totalSupply()`; interfaces 0x80ac58cd (ERC‑721), 0xd9b67a26 (ERC‑1155).
- dApp registry: curated mapping of known addresses (AMM routers, bridges, lending, CEX hot wallets) from mixed sources (curated CSV import, explorer APIs, public lists) with confidence.
- Proxy detection (EIP‑1967, UUPS): read implementation storage slots.
- Label merge: dedup sources, compute final score.

Output: {type: EOA|CONTRACT, role: TOKEN20|NFT721|NFT1155|DAPPSVC|UNKNOWN, labels: [], confidence: 0..1}.

## 9) Database Decision

- Decision: Use ClickHouse with native Vector Index (HNSW) as the single database for the MVP to unify event-centric analytics and semantic search.
- Rationale: Single engine reduces ops cost and latency; strong performance on append-only event workloads; built-in ANN for similarity.
- Mitigations: Use ReplacingMergeTree and logical keys for idempotency/dedup; design append-only APIs and pre-aggregates to avoid OLTP contention.
- Future: If advanced vector features or multi-tenant RAG are needed, add Weaviate/Milvus as a secondary index while keeping ClickHouse as the source of truth.

## 10) Ingestion Plan (Details)

- Batch by block ranges with cursor (e.g., 5k), pipeline per data type (tx, logs, traces).
- Idempotency: logical keys (tx_hash, log_index) for logs; (tx_hash, trace_id) for traces; in ClickHouse use ReplacingMergeTree and a monotonic version column to deduplicate.
- Confirmations: update `addresses.last_synced_block` only for blocks with ≥ N confirmations (default 12); keep a rolling reorg window for the last N blocks and re-upsert.
- Rate limiting: exponential backoff + circuit breaker; shared HTTP client with timeouts.
- Checkpoints: per data type; resume safely after interruptions.
- Testing: recorded fixtures for all RPC calls; forbid live-chain calls in CI.
