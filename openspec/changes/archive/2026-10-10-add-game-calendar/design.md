# Design

## Context

See `proposal.md` for motivation and `specs/game-calendar/spec.md` for the behavior contract.

`Game.gameLoop` already accumulates full elapsed runtime independently of its four-tick catch-up cap. Whole seconds and a subsecond remainder are protected by `Game.timeStateMu`. The accumulated tick/runtime pair is persisted every 20 seconds and on shutdown. `ecs.TimeState.Now` is runtime time, while `UnixMs`/`WallNow` are wall time. ADR 0005 describes this split but remains marked Proposed; the current implementation is the authority for integration.

`/time` currently reads `TimeState.RuntimeSecondsTotal`. `Game.handlePing` runs on the network path and supplies current wall time in `S2C_Pong.server_time_ms`. The client consumes Pong in `GameConnection.handlePong`, updates `TimeSync`, and already sends a ping immediately after successful authentication and every five seconds. World stream resets can occur within one persistent connection.

`S2C_PlayerEnterWorld` currently carries geometry constants, tick rate, directional support, and per-entry entity/epoch/audio state. Calendar constants are not transmitted. `handleAuth` enqueues successful authentication and then starts `spawnAndLogin` asynchronously; a connection's read loop invokes packet handlers sequentially. The client initializes rendering before connecting, and geometry helpers currently contain fallback values, so removing those values requires explicit readiness gating.

The user's [earlier TimeController](https://github.com/arksu/origin_webgl/blob/master/backend/src/com/origin/TimeController.kt), inspected on 2026-10-10, derives calendar fields from accumulated ticks. Reuse its single-coordinate calendar idea. Its day is zero-based, month is an absolute counter without year conversion, and integer ticks-per-minute truncation can produce hour 24. Its startup adds a save interval to time. These behaviors are not carried into the new calendar.

## Goals / Non-Goals

**Goals:**

- Constant-time, allocation-free server calendar conversion from existing runtime.
- A coherent, thread-safe runtime anchor for network synchronization without shard access.
- Exact integer date conversion in Go and TypeScript, including large supported runtime values.
- A client API available before world entry and stable through layer transfers.
- One required immutable constants snapshot per connection, sourced only from server values and retained through world transfers.

**Non-Goals:**

- Replacing the server's runtime clock, persistence policy, or existing timer domains.
- A second stored calendar epoch, configurable calendar balance, calendar-triggered events, UI, or lighting.
- Changing wall-time RTT synchronization or adding timers and per-player calendar broadcasts.
- Runtime mutation of global constants, a constants revision protocol, or separate calendar and world-constants packets.

## Decisions

### 1. Derive calendar fields with integer quotient and remainder

Add `internal/timeutil/game_calendar.go` with constants for 28,800 real uptime seconds per game day, 24 game hours, 30 days per month, and 12 months per year. Provide a pure conversion accepting nonnegative `int64` runtime seconds and returning date/time, zero-based absolute day/month/year indexes, and day phase. Invalid negative input returns an explicit error.

```text
dayIndex = runtimeSeconds / 28800
runtimeSecondOfDay = runtimeSeconds % 28800
gameSecondOfDay = runtimeSecondOfDay * 3
monthIndex = dayIndex / 30
yearIndex = dayIndex / 360
year = yearIndex + 1
month = monthIndex % 12 + 1
day = dayIndex % 30 + 1
hour = gameSecondOfDay / 3600
minute = gameSecondOfDay / 60 % 60
second = gameSecondOfDay % 60
dayPhase = runtimeSecondOfDay / 28800.0
```

Only bounded within-day values are multiplied. Do not multiply all runtime seconds by three or convert accumulated runtime to `time.Duration`; either can overflow while the input still fits `int64`. Years and absolute indexes use `int64` on the server. Conversion at `MaxInt64` yields year 889599926395, month 3, day 2, 22:30:21.

Alternative: derive the calendar from ticks. Rejected because dropped catch-up ticks and tick-rate changes would change the effective day length. A separate persisted day counter is unnecessary and would introduce another source of truth.

### 2. Reuse existing persistence and expand administrator inspection

No new global variable or migration is required. Runtime zero is the approved epoch, so existing world age applies immediately. The calendar inherits existing crash rollback and loss of the unpersisted subsecond remainder; it does not artificially skip forward on boot.

Object state and clock checkpoints are persisted independently, so a crash can restore clocks older than an object's saved state and extend its remaining timer. The user explicitly accepts this delay as an acceptable gameplay trade-off in order to keep persistence simple. Do not add a startup time allowance, a durable clock watermark, or transactions coupling object saves to clock saves as part of this change. Preserve both runtime-based and tick-based timer behavior.

