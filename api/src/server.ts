import Fastify from 'fastify'
import { z } from 'zod'
import { fileURLToPath } from 'url'

// Minimal Fastify server scaffold. Final API will expose endpoints for sync,
// summary, lists, and semantic search.
export const app = Fastify({ logger: true })

app.get('/health', async () => ({ status: 'ok' }))

app.post('/v1/address/:address/sync', async (req, reply) => {
  const schema = z.object({ address: z.string().regex(/^0x[a-fA-F0-9]{40}$/) })
  const params = schema.safeParse((req as any).params)
  if (!params.success) return reply.status(400).send({ error: 'invalid address' })
  // TODO: enqueue backfill/delta job
  return { accepted: true }
})

export async function start() {
  const port = Number(process.env.PORT || 3000)
  await app.listen({ port, host: '0.0.0.0' })
}

const isMain = process.argv[1] === fileURLToPath(import.meta.url)
/* c8 ignore next 5 */
if (isMain) {
  start().catch((err) => {
    app.log.error(err)
    process.exit(1)
  })
}
