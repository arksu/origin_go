# Config Package Notes

This directory defines runtime/server configuration schema and defaults.

## Rules

- Keep `mapstructure` tags stable when adding fields.
- Every new config field must have:
  - struct field in `GameConfig`/related config struct,
  - default in `setDefaults`,
  - validation (if constrained domain).

## Game Behavior Runtime Config

- `game.env`: one of `dev|stage|prod`.
  - `dev` may enable diagnostic fallbacks.
  - `prod` must disable expensive debug-only runtime safety scans.
- `game.object_behavior_budget_per_tick`: max dirty behavior objects processed per tick (default `512`).
- `game.behavior_tick_global_budget_per_tick`: max scheduled behavior ticks processed per server tick (default `200`).
- `game.behavior_tick_catchup_limit_ticks`: max restore catch-up steps per object during behavior init (default `2000`).
- `game.player_stats_ttl_ms`: coalescing window for `S2C_PlayerStats` push scheduling (default `1000`, must be `> 0`).
  - Used by `EntityStatsUpdateState` to delay next due push after last send.
  - Does not force periodic packets: send happens only when stats are marked dirty and rounded network snapshot changed.

## Server Time Bootstrap Contract

- `game.tick_rate` is persisted once as global var `SERVER_TICK_RATE`.
- Server start moment is persisted once as global var `SERVER_START_TIME` (Unix seconds).
- Boot must fail fast when only one bootstrap var exists, values are invalid, or persisted tick rate does not match config.
- Do not add silent migration/fallback logic here; bootstrap invariants are strict.

## Safety

- Invalid enum-like config values should fail fast during boot (`logger.Fatal`).
- Prefer explicit defaults over implicit zero-values for operational predictability.
