# Dropped-item durability and interaction fixes

## Goal

Make player drop and pickup crash-safe, give primary click/tap priority to
dropped-item pickup, harden generic object deletion, and migrate legacy
Unix-based dropped-item timestamps to the persisted runtime-clock model.

## Approved decisions

- A drop or pickup commits the affected player inventory and the dropped
  object's object/inventory rows in one database transaction.
- If the transaction fails, the operation restores its temporary ECS changes
  and reports failure without exposing a new dropped entity to spatial queries.
- Inventory persistence is version-guarded so an older queued character-save
  snapshot cannot overwrite a committed transfer.
- Primary click/tap on a dropped item takes precedence over active build and
  lift placement modes. RMB and long press remain context interaction.
- Generic object deletion verifies that the object row was actually deleted
  before deleting its owned inventory rows.
- Legacy dropped-item data without a runtime time-basis marker and with a
  Unix-style timestamp is migrated lazily on chunk load. Its `drop_time` is
  reset to the current server runtime time, persisted with a
  `runtime_seconds_v1` marker, and receives one fresh lifetime.

## Architecture

### Durable transfer boundary

`InventoryOperationService` prepares and validates the operation, then uses a
specialized persistence capability implemented by `DroppedItemPersisterDB`.
It serializes the updated player root inventories and passes those snapshots
with either the new dropped-item records or the deleted dropped-item ID.

The Postgres implementation uses a single `WithTx` callback to:

1. upsert or delete the dropped object and its root inventory;
2. upsert the player's serialized root inventories; and
3. commit only if every write succeeds.

For drop, the new ECS entities are created before the transaction but are not
registered in chunk spatial state until commit. The source inventory is
temporarily changed to produce the persistence snapshot and restored if the
transaction fails. For pickup, the destination inventory is temporarily
changed, then restored if the transaction fails; the dropped ECS entity is
removed only after commit.

Each root inventory version is monotonic. Database upserts reject writes whose
version is older than the stored row, preventing a stale asynchronous saver
snapshot from undoing a durable transfer. A nested-container mutation also
bumps the root inventory version whose serialized JSON contains that nested
data.

### Input priority

The renderer hit-tests for a dropped item before updating a build/lift ghost or
delegating to the page callback. This makes the primary interaction invariant
independent of desktop or mobile input mode.

### Safe deletion and timestamp migration

Object deletion uses a query that returns success only when a live object row
matched its `(region, id)`. Inventory deletion happens only after that result
inside the same transaction.

New dropped metadata writes `time_basis: "runtime_seconds_v1"`. On load, a
missing marker with a Unix-scale timestamp is converted to the current runtime
time and written back before the entity is spawned. A failed migration prevents
the object from loading, rather than silently resetting its lifetime on every
restart.

## Verification

- Go unit tests cover transaction success/failure and ECS rollback for drop and
  pickup, stale inventory versions, safe deletion, and legacy timestamp
  migration.
- Client tests cover primary pickup while build/lift modes are armed.
- Run `go test ./...`, `go vet ./...`, frontend type checking, frontend build,
  and `git diff --check`.
