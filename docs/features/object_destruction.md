# Object damage and durable container drops

This step implements a generic object damage receiver. It accepts calculated
`Draw`, uses object armor `0`, and changes fractional `ObjectInternalState.HP`.
It requires no extra ECS component, health migration or protobuf field.

Definitions may set `indestructible: true` (default `false`, positive HP still
required). The existing `EntityInfo` carries that policy. `ObjectDamageService`
returns `ErrObjectDamageTargetIndestructible` before preparation or calculation,
without health writes, dirty notifications, reservations or quarantine. Melee
excludes the target before selecting nearest/all hits; an immune-only sector
uses the ordinary paid-miss completion. `player_dead` enables the flag. Generic
destruction/deletion and administrative `/destroy` remain available; this is a
damage policy rather than a prohibition on lifecycle removal.

`player_dead` also uses this pipeline for timed corpse decay. That operation
releases the same captured inventory tree but replaces the source with an
inventory-free skeleton under its original `EntityID`. See
[corpse decay](corpse_decay.md) for its server-runtime deadline and identity rules.

## Ownership and admission

`ObjectDamageService` and `ObjectDestructionService` belong to one World.
Construct them during shard setup, and call `PrepareTarget`/`Apply` under its
lock. Preparation registers the exact generational handle. Spawn/setup component
observers prepare eligible objects; player health and special dropped items are
excluded. `Apply` returns a value result and sentinel errors, with no formatting,
I/O, serialization, goroutines or inventory traversal.

Zero and nonfatal damage validate identity, preparation, current HP and Draw.
Zero damage is a no-op; nonfatal damage writes HP and dirty intent together through
the component observer path. Errors leave HP and notification state unchanged.

A fatal hit reserves one of **512** operation slots and a source chunk pin before
writing HP=0. A busy persistence gate or full operation pool rejects the hit
without mutation. The synchronous quarantine removes spatial/collider visibility,
station processing, behavior ticks, links, open root/nested windows and carry.
It also cancels unlinked approaches, pending context/lift transitions and owned
actions through exact-handle reverse indexes. Matching movement stops through
the existing publication path; independent stun and newer unrelated routes are
preserved. Component observers keep these indexes current without world scans
on destruction or allocations for repeated writes of an unchanged target.
The source and its inventory entities remain alive for capture. Pending guards
reject interactions, transfers, transforms and administrative removal. Breaking
the last build-site link cannot automatically delete a pending build-site.

## Capture and replacement

`InventoryRefIndex` maintains a reverse owner index ordered by Kind/Key. Capture
enumerates all roots with `OwnerID == sourceID`, regardless of Kind/Key, in pages
of at most **100** refs per read lock. `ObjectLootCapture` also limits every lock
to **100** item/container/verification records and copies an owned inventory tree.
It rejects stale/mismatched refs, unknown definitions, invalid quantities/quality,
duplicate ItemIDs, duplicate container handles and cycles. Invalid capture keeps
the source quarantined and retries; it never silently discards malformed items.

Every root stack unit becomes one quantity-1 dropped item. Its first unit keeps
the original ItemID; the remaining units use one worker-reserved ID range. Bags
retain their nested contents and dimensions. Build-site `PutItems`, station
numeric resources, wood and other non-inventory resources are ignored.
Ordinary object/character saves and player drops share the complete inventory-tree
serializer, so later persistence does not truncate accepted nested contents.
Missing known bag inventories, stale/mismatched refs and cycles reject capture.
Chunk deactivation uses the shared exact-ref cleanup for the entire saved tree.
Character capture propagates strict inventory errors before accepting a snapshot.

`DestroyedObjectDropSpread = 10` supplies independent integer X/Y offsets.
Positions use the existing integer projection, clamp to half-open world bounds,
and determine the actual destination chunk. No collision/free-space search runs.
An operation stores its seed, origin and runtime drop time once. Owned payloads
and ID reservations survive retries; counter-based offsets reproduce coordinates
without expanding stacks into an in-memory array of persistence records.

Existing chunk save workers perform the replacement. One transaction streams SQL
batches of at most **100** records, upserts every dropped object/root payload,
advances `LAST_USED_ID` monotonically, and deletes the source and all source-root
inventory rows. An empty replacement and an unsaved source are valid. Replaying
the same replacement after an ambiguous commit is idempotent. Dedicated drop-root
upserts replace an older deleted root regardless of its previous inventory version.

