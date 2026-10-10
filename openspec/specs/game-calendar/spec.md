# game-calendar Specification

## Purpose

Define one world calendar from accumulated server runtime, make the same date and time available to administrators and connected clients, and display server-sampled time of day in the client HUD.

## Requirements

### Requirement: Game calendar uses accumulated runtime with an exact scale
The calendar SHALL advance by 24 game hours for every 28,800 seconds of accumulated server runtime. A month SHALL contain 30 game days and a year SHALL contain 12 game months. Calendar conversion SHALL preserve exact date boundaries for every nonnegative signed 64-bit runtime-second value and SHALL reject negative runtime input. The calendar SHALL expose time of day with hours in 0–23, minutes and seconds in 0–59, and a day fraction in [0, 1).

#### Scenario: Minute and hour boundaries
- **WHEN** accumulated runtime reaches 20 seconds and 1,200 seconds
- **THEN** game time is respectively 00:01:00 and 01:00:00 on the first game day

#### Scenario: Day boundary
- **WHEN** accumulated runtime changes from 28,799 to 28,800 seconds
- **THEN** calendar time changes from day 1 at 23:59:57 to day 2 at 00:00:00

#### Scenario: Invalid or very large runtime
- **WHEN** conversion receives a negative runtime value
- **THEN** it reports invalid input instead of returning a wrapped date
- **AND** conversion of the maximum signed 64-bit runtime-second value remains exact and does not overflow

### Requirement: Calendar epoch and indexes reflect existing world age
Runtime zero SHALL represent year 1, month 1, day 1 at 00:00:00. Displayed years SHALL start at 1, months SHALL be in 1–12, and days SHALL be in 1–30. The calendar SHALL also provide zero-based absolute day, month, and year indexes for elapsed-period accounting. Existing accumulated runtime SHALL determine the initial calendar without introducing a new epoch or resetting world age.

#### Scenario: Existing world adopts the calendar
- **WHEN** a world with 364,686 accumulated runtime seconds starts supporting the calendar
- **THEN** its date is year 1, month 1, day 13 at 15:54:18
- **AND** its absolute day, month, and year indexes are 12, 0, and 0

#### Scenario: Month and year rollover
- **WHEN** accumulated runtime reaches 864,000 seconds and 10,368,000 seconds
- **THEN** the dates are respectively year 1, month 2, day 1 and year 2, month 1, day 1 at 00:00:00

### Requirement: Calendar inherits runtime lifecycle and remains independent of simulation ticks
Calendar progression SHALL use the existing runtime persistence and offline pause semantics, including explicit administrator runtime advances. Offline duration SHALL NOT advance the calendar. Changing tick rate or discarding simulation catch-up ticks SHALL NOT alter its runtime-to-calendar scale. Restart SHALL NOT add an artificial calendar advance.

#### Scenario: Restart after offline duration
- **WHEN** the server stops with a persisted runtime value and restarts after any offline duration
- **THEN** the calendar resumes from that persisted runtime value regardless of the offline duration

#### Scenario: Simulation lag
- **WHEN** one second of runtime elapses while the simulation processes fewer than its nominal number of ticks
- **THEN** the calendar advances by three game seconds
- **AND** any calendar query is evaluated from the runtime available when that query executes

### Requirement: Administrator time command reports the calendar
The existing administrator `/time` command SHALL report the game year, month, day, and time of day together with raw runtime seconds. Application-provided text SHALL be English and SHALL use the same calendar conversion as server consumers.

#### Scenario: Time command at the epoch
- **WHEN** an administrator executes `/time` at runtime zero
- **THEN** the response reports year 1, month 1, day 1, 00:00:00, and runtime seconds 0

### Requirement: Administrator can queue a global runtime advance
The administrator `/addtime <seconds>` command SHALL accept exactly one positive whole-second integer with the same access as `/give` and no additional privilege check. It SHALL queue a global runtime advance through `AdminTimeExecutor.RequestAdminAddTime(seconds int64) error`, sum concurrent requests in a single pending counter protected by `timeStateMu`, and reject requests whose resulting runtime including pending advances exceeds `math.MaxInt64 / int64(time.Second)` (9,223,372,036 seconds). Rejection SHALL NOT change pending or applied runtime. Recognized commands, including invalid requests, SHALL be consumed without local chat broadcast. Application-provided responses SHALL be private and English.

#### Scenario: Valid request is acknowledged privately
- **WHEN** an administrator executes `/addtime 60`
- **THEN** the requesting player receives `Queued +60 runtime seconds; applies next tick.` privately
- **AND** no local chat broadcast occurs

#### Scenario: Invalid request leaves time unchanged
- **WHEN** `/addtime` receives no argument, extra arguments, a fraction, zero, a negative number, or an integer outside the accepted range
- **THEN** the requesting player receives a private English error
- **AND** the command is consumed without changing pending or applied runtime

#### Scenario: Concurrent requests respect the combined limit
- **WHEN** concurrent requests would together exceed the supported resulting runtime
- **THEN** accepted requests are summed once and any request exceeding the combined limit is rejected without changing the sum

