export const config = {
  API_BASE_URL: import.meta.env.VITE_API_BASE_URL || '/api',
  WS_URL: import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws',
  DEBUG: import.meta.env.VITE_DEBUG === 'true' || import.meta.env.DEV,
  BUILD_INFO: {
    version: __APP_VERSION__,
    buildTime: __BUILD_TIME__,
    commitHash: __COMMIT_HASH__,
  },
  PING_INTERVAL_MS: 5000,
  CLIENT_VERSION: '0.1.0',
} as const

declare global {
  const __APP_VERSION__: string
  const __BUILD_TIME__: string
  const __COMMIT_HASH__: string
}

export type Config = typeof config
