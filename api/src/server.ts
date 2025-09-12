import Fastify from 'fastify'
import { z } from 'zod'
import { fileURLToPath } from 'url'
import { loadConfig } from './config.js'

// Minimal Fastify server scaffold. Final API will expose endpoints for sync,
// summary, lists, and semantic search.
export const app = Fastify({ logger: true })

app.get('/health', async () => {
  // Lightweight ClickHouse health check: only if configured; ignore errors.
  try {
    const cfg = loadConfig()
    const dsn = buildClickHouseDSN(cfg)
    if (dsn) {
      const u = new URL(dsn)
      const q = new URLSearchParams(u.search)
      q.set('query', 'SELECT 1')
      u.search = q.toString()
      await fetch(u, { method: 'GET' })
    }
  } catch {
    // ignore
  }
  return { status: 'ok' }
})

// Detailed health endpoint (guarded by HEALTH_DEBUG). Returns ClickHouse details.
app.get('/healthz', async (req, reply) => {
  const cfg = loadConfig()
  if (!cfg.healthDebug) {
    return reply.code(404).send({ error: 'not found' })
  }
  const dsn = buildClickHouseDSN(cfg)
  let ch: any = { configured: !!dsn, ok: false as boolean }
  if (dsn) {
    try {
      const u = new URL(dsn)
      const q = new URLSearchParams(u.search)
      q.set('query', 'SELECT 1')
      u.search = q.toString()
      const r = await fetch(u, { method: 'GET' })
      ch.ok = r.ok
      ch.status = r.status
    } catch (e: any) {
      ch.ok = false
      ch.error = String(e?.message || e)
    }
  }
  return { status: 'ok', clickhouse: ch }
})

/* c8 ignore start */
function buildClickHouseDSN(cfg: ReturnType<typeof loadConfig>): string {
  const dsn = cfg.clickhouse.dsn
  if (dsn) return dsn
  const base = cfg.clickhouse.url
  const db = cfg.clickhouse.db
  if (!base || !db) return ''
  try {
    const u = new URL(base)
    if (cfg.clickhouse.user || cfg.clickhouse.pass) {
      const user = cfg.clickhouse.user ?? ''
      const pass = cfg.clickhouse.pass ?? ''
      u.username = user
      u.password = pass
    }
    const p = u.pathname.replace(/\/+$/, '')
    u.pathname = p.endsWith('/' + db) ? p : p + '/' + db
    return u.toString()
  } catch {
    const p = base.replace(/\/+$/, '')
    return p + '/' + db
  }
}
/* c8 ignore stop */

app.post('/v1/address/:address/sync', async (req, reply) => {
  const schema = z.object({ address: z.string().regex(/^0x[a-fA-F0-9]{40}$/) })
  const params = schema.safeParse((req as any).params)
  if (!params.success) return reply.status(400).send({ error: 'invalid address' })
  // TODO: enqueue backfill/delta job
  return { accepted: true }
})

// start prepares the Fastify app for use (routes/plugins ready) without
// binding a network socket. Tests and embedded usage call start(); the CLI
// path below performs the actual listen on 0.0.0.0 for production.
export async function start() {
  await app.ready()
}

const isMain = process.argv[1] === fileURLToPath(import.meta.url)
/* c8 ignore start */
if (isMain) {
  const { port } = loadConfig()
  app
    .listen({ port, host: '0.0.0.0' })
    .catch((err) => {
      app.log.error(err)
      process.exit(1)
    })
}
/* c8 ignore stop */
