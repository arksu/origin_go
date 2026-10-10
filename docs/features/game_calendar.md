# Game calendar and server constants

The world calendar derives from `RuntimeSecondsTotal`, the accumulated real time
that the server has been running plus explicit administrator time advances.
Simulation ticks remain independent: bounded tick catch-up and discarded ticks
do not change the length of a game day.

The server defines the scale:

| Calendar period | Length |
| --- | --- |
| Game day | 28,800 runtime seconds (8 real hours) |
| Game day clock | 24 game hours |
| Game month | 30 game days |
| Game year | 12 game months |

Runtime zero is year 1, month 1, day 1, 00:00:00. Existing runtime determines the
age of an existing world. Absolute day, month, and year indexes start at zero.
At runtime 364,686 seconds the date is year 1, month 1, day 13, 15:54:18.
The administrator `/time` command reports the calendar and raw runtime seconds.

Calendar conversion uses integer quotient and remainder. Only bounded values
within the day are scaled; accumulated runtime is never multiplied by the clock
speed or converted to `time.Duration`. Negative runtime is invalid. Calendar
conversion supports every nonnegative signed 64-bit runtime-second value.

## Persistence

The calendar uses the existing runtime/tick checkpoint without a new stored
epoch, database key, or migration. Runtime pauses while the server is offline.
Restart resumes the persisted runtime and adds no artificial time allowance.

Clock and object checkpoints are independent. A crash can restore a clock older
than an object's saved timer state and extend the remaining wait. This is an
accepted gameplay trade-off; there is no startup compensation, durable clock
watermark, or transaction coupling object saves to clock saves. Existing gameplay
timers keep their runtime, tick, or wall-time domains.

## Administrator runtime advance

`/addtime <seconds>` queues an advance of the shared server runtime. It accepts
exactly one positive whole-second integer, for example `/addtime 60`, and has the
same access as `/give`, with no additional permission check. Success sends the
requesting player a private English response:
`Queued +60 runtime seconds; applies next tick.` Errors are also private English
responses; recognized commands are never broadcast as local chat, including
invalid requests.

Concurrent requests are summed in one pending counter under `timeStateMu`.
The resulting runtime, including pending advances, must not exceed
`math.MaxInt64 / int64(time.Second)` (9,223,372,036 seconds), because the current
clock bootstrap converts persisted runtime to `time.Duration`. Rejected requests
leave both pending and applied runtime unchanged. This command limit does not
limit the pure calendar converter's supported signed 64-bit input range.

After all shards finish the current tick, the game loop applies the pending
advance once to both `RuntimeSecondsTotal` and the runtime clock. It preserves
the subsecond remainder and original sampling instant. The next tick receives
the advanced runtime and `TimeState.Now` identically in every layer, including a
catch-up tick within the same loop iteration. `/time` executed in the same
command batch as `/addtime` still reads that tick's earlier time snapshot.

The advance changes the calendar and runtime-based timers, which continue through
their existing systems and schedules. It does not add simulation ticks, increase
`Delta` or catch-up work, change wall time, or increase process `Uptime`. Applied
advances use the existing 20-second and shutdown checkpoints; the command does
not force an immediate save. Clients see the advance in their next ordinary
Pong, without a new packet or broadcast.

## Connection bootstrap

After successful authentication the server enqueues `AuthResult(success)`, then
one `S2C_ServerConstants`, before starting world bootstrap. Mandatory handshake
delivery uses `SendCritical`; preparation, serialization, or enqueue failure ends
the connection before spawn. Existing disconnect cleanup clears the character's
online state. Repeated authentication does not create a second identity or resend
constants.

`ServerMessage.server_constants` uses tag 53 and contains exactly these fields:

| Tag | Field | Source |
| --- | --- | --- |
| 1 | `coord_per_tile` | Server world geometry |
| 2 | `chunk_size` | Server world geometry |
| 3 | `tick_rate` | Initialized game loop |
| 4 | `directional_movement_supported` | Server capability |
| 5 | `real_seconds_per_game_day` | Shared server calendar constant |
| 6 | `hours_per_day` | Shared server calendar constant |
| 7 | `days_per_month` | Shared server calendar constant |
| 8 | `months_per_year` | Shared server calendar constant |

Numeric fields are positive `uint32` values; directional support is a boolean and
false is valid. The snapshot is immutable for the server run and identical across
connections and layers. It is sent once per authenticated connection, again on
reconnect, and never repeated for a Ping, world entry, leave, or layer transfer.
There is no request, acknowledgment, revision, or refresh timer for constants.
Standard 60-minute hours, 60-second minutes, and calendar numbering are protocol
conventions; derived calendar periods are computed from the received fields.

