# Timeutil Package Notes

`internal/timeutil` owns game clock primitives and server time bootstrap logic.

## Server Bootstrap Contract

- Bootstrap source of truth is pair:
  - `SERVER_START_TIME` (Unix seconds),
  - `SERVER_TICK_RATE`.
- On first boot, persist both values in a single transaction.
- On next boots, require both values to exist and be valid; fail fast on mismatch/incomplete pair.
- Effective game tick is computed from elapsed time since `SERVER_START_TIME` using persisted/configured tick rate.

## Runtime Rules

- Use monotonic game clock (`Clock`) for runtime tick progression.
- Do not reintroduce periodic persistence of "current tick/server time" global vars.
- Keep bootstrap errors explicit and actionable for operations.
