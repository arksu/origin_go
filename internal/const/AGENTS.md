# Const Package Notes

`internal/const` stores stable cross-package constants and global-var keys.

## Global Var Keys

- `SERVER_START_TIME` — persisted bootstrap server start time (Unix seconds).
- `SERVER_TICK_RATE` — persisted bootstrap tick rate.
- `LAST_USED_ID` — entity id allocator state.

## Rules

- Treat persisted key names as schema contract; avoid renames.
- Add new keys only when they are globally relevant and cross-cutting.
- Keep gameplay tuning constants here only if they are true shared constants (not runtime config).
