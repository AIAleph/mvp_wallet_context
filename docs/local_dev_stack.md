Local Dev Stack (Docker Compose)

Services
- ClickHouse server: `clickhouse/clickhouse-server@sha256:1ffa82…` exposed on `8123` (HTTP) and `9000` (native TCP)
- Redis: `redis@sha256:395ccd…` exposed on `${REDIS_HOST_PORT:-6380}` (host) -> `6379` (container)

Usage
- Start: `make dev-up`
- Logs: `make dev-logs`
- Stop: `make dev-down`
- Apply dev schema using container client: `make schema-dev-dc` (uses `sql/schema_dev.sql` and DB from `CLICKHOUSE_DB`, default `wallets`)
- Purge everything (containers, network, volumes): `make dev-nuke`
- Interactive ClickHouse shell: `make ch-client` (uses `CLICKHOUSE_DB`/`CH_DB`, default `wallets`)

Environment
- ClickHouse: override with `.env` or shell env vars consumed by Compose
  - `REDIS_HOST_PORT` (default: `6380`) — host port mapped to Redis container `6379`
  - `CLICKHOUSE_DB` (default: `wallets`)
  - `CLICKHOUSE_USER` (default: `default`)
  - `CLICKHOUSE_PASSWORD` (default: empty; used by the container)
- App config (example):
  - `CLICKHOUSE_URL=http://localhost:8123`
  - `CLICKHOUSE_DB=wallets`
  - `CLICKHOUSE_USER=default`
  - `CLICKHOUSE_PASS=` (used by the app; distinct from `CLICKHOUSE_PASSWORD`)
  - `REDIS_URL=redis://localhost:6380` (or match your `REDIS_HOST_PORT`)

Notes
- Minimal stack does not use Keeper; our dev schema does not rely on replication.
- Persisted volumes: `ch_data`, `redis_data`.

Design Choices (clean + minimal)
- No container_name overrides; rely on Compose service names.
- No log volumes; use Docker logs (ephemeral) to avoid clutter.
- Only required ports exposed (8123/9000 for ClickHouse; 6380 for Redis).
- Data volumes persist only necessary state (ClickHouse, Redis data).

Image Pinning
- Images are pinned by immutable digest to ensure reproducible local environments.
- To update to a newer version, bump tags temporarily, pull, inspect digests, then replace digests in `docker-compose.yml`.

Healthchecks
- ClickHouse: healthcheck runs `SELECT 1` using `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` when set (defaults user to `default`).
- Redis: `redis-cli ping` returns `PONG`.
Startup Ordering
- Services are independent; start order does not matter for local development.
