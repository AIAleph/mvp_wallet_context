import { describe, it, expect, beforeEach, afterEach } from 'vitest'

import { loadConfig } from './config.js'

const savedEnv = { ...process.env }

describe('config env parsing', () => {
  beforeEach(() => {
    process.env = { ...savedEnv }
  })
  afterEach(() => {
    process.env = { ...savedEnv }
  })

  it('applies defaults when env is missing', async () => {
    delete process.env.PORT
    delete process.env.RATE_LIMIT
    const cfg = loadConfig()
    expect(cfg.port).toBe(3000)
    expect(cfg.rateLimit).toBe(0)
  })

  it('parses numeric env vars', async () => {
    process.env.PORT = '4321'
    process.env.RATE_LIMIT = '25'
    const cfg = loadConfig()
    expect(cfg.port).toBe(4321)
    expect(cfg.rateLimit).toBe(25)
  })

  it('falls back on invalid numeric env vars', async () => {
    process.env.PORT = 'not-a-number'
    process.env.RATE_LIMIT = 'NaN'
    const cfg = loadConfig()
    expect(cfg.port).toBe(3000)
    expect(cfg.rateLimit).toBe(0)
  })

  it('exposes clickhouse and optional vars', async () => {
    process.env.CLICKHOUSE_URL = 'http://localhost:8123'
    process.env.CLICKHOUSE_DB = 'test'
    process.env.CLICKHOUSE_USER = 'u'
    process.env.CLICKHOUSE_PASS = 'p'
    process.env.REDIS_URL = 'redis://localhost:6379'
    process.env.EMBEDDING_MODEL = 'text-embedding-3-small'
    const cfg = loadConfig()
    expect(cfg.clickhouse.url).toContain('localhost')
    expect(cfg.clickhouse.db).toBe('test')
    expect(cfg.clickhouse.user).toBe('u')
    expect(cfg.clickhouse.pass).toBe('p')
    expect(cfg.redisUrl).toContain('redis://')
    expect(cfg.embeddingModel).toBe('text-embedding-3-small')
  })

  it('parses HEALTH_DEBUG boolean variations', async () => {
    process.env.HEALTH_DEBUG = 'on'
    let cfg = loadConfig()
    expect(cfg.healthDebug).toBe(true)
    process.env.HEALTH_DEBUG = 'off'
    cfg = loadConfig()
    expect(cfg.healthDebug).toBe(false)
    delete process.env.HEALTH_DEBUG
    cfg = loadConfig()
    expect(cfg.healthDebug).toBe(false)
  })

  it('clamps health config extremes', async () => {
    process.env.HEALTH_CACHE_CAPACITY = '9999'
    process.env.HEALTH_RATE_LIMIT_RPS = '500'
    process.env.HEALTH_PING_TIMEOUT_MS = '999999'
    process.env.HEALTH_CACHE_TTL_MS = '9999999'
    process.env.HEALTH_CIRCUIT_BREAKER_FAILURES = '50'
    process.env.HEALTH_CIRCUIT_BREAKER_RESET_MS = '9999999'
    process.env.HEALTH_BREAKER_CAPACITY = '999'
    let cfg = loadConfig()
    expect(cfg.healthCacheCapacity).toBe(256)
    expect(cfg.healthRateLimitRps).toBe(50)
    expect(cfg.healthPingTimeoutMs).toBe(60000)
    expect(cfg.healthCacheTtlMs).toBe(300000)
    expect(cfg.healthCircuitBreaker.failureThreshold).toBe(10)
    expect(cfg.healthCircuitBreaker.resetMs).toBe(300000)
    expect(cfg.healthCircuitBreaker.maxEntries).toBe(256)

    process.env.HEALTH_CACHE_CAPACITY = '0'
    process.env.HEALTH_RATE_LIMIT_RPS = '-1'
    process.env.HEALTH_PING_TIMEOUT_MS = '-5'
    process.env.HEALTH_CACHE_TTL_MS = '-50'
    process.env.HEALTH_CIRCUIT_BREAKER_FAILURES = '-5'
    process.env.HEALTH_CIRCUIT_BREAKER_RESET_MS = '-10'
    process.env.HEALTH_BREAKER_CAPACITY = '-3'
    cfg = loadConfig()
    expect(cfg.healthCacheCapacity).toBe(1)
    expect(cfg.healthRateLimitRps).toBe(0)
    expect(cfg.healthPingTimeoutMs).toBe(100)
    expect(cfg.healthCacheTtlMs).toBe(0)
    expect(cfg.healthCircuitBreaker.failureThreshold).toBe(0)
    expect(cfg.healthCircuitBreaker.resetMs).toBe(0)
    expect(cfg.healthCircuitBreaker.maxEntries).toBe(1)
  })
})