Update `ChatAdminCommandHandler.handleTime` to use the pure helper and emit English text, for example `Year 1, Month 1, Day 13, 15:54:18; runtime_seconds: 364686`. Keep raw runtime in the response for diagnostics. Handle a conversion error deliberately rather than wrapping an invalid value into a date.

Alternative: initialize a separate epoch on deployment. Rejected by the user's explicit choice to derive age from existing runtime zero.

### 3. Send immutable server constants once per authenticated connection

Add one flat typed packet and `S2C_ServerConstants server_constants = 53` to `ServerMessage.payload` (53 is currently free):

```protobuf
message S2C_ServerConstants {
  uint32 coord_per_tile = 1;
  uint32 chunk_size = 2;
  uint32 tick_rate = 3;
  bool directional_movement_supported = 4;
  uint32 real_seconds_per_game_day = 5;
  uint32 hours_per_day = 6;
  uint32 days_per_month = 7;
  uint32 months_per_year = 8;
}
```

Source geometry from the existing server constants, tick rate from the actual initialized game loop, directional support from the server capability, and calendar fields from the same server constants used by the calendar converter. Calendar values are currently 28,800/24/30/12. Capture this immutable snapshot at server initialization. No value changes during the server run; no hot reload, configuration revision, request packet, or update scheduler is introduced. Standard minute/second units and runtime-zero calendar numbering remain protocol conventions.

After authentication succeeds, synchronously enqueue `AuthResult(success)`, then `ServerConstants`, then start `spawnAndLogin`. Use the existing `SendCritical` path for both mandatory bootstrap messages. On preparation, serialization, or enqueue failure, close the connection and abort spawn; preserve the authenticated-client association until existing disconnect cleanup clears the character's online flag, including when no entity has been attached yet. Do not add an independent parallel offline write that bypasses the existing stale-session guard. The connection read loop cannot process the immediate Ping until the auth handler returns, so the common FIFO queue places constants before that Pong and before world bootstrap. Send exactly one constants packet per authenticated connection; guard repeated authentication using the existing authenticated-client association rather than resending configuration. Every reconnect receives a fresh snapshot. No constants are sent on failed authentication, additional Pings, world entry, layer transfer, or world leave.

Actually remove `coord_per_tile = 3`, `chunk_size = 4`, `tick_rate = 5`, and `directional_movement_supported = 10` from `S2C_PlayerEnterWorld`; reserve both those numbers and names. Keep entity ID/name, `stream_epoch = 9`, and `audio = 11` in the per-entry message. Audio includes player-dependent hearing and does not belong in global constants. Do not retain permanent duplicate constants in world-entry packets.

Add a small connection-owned `web_new/src/network/ServerConstants.ts` holder for a validated, read-only snapshot. Process the packet directly in `GameConnection`, alongside AuthResult/Pong. All seven numeric fields must be positive integers within their protobuf ranges; boolean false is a valid directional capability. Validate shape/ranges without comparing calendar values to 28,800/24/30/12 on the client. Derived periods and speed are computed from received values with exact arithmetic. An identical repeated snapshot can be ignored defensively; a changed snapshot in one connection is a protocol error, not a live update. Missing/invalid constants must not be replaced by client defaults.

World/bootstrap consumers read the received snapshot rather than removed enter-world fields. Geometry, tick-rate configuration, and directional capability persist independently of world stream state; reapply their received values to reset consumers when needed without a new packet. Separate movement tick-rate configuration from stream-epoch changes. The current `WorldParams` interface may expose values derived from the connection snapshot for consumers, but it must not become another writable source of constants.

Remove active fallback values in the world-entry handler, geometry helpers, and movement tick-rate configuration. Because `GameView` initializes the renderer before connecting, guard parameter-dependent work such as `Render.updateChunkBuilds` and terrain camera/chunk calculations until valid constants exist. Allow empty renderer initialization; do not replace defaults with getters that throw during the pre-handshake render loop. Require valid constants before processing world bootstrap or enabling input. A world bootstrap without constants is a protocol error; close through the existing connection-error path instead of rendering with guessed values. No new visible UI is needed.

This is a breaking bootstrap migration: old clients depend on fields now removed, and new clients require the new packet. Deploy and roll back matching server/client versions together. Preserve remaining protobuf numbers and reserve all removed fields. The optional Pong extension itself remains additive; it does not make the overall migration compatible with mixed versions.

Alternatives: keep transmitting constants in every world entry, or add a calendar-only configuration packet. Both duplicate immutable connection-wide data. A generic map of settings or nested configuration hierarchy is unnecessary for the eight known fields.