`TransformObjectWithDroppedItems` shares this streaming transaction with ordinary
destruction. Instead of deleting the source object, it deletes all source-root
inventory rows and upserts the immutable replacement snapshot under the same
region and ID. Replacement requires a registered normal definition with positive
HP and no inventory components, and valid finite positive persisted HP. An empty
loot set and an unsaved source are valid. Iterator failures, invalid batches and
replacement write failures roll back the drops, inventory removal, replacement
and ID watermark together. Replaying the captured snapshot preserves identity
without duplicating drops.

All affected chunks are pinned and their I/O gates are locked in coordinate
order by the worker. Tick admission/activation/deactivation uses nonblocking gate
checks. Earlier chunk saves finish before replacement, and pins prevent new
ordinary chunk saves from resurrecting pending sources. A raw-cache revision fence preserves
newer committed rows and deletions if an older load overlaps cache application.
This merges authoritative committed data instead of issuing an extra full reload.
`ReplaceCommittedSource` applies the durable replacement through the same cache
revision fence with an explicitly empty root-inventory list, so a late load cannot
restore the old corpse type or its removed inventory rows. It owns a copy of the
replacement JSON and preserves operation pins; runtime transformation remains an
owning-shard action after persistence succeeds.

## Completion, capacity and shutdown

Worker completion remains in its operation slot until the shard applies it;
there is no lossy completion channel. DB/capture failures are logged and use
1–60 second exponential backoff by `ecs.TimeState.Now`. Save-queue backpressure
retains the admitted operation and retries submission without reverting HP.

After commit, cleanup removes only captured exact inventory refs and handles,
then removes the source raw cache and entity. Cleanup has a shared budget of
**100** records per shard update. It does not recursively delete arbitrary
current owned inventories. Nested contents are already owned by durable JSON
before their old runtime containers disappear.

The shard installs at most **32** committed dropped records per tick. Standard
chunk activation has a global **64** dropped-item budget. Active chunks
materialize through `ObjectFactory`; inactive chunks retain raw records until
interest returns. The factory checks capacity for the complete inventory tree
before spawning. Insufficient capacity retains durable raw data for activation
retry. If free ECS handles remain, an oversized dropped inventory tree is retained
while activation continues to smaller drops within the same 64-record budget.
A completely full World still stops activation early.

Drops retain the existing runtime lifetime; retry does not renew their timestamp.
Expired raw drops are deleted by existing save workers before materialization.
That deletion has bounded retained operations, completion and retry, so delayed
replacement cannot introduce synchronous DB deletion into chunk activation.

Shutdown stops admission, pumps both destruction and expired-drop completions
under short shard locks, then performs ordinary chunk saving. Retry time advances
from the world's current `Now`; persisted runtime seconds do not advance. A
30-second drain limit reports pending failures explicitly. Pinned chunks are
counted as failed saves rather than persisting the quarantined source. A crash
after replacement commit restores the dropped records from PostgreSQL without
requiring an in-memory completion acknowledgement.

## Validation and measurements

Run with `CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache`:

```sh
ORIGIN_OBJECT_HEALTH_TEST_DSN='postgres://user@host/database?sslmode=disable' \
  go test ./internal/game ./internal/game/world ./internal/game/inventory \
    ./internal/core ./internal/ecs ./internal/ecs/systems -count=1
make test
go test ./internal/game -run '^$' -bench 'BenchmarkObject(Damage|Destruction)' -benchmem -count=5 -benchtime=200ms
go test ./internal/game/inventory -run '^$' -bench '^BenchmarkObjectLootCapture$' -benchmem -count=5 -benchtime=200ms
go test ./internal/game/world -run '^$' -bench '^BenchmarkObjectPersistence$' -benchmem -count=5 -benchtime=200ms
git diff --check
```