### Requirement: Runtime advances apply between shard ticks
The game loop SHALL apply pending administrator runtime seconds once after all shards finish the current tick. It SHALL advance both accumulated runtime and the runtime clock while preserving the subsecond remainder and original runtime sampling instant. Every layer SHALL receive the same advanced `TimeState.Now` and `RuntimeSecondsTotal` on the next tick, including a catch-up tick in the same loop iteration. Runtime consumers SHALL continue processing through their existing systems and schedules. The advance SHALL NOT change simulation tick count, fixed `Delta`, wall time, process `Uptime`, or the amount of catch-up work.

#### Scenario: Command batch retains its original snapshot
- **WHEN** `/addtime 60` and `/time` execute in the same command batch
- **THEN** `/time` reports the batch's original runtime snapshot
- **AND** the next tick receives the 60-second advance identically across layers

#### Scenario: Advance crosses a calendar boundary
- **WHEN** one runtime second is applied after a tick at runtime 28,799 seconds
- **THEN** the next tick's calendar is day 2 at 00:00:00
- **AND** runtime-based timers observe the advance when their existing schedules run
- **AND** no simulation ticks or wall-time seconds are added by the command

#### Scenario: Applied advance uses existing synchronization and persistence
- **WHEN** an administrator runtime advance has been applied
- **THEN** the next authenticated Pong includes the advanced runtime through the existing runtime field
- **AND** the existing 20-second or shutdown checkpoint saves the advanced runtime without an immediate command-specific save, protocol change, or database migration

### Requirement: Existing Ping and Pong exchange synchronizes game time
Every authenticated client's existing Pong response SHALL include `optional int64 runtime_seconds_total = 3`, containing nonnegative accumulated runtime including applied administrator advances, rounded down to whole seconds. The existing immediate Ping after successful authentication, each periodic Ping at the current five-second interval, and any additional Ping SHALL receive the field in the corresponding Pong. This scalar SHALL be the only added Pong field: Pong SHALL NOT add a nested message, fractional runtime, calendar parameters, ready-made date fields, or another timestamp. The client SHALL use calendar parameters received through the one-time server-constants packet, without embedding their numeric values in client code. Runtime synchronization SHALL NOT require a new client request, a separate periodic broadcast, or iteration over all players. Server runtime SHALL remain authoritative.

#### Scenario: First synchronization
- **WHEN** a client has valid server constants and receives its first valid `runtime_seconds_total` field in a Pong after authentication
- **THEN** its calendar becomes synchronized without waiting for another periodic ping or a world-entry message

#### Scenario: Periodic refresh
- **WHEN** an authenticated client performs the existing periodic ping exchange every five seconds
- **THEN** every corresponding Pong includes refreshed whole runtime seconds
- **AND** calendar synchronization creates no separate WebSocket message

#### Scenario: Additional Ping receives runtime
- **WHEN** an authenticated client sends an additional Ping between periodic pings
- **THEN** the corresponding Pong also includes `runtime_seconds_total`
- **AND** runtime synchronization is not separately throttled

#### Scenario: Whole-second precision is sufficient
- **WHEN** accumulated runtime at the Pong response instant is 1,000 seconds plus 900 milliseconds
- **THEN** `runtime_seconds_total` is 1,000 with no fractional wire value
- **AND** accepted rounding error is less than one real second, equivalent to less than three game seconds, excluding network/time-estimation error

#### Scenario: Explicit zero is a supported sample
- **WHEN** a Pong explicitly includes `runtime_seconds_total` equal to zero
- **THEN** the client accepts the epoch as a valid synchronization value
- **AND** an absent field remains distinguishable from zero

### Requirement: Runtime and wall synchronization samples are coherent
The whole runtime seconds in a Pong SHALL describe runtime rounded down at the same response instant as its existing `server_time_ms`. That timestamp SHALL retain its current wall-time/RTT synchronization meaning and SHALL also anchor the runtime sample; it SHALL NOT be replaced by game time. Sampling SHALL be safe across the game-loop and network execution paths without reading live shard state from network workers or changing persisted runtime and gameplay timers.

#### Scenario: Ping between game-loop samples
- **WHEN** a Pong is produced after the latest runtime sample was published
- **THEN** its whole runtime seconds reflect elapsed runtime through the Pong response instant
- **AND** its existing RTT timestamp describes that same response instant
- **AND** no extra runtime timestamp is transmitted

### Requirement: Client calendar exposes validated server snapshots
The client SHALL expose a reactive read-only calendar and day-fraction snapshot derived only from received whole runtime seconds and server constants. It SHALL NOT compensate delivery age or advance the snapshot using local wall time, monotonic elapsed time, timers, or frame callbacks. Integer runtime data SHALL retain its precision through calendar conversion. Missing, malformed, or lower authoritative runtime samples SHALL NOT initialize or replace a valid calendar snapshot; explicit zero SHALL be valid. An identical runtime and wall-timestamp pair SHALL be treated as a duplicate, while a fresh Pong with unchanged whole runtime seconds SHALL remain valid. Until valid server constants and a valid runtime sample have both been received, the API SHALL report an unsynchronized state. A valid sample received before constants SHALL be converted when those constants arrive.

