/**
 * Chunk cache configuration constants
 */

// LRU Cache limits
export const CACHE_MAX_ENTRIES = 64
export const CACHE_TTL_MS = 180_000 // 4 minutes

// TTL sweep interval
export const CACHE_SWEEP_INTERVAL_MS = 10_000 // 10 seconds

// Build queue configuration
export const BUILD_TIME_BUDGET_MS = 2 // ms per frame for chunk building
export const BUILD_QUEUE_MAX_LENGTH = 64
export const MAX_IN_FLIGHT_BUILDS = 2
export const MAX_IN_FLIGHT_GPU_UPLOADS = 2

// Priority levels
export const enum BuildPriority {
  P0_VISIBLE = 0,    // Currently visible chunks - build immediately
  P1_NEARBY = 1,     // Adjacent chunks - build in background
  P2_DISTANT = 2,    // Distant chunks in 3x3 zone - build on idle
}

// Border refresh configuration
export const BORDER_REFRESH_DELAY_MS = 50 // Debounce time for border refresh tasks