The client retains one read-only connection snapshot. It uses received constants
for geometry, movement, and calendar calculations without numeric fallbacks.
Rendering can initialize before authentication; work depending on these values
waits for valid constants. World bootstrap without valid constants is a protocol
error. Identical duplicate constants are harmless; changed constants on the same
connection are a protocol error. New connections can receive different values.

The migrated `S2C_PlayerEnterWorld` fields 3 (`coord_per_tile`), 4 (`chunk_size`),
5 (`tick_rate`), and 10 (`directional_movement_supported`) and their names are
reserved. Entity identity/name, stream epoch 9, and audio 11 remain per entry;
hearing can depend on the player and is not a global constant.

**Deployment requires matching server and client builds.** Older clients rely on
the removed enter-world fields; new clients require server constants. Deploy and
roll back both together. Do not reuse the reserved tags or names.

## Runtime synchronization

`S2C_Pong` keeps its wall-time fields and adds only
`optional int64 runtime_seconds_total = 3`. This value is accumulated runtime,
including applied administrator advances, rounded down to whole seconds.
Presence distinguishes valid zero from a legacy
Pong with no runtime. Every authenticated Pong contains it: the immediate Ping
after authentication, the existing five-second periodic Ping, and extra Pings.
No separate periodic calendar message or player scan is introduced.

The existing `server_time_ms` anchors that runtime at the same response instant.
The server copies a coherent runtime accumulator, bounded remainder, and original
monotonic sampling instant under `timeStateMu`, then projects through the response
instant without changing ECS time, persistence, or gameplay timers. Serialization
runs outside the lock. Pong carries no constants, fractions, date fields, or
additional timestamp.

The client first updates existing wall-time `TimeSync`, then converts the received
whole runtime seconds using the received calendar constants. `GameCalendarSync`
retains an immutable reactive snapshot of that server sample. It does not
compensate network delivery age or advance from local wall/monotonic clocks.
Calendar values remain unchanged between accepted Pongs. Runtime and absolute
indexes retain exact integer precision. Calendar queries remain unsynchronized
until valid constants and a runtime sample exist; a sample received before
constants is converted once the constants arrive. Missing, malformed, negative,
or decreasing runtime is ignored. Identical runtime/timestamp pairs are duplicates,
while equal runtime with a fresh timestamp is a valid sample.

Connection start, disconnect, authentication failure, and connection errors clear
constants and calendar synchronization. Entry, leave, and layer transfers on the
same connection retain them. Retired socket callbacks cannot update a replacement
connection; reconnect can accept lower runtime after crash rollback.

Flooring omits less than one real second, equivalent to less than three game
seconds at the current scale. The displayed sample also ages through network
delivery and until the next Pong; the client deliberately does not predict time
between samples. The client snapshot is presentation data and never controls gameplay.
Legacy Pongs still update wall synchronization without inventing calendar time.

## Day-time HUD

`web_new/src/components/ui/DayTime.vue` displays the original `origin_webgl`
day/night sky, landscape, and sun artwork at 134 by 71 pixels, followed by
zero-padded `HH:mm`. It sits centered under the hotbar with an 8-pixel gap and
shares the map and other HUD elements' visibility. It has no separate connection
or world-entry mount condition. Before a valid snapshot, it shows `--:--` with
the graphical layers hidden. Connection resets restore that placeholder; layer
transfers preserve the current sample.

The five PNG files in `web_new/src/assets/img/daytime/` are unchanged copies of
`origin_webgl/frontend/assets/img/`. They retain their original alpha and layer
order: day sky, night sky, sun, day landscape, night landscape. The label uses
the original green 12-pixel type and black shadow. The widget does not capture
pointer input.

The decorative profile retains the old artwork's normalized day fractions:
dawn at 5/24 through 6/24, daylight until 21/24, and dusk until 22/24. These are
artwork phases, not calendar-duration defaults. Both night layers use the same
opacity. Sun progress is clamped from 5/24 to 21/24 and uses the original
23-pixel radius, (60, 26) offset, and angle `progress * (pi + 0.6) - 0.2`.
Position uses the full sampled day phase rather than an integer hour. Clock text,
sun position, and night opacity update only from server snapshots; there are no
local timers, frame callbacks, or CSS time animations. The HUD adds no date,
moon, world-lighting changes, server fields, or network messages.
