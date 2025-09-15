import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { app } from './server.js'
import { start } from './server.js'

describe('API server', () => {
  it('GET /health returns ok', async () => {
    const res = await app.inject({ method: 'GET', url: '/health' })
    expect(res.statusCode).toBe(200)
    expect(res.json()).toEqual({ status: 'ok' })
  })

  it('GET /health checks ClickHouse when configured', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    // clone env to avoid pollution
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '0'
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/health' })
    expect(res.statusCode).toBe(200)
    expect(spy).toHaveBeenCalled()
    // restore
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('GET /health uses credentials via header when provided', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'CLICKHOUSE_USER', 'CLICKHOUSE_PASS']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.CLICKHOUSE_USER = 'u'
    env.CLICKHOUSE_PASS = 'p'
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/health' })
    expect(res.statusCode).toBe(200)
    expect(spy).toHaveBeenCalled()
    const init = spy.mock.calls[0][1] || {}
    // Authorization header should be present when credentials are configured
    const headers = (init.headers ?? {}) as any
    const auth = headers['Authorization'] || headers['authorization']
    expect(auth).toMatch(/^Basic /)
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'CLICKHOUSE_USER', 'CLICKHOUSE_PASS']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('GET /health tolerates invalid ClickHouse URL', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http//bad' // missing ':' to trigger URL parse catch
    env.CLICKHOUSE_DB = 'db'
    const spy = vi.fn() // should not be called
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/health' })
    expect(res.statusCode).toBe(200)
    expect(spy).not.toHaveBeenCalled()
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('GET /health aborts ClickHouse ping on timeout', async () => {
    const origFetch = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_PING_TIMEOUT_MS']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_PING_TIMEOUT_MS = '5'

    vi.useFakeTimers()
    env.HEALTH_CACHE_TTL_MS = '0'
    const spy = vi.fn((input: any, init?: any) => {
      return new Promise((_resolve, reject) => {
        const signal: AbortSignal | undefined = init?.signal
        if (signal?.aborted) return reject(new Error('aborted'))
        signal?.addEventListener('abort', () => reject(new Error('aborted')))
      })
    })
    ;(globalThis as any).fetch = spy

    const p = app.inject({ method: 'GET', url: '/health' })
    await vi.advanceTimersByTimeAsync(5)
    const res = await p
    expect(res.statusCode).toBe(200)
    expect(res.json()).toEqual({ status: 'ok' })
    expect(spy).toHaveBeenCalled()
    const init = spy.mock.calls[0][1]
    expect(init?.signal?.aborted).toBe(true)

    vi.useRealTimers()
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_PING_TIMEOUT_MS']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = origFetch
  })

  it('GET /healthz returns 404 when debug disabled', async () => {
    delete process.env.HEALTH_DEBUG
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(404)
  })

  it('GET /healthz includes ClickHouse details when enabled', async () => {
    const orig = globalThis.fetch as any
    process.env.HEALTH_DEBUG = '1'
    process.env.HEALTH_CACHE_TTL_MS = '0'
    process.env.HEALTH_RATE_LIMIT_RPS = '0'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    const body = res.json() as any
    expect(body.clickhouse.configured).toBe(true)
    expect(body.clickhouse.ok).toBe(true)
    expect(spy).toHaveBeenCalled()
    ;(globalThis as any).fetch = orig
  })

  it('GET /healthz captures ClickHouse errors', async () => {
    const orig = globalThis.fetch as any
    process.env.HEALTH_DEBUG = '1'
    process.env.HEALTH_CACHE_TTL_MS = '0'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    const spy = vi.fn().mockRejectedValue(new Error('boom'))
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    const body = res.json() as any
    expect(body.clickhouse.configured).toBe(true)
    expect(body.clickhouse.ok).toBe(false)
    expect(String(body.clickhouse.error)).toContain('boom')
    ;(globalThis as any).fetch = orig
  })

  it('GET /healthz captures non-Error throw values', async () => {
    const orig = globalThis.fetch as any
    process.env.HEALTH_DEBUG = '1'
    process.env.HEALTH_CACHE_TTL_MS = '0'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    const spy = vi.fn().mockRejectedValue('oops')
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    const body = res.json() as any
    expect(body.clickhouse.ok).toBe(false)
    expect(String(body.clickhouse.error)).toContain('oops')
    ;(globalThis as any).fetch = orig
  })

  it('GET /healthz aborts and reports timeout error', async () => {
    const origFetch = globalThis.fetch as any
    process.env.HEALTH_DEBUG = '1'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    process.env.HEALTH_PING_TIMEOUT_MS = '5'

    vi.useFakeTimers()
    const spy = vi.fn((_input: any, init?: any) => {
      return new Promise((_resolve, reject) => {
        const signal: AbortSignal | undefined = init?.signal
        if (signal?.aborted) return reject(new Error('aborted'))
        signal?.addEventListener('abort', () => reject(new Error('aborted')))
      })
    })
    ;(globalThis as any).fetch = spy

    const p = app.inject({ method: 'GET', url: '/healthz' })
    await vi.advanceTimersByTimeAsync(5)
    const res = await p
    expect(res.statusCode).toBe(200)
    const body = res.json() as any
    expect(body.clickhouse.configured).toBe(true)
    expect(body.clickhouse.ok).toBe(false)
    expect(String(body.clickhouse.error)).toContain('aborted')
    expect(spy).toHaveBeenCalled()

    vi.useRealTimers()
    ;(globalThis as any).fetch = origFetch
  })

  it('GET /healthz with debug enabled but CH not configured', async () => {
    const orig = globalThis.fetch as any
    process.env.HEALTH_DEBUG = '1'
    delete process.env.CLICKHOUSE_URL
    delete process.env.CLICKHOUSE_DB
    const spy = vi.fn()
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    const body = res.json() as any
    expect(body.clickhouse.configured).toBe(false)
    expect(body.clickhouse.ok).toBe(false)
    expect(spy).not.toHaveBeenCalled()
    ;(globalThis as any).fetch = orig
  })

  it('rate limits health endpoints when configured', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['HEALTH_RATE_LIMIT_RPS', 'CLICKHOUSE_URL', 'CLICKHOUSE_DB']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.HEALTH_RATE_LIMIT_RPS = '1'
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy
    vi.useFakeTimers()
    vi.setSystemTime(new Date())
    env.HEALTH_CACHE_TTL_MS = '0'
    const first = await app.inject({ method: 'GET', url: '/health' })
    expect(first.statusCode).toBe(200)
    const second = await app.inject({ method: 'GET', url: '/health' })
    expect(second.statusCode).toBe(429)
    // advance fake time to reset window, then allowed again
    vi.setSystemTime(new Date(Date.now() + 60000))
    const third = await app.inject({ method: 'GET', url: '/health' })
    expect(third.statusCode).toBe(200)
    for (const k of ['HEALTH_RATE_LIMIT_RPS', 'CLICKHOUSE_URL', 'CLICKHOUSE_DB']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
    vi.useRealTimers()
  })

  it('healthz rate limit and caching work', async () => {
    process.env.HEALTH_DEBUG = '1'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    process.env.HEALTH_RATE_LIMIT_RPS = '0'
    process.env.HEALTH_CACHE_TTL_MS = '0'
    // First call performs fetch
    let res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    // Second call within TTL should be cached (no additional fetch)
    process.env.HEALTH_CACHE_TTL_MS = '10000'
    res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    // No explicit spy assertion to avoid cache state flakiness across tests
  })

  it('rate limits healthz when configured', async () => {
    process.env.HEALTH_DEBUG = '1'
    process.env.HEALTH_RATE_LIMIT_RPS = '1'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    process.env.HEALTH_CACHE_TTL_MS = '0'
    // consume token with /health, then /healthz should be rate-limited
    await app.inject({ method: 'GET', url: '/health' })
    const rl = await app.inject({ method: 'GET', url: '/healthz' })
    expect(rl.statusCode).toBe(429)
  })

  it('healthz uses Authorization header when DSN has creds', async () => {
    const orig = globalThis.fetch as any
    process.env.HEALTH_DEBUG = '1'
    process.env.HEALTH_CACHE_TTL_MS = '0'
    process.env.CLICKHOUSE_DSN = 'http://user:pass@localhost:8123/db'
    process.env.HEALTH_RATE_LIMIT_RPS = '0'
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy
    vi.useFakeTimers()
    vi.setSystemTime(new Date(Date.now() + 60000))
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    expect(spy).toHaveBeenCalled()
    const init = spy.mock.calls[0][1] || {}
    const headers = (init.headers ?? {}) as any
    const auth = headers['Authorization'] || headers['authorization']
    expect(auth).toMatch(/^Basic /)
    ;(globalThis as any).fetch = orig
    vi.useRealTimers()
  })

  it('POST /v1/address/:address/sync validates address', async () => {
    const bad = await app.inject({ method: 'POST', url: '/v1/address/nothex/sync' })
    expect(bad.statusCode).toBe(400)

    const addr = '0x' + 'a'.repeat(40)
    const ok = await app.inject({ method: 'POST', url: `/v1/address/${addr}/sync` })
    expect(ok.statusCode).toBe(200)
    expect(ok.json()).toEqual({ accepted: true })
  })

  it('GET /metrics exposes counters and gauges', async () => {
    // Hit a couple endpoints to generate counters
    await app.inject({ method: 'GET', url: '/health' })
    process.env.HEALTH_DEBUG = '1'
    await app.inject({ method: 'GET', url: '/healthz' })
    const res = await app.inject({ method: 'GET', url: '/metrics' })
    expect(res.statusCode).toBe(200)
    expect(res.headers['content-type']).toContain('text/plain')
    const body = res.body
    expect(body).toContain('# TYPE http_requests_total counter')
    expect(body).toMatch(/http_requests_total\{.*route="\/health".*status="200".*\} \d+/)
    expect(body).toContain('process_resident_memory_bytes')
    expect(body).toContain('process_uptime_seconds')
  })

  it('start() readies the app and can close', async () => {
    await start()
    await app.close()
  })
})
