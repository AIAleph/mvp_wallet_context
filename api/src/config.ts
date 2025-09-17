import { z } from 'zod'

const intFromEnv = (key: string, def: number): number => {
  const v = process.env[key]
  if (!v) return def
  const n = Number(v)
  return Number.isFinite(n) ? n : def
}

const clamp = (value: number, min: number, max: number): number => {
  if (value < min) return min
  if (value > max) return max
  return value
}

const RATE_LIMIT_MAX = 200
const HEALTH_PING_TIMEOUT_RANGE = { min: 100, max: 60000 }
const HEALTH_CACHE_TTL_RANGE = { min: 0, max: 300000 }
const HEALTH_RATE_LIMIT_MAX = 50
const HEALTH_CACHE_CAPACITY_RANGE = { min: 1, max: 256 }
const HEALTH_BREAKER_CAPACITY_RANGE = { min: 1, max: 256 }
const HEALTH_BREAKER_CAPACITY_DEFAULT = 16
const HEALTH_BREAKER_FAILURE_MAX = 10
const HEALTH_BREAKER_RESET_RANGE = { min: 0, max: 300000 }

const strFromEnv = (key: string, def = ''): string => process.env[key] ?? def

const boolFromEnv = (key: string, def = false): boolean => {
  const v = process.env[key]
  if (!v) return def
  const s = String(v).toLowerCase()
  return s === '1' || s === 'true' || s === 'yes' || s === 'on'
}

// Validate env shape (strings present or undefined)
const envSchema = z.object({
  PORT: z.string().optional(),
  ETH_PROVIDER_URL: z.string().optional(),
  CLICKHOUSE_URL: z.string().optional(),
  CLICKHOUSE_DB: z.string().optional(),
  CLICKHOUSE_USER: z.string().optional(),
  CLICKHOUSE_PASS: z.string().optional(),
  CLICKHOUSE_DSN: z.string().optional(),
  RATE_LIMIT: z.string().optional(),
  REDIS_URL: z.string().optional(),
  EMBEDDING_MODEL: z.string().optional(),
  HEALTH_DEBUG: z.string().optional(),
  HEALTH_PING_TIMEOUT_MS: z.string().optional(),
  HEALTH_CACHE_TTL_MS: z.string().optional(),
  HEALTH_RATE_LIMIT_RPS: z.string().optional(),
  HEALTH_CACHE_CAPACITY: z.string().optional(),
  HEALTH_CIRCUIT_BREAKER_FAILURES: z.string().optional(),
  HEALTH_CIRCUIT_BREAKER_RESET_MS: z.string().optional(),
  HEALTH_BREAKER_CAPACITY: z.string().optional(),
})

export function loadConfig() {
  envSchema.parse(process.env)
  const port = intFromEnv('PORT', 3000)
  const rateLimit = clamp(intFromEnv('RATE_LIMIT', 0), 0, RATE_LIMIT_MAX)
  return {
    port,
    ethProviderUrl: strFromEnv('ETH_PROVIDER_URL', ''),
    clickhouse: {
      url: strFromEnv('CLICKHOUSE_URL', ''),
      db: strFromEnv('CLICKHOUSE_DB', ''),
      user: strFromEnv('CLICKHOUSE_USER', ''),
      pass: strFromEnv('CLICKHOUSE_PASS', ''), // do not log
      dsn: strFromEnv('CLICKHOUSE_DSN', ''), // preferred if set
    },
    rateLimit,
    redisUrl: strFromEnv('REDIS_URL', ''),
    embeddingModel: strFromEnv('EMBEDDING_MODEL', ''),
    healthDebug: boolFromEnv('HEALTH_DEBUG', false),
    // Slightly higher default for remote CH instances
    healthPingTimeoutMs: clamp(intFromEnv('HEALTH_PING_TIMEOUT_MS', 3000), HEALTH_PING_TIMEOUT_RANGE.min, HEALTH_PING_TIMEOUT_RANGE.max),
    healthCacheTtlMs: clamp(intFromEnv('HEALTH_CACHE_TTL_MS', 5000), HEALTH_CACHE_TTL_RANGE.min, HEALTH_CACHE_TTL_RANGE.max),
    healthRateLimitRps: clamp(intFromEnv('HEALTH_RATE_LIMIT_RPS', 0), 0, HEALTH_RATE_LIMIT_MAX),
    healthCacheCapacity: clamp(intFromEnv('HEALTH_CACHE_CAPACITY', 8), HEALTH_CACHE_CAPACITY_RANGE.min, HEALTH_CACHE_CAPACITY_RANGE.max),
    healthCircuitBreaker: {
      failureThreshold: clamp(intFromEnv('HEALTH_CIRCUIT_BREAKER_FAILURES', 3), 0, HEALTH_BREAKER_FAILURE_MAX),
      resetMs: clamp(intFromEnv('HEALTH_CIRCUIT_BREAKER_RESET_MS', 30000), HEALTH_BREAKER_RESET_RANGE.min, HEALTH_BREAKER_RESET_RANGE.max),
      maxEntries: clamp(intFromEnv('HEALTH_BREAKER_CAPACITY', HEALTH_BREAKER_CAPACITY_DEFAULT), HEALTH_BREAKER_CAPACITY_RANGE.min, HEALTH_BREAKER_CAPACITY_RANGE.max),
    },
  }
}

export type AppConfig = ReturnType<typeof loadConfig>