### 4. Add only whole runtime seconds to each existing Pong

The Pong message changes only by adding one scalar field:

```protobuf
message S2C_Pong {
  int64 client_time_ms = 1;
  int64 server_time_ms = 2;
  optional int64 runtime_seconds_total = 3;
}
```

Keep fields 1 and 2 and their RTT/wall-time meaning unchanged. The added field is accumulated real runtime in whole seconds, not accelerated game seconds. `optional` distinguishes an unsupported legacy Pong from valid runtime zero. The user explicitly accepts whole-second precision: flooring introduces less than one real second, or less than three game seconds, of quantization error. This bound does not include network/time-estimation error.

Include the field in every authenticated Pong, with no separate throttling: the existing immediate Ping after successful authentication, the existing five-second periodic Ping, and any additional Ping all receive it. Do not change the Ping message, cadence, or client request behavior.

Reuse `server_time_ms` as the runtime sample's wall-time anchor. To keep both fields coherent at response time, retain the runtime accumulator's whole seconds, bounded subsecond remainder, and original sampling `time.Time` together under `Game.timeStateMu`. Initialize this internal anchor at the start of runtime accumulation, before entering the running state, and use that same instant as the loop's initial elapsed-time origin so startup loading is not counted. Update the anchor alongside each per-loop runtime accumulation.

In `handlePing`, copy the coherent accumulator and sample response `now` under the read lock. After releasing the lock, project runtime to that same `now` using nonnegative elapsed time from the internal sampling instant, then floor to whole seconds; set `server_time_ms` from `now.UnixMilli()`. Preserve the original `time.Time` values so elapsed-time subtraction uses the production clock's monotonic component. Do not reconstruct the internal anchor from Unix milliseconds. Combine whole-second carries and bounded remainders without converting total accumulated runtime to `time.Duration`. The projection is only a network-time estimate: it does not advance or mutate the runtime accumulator, ECS, timers, or persistence. The loop later accounts for the elapsed interval through its normal path.

Simply attaching a fresh wall timestamp to the previous loop's runtime would introduce publication delay, which can grow during a slow loop; response-time projection avoids needing an extra wire timestamp. Network handlers must not read shard `ecs.TimeState`, and serialization/I/O must occur outside the lock. The persistence snapshot remains its existing tick/runtime pair.

Calendar parameters are sent only in the one-time `ServerConstants` packet. Do not add constants, a fractional runtime field, a nested calendar message, ready-made date fields, or another timestamp to Pong.

No additional periodic message, world-entry field, or broadcast system is required. At the current five-second ping cadence and 30,000 clients, about 6,000 existing Pong responses per second gain one scalar field; the constants packet adds one small message per successful connection, with no player scans. Locking, projection, and snapshot copying remain O(1).

Alternatives: runtime milliseconds or a nested Pong snapshot with fractional precision and calendar metadata. Rejected because whole seconds are sufficient and immutable metadata belongs in `ServerConstants`. A new periodic time broadcast duplicates the existing exchange.

### 5. Keep the client calendar in a dedicated synchronization helper

Add `web_new/src/network/GameCalendarSync.ts` with a testable helper class and singleton matching the existing `TimeSync` organization. Expose snapshot acceptance, nullable/unsynchronized calendar queries, day phase, absolute indexes, and reset. Configure calendar conversion exclusively from the received `ServerConstants`; no client literal fallback for calendar balance is allowed. The API remains unsynchronized until both constants and a valid runtime Pong exist. Export the API from `network/index.ts`; it does not require Pinia state or Pixi objects.

In `GameConnection.handlePong`, update the existing `timeSync` first, then give the optional `runtime_seconds_total` and existing `server_time_ms` to the calendar helper. On acceptance, compensate for delivery age using `timeSync.estimateServerNowMs() - server_time_ms`, clamped at zero, and record the corrected runtime coordinate against `performance.now()`. Subsequent queries use monotonic elapsed time until the next accepted sample. The wire value has no fractional part; local extrapolation can still progress smoothly through second and date boundaries. Accept refreshed authoritative corrections even if they move an earlier extrapolated estimate backwards within the accepted quantization/estimation error.

Compare incoming whole runtime seconds against the previous authoritative sample, not against the extrapolated current estimate; reject lower runtime within the same connection. Identical runtime and wall-anchor pairs are duplicates. An additional Pong can legitimately have the same whole runtime seconds but a different response timestamp; accept that as a fresh anchor rather than discarding all equal-second samples.

