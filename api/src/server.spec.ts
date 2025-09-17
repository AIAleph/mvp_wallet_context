import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { app, start, __resetHealthStateForTests, __testInternals } from './server.js'
import { loadConfig } from './config.js'

describe('API server', () => {
  beforeEach(() => {
    __resetHealthStateForTests()
  })

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
    expect(res.statusCode).toBe(503)
    expect(res.json()).toEqual({ status: 'degraded' })
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
    const err = new Error('connection refused')
    const spy = vi.fn().mockRejectedValue(err)
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(503)
    const body = res.json() as any
    expect(body.status).toBe('degraded')
    expect(body.clickhouse.configured).toBe(true)
    expect(body.clickhouse.ok).toBe(false)
    expect(body.clickhouse.error).toBe('unreachable')
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
    expect(res.statusCode).toBe(503)
    const body = res.json() as any
    expect(body.status).toBe('degraded')
    expect(body.clickhouse.ok).toBe(false)
    expect(body.clickhouse.error).toBe('unavailable')
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
        if (signal?.aborted) {
          const error = new Error('aborted')
          error.name = 'AbortError'
          return reject(error)
        }
        signal?.addEventListener('abort', () => {
          const error = new Error('aborted')
          error.name = 'AbortError'
          reject(error)
        })
      })
    })
    ;(globalThis as any).fetch = spy

    const p = app.inject({ method: 'GET', url: '/healthz' })
    await vi.advanceTimersByTimeAsync(5)
    const res = await p
    expect(res.statusCode).toBe(503)
    const body = res.json() as any
    expect(body.clickhouse.configured).toBe(true)
    expect(body.clickhouse.ok).toBe(false)
    expect(body.clickhouse.error).toBe('timeout')
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
    const orig = globalThis.fetch as any
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy
    process.env.HEALTH_DEBUG = '1'
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'db'
    process.env.HEALTH_RATE_LIMIT_RPS = '0'
    process.env.HEALTH_CACHE_TTL_MS = '10000'
    let res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    expect(spy).toHaveBeenCalledTimes(1)
    res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(200)
    expect(spy).toHaveBeenCalledTimes(1)
    ;(globalThis as any).fetch = orig
  })

  it('healthz returns fallback when circuit is open', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['HEALTH_DEBUG', 'CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CIRCUIT_BREAKER_FAILURES']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.HEALTH_DEBUG = '1'
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '0'
    env.HEALTH_CIRCUIT_BREAKER_FAILURES = '1'
    const err = new Error('refused')
    const spy = vi.fn().mockRejectedValue(err)
    ;(globalThis as any).fetch = spy

    // Prime breaker via /health failure
    await app.inject({ method: 'GET', url: '/health' })
    spy.mockClear()

    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(503)
    const body = res.json() as any
    expect(body.status).toBe('degraded')
    expect(body.clickhouse.error).toBe('temporarily unavailable')
    expect(spy).not.toHaveBeenCalled()

    for (const k of ['HEALTH_DEBUG', 'CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CIRCUIT_BREAKER_FAILURES']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('healthz handles non-200 ClickHouse responses', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['HEALTH_DEBUG', 'CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.HEALTH_DEBUG = '1'
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '0'
    const spy = vi.fn().mockResolvedValue({ ok: false, status: 500 })
    ;(globalThis as any).fetch = spy
    const res = await app.inject({ method: 'GET', url: '/healthz' })
    expect(res.statusCode).toBe(503)
    const body = res.json() as any
    expect(body.status).toBe('degraded')
    expect(body.clickhouse.ok).toBe(false)
    expect(body.clickhouse.error).toBe('unavailable')
    for (const k of ['HEALTH_DEBUG', 'CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
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

  it('GET /health caches successful checks and respects TTL', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CACHE_CAPACITY']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '1000'
    env.HEALTH_CACHE_CAPACITY = '2'
    const spy = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    ;(globalThis as any).fetch = spy

    const first = await app.inject({ method: 'GET', url: '/health' })
    expect(first.statusCode).toBe(200)
    expect(spy).toHaveBeenCalledTimes(1)

    const second = await app.inject({ method: 'GET', url: '/health' })
    expect(second.statusCode).toBe(200)
    expect(spy).toHaveBeenCalledTimes(1)

    vi.useFakeTimers()
    vi.setSystemTime(new Date(Date.now() + 2000))
    const third = await app.inject({ method: 'GET', url: '/health' })
    expect(third.statusCode).toBe(200)
    expect(spy).toHaveBeenCalledTimes(2)
    vi.useRealTimers()

    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CACHE_CAPACITY']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('GET /health caches degraded status during TTL', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '1000'
    const spy = vi.fn().mockRejectedValue(new Error('boom'))
    ;(globalThis as any).fetch = spy

    const first = await app.inject({ method: 'GET', url: '/health' })
    expect(first.statusCode).toBe(503)
    expect(spy).toHaveBeenCalledTimes(1)

    const second = await app.inject({ method: 'GET', url: '/health' })
    expect(second.statusCode).toBe(503)
    expect(spy).toHaveBeenCalledTimes(1)

    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('GET /health opens and recovers circuit breaker', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CIRCUIT_BREAKER_FAILURES', 'HEALTH_CIRCUIT_BREAKER_RESET_MS']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '0'
    env.HEALTH_CIRCUIT_BREAKER_FAILURES = '2'
    env.HEALTH_CIRCUIT_BREAKER_RESET_MS = '1000'
    const spy = vi
      .fn()
      .mockRejectedValueOnce(new Error('refused'))
      .mockRejectedValueOnce(new Error('refused'))
    ;(globalThis as any).fetch = spy
    vi.useFakeTimers()

    const first = await app.inject({ method: 'GET', url: '/health' })
    expect(first.statusCode).toBe(503)
    const second = await app.inject({ method: 'GET', url: '/health' })
    expect(second.statusCode).toBe(503)

    spy.mockClear()
    const third = await app.inject({ method: 'GET', url: '/health' })
    expect(third.statusCode).toBe(503)
    expect(spy).not.toHaveBeenCalled()

    vi.setSystemTime(new Date(Date.now() + 2000))
    spy.mockResolvedValueOnce({ ok: true, status: 200 })
    const fourth = await app.inject({ method: 'GET', url: '/health' })
    expect(fourth.statusCode).toBe(200)
    expect(spy).toHaveBeenCalledTimes(1)

    vi.useRealTimers()
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CIRCUIT_BREAKER_FAILURES', 'HEALTH_CIRCUIT_BREAKER_RESET_MS']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  it('GET /health skips circuit breaker when disabled', async () => {
    const orig = globalThis.fetch as any
    const env = process.env
    const backup: Record<string, string> = {}
    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CIRCUIT_BREAKER_FAILURES']) {
      if (env[k] !== undefined) backup[k] = env[k] as string
    }
    env.CLICKHOUSE_URL = 'http://localhost:8123'
    env.CLICKHOUSE_DB = 'db'
    env.HEALTH_CACHE_TTL_MS = '0'
    env.HEALTH_CIRCUIT_BREAKER_FAILURES = '0'
    const error = new Error('refused')
    const spy = vi.fn().mockRejectedValue(error)
    ;(globalThis as any).fetch = spy

    const first = await app.inject({ method: 'GET', url: '/health' })
    expect(first.statusCode).toBe(503)
    const second = await app.inject({ method: 'GET', url: '/health' })
    expect(second.statusCode).toBe(503)
    expect(spy).toHaveBeenCalledTimes(2)

    for (const k of ['CLICKHOUSE_URL', 'CLICKHOUSE_DB', 'HEALTH_CACHE_TTL_MS', 'HEALTH_CIRCUIT_BREAKER_FAILURES']) {
      if (backup[k] !== undefined) env[k] = backup[k]
      else delete env[k]
    }
    ;(globalThis as any).fetch = orig
  })

  describe('internal helpers', () => {
    it('TimedLRUCache supports eviction and TTL', () => {
      const { TimedLRUCache } = __testInternals as any
      expect(() => new TimedLRUCache<string, number>(0)).toThrow()
      const cache = new TimedLRUCache<string, number>(2)
      cache.set('a', 1, 1000, 0)
      expect(cache.get('a', 0)).toBe(1)
      cache.set('b', 2, 1000, 0)
      cache.set('c', 3, 1000, 0)
      expect(cache.get('a', 0)).toBeUndefined()
      expect(cache.get('b', 0)).toBe(2)
      expect(cache.get('c', 0)).toBe(3)
      cache.set('c', 5, 1000, 0)
      expect(cache.get('c', 0)).toBe(5)
      cache.set('b', 2, 0, 0)
      expect(cache.get('b', 0)).toBeUndefined()
      cache.set('d', 4, 1000, 0)
      expect(cache.get('d', 2001)).toBeUndefined()
      cache.prune(2001)
      expect(() => cache.resize(0)).toThrow()
    })

    it('CoalescingMap coalesces concurrent factories', async () => {
      const { CoalescingMap } = __testInternals as any
      const map = new CoalescingMap<string, number>()
      const loader = vi.fn(async () => 42)
      const results = await Promise.all([map.run('x', loader), map.run('x', loader)])
      expect(results).toEqual([42, 42])
      expect(loader).toHaveBeenCalledTimes(1)
      map.clear()
      await map.run('x', loader)
      expect(loader).toHaveBeenCalledTimes(2)
    })

    it('metrics helpers escape labels and accumulate', () => {
      const { inc, labelKey, recordMetrics } = __testInternals as any
      const metrics = new Map<string, number>()
      inc(metrics, 'route:/health', 1)
      inc(metrics, 'route:/health', 2)
      expect(metrics.get('route:/health')).toBe(3)
      const escaped = labelKey({ path: '"/\\test"', status: '200' })
      expect(escaped).toContain('\"')
      expect(escaped).toContain('\\\\')
      expect(recordMetrics({ url: '/metrics' }, { statusCode: 200 })).toBeUndefined()
      const key = recordMetrics({ method: 'POST', routeOptions: { url: '/foo' } }, { statusCode: 201 })
      expect(typeof key).toBe('string')
      const fallbackKey = recordMetrics({ method: 'PUT', url: undefined }, { statusCode: 204 })
      expect(fallbackKey).toContain('route=""')
      const defaultMethodKey = recordMetrics({ url: '/default' }, { statusCode: 200 })
      expect(defaultMethodKey).toContain('method="GET"')
    })

    it('CircuitBreaker transitions across states', () => {
      const { CircuitBreaker } = __testInternals as any
      const breaker = new CircuitBreaker(2, 1000)
      expect(breaker.allow(0)).toBe(true)
      breaker.recordFailure(0)
      expect(breaker.allow(1)).toBe(true)
      breaker.recordFailure(1)
      expect(breaker.allow(2)).toBe(false)
      breaker.recordFailure(3)
      expect(breaker.allow(500)).toBe(false)
      expect(breaker.allow(1000 + 1)).toBe(true)
      breaker.recordFailure(1001)
      expect(breaker.allow(1002)).toBe(false)
      breaker.recordSuccess()
      expect(breaker.allow(1003)).toBe(true)

      const disabled = new CircuitBreaker(0, 0)
      expect(disabled.allow()).toBe(true)
      disabled.recordFailure()
      disabled.recordSuccess()
    })

    it('sanitizeHealthError maps patterns', () => {
      const { sanitizeHealthError } = __testInternals as any
      expect(sanitizeHealthError({ name: 'AbortError' })).toBe('timeout')
      const timeoutErr = new Error('request timed out')
      expect(sanitizeHealthError(timeoutErr)).toBe('timeout')
      const unreachableErr = new Error('dns lookup failed')
      expect(sanitizeHealthError(unreachableErr)).toBe('unreachable')
      expect(sanitizeHealthError('other')).toBe('unavailable')
    })

    it('ensureHealthCacheCapacity only rebuilds when needed', () => {
      const { ensureHealthCacheCapacity } = __testInternals as any
      ensureHealthCacheCapacity(8)
      ensureHealthCacheCapacity(3)
      ensureHealthCacheCapacity(3)
      ensureHealthCacheCapacity(0)
      ensureHealthCacheCapacity(Number.NaN)
      __resetHealthStateForTests()
    })

    it('BreakerRegistry enforces capacity and resizing', () => {
      const { BreakerRegistry, ensureBreakerCapacity, getClickHouseBreaker } = __testInternals as any
      expect(() => new BreakerRegistry(0)).toThrow()
      const registry = new BreakerRegistry(2)
      const a1 = registry.get('a', 1, 1000)
      expect(registry.get('a', 1, 1000)).toBe(a1)
      registry.get('b', 1, 1000)
      registry.get('c', 1, 1000) // evicts 'a'
      const a2 = registry.get('a', 1, 1000)
      expect(a2).not.toBe(a1)
      registry.resize(3)
      const d = registry.get('d', 1, 1000)
      expect(d).toBeInstanceOf(Object)
      registry.resize(1)
      expect(() => registry.resize(0)).toThrow()
      registry.get('e', 1, 1000)
      const base = loadConfig()
      const cfg = {
        ...base,
        healthCircuitBreaker: { failureThreshold: 1, resetMs: 1000, maxEntries: 1 },
      }
      ensureBreakerCapacity(1)
      ensureBreakerCapacity(Number.NaN)
      const breaker1 = getClickHouseBreaker(cfg, 'x')
      const breaker2 = getClickHouseBreaker(cfg, 'y')
      expect(breaker2).not.toBe(breaker1)
      __resetHealthStateForTests()
    })

    it('getClickHouseBreaker reuses existing breakers', () => {
      const { getClickHouseBreaker } = __testInternals as any
      const base = loadConfig()
      const cfg = {
        ...base,
        healthCircuitBreaker: { failureThreshold: 1, resetMs: 1000, maxEntries: 4 },
      }
      const breaker1 = getClickHouseBreaker(cfg, 'dsn')
      const breaker2 = getClickHouseBreaker(cfg, 'dsn')
      expect(breaker1).toBe(breaker2)
      const cfg2 = {
        ...cfg,
        healthCircuitBreaker: { failureThreshold: 2, resetMs: 2000, maxEntries: 4 },
      }
      const breaker3 = getClickHouseBreaker(cfg2, 'dsn')
      expect(breaker3).not.toBe(breaker1)
      const defaultKeyBreaker = getClickHouseBreaker(cfg2, '')
      expect(defaultKeyBreaker).toBeDefined()
      __resetHealthStateForTests()
    })

    it('buildClickHouseDSN and sanitizeDSNForRequest handle credentials', () => {
      const { buildClickHouseDSN, sanitizeDSNForRequest, redactDSN } = __testInternals as any
      const base = loadConfig()
      const cfg = {
        ...base,
        clickhouse: { url: 'http://user:pass@localhost:8123', db: 'wallets', user: 'alice', pass: 'secret', dsn: '' },
      }
      const dsn = buildClickHouseDSN(cfg)
      expect(dsn.endsWith('/wallets')).toBe(true)
      const cfgWithDb = {
        ...cfg,
        clickhouse: { url: 'http://localhost:8123/wallets', db: 'wallets', user: '', pass: '', dsn: '' },
      }
      expect(buildClickHouseDSN(cfgWithDb)).toBe('http://localhost:8123/wallets')
      const sanitized = sanitizeDSNForRequest('http://bob:pw@localhost:8123/db', cfg)
      expect(sanitized.url).not.toContain('bob:pw')
      expect(sanitized.authHeader).toMatch(/^Basic /)
      const fallback = sanitizeDSNForRequest('http://localhost:8123/db', cfg)
      expect(fallback.authHeader).toMatch(/^Basic /)
      const plain = sanitizeDSNForRequest('http://localhost:8123/db', cfgWithDb)
      expect(plain.authHeader).toBeUndefined()
      const missingUser = sanitizeDSNForRequest('http://:pw@localhost:8123/db', cfgWithDb)
      expect(missingUser.url).toBe('http://localhost:8123/db')
      const invalid = sanitizeDSNForRequest('::::', cfg)
      expect(invalid.url).toBe('::::')
      expect(invalid.authHeader).toBeUndefined()
      expect(redactDSN('http://bob:pw@localhost/db')).toBe('http://bob:***@localhost/db')
      expect(redactDSN('http://:pw@localhost/db')).toBe('http://***:***@localhost/db')
      expect(redactDSN('foo//bob:pw@host')).toBe('foo//bob:***@host')
      expect(redactDSN('foo//:pw@host')).toBe('foo//***:***@host')
      expect(redactDSN('http://localhost/db')).toBe('http://localhost/db')
      expect(redactDSN('plaintext')).toBe('plaintext')
      expect(redactDSN('')).toBe('')
    })

    it('fetchWithTimeout wraps fetch with AbortSignal timeout', async () => {
      const { fetchWithTimeout } = __testInternals as any
      const origFetch = globalThis.fetch
      const spy = vi.fn(async (_input: any, init?: RequestInit) => {
        expect(init?.signal).toBeInstanceOf(AbortSignal)
        expect(init?.headers).toEqual({ Authorization: 'Basic abc' })
        return { ok: true, status: 200 } as any
      })
      ;(globalThis as any).fetch = spy
      try {
        const res = await fetchWithTimeout(new URL('http://localhost/ping'), 50, 'Basic abc')
        expect(spy).toHaveBeenCalledTimes(1)
        expect(res.ok).toBe(true)
      } finally {
        ;(globalThis as any).fetch = origFetch
      }
    })
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

  it('GET /metrics handles empty counters', async () => {
    __resetHealthStateForTests()
    const empty = await app.inject({ method: 'GET', url: '/metrics' })
    expect(empty.statusCode).toBe(200)
    expect(empty.body).toContain('# TYPE http_requests_total counter')
    await app.inject({ method: 'GET', url: '/health' })
    const populated = await app.inject({ method: 'GET', url: '/metrics' })
    expect(populated.statusCode).toBe(200)
    expect(populated.body).toMatch(/http_requests_total\{.*route="\/health".*\} \d+/)
  })

  it('start() readies the app and can close', async () => {
    await start()
    await app.close()
  })
})
