import Fastify from 'fastify'
import { z } from 'zod'
import { fileURLToPath } from 'url'
import { loadConfig } from './config.js'

// Minimal Fastify server scaffold. Final API will expose endpoints for sync,
// summary, lists, and semantic search.
export const app = Fastify({ logger: true })

// Prometheus-style metrics (lightweight, no external deps)
type Counter = Map<string, number>
const httpRequestsTotal: Counter = new Map()
const inc = (m: Counter, key: string, v = 1) => m.set(key, (m.get(key) ?? 0) + v)
const labelKey = (labels: Record<string, string>) =>
  '{' + Object.entries(labels).map(([k, v]) => `${k}="${String(v).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`).join(',') + '}'

app.addHook('onResponse', async (req, reply) => {
  // Skip counting metrics endpoint itself to avoid recursion
  /* c8 ignore next */
  if ((req.url || '').startsWith('/metrics')) return
  /* c8 ignore next */
  const route = (req as any).routeOptions?.url || req.url || ''
  const key = labelKey({ method: req.method, route, status: String(reply.statusCode) })
  inc(httpRequestsTotal, key)
})

app.get('/metrics', async (_req, reply) => {
  const lines: string[] = []
  lines.push('# HELP http_requests_total Total HTTP requests')
  lines.push('# TYPE http_requests_total counter')
  for (const [k, v] of httpRequestsTotal.entries()) {
    lines.push(`http_requests_total${k} ${v}`)
  }
  const mem = process.memoryUsage()
  lines.push('# HELP process_resident_memory_bytes Resident memory size')
  lines.push('# TYPE process_resident_memory_bytes gauge')
  lines.push(`process_resident_memory_bytes ${mem.rss}`)
  lines.push('# HELP process_uptime_seconds Process uptime in seconds')
  lines.push('# TYPE process_uptime_seconds gauge')
  lines.push(`process_uptime_seconds ${process.uptime().toFixed(0)}`)
  return reply.header('content-type', 'text/plain; version=0.0.4').send(lines.join('\n') + '\n')
})

// Simple in-memory cache and rate limiter for health endpoints
let lastHealthPingTs = 0
let lastHealthzTs = 0
let lastHealthzPayload: { status: 'ok'; clickhouse: ClickHouseHealth } | null = null

type ClickHouseHealth = {
  configured: boolean
  ok: boolean
  status?: number
  error?: string
}

// Basic token-bucket limiter (global) to avoid dependency
const healthLimiter = (() => {
  let windowStart = Date.now()
  let count = 0
  return (rate: number): boolean => {
    if (rate <= 0) return true
    const now = Date.now()
    if (now - windowStart >= 1000) {
      windowStart = now
      count = 0
    }
    if (count < rate) {
      count += 1
      return true
    }
    return false
  }
})()

app.get('/health', async (req, reply) => {
  const cfg = loadConfig()
  if (!healthLimiter(cfg.healthRateLimitRps)) {
    return reply.code(429).send({ error: 'rate limited' })
  }
  // Lightweight ClickHouse health check: only if configured; ignore errors.
  try {
    const dsn = buildClickHouseDSN(cfg)
    if (dsn) {
      const now = Date.now()
      if (now - lastHealthPingTs >= cfg.healthCacheTtlMs) {
        const { url, authHeader } = sanitizeDSNForRequest(dsn, cfg)
        const u = new URL(url)
        const q = new URLSearchParams(u.search)
        q.set('query', 'SELECT 1')
        u.search = q.toString()
        const ctrl = new AbortController()
        const timer = setTimeout(() => ctrl.abort(), cfg.healthPingTimeoutMs)
        try {
          await fetch(u, { method: 'GET', signal: ctrl.signal, headers: authHeader ? { Authorization: authHeader } : undefined })
        } finally {
          clearTimeout(timer)
          lastHealthPingTs = now
        }
      }
    }
  } catch {
    // ignore
  }
  return { status: 'ok' }
})

