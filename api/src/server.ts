import Fastify from 'fastify'
import { z } from 'zod'
import { fileURLToPath } from 'url'
import { loadConfig } from './config.js'
import type { AppConfig } from './config.js'

// Minimal Fastify server scaffold. Final API will expose endpoints for sync,
// summary, lists, and semantic search.
export const app = Fastify({ logger: true })

class TimedLRUCache<K, V> {
  #store = new Map<K, { value: V; expiresAt: number }>()
  constructor(private readonly capacity: number) {
    if (!Number.isFinite(capacity) || capacity <= 0) {
      throw new Error('TimedLRUCache capacity must be positive')
    }
  }

  get(key: K, now = Date.now()): V | undefined {
    const entry = this.#store.get(key)
    if (!entry) return undefined
    if (entry.expiresAt <= now) {
      this.#store.delete(key)
      return undefined
    }
    this.#store.delete(key)
    this.#store.set(key, entry)
    return entry.value
  }

  set(key: K, value: V, ttlMs: number, now = Date.now()): void {
    if (!Number.isFinite(ttlMs) || ttlMs <= 0) {
      this.#store.delete(key)
      return
    }
    this.prune(now)
    if (this.#store.has(key)) {
      this.#store.delete(key)
    }
    this.#store.set(key, { value, expiresAt: now + ttlMs })
    while (this.#store.size > this.capacity) {
      const oldestKey = this.#store.keys().next().value
      /* c8 ignore next */
      if (oldestKey === undefined) break
      this.#store.delete(oldestKey)
    }
  }

  prune(now = Date.now()): void {
    for (const [key, entry] of this.#store.entries()) {
      if (entry.expiresAt <= now) {
        this.#store.delete(key)
      }
    }
  }
}

class CoalescingMap<K, V> {
  #pending = new Map<K, Promise<V>>()
  async run(key: K, factory: () => Promise<V>): Promise<V> {
    const existing = this.#pending.get(key)
    if (existing) {
      return existing
    }
    const task = (async () => {
      try {
        return await factory()
      } finally {
        this.#pending.delete(key)
      }
    })()
    this.#pending.set(key, task)
    return task
  }

  clear(): void {
    this.#pending.clear()
  }
}

class CircuitBreaker {
  #state: 'closed' | 'open' | 'half-open' = 'closed'
  #failures = 0
  #openedAt = 0
  constructor(private readonly failureThreshold: number, private readonly resetTimeoutMs: number) {}

  allow(now = Date.now()): boolean {
    if (this.failureThreshold <= 0 || this.resetTimeoutMs <= 0) {
      return true
    }
    if (this.#state === 'open') {
      if (now - this.#openedAt >= this.resetTimeoutMs) {
        this.#state = 'half-open'
        return true
      }
      return false
    }
    return true
  }

  recordSuccess(): void {
    if (this.failureThreshold <= 0 || this.resetTimeoutMs <= 0) {
      return
    }
    this.#failures = 0
    this.#state = 'closed'
    this.#openedAt = 0
  }

  recordFailure(now = Date.now()): void {
    if (this.failureThreshold <= 0 || this.resetTimeoutMs <= 0) {
      return
    }
    if (this.#state === 'open' && now - this.#openedAt < this.resetTimeoutMs) {
      return
    }
    if (this.#state === 'half-open') {
      this.#state = 'open'
      this.#openedAt = now
      this.#failures = this.failureThreshold
      return
    }
    this.#failures += 1
    if (this.#failures >= this.failureThreshold) {
      this.#state = 'open'
      this.#openedAt = now
    }
  }
}

const DEFAULT_HEALTH_CACHE_CAPACITY = 8
type HealthSummaryStatus = 'ok' | 'degraded'
type HealthPingState = { at: number; ok: boolean }
type CachedHealthPayload = { body: { status: HealthSummaryStatus; clickhouse: ClickHouseHealth }; statusCode: number }