Parse protobuf `int64` runtime exactly through its decimal representation into `bigint`; reject imprecise numeric inputs rather than silently rounding. Keep accumulated arithmetic and absolute indexes exact and convert only bounded time-of-day values to `number`. Validate nonnegative signed-64-bit runtime and a valid existing wall timestamp before installing an anchor. Distinguish an absent optional field from explicit zero. Calendar parameters are validated once by the constants holder; Pong has no metadata or fractional wire parsing. Missing or invalid runtime data leaves the current anchor unchanged, and legacy Pongs still update wall-time synchronization. Verify the client formula with server-profile boundary vectors and at least one different valid received calendar profile so hidden fixed values are caught.

Use dependency-injected monotonic/wall estimate functions in tests. No interval or animation callback is added: calendar values are computed on demand.

Alternative: estimate continuously from `Date.now()`. Rejected because local wall-clock adjustments can disturb progression between server samples. The existing wall estimator remains useful to account for sample age at acceptance.

### 6. Scope constants and calendar lifetime to the connection

Clear constants, their dependent readiness/configuration, and the calendar helper when a connection starts and when it disconnects or enters an error state, including authentication failure and remote socket close. Keep reset in the connection lifecycle so direct `gameConnection` usage behaves consistently. Retain existing socket listener cleanup and gate constants/calendar acceptance on the current authenticated connection and socket identity or local connection generation. A late callback from a retired socket must not reinstall cleared configuration or replace a new connection's state; cover this with a lifecycle regression test rather than relying on cleanup alone.

Do not reset constants or calendar on `PlayerEnterWorld`, `PlayerLeaveWorld`, or a layer stream epoch change. These are server-global values, and a valid Pong can arrive before world entry. World identity, epoch, and held input still reset through their existing world lifecycle. A new connection starts without constants or an anchor, accepts that connection's constants, and can then accept lower runtime after server crash rollback. No calendar stream epoch is necessary.

## Risks / Trade-offs

- Existing runtime snapshots may roll back after a crash, and independently saved object timers may wait longer; in-flight writes or storage failures can increase the loss beyond the save interval → this is an explicitly accepted gameplay trade-off. Document inherited behavior and allow a fresh connection to install the restored value without adding startup compensation or persistence coordination.
- Whole-second wire samples omit the subsecond runtime remainder → accept the user-approved quantization error of less than one real second (less than three game seconds), extrapolate locally between Pongs, and add no precision fields to the protocol.
- Wall synchronization is an estimate and system clock adjustments can briefly affect sample-age compensation → use coherent anchors, monotonic local progression, regular existing Pong refreshes, and allow authoritative correction; do not claim perfect accuracy under clock jumps or network stalls.
- A disconnected or stalled server cannot provide a fresh authoritative sample → connection loss clears the estimate; while connected, extrapolation remains a presentation estimate and never controls server gameplay.
- Server/client formulas could diverge at boundaries → use matching fixed test vectors for epoch, minute/hour/day/month/year rollovers, subsecond rollover, existing world age, and maximum runtime, plus protobuf round trips.
- Added payload and locking occur on the existing Ping path → keep reads bounded and serialization outside the lock; avoid scans, per-client timers, and conversions of large absolute values to floating point.
- Relocating existing enter-world fields breaks mixed-version bootstrap → reserve removed names/numbers, update directional-movement requirements and contract tests, and deploy matching server/client builds together rather than keeping duplicated constants.
- Renderer initialization precedes constants arrival and may continue across reconnect → explicitly gate parameter-dependent work and clear readiness on connection reset; test first load and reconnect with different constants without fallback values.
- Production clock construction already uses a `time.Duration` conversion for bootstrap beyond roughly 292 years of accumulated uptime → keep that unrelated existing limitation outside this change; the pure calendar converter itself supports the full nonnegative `int64` range.

## Migration Plan

1. Implement the server calendar helper, one-time constants bootstrap, enter-world relocation/reservations, coherent runtime anchor, optional Pong extension, administrator output, and client constants/calendar lifecycle and readiness integration.
2. Regenerate Go protobuf with `make proto` and client protobuf with `npm run proto` in `web_new`; do not edit generated outputs manually or overwrite unrelated generated-file changes without inspection.
3. Run focused server/client calendar and protocol tests, then the required server and client checks. Review lint's auto-fix diff.
4. Deploy matching server and client builds together; the constants migration does not support mixed-version world bootstrap. The unchanged wall-time exchange remains usable independently, but a new client must not enter a legacy world using guessed constants.
5. Roll back server and client together if needed. No data migration is required and runtime persistence remains usable. Never reuse reserved enter-world fields or an abandoned constants/Pong tag in a subsequent protocol revision.
