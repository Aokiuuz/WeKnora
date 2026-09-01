import assert from 'node:assert/strict'
import { Buffer } from 'node:buffer'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { build, type Plugin } from 'esbuild'

interface RequestModule {
  createTokenRefreshCoordinator: () => {
    begin: () => boolean
    wait: () => Promise<string>
    resolve: (token: string) => void
    reject: (error: unknown) => void
    finish: () => void
    isRefreshing: () => boolean
    pendingCount: () => number
  }
  coordinateTokenRefresh: (
    coordinator: ReturnType<RequestModule['createTokenRefreshCoordinator']>,
    refresh: () => Promise<string>,
  ) => Promise<string>
  getDown: (
    url: string,
    config?: { signal?: AbortSignal; timeout?: number },
  ) => Promise<Blob>
}

const virtualDependencies: Plugin = {
  name: 'request-test-dependencies',
  setup(builder) {
    const modules: Record<string, string> = {
      axios: `
        const client = {
          delete: async () => ({}),
          get: async (url, config) => {
            globalThis.__requestGetCalls?.push({ url, config })
            return new Blob(['download'])
          },
          interceptors: {
            request: { use() {} },
            response: { use() {} },
          },
          patch: async () => ({}),
          post: async () => ({}),
          put: async () => ({}),
        }
        export default { create: () => client }
      `,
      '@/i18n': `export default {
        global: { locale: { value: 'en-US' }, t: (key) => key }
      }`,
      './index': `
        export const MAX_FILE_SIZE_MB = 50
        export const generateRandomString = () => 'request-id'
      `,
      './api-base': `export const getApiBaseUrl = () => ''`,
      '../api/auth/index': `export const refreshToken = async () => ({ success: false })`,
    }

    builder.onResolve({ filter: /^(?:axios|@\/i18n|\.\/index|\.\/api-base|\.\.\/api\/auth\/index)$/ }, args => ({
      path: args.path,
      namespace: 'request-test',
    }))
    builder.onLoad({ filter: /.*/, namespace: 'request-test' }, args => ({
      contents: modules[args.path],
      loader: 'js',
    }))
  },
}

async function loadRequestModule(): Promise<RequestModule> {
  const entry = fileURLToPath(new URL('./request.ts', import.meta.url))
  const result = await build({
    entryPoints: [entry],
    bundle: true,
    format: 'esm',
    logLevel: 'silent',
    platform: 'node',
    plugins: [virtualDependencies],
    target: 'node22',
    write: false,
  })
  const source = result.outputFiles[0].text
  const url = `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
  return import(url) as Promise<RequestModule>
}

test('missing refresh token rejects every concurrent waiter and releases the refresh lock', async () => {
  const { createTokenRefreshCoordinator, coordinateTokenRefresh } = await loadRequestModule()
  const coordinator = createTokenRefreshCoordinator()
  const missingToken = { message: 'please re-login' }
  let refreshAttempts = 0

  const refreshWithoutToken = async (): Promise<string> => {
    refreshAttempts += 1
    throw missingToken
  }

  const requests = Array.from({ length: 4 }, () => (
    coordinateTokenRefresh(coordinator, refreshWithoutToken)
  ))
  const settled = await Promise.allSettled(requests)

  assert.equal(refreshAttempts, 1)
  assert.equal(coordinator.pendingCount(), 0)
  assert.equal(coordinator.isRefreshing(), false)
  assert.deepEqual(
    settled.map(result => result.status),
    ['rejected', 'rejected', 'rejected', 'rejected'],
  )
  settled.forEach(result => {
    assert.equal(result.status, 'rejected')
    if (result.status === 'rejected') assert.equal(result.reason, missingToken)
  })

  assert.equal(coordinator.begin(), true, 'the next 401 can start a new refresh cycle')
  coordinator.finish()
})

test('one successful refresh resolves all concurrent waiters with the same token', async () => {
  const { createTokenRefreshCoordinator, coordinateTokenRefresh } = await loadRequestModule()
  const coordinator = createTokenRefreshCoordinator()
  let release!: (token: string) => void
  const token = new Promise<string>(resolve => {
    release = resolve
  })
  let refreshAttempts = 0

  const refresh = () => {
    refreshAttempts += 1
    return token
  }
  const first = coordinateTokenRefresh(coordinator, refresh)
  const second = coordinateTokenRefresh(coordinator, refresh)
  assert.equal(coordinator.pendingCount(), 1)

  release('fresh-token')
  assert.deepEqual(await Promise.all([first, second]), ['fresh-token', 'fresh-token'])
  assert.equal(refreshAttempts, 1)
  assert.equal(coordinator.isRefreshing(), false)
})

test('blob downloads override the client default with caller timeout and AbortSignal', async () => {
  const { getDown } = await loadRequestModule()
  const calls: Array<{
    config: { responseType: string; signal?: AbortSignal; timeout?: number }
    url: string
  }> = []
  ;(globalThis as any).__requestGetCalls = calls
  const controller = new AbortController()

  await getDown('/large-export', { signal: controller.signal, timeout: 600_000 })

  assert.equal(calls.length, 1)
  assert.equal(calls[0].url, '/large-export')
  assert.equal(calls[0].config.responseType, 'blob')
  assert.equal(calls[0].config.timeout, 600_000)
  assert.equal(calls[0].config.signal, controller.signal)
})
