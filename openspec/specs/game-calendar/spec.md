# game-calendar Specification

## Purpose

Define one world calendar from accumulated server runtime and make the same date and time available to administrators and connected clients without adding visual presentation.

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
Calendar progression SHALL use the existing runtime persistence and offline pause semantics. Offline duration SHALL NOT advance the calendar. Changing tick rate or discarding simulation catch-up ticks SHALL NOT alter its runtime-to-calendar scale. Restart SHALL NOT add an artificial calendar advance.

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

### Requirement: Existing Ping and Pong exchange synchronizes game time
Every authenticated client's existing Pong response SHALL include `optional int64 runtime_seconds_total = 3`, containing nonnegative accumulated real runtime rounded down to whole seconds. The existing immediate Ping after successful authentication, each periodic Ping at the current five-second interval, and any additional Ping SHALL receive the field in the corresponding Pong. This scalar SHALL be the only added Pong field: Pong SHALL NOT add a nested message, fractional runtime, calendar parameters, ready-made date fields, or another timestamp. The client SHALL use calendar parameters received through the one-time server-constants packet, without embedding their numeric values in client code. Runtime synchronization SHALL NOT require a new client request, a separate periodic broadcast, or iteration over all players. Server runtime SHALL remain authoritative.

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

### Requirement: Client calendar supports runtime estimation and validation
The client SHALL expose an on-demand calendar and day-fraction API without adding clock UI, rendering, or a periodic calendar timer. It SHALL derive calendar values using the received server constants, account for delivery age using the Pong's existing `server_time_ms` and wall-time synchronization, then advance its estimate using monotonic local elapsed time between accepted samples. Integer runtime data SHALL retain its precision through calendar conversion. Missing, malformed, or lower authoritative runtime samples SHALL NOT initialize or replace a valid calendar anchor; explicit zero SHALL be valid. An identical runtime and wall-timestamp pair SHALL be treated as a duplicate, while a fresh Pong with unchanged whole runtime seconds SHALL be eligible to refresh the anchor. Until valid server constants and a valid runtime sample have both been received, the API SHALL report an unsynchronized state. A valid refreshed sample SHALL be allowed to correct earlier local extrapolation.

#### Scenario: Constants have not arrived
- **WHEN** the client has not received valid server calendar constants
- **THEN** its calendar remains unsynchronized
- **AND** it does not substitute embedded calendar values

#### Scenario: Local extrapolation crosses midnight
- **WHEN** the accepted whole runtime sample is 28,799 seconds with zero estimated delivery age and one second of monotonic local time elapses
- **THEN** the estimated calendar crosses from the first day into day 2 at 00:00:00

#### Scenario: Invalid or absent synchronization data
- **WHEN** a client receives a Pong without `runtime_seconds_total`, with malformed or negative runtime, an invalid wall timestamp, or a lower authoritative runtime sample
- **THEN** its current calendar anchor is unchanged
- **AND** an uninitialized calendar remains unsynchronized

#### Scenario: Two valid Pongs within the same runtime second
- **WHEN** two valid Pongs contain the same whole runtime seconds and different response timestamps
- **THEN** the second Pong can refresh the anchor without being rejected merely because runtime seconds are equal

### Requirement: Calendar lifetime follows the connection rather than the world stream
Starting a connection, disconnecting, or entering a connection error state SHALL clear the client's calendar anchor. World entry, world leave during transfer, and layer changes within the same connection SHALL retain the server-global calendar anchor. A new connection SHALL accept its first valid snapshot even if server runtime is lower than in the previous connection.

#### Scenario: Pong arrives before world entry
- **WHEN** a client accepts game-time data and then enters the world
- **THEN** its accepted calendar anchor remains available

#### Scenario: Transfer within one server connection
- **WHEN** a player leaves one layer and enters another without replacing the connection
- **THEN** the calendar remains synchronized throughout the transfer

#### Scenario: Reconnect after crash rollback
- **WHEN** a client reconnects to a server restored to an earlier persisted runtime value
- **THEN** the previous connection's anchor has been cleared
- **AND** the new lower runtime is accepted as authoritative

#### Scenario: Retired connection delivers a late callback
- **WHEN** a game-time callback belongs to a disconnected, failed, or replaced connection
- **THEN** it cannot initialize or replace the current calendar anchor

### Requirement: Runtime synchronization preserves existing wall-time consumers
Game-time synchronization SHALL extend Pong additively while preserving its existing protobuf field numbers and wall-time behavior. A client receiving a legacy Pong SHALL continue wall-time synchronization while leaving its game calendar unsynchronized. The separate server-constants bootstrap migration SHALL require matching server/client versions and SHALL NOT be described as mixed-version compatible. Existing movement interpolation behavior after valid bootstrap, action cooldowns, and gameplay timer domains SHALL retain their current behavior. Calendar synchronization SHALL NOT introduce visual changes.

#### Scenario: Legacy Pong remains usable
- **WHEN** a new client receives a Pong containing only the original client and server wall timestamps
- **THEN** existing RTT and wall-offset synchronization continue normally
- **AND** no game-time value is inferred from those wall timestamps alone
