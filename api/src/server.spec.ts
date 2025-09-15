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

  it('GET /health uses credentials when provided', async () => {
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

  it('POST /v1/address/:address/sync validates address', async () => {
    const bad = await app.inject({ method: 'POST', url: '/v1/address/nothex/sync' })
    expect(bad.statusCode).toBe(400)

    const addr = '0x' + 'a'.repeat(40)
    const ok = await app.inject({ method: 'POST', url: `/v1/address/${addr}/sync` })
    expect(ok.statusCode).toBe(200)
    expect(ok.json()).toEqual({ accepted: true })
  })

  it('start() readies the app and can close', async () => {
    await start()
    await app.close()
  })
})
