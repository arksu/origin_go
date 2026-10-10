# Proposal

## Why

The server has persistent uptime and simulation ticks but no shared game calendar. Clients need server-owned calendar rules and immutable world/movement constants once per connection, while current world entry mixes those constants with per-entry state.

## What Changes

- Derive the world calendar from existing accumulated server runtime: one game day is 28,800 runtime seconds (eight real uptime hours), with 24 game hours per day, 30 days per month, and 12 months per year.
- Define runtime zero as year 1, month 1, day 1 at 00:00:00. Existing worlds immediately reflect their accumulated age; offline time remains paused.
- Provide deterministic calendar calculations, absolute day/month/year indexes, and the fraction of the current day.
- Add one `S2C_ServerConstants` packet after successful authentication containing `coord_per_tile`, `chunk_size`, `tick_rate`, `directional_movement_supported`, `real_seconds_per_game_day`, `hours_per_day`, `days_per_month`, and `months_per_year`. These values are defined only by the server, immutable during its run, and sent once per authenticated connection.
- **BREAKING**: Move the four existing world/movement constants out of `S2C_PlayerEnterWorld` and reserve their old names and field numbers. Updated clients require the constants packet before world bootstrap; deploy matching server and client versions together.
- Extend the administrator `/time` response with the calendar date and game time while retaining the raw runtime value.
- Synchronize accumulated whole runtime seconds through one new field, `optional int64 runtime_seconds_total = 3`, in every existing Pong. Refresh immediately after authentication, at the existing five-second ping cadence, and in response to any additional Ping; use the existing `server_time_ms` as the sample's wall-time anchor.
- Add a client calendar API that derives and estimates game time, resets across connections, and has no visible UI.

## Capabilities

### New Capabilities

- `game-calendar`: Runtime-based world date/time, administrator inspection, and server-authoritative client synchronization using received calendar constants without rendering.
- `server-constants`: One required connection-scoped packet carrying immutable server world, movement, and calendar constants.

### Modified Capabilities

- `directional-movement`: Advertise support in server constants rather than each enter-world snapshot; require constants and an active world before directional input, with coordinated deployment for the bootstrap change. Movement execution, action, decay, and wall-clock behavior remain unchanged.

## Impact

- Server: `internal/timeutil`, runtime sampling in `internal/game/game.go`, authentication/bootstrap in `internal/game/game_auth.go`, and `internal/game/chat_admin_commands.go`.
- Protocol: one `S2C_ServerConstants` payload, relocation/reservation of enter-world fields, and `optional int64 runtime_seconds_total = 3` in `S2C_Pong`, with regenerated Go and client bindings. Whole runtime seconds are sufficient; Pong carries no constants, fractional runtime, or extra timestamp.
- Client: connection-owned constants and calendar helpers, world/bootstrap consumers in `network/handlers.ts`, movement/geometry/render readiness, and focused tests using the existing TypeScript test harness. No client fallback values for the transmitted constants.
- Existing `global_var` runtime persistence is reused; no database migration, new dependency, gameplay timer conversion, or per-player broadcast scheduler is required.
- Clock/date UI, day/night lighting, sun position, moon phases, and seasons are outside this change.
