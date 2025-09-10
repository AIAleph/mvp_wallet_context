import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    pool: 'forks',
    maxWorkers: 1,
    minWorkers: 1,
    coverage: {
      provider: 'v8',
      reportsDirectory: './coverage',
      reporter: ['text', 'lcov'],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 70,
        statements: 80,
      },
    },
  },
})
