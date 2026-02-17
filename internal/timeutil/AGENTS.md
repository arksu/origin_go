# Timeutil Package Notes

`internal/timeutil` owns game clock primitives and server time bootstrap logic.

## Server Bootstrap Contract

- Bootstrap source of truth is pair:
  - `SERVER_TICK_TOTAL`,
  - `SERVER_RUNTIME_SECONDS_TOTAL`.
- On first boot, persist both values in a single transaction.
- On next boots:
  - missing key is recovered as `0` and persisted atomically with the pair,
  - negative values are invalid and must fail fast.
- Tick-rate mismatch is not validated against persisted state.

## Runtime Rules

- Use monotonic game clock (`Clock`) for runtime tick progression.
- Runtime seconds advance only while server process runs.
- Keep periodic persistence of runtime/tick state in a dedicated goroutine (20s interval).
- Persist `SERVER_TICK_TOTAL` and `SERVER_RUNTIME_SECONDS_TOTAL` atomically in one transaction.
- Do final persist on shutdown; persist errors are logged only.
- Keep bootstrap errors explicit and actionable for operations.
