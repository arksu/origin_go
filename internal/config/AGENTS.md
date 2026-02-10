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

## Safety

- Invalid enum-like config values should fail fast during boot (`logger.Fatal`).
- Prefer explicit defaults over implicit zero-values for operational predictability.
