API (TypeScript)

Overview
- Node 20+, ESM with `module`/`moduleResolution` set to `NodeNext`.
- Fastify server scaffold with strict Zod validation.
- Tests via Vitest with v8 coverage and 100% thresholds.

Build & Test
- Install: `npm ci`
- Build: `npm run build` (uses `tsconfig.build.json` and excludes tests)
- Test (CI/default): `npm test` (runs Vitest with coverage, pool=forks)
- Test (local fallback): `npm run test:threads` if your environment restricts forking.

ESM Path Rules (NodeNext)
- Relative imports must include file extensions at runtime. In tests, import compiled files with `.js` extension, e.g. `import { app } from './server.js'`.

tsconfig Split
- `tsconfig.json`: base config for editor tooling and tests.
- `tsconfig.build.json`: extends base and excludes `**/*.spec.ts` and `**/*.test.ts`. `npm run build` targets this file.

Server Startup Semantics
- `start()` prepares the Fastify app (`app.ready()`) but does not bind a socket. This keeps tests hermetic and sandbox‑friendly.
- The CLI path (when the module is executed directly) performs `app.listen({ host: '0.0.0.0', port })` and is wrapped in `/* c8 ignore start/stop */` to avoid penalizing coverage for untestable process paths.

Notes
- Coverage thresholds are enforced at 100% in `vitest.config.ts`. If you add new endpoints, ensure tests cover success and error paths or use targeted `/* c8 ignore */` for truly untestable branches.

Health Endpoints
- `GET /health`: returns `{ status: 'ok' }`. If ClickHouse is configured via env, the server performs a lightweight `SELECT 1` probe using the HTTP interface, but errors are ignored to keep this endpoint always green.
- `GET /healthz`: detailed health; only enabled when `HEALTH_DEBUG=1|true|yes|on`. Responds with `{ status: 'ok', clickhouse: { configured, ok, status?, error? } }`.
 - Timeout: ClickHouse pings use `HEALTH_PING_TIMEOUT_MS` (default `3000`) to avoid hanging when the host is unreachable.
 - Rate limit: set `HEALTH_RATE_LIMIT_RPS` to limit requests per second (default `0` disables limiting).
 - Caching: health checks are cached for `HEALTH_CACHE_TTL_MS` (default `5000`), avoiding repeated work under load.

ClickHouse Config
- Preferred DSN: set `CLICKHOUSE_DSN` (e.g., `http://user:pass@localhost:8123/wallets`).
- Or provide parts: `CLICKHOUSE_URL`, `CLICKHOUSE_DB`, optional `CLICKHOUSE_USER`, `CLICKHOUSE_PASS`.
- The server constructs the DSN for health checks using these values; credentials are not logged and are sent via `Authorization: Basic ...` header (not embedded in the URL at request time).
 
Health-related env
- `HEALTH_PING_TIMEOUT_MS` (default `3000`)
- `HEALTH_CACHE_TTL_MS` (default `5000`)
- `HEALTH_RATE_LIMIT_RPS` (default `0`, disabled)
 - Health probe timeout configurable via `HEALTH_PING_TIMEOUT_MS` (milliseconds; default `1000`).
