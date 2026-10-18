# Character Activity

## Purpose

Track online seconds between authenticated client pings and expose the owner's
last received ping through the existing Hearth text window.

## Requirements

### Requirement: Server-owned ping accounting

The server SHALL store cumulative whole seconds in `character.online_time` and
the most recent received ping wall timestamp in nullable `last_ping_at`.
The first ping of each authenticated connection SHALL establish an anchor;
subsequent pings SHALL add positive server-measured elapsed intervals.
Client timestamps SHALL NOT determine duration or last-seen metadata.
Unauthenticated, replaced, closed and dead-observer sessions SHALL NOT accrue
time. A persistent connection's world entry and transfers SHALL NOT reset its
anchor. Offline gaps and time after the last ping SHALL NOT be added.

#### Scenario: Fractional seconds survive reconnect

- **WHEN** ping intervals cross a whole second across multiple sessions
- **THEN** the whole second SHALL be retained, excluding the offline interval
- **AND** the subsecond remainder SHALL persist only until process restart

### Requirement: Batched durable activity snapshots

Dirty activity SHALL flush once per second in batches of at most 512 characters,
without SQL, ECS access or shard locks on receipt. Pending work SHALL coalesce
by character. Absolute cumulative totals and timestamps SHALL never move
backwards, and retry after an ambiguous commit SHALL NOT double-count time.
Acknowledgement SHALL NOT discard pings received during a write. Failures SHALL
be logged and retained for the next periodic checkpoint.

Current-session disconnect SHALL flush its pending activity outside shard locks.
Graceful shutdown SHALL drain pending activity after network/auth work finishes,
with bounded independent contexts. A crash MAY lose unflushed pings. Restart
SHALL retain persisted whole seconds, discard fractional carry and establish a
fresh session anchor without counting offline time. Ordinary character saves
SHALL NOT overwrite activity columns. A delayed pre-death checkpoint MAY update
historical metadata on a soft-deleted character without changing its lifecycle.

### Requirement: Last-seen window metadata

Hearth SHALL show `Last seen at` after `Owner` using the latest received ping,
including live metadata newer than the SQL checkpoint. Missing history SHALL
show `Unknown`. Relative age SHALL be calculated only when opening the window,
using `just now` or whole English minutes, hours and days rounded down. Opening
SHALL perform no SQL and SHALL reuse the existing text-window protocol. Existing
`disconnect_at` storage SHALL remain independent of last-seen presentation.