PostgreSQL tests use isolated temporary schemas. Acceptance includes streamed
rollback, repeated replacement, monotonic IDs, factory materialization, all
station roots, nested bags, inactive destinations, ECS exhaustion, the 32-item
budget, quarantine, shutdown and restart. Functional tests cover fractional/zero
damage, invalid inputs/handles, full admission, immutable retry, bounded cleanup,
owner-index paging, stale commands, station/build-site guards and cache fencing.
`AllocsPerRun` enforces zero allocations for zero/nonfatal damage and errors.

Measured 2026-10-08 on macOS/arm64, Apple M3 Max: medians of five samples.
Recorded activation measurements used the earlier budget of 32, before it was
raised to 64.
The baseline was captured before installing destruction runtime/queue guards.
New operations have no equivalent pre-implementation receiver benchmark.

| Existing operation | Before ns/op | After ns/op | B/op (unchanged) | allocs/op (unchanged) |
|---|---:|---:|---:|---:|
| HP read, 1k / 100k unrelated entities | 29.30 / 29.45 | 26.26 / 26.46 | 0 | 0 |
| HP set, 1k / 100k unrelated entities | 67.30 / 67.88 | 63.38 / 66.65 | 0 | 0 |
| Simple Build | 690.3 | 572.9 | 0 | 0 |
| Simple Serialize | 313.1 | 239.1 | 256 | 1 |
| Simple portable Capture | 484.8 | 369.9 | 320 | 4 |
| Container Build | 6288 | 6044 | 1009 | 15 |
| Container Serialize | 297.0 | 233.9 | 256 | 1 |
| Container portable Capture | 2414 | 2295 | 2011 | 20 |

| New operation | ns/op, 1k / 100k unrelated entities | B/op | allocs/op |
|---|---:|---:|---:|
| Nonfatal Apply, including fixed HP reset | 64.73 / 66.85 | 0 | 0 |
| Zero Apply, including fixed HP reset | 26.80 / 27.66 | 0 | 0 |
| Invalid Draw, including fixed HP reset | 27.14 / 27.06 | 0 | 0 |
| Owner enumeration with reusable buffer | 5.066 / 5.081 | 0 | 0 |
| Owned nested capture, with unrelated owners | 1859 / 1850 | 1720 | 20 |
| Admission with minimal quarantine callback | 320.3 / 308.7 | 0 | 0 |
| Factory materialization + teardown, simple | 2457 / 2643 | 240 / 249 | 9 |
| Factory materialization + teardown, nested bag | 3669 / 3895 | 384 / 396 | 12 |

Empty capture measured 133.1 ns, 96 B and 2 allocations. Nested capture without
unrelated owners measured 1787 ns, 1720 B and 20 allocations. Full destruction,
capture and materialization deliberately have no zero-allocation claim. Admission
benchmarks use a minimal quarantine callback and exclude production link/UI
cleanup, worker capture and SQL. No timing thresholds are enforced in CI.

The final Apply/admission/materialization rows above use an isolated
`GOMAXPROCS=1` run. A primary 100k-entity zero-damage sample was 33.6 ns; the
isolated repeat was 27.66 ns, so its apparent timing increase did not reproduce.
Existing baseline operations show no confirmed regression above 10%.

For large raw backlogs, activation retains the unvisited suffix without scanning
or copying it, and committed cache inserts/removals use an ID index. A retry
cursor visits untouched records before revisiting retained failures. This prevents
expired drops awaiting a failed DB deletion from consuming every pass ahead of
valid drops. Removal
holes consume the same 64-entry activation budget. Fences only live while a
protected load is in flight, and pinned inactive chunks regain LRU eligibility
after release. Five-sample backlog checks measured:

| Operation | ns/op, 100 / 10,000 raw records | B/op (same) | allocs/op (same) |
|---|---:|---:|---:|
| Committed raw cache update | 16.69 / 16.60 | 0 | 0 |
| Full-ECS live-drop retry | 720.0 / 723.4 | 96 | 6 |
| Expired-drop retry, 32 visited records | 19097 / 19736 | 3072 | 192 |

These activation allocations are bounded metadata decoding, independent of the
unvisited backlog. `BuildForChunk` parses normal drop metadata once, has the same
restore allocation count as public `Build`, and defers expired-row deletion to
workers. A later review found and corrected nested-tree truncation, missing
approach cancellation and activation starvation; the corresponding regressions
include real PostgreSQL destruction/deactivation/save/reactivation. The optional
race run could not link because the local macOS SDK rejects `arm64e.x1` metadata;
it is not counted as a passed check. The shutdown deadline branch is tested with
a zero test timeout, while production retains the 30-second limit.