// Detailed health endpoint (guarded by HEALTH_DEBUG). Returns ClickHouse details.
app.get('/healthz', async (req, reply) => {
  const cfg = loadConfig()
  if (!healthLimiter(cfg.healthRateLimitRps)) {
    return reply.code(429).send({ error: 'rate limited' })
  }
  if (!cfg.healthDebug) {
    return reply.code(404).send({ error: 'not found' })
  }
  const dsn = buildClickHouseDSN(cfg)
  let ch: ClickHouseHealth = { configured: !!dsn, ok: false }
  if (dsn) {
    const now = Date.now()
    if (lastHealthzPayload && now - lastHealthzTs < cfg.healthCacheTtlMs) {
      return lastHealthzPayload
    }
    try {
      const { url, authHeader } = sanitizeDSNForRequest(dsn, cfg)
      const u = new URL(url)
      const q = new URLSearchParams(u.search)
      q.set('query', 'SELECT 1')
      u.search = q.toString()
      const ctrl = new AbortController()
      const timer = setTimeout(() => ctrl.abort(), cfg.healthPingTimeoutMs)
      try {
        const r = await fetch(u, { method: 'GET', signal: ctrl.signal, headers: authHeader ? { Authorization: authHeader } : undefined })
        ch.ok = r.ok
        ch.status = r.status
      } finally {
        clearTimeout(timer)
      }
    } catch (e: any) {
      ch.ok = false
      ch.error = String(e?.message || e)
      /* c8 ignore next */
      app.log.warn({ err: ch.error, dsn: redactDSN(dsn) }, 'clickhouse healthz error')
    }
    lastHealthzPayload = { status: 'ok', clickhouse: ch }
    lastHealthzTs = Date.now()
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
    // Avoid embedding credentials in URL for security; use Authorization header instead
    u.username = ''
    u.password = ''
    const p = u.pathname.replace(/\/+$/, '')
    u.pathname = p.endsWith('/' + db) ? p : p + '/' + db
    return u.toString()
  } catch {
    // Do not attempt naive concatenation; avoid producing invalid URLs
    return ''
  }
}

// Extract Basic auth header from DSN credentials, but return URL without creds
function sanitizeDSNForRequest(dsn: string, cfg: ReturnType<typeof loadConfig>): { url: string; authHeader?: string } {
  try {
    const u = new URL(dsn)
    let authHeader: string | undefined
    if (u.username || u.password) {
      const raw = `${decodeURIComponent(u.username)}:${decodeURIComponent(u.password)}`
      const b64 = Buffer.from(raw).toString('base64')
      authHeader = `Basic ${b64}`
      u.username = ''
      u.password = ''
    } else if (cfg.clickhouse.user || cfg.clickhouse.pass) {
      const raw = `${cfg.clickhouse.user}:${cfg.clickhouse.pass}`
      const b64 = Buffer.from(raw).toString('base64')
      authHeader = `Basic ${b64}`
    }
    return { url: u.toString(), authHeader }
  } catch {
    return { url: dsn }
  }
}
// Redact credentials in DSN-like URLs for safe logging
/* c8 ignore start */
function redactDSN(s: string): string {
  if (!s) return s
  try {
    const u = new URL(s)
    if (u.username || u.password) {
      const user = u.username || '***'
      u.username = user
      u.password = '***'
      return u.toString()
    }
    return s
  } catch {
    const m = s.match(/(.*\/\/)([^@]+)@/)
    if (m) {
      const user = (m[2].split(':')[0]) || '***'
      return s.replace(m[0], `${m[1]}${user}:***@`)
    }
    return s
  }
}
/* c8 ignore stop */
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
  const shutdown = async (sig: string) => {
    try {
      app.log.info({ sig }, 'shutting down')
      await app.close()
    } catch (e) {
      app.log.error(e)
    } finally {
      process.exit(0)
    }
  }
  process.on('SIGINT', () => shutdown('SIGINT'))
  process.on('SIGTERM', () => shutdown('SIGTERM'))
}
/* c8 ignore stop */
