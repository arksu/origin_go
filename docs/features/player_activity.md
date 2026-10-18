# Character online time and last seen

`character.online_time` stores accumulated online seconds as `BIGINT`.
`character.last_ping_at` is a nullable `TIMESTAMPTZ` containing the latest
authenticated client ping received by the server. Apply
`migrations/20261017_character_last_ping.sql` before deploying this server.
Existing totals remain unchanged and old last-ping values start as `NULL`.
The schema places `last_ping_at` after `online_time`; PostgreSQL appends the new
column physically when migrating an existing table, without rebuilding it.

## Accounting

The server registers the connection after authentication. Its first ping sets
the session's accounting anchor without adding time. Each subsequent ping adds
the positive server-measured elapsed interval; client timestamps are ignored.
Production `time.Time` samples retain their monotonic component, so clock
adjustments and administrator runtime advances cannot inflate online duration.
Pings are accepted before world entry and throughout transfers on the same
connection. Unauthenticated, replaced, closed and dead-observer sessions do not
accrue time. Extra pings cannot accelerate the total beyond elapsed real time.

Whole seconds accumulate without rounding each ping independently. The
subsecond remainder is retained per character across reconnects in the same
process and deliberately discarded on server restart. A reconnect's first ping
starts a new anchor, excluding offline time; no interval after the last ping is
added on disconnect or shutdown. Existing saved seconds seed the counter, and
a newer runtime total takes precedence over a stale login row. The total
saturates at the signed 64-bit limit rather than wrapping.

## Persistence and cost

Pings only update mutex-protected metadata in memory, with no ECS access, shard
locks, SQL or steady-state allocations. A single background worker flushes dirty
characters once per second in batches of at most 512 rows. Only the latest
checkpoint per character is pending. SQL sets absolute cumulative totals and
last-ping timestamps using `GREATEST`, making retry after an ambiguous commit
idempotent and preventing older snapshots from replacing newer data.

Acknowledging an older snapshot does not clear a ping received during its write.
Failures are logged and retained for the next periodic flush. A current session's
disconnect flushes that character after normal cleanup and outside shard locks.
Graceful shutdown drains all remaining checkpoints after network/auth work ends,
using independent five-second contexts even though `g.ctx` is already canceled.
No ping is synthesized at shutdown or startup. A crash can lose the latest
unflushed pings, as explicitly accepted for batched persistence.

The tracker retains one entry per character encountered during the process
lifetime to preserve fractional seconds across reconnects. Character position,
inventory and health checkpoints do not write these columns. A delayed activity
checkpoint may update historical metadata on a soft-deleted character without
changing its deleted status or counting dead-observer pings.

## Hearth window

The existing text window shows `Owner`, followed by `Last seen at` with the
relative age of `last_ping_at`. It uses `just now`, whole minutes, hours or days
rounded down, and `Unknown` for missing history. Received pings update the
feature-owned Hearth index immediately, so reopening the window sees current
metadata without waiting for the SQL checkpoint. Offline owners are hydrated by
the existing ownership queries. The displayed snapshot changes only on reopen;
opening performs no SQL, and the protocol and client renderer are unchanged.

`disconnect_at` retains its separate session-disconnect semantics and is no
longer used as the Hearth's last-seen value.
