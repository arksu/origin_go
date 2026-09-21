// Chat message timing constants
export const CHAT_MESSAGE_LIFETIME_MS = 25000  // 5 seconds full visibility
export const CHAT_FADEOUT_DURATION_MS = 1000  // 1 second fade out
export const CHAT_CLEANUP_INTERVAL_MS = 500   // Check every 500ms
export const CHAT_MAX_MESSAGES = 50           // Prevent memory leaks

// Chat balloons over world objects (local chat speech bubbles)
export const CHAT_BALLOON_MAX_CHARS = 20      // Text longer than this is truncated with "..."
export const CHAT_BALLOON_LIFETIME_MS = 5000  // Balloon visible duration
export const CHAT_BALLOON_FADEOUT_MS = 500    // Fade out at the end of the lifetime