The existing permanent-death character-save ordering is outside the chunk gate:
an already accepted player inventory snapshot can recreate soft-deleted corpse
owner inventory rows after deletion. The deleted object stays absent, so these
rows do not recreate a visible corpse or duplicate ground loot. General
cross-owner inventory moves also retain their existing persistence boundary.
This step guarantees the atomic source-to-drops replacement, without changing
those older character/inventory writer contracts.

## Review fixes, 2026-10-08

The three review findings are covered by complete-tree persistence/cleanup,
target-specific approach cancellation, and fair bounded activation retries.
The cursor's cycle end is fixed: a continuous stream of committed appends cannot
postpone retries of older failures indefinitely. Cursor state adds two integers
per chunk and does not allocate during activation.

Partial-capacity regressions also cover a three-handle bag preceding a two-handle
drop with only two free handles, and 129 oversized bags preceding a fitting drop.
The latter advances in bounded passes of 64, 64 and 2 records. Deferred trees keep
their raw inventory payloads, allocate no partial entities, and restore exactly
once when capacity becomes available. Both regressions failed before the fix and
passed 20 consecutive runs afterward.

Five-sample `BenchmarkActivationRawBacklog` checks with `GOMAXPROCS=1` and 100ms per
sample measured full-ECS retries at 776.7 to 787.2 ns for 100 raw records and 748.6
to 760.0 ns for 10,000 records. Both retain 96 B/op and 6 allocs/op. Expired-record
retry medians were 15,269 to 16,252 ns and 20,384 to 22,203 ns respectively, with
unchanged bytes and median allocation counts; no median exceeded a 10% increase.

Targeted game/world/inventory/lifecycle/core/ECS/system/combat tests and `make test`
passed with both `ORIGIN_OBJECT_HEALTH_TEST_DSN` and
`ORIGIN_CHARACTER_SAVE_TEST_DSN` against a temporary PostgreSQL cluster. Tests use
isolated schemas, including destruction with three nested bags, modification of
the deepest contents, deactivation, ordinary save and reactivation. Generated
protobuf/sqlc checksums are unchanged by the review fixes. Independent read-only
reviews of tree serialization, activation and approach cancellation found no
remaining blockers.

Paired before/after measurements for these fixes use `GOMAXPROCS=1`,
`-benchmem -count=5 -benchtime=100ms`, with the same fixtures:

| Operation | Before ns/op | After ns/op | B/op (unchanged) | allocs/op (unchanged) |
|---|---:|---:|---:|---:|
| Simple Build | 791.9 | 765.3 | 0 | 0 |
| Simple Serialize | 318.6 | 292.8 | 256 | 1 |
| Simple portable Capture | 503.4 | 464.0 | 320 | 4 |
| Container Build | 8015 | 7904 | 1008 | 15 |
| Container Serialize | 329.7 | 292.1 | 256 | 1 |
| Container portable Capture | 2977 | 3035 | 2008 | 20 |
| Full-ECS drop retry, 100 / 10,000 raw rows | 1009 / 967.8 | 1004 / 969.3 | 96 | 6 |

An isolated 300ms repeat of Container/Capture measured 3531 ns/op, with the same
2008 B/op and 20 allocations. Its additional roughly 0.55 microseconds over the
baseline is the cost of checking all item identities, known bag requirements,
refs and cycles. This correctness work belongs to the already allocating snapshot
path; zero/nonfatal/error `Apply` retains zero bytes and allocations. The expired
backlog benchmark now reaches partial spans at the end of each cursor cycle:
100 rows average 25 visits instead of repeating the same 32 forever. Its lower
average allocation count therefore reflects different work per pass, rather
than a claimed per-record optimization. Allocation equivalence tests use full
32-record spans in 128- and 10,240-row backlogs. One final 100,000-entity nonfatal
Apply sample showed a 125.9 ns median; a separate five-sample 300ms repeat measured
84.98 ns, close to the preceding 83.96 ns measurement. The timing increase did
not reproduce; both runs retained 0 B/op and 0 allocs/op.