let healthPingCache = new TimedLRUCache<string, HealthPingState>(DEFAULT_HEALTH_CACHE_CAPACITY)
let healthPayloadCache = new TimedLRUCache<string, CachedHealthPayload>(DEFAULT_HEALTH_CACHE_CAPACITY)
let currentHealthCacheCapacity = DEFAULT_HEALTH_CACHE_CAPACITY

const healthPingCoalescer = new CoalescingMap<string, void>()
const healthPayloadCoalescer = new CoalescingMap<string, CachedHealthPayload>()
const clickhouseBreakers = new Map<string, { breaker: CircuitBreaker; threshold: number; resetMs: number }>()

function ensureHealthCacheCapacity(capacity: number): void {
  let next = Number.isFinite(capacity) && capacity > 0 ? Math.floor(capacity) : DEFAULT_HEALTH_CACHE_CAPACITY
  if (next === currentHealthCacheCapacity) {
    return
  }
  healthPingCache = new TimedLRUCache<string, HealthPingState>(next)
  healthPayloadCache = new TimedLRUCache<string, CachedHealthPayload>(next)
  currentHealthCacheCapacity = next
}

function getClickHouseBreaker(cfg: AppConfig, key: string): CircuitBreaker | undefined {
  const { failureThreshold, resetMs } = cfg.healthCircuitBreaker
  if (failureThreshold <= 0 || resetMs <= 0) {
    return undefined
  }
  const mapKey = key || 'default'
  const existing = clickhouseBreakers.get(mapKey)
  if (existing && existing.threshold === failureThreshold && existing.resetMs === resetMs) {
    return existing.breaker
  }
  const breaker = new CircuitBreaker(failureThreshold, resetMs)
  clickhouseBreakers.set(mapKey, { breaker, threshold: failureThreshold, resetMs })
  return breaker
}

function sanitizeHealthError(err: unknown): string {
  if (err && typeof err === 'object' && 'name' in err && (err as any).name === 'AbortError') {
    return 'timeout'
  }
  if (err instanceof Error) {
    const msg = err.message.toLowerCase()
    if (msg.includes('timeout') || msg.includes('timed out') || msg.includes('abort')) {
      return 'timeout'
    }
    if (msg.includes('refused') || msg.includes('unreachable') || msg.includes('dns')) {
      return 'unreachable'
    }
  }
  return 'unavailable'
}

export function __resetHealthStateForTests() {
  healthPingCache = new TimedLRUCache<string, HealthPingState>(DEFAULT_HEALTH_CACHE_CAPACITY)
  healthPayloadCache = new TimedLRUCache<string, CachedHealthPayload>(DEFAULT_HEALTH_CACHE_CAPACITY)
  currentHealthCacheCapacity = DEFAULT_HEALTH_CACHE_CAPACITY
  healthPingCoalescer.clear()
  healthPayloadCoalescer.clear()
  clickhouseBreakers.clear()
}

