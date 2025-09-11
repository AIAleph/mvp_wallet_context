API (TypeScript)

Overview
- Node 20+, ESM with `module`/`moduleResolution` set to `NodeNext`.
- Fastify server scaffold with strict Zod validation.
- Tests via Vitest with v8 coverage and 100% thresholds.

Build & Test
- Install: `npm ci`
- Build: `npm run build` (uses `tsconfig.build.json` and excludes tests)
- Test (CI/default): `npm test` (runs Vitest with coverage, pool=forks)
- Test (local fallback): `npx vitest run --coverage --pool=threads` if your environment restricts forking.

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

