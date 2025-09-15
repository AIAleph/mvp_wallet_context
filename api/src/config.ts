import { z } from 'zod'

const intFromEnv = (key: string, def: number): number => {
  const v = process.env[key]
  if (!v) return def
  const n = Number(v)
  return Number.isFinite(n) ? n : def
}

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
})

export function loadConfig() {
  envSchema.parse(process.env)
  const port = intFromEnv('PORT', 3000)
  const rateLimit = intFromEnv('RATE_LIMIT', 0)
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
    healthPingTimeoutMs: intFromEnv('HEALTH_PING_TIMEOUT_MS', 1000),
  }
}

export type AppConfig = ReturnType<typeof loadConfig>