export const __testInternals = {
  TimedLRUCache,
  CoalescingMap,
  CircuitBreaker,
  sanitizeHealthError,
  ensureHealthCacheCapacity,
  getClickHouseBreaker,
}

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
  ensureHealthCacheCapacity(cfg.healthCacheCapacity)
  const dsn = buildClickHouseDSN(cfg)
  if (!dsn) {
    return { status: 'ok' }
  }
  const cacheKey = dsn
  const breaker = getClickHouseBreaker(cfg, cacheKey)
  const cached = healthPingCache.get(cacheKey)
  if (cached) {
    if (cached.ok) {
      return { status: 'ok' }
    }
    return reply.code(503).send({ status: 'degraded' })
  }
  if (breaker && !breaker.allow()) {
    const now = Date.now()
    healthPingCache.set(cacheKey, { at: now, ok: false }, cfg.healthCacheTtlMs)
    return reply.code(503).send({ status: 'degraded' })
  }
  let failure: unknown
  try {
    await healthPingCoalescer.run(cacheKey, async () => {
      const startedAt = Date.now()
      try {
        const { url, authHeader } = sanitizeDSNForRequest(dsn, cfg)
        const u = new URL(url)
        const q = new URLSearchParams(u.search)
        q.set('query', 'SELECT 1')
        u.search = q.toString()
        await fetchWithTimeout(u, cfg.healthPingTimeoutMs, authHeader)
        breaker?.recordSuccess()
        healthPingCache.set(cacheKey, { at: startedAt, ok: true }, cfg.healthCacheTtlMs)
      } catch (err) {
        breaker?.recordFailure(startedAt)
        healthPingCache.set(cacheKey, { at: startedAt, ok: false }, cfg.healthCacheTtlMs)
        throw err
      }
    })
  } catch (err) {
    failure = err
  }
  const latest = healthPingCache.get(cacheKey)
  if (latest && latest.ok) {
    return { status: 'ok' }
  }
  if (failure) {
    return reply.code(503).send({ status: 'degraded' })
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
  ensureHealthCacheCapacity(cfg.healthCacheCapacity)
  const dsn = buildClickHouseDSN(cfg)
  if (!dsn) {
    const payload: { status: 'ok'; clickhouse: ClickHouseHealth } = {
      status: 'ok',
      clickhouse: { configured: false, ok: false },
    }
    return payload
  }
  const cacheKey = dsn
  const breaker = getClickHouseBreaker(cfg, cacheKey)
  if (breaker && !breaker.allow()) {
    const fallback: CachedHealthPayload = {
      body: {
        status: 'degraded',
        clickhouse: { configured: true, ok: false, error: 'temporarily unavailable' },
      },
      statusCode: 503,
    }
    healthPayloadCache.set(cacheKey, fallback, cfg.healthCacheTtlMs)
    return reply.code(fallback.statusCode).send(fallback.body)
  }
  const cached = healthPayloadCache.get(cacheKey)
  if (cached) {
    return reply.code(cached.statusCode).send(cached.body)
  }
  const result = await healthPayloadCoalescer.run(cacheKey, async () => {
    const startedAt = Date.now()
    const ch: ClickHouseHealth = { configured: true, ok: false }
    let statusCode = 200
    try {
      const { url, authHeader } = sanitizeDSNForRequest(dsn, cfg)
      const u = new URL(url)
      const q = new URLSearchParams(u.search)
      q.set('query', 'SELECT 1')
      u.search = q.toString()
      const res = await fetchWithTimeout(u, cfg.healthPingTimeoutMs, authHeader)
      ch.ok = res.ok
      ch.status = res.status
      if (res.ok) {
        breaker?.recordSuccess()
      } else {
        statusCode = 503
        ch.error = 'unavailable'
        breaker?.recordFailure(startedAt)
      }
    } catch (err) {
      statusCode = 503
      ch.ok = false
      ch.error = sanitizeHealthError(err)
      breaker?.recordFailure(startedAt)
      const logErr = err instanceof Error ? err.message : String(err)
      /* c8 ignore next */
      app.log.warn({ err: logErr, dsn: redactDSN(dsn) }, 'clickhouse healthz error')
    }
    const status: HealthSummaryStatus = ch.ok ? 'ok' : 'degraded'
    const payload: CachedHealthPayload = { body: { status, clickhouse: ch }, statusCode }
    healthPayloadCache.set(cacheKey, payload, cfg.healthCacheTtlMs)
    return payload
  })
  return reply.code(result.statusCode).send(result.body)
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

async function fetchWithTimeout(u: URL, timeoutMs: number, authHeader?: string): Promise<Response> {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  try {
    return await fetch(u, {
      method: 'GET',
      signal: ctrl.signal,
      headers: authHeader ? { Authorization: authHeader } : undefined,
    })
  } finally {
    clearTimeout(timer)
    if (!ctrl.signal.aborted) {
      ctrl.abort()
    }
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
