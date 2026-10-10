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

## Game Calendar

- `game_calendar.go` owns the shared server calendar constants and pure
  `GameCalendarFromRuntime` conversion. Server constants bootstrap uses these same
  values; the client receives them once and does not embed calendar balance.
- One game day is 28,800 runtime seconds and contains 24 game hours; months have
  30 days and years have 12 months. Runtime zero is year 1, month 1, day 1, 00:00:00.
- Absolute day/month/year indexes are zero-based. Reject negative runtime and
  retain exact conversion across the nonnegative int64 range. Scale only bounded
  within-day values; never multiply total runtime or convert it to `time.Duration`.
- Use existing persistence and offline pause behavior. Add no artificial startup
  advance or object/clock persistence coordination. Crash rollback may extend
  object timer waits; that gameplay trade-off is intentional.
