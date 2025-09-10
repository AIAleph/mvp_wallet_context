import { describe, it, expect } from 'vitest'
import { app } from './server'
import { start } from './server'

describe('API server', () => {
  it('GET /health returns ok', async () => {
    const res = await app.inject({ method: 'GET', url: '/health' })
    expect(res.statusCode).toBe(200)
    expect(res.json()).toEqual({ status: 'ok' })
  })

  it('POST /v1/address/:address/sync validates address', async () => {
    const bad = await app.inject({ method: 'POST', url: '/v1/address/nothex/sync' })
    expect(bad.statusCode).toBe(400)

    const addr = '0x' + 'a'.repeat(40)
    const ok = await app.inject({ method: 'POST', url: `/v1/address/${addr}/sync` })
    expect(ok.statusCode).toBe(200)
    expect(ok.json()).toEqual({ accepted: true })
  })

  it('start() listens using PORT env and can close', async () => {
    const prev = process.env.PORT
    process.env.PORT = '0'
    try {
      await start()
      await app.close()
    } finally {
      process.env.PORT = prev
    }
  })
})