#### Scenario: Constants have not arrived
- **WHEN** the client has not received valid server calendar constants
- **THEN** its calendar remains unsynchronized
- **AND** it does not substitute embedded calendar values

#### Scenario: Local time does not cross midnight
- **WHEN** the accepted whole runtime sample is 28,799 seconds and local time elapses without another valid Pong
- **THEN** the calendar remains on the first day at 23:59:57 regardless of delivery age or local clock changes
- **AND** only a subsequent accepted runtime sample of 28,800 seconds changes it to day 2 at 00:00:00

#### Scenario: Invalid or absent synchronization data
- **WHEN** a client receives a Pong without `runtime_seconds_total`, with malformed or negative runtime, an invalid wall timestamp, or a lower authoritative runtime sample
- **THEN** its current calendar snapshot is unchanged
- **AND** an uninitialized calendar remains unsynchronized

#### Scenario: Two valid Pongs within the same runtime second
- **WHEN** two valid Pongs contain the same whole runtime seconds and different response timestamps
- **THEN** the second Pong is accepted without advancing the calendar merely because its wall timestamp is later

#### Scenario: Pong arrives before calendar constants
- **WHEN** a valid runtime sample arrives before valid calendar constants
- **THEN** the calendar remains unsynchronized until those constants arrive
- **AND** it then exposes the retained server sample without adding locally elapsed time

### Requirement: Calendar lifetime follows the connection rather than the world stream
Starting a connection, disconnecting, or entering a connection error state SHALL clear the client's calendar snapshot. World entry, world leave during transfer, and layer changes within the same connection SHALL retain the server-global calendar snapshot. A new connection SHALL accept its first valid snapshot even if server runtime is lower than in the previous connection.

#### Scenario: Pong arrives before world entry
- **WHEN** a client accepts game-time data and then enters the world
- **THEN** its accepted calendar snapshot remains available

#### Scenario: Transfer within one server connection
- **WHEN** a player leaves one layer and enters another without replacing the connection
- **THEN** the calendar remains synchronized throughout the transfer

#### Scenario: Reconnect after crash rollback
- **WHEN** a client reconnects to a server restored to an earlier persisted runtime value
- **THEN** the previous connection's snapshot has been cleared
- **AND** the new lower runtime is accepted as authoritative

#### Scenario: Retired connection delivers a late callback
- **WHEN** a game-time callback belongs to a disconnected, failed, or replaced connection
- **THEN** it cannot initialize or replace the current calendar snapshot

### Requirement: Runtime synchronization preserves existing wall-time consumers
Game-time synchronization SHALL extend Pong additively while preserving its existing protobuf field numbers and wall-time behavior. A client receiving a legacy Pong SHALL continue wall-time synchronization while leaving its game calendar unsynchronized. The separate server-constants bootstrap migration SHALL require matching server/client versions and SHALL NOT be described as mixed-version compatible. Existing movement interpolation behavior after valid bootstrap, action cooldowns, and gameplay timer domains SHALL retain their current behavior. The day-time HUD SHALL consume calendar snapshots without changing world lighting or wall-time consumers.

#### Scenario: Legacy Pong remains usable
- **WHEN** a new client receives a Pong containing only the original client and server wall timestamps
- **THEN** existing RTT and wall-offset synchronization continue normally
- **AND** no game-time value is inferred from those wall timestamps alone

### Requirement: Day-time indicator shares the game HUD and server samples
The client SHALL display the original day/night sky, landscape, and sun artwork centered under the hotbar with an 8-pixel gap, followed by zero-padded `HH:mm`. The indicator SHALL share the map and common HUD's visibility without a separate connection or world-entry mount condition. Before a valid calendar snapshot it SHALL show `--:--` with graphical layers hidden. It SHALL preserve the original artwork scale, layering, label treatment, sun geometry, and decorative dawn/day/dusk profile. Sun position SHALL use the full sampled day phase rather than integer-hour steps. Clock text, sun position, and night opacity SHALL remain unchanged between accepted server samples; no local clock, polling timer, frame callback, or CSS time animation SHALL advance them. The indicator SHALL pass pointer input through to gameplay and SHALL NOT add a date, moon, or world-lighting effect.

#### Scenario: HUD is visible before time is synchronized
- **WHEN** the map and common HUD are visible without a valid calendar snapshot
- **THEN** the indicator remains present and displays `--:--` without graphical time-of-day layers

#### Scenario: Accepted Pong updates the indicator
- **WHEN** a valid Pong supplies a new runtime sample
- **THEN** the clock, sun position, and night overlays update from that server sample and received calendar constants
- **AND** they remain fixed until another valid sample arrives

#### Scenario: Layer transfer preserves the visible sample
- **WHEN** the player transfers layers while the map and common HUD remain visible
- **THEN** the same indicator retains the last accepted server sample without remounting or restarting time

#### Scenario: Connection reset clears displayed time
- **WHEN** connection time state resets
- **THEN** the indicator returns to `--:--` without its own connection-dependent mount condition

#### Scenario: Pointer input passes through the indicator
- **WHEN** the player clicks the map where the indicator is drawn
- **THEN** the indicator does not intercept gameplay input
