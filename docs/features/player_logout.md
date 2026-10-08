# Player logout retention

Implementation and verification report, 2026-10-08.

## Runtime contract

Closing a connection detaches its living body immediately. Every normal logout,
including `DisconnectDelay=0`, goes through `DetachedEntities`; removal is allowed
only after the base delay, snapshot retry deadline and all registered policies
permit it. Death and shutdown retain their specialized lifecycle paths. Shutdown
does not wait for combat deadlines.

Each shard owns a `PlayerLogoutService` with a copied, immutable list of
`ecs.LogoutPolicy` values. `LogoutContext` contains exact Handle/EntityID, the
original disconnect data and a copy of the tick's `TimeState`. Policies only read
state: no mutation, I/O, world traversal or logging. A blocked policy returns a
retry hint; an unknown deadline or policy error retries after one second. Hints
schedule a check and never authorize deletion. Additional game systems can add a
policy at shard construction and request an earlier check through
`PlayerLogoutService.RequestRecheck` under the owning shard lock.

The default policies are base disconnect delay and combat. Combat retains a body
while `UnixMs < LastCombatEventAtUnixMs + 30_000`, or while a nonzero KO deadline
satisfies `UnixMs <= KOUntilUnixMs`. Lying without active KO does not block logout.
Invalid identity, health or time retains the body with a sentinel error.

## Combat events and ordering

`CombatActivityState` replaces the creature receiver's target registration; there
is no second combat registry. Each exact generational Handle owns its EntityID
and `{HasEvent, LastCombatEventAtUnixMs}`. `HasEvent` preserves an accepted event
at UnixMs zero. A new event records `max(previous, eventTime)`.

Accepted starts use action `Combat` metadata, without branches by action ID.
Successful melee completion records the attacker even for a miss or an object
hit. The common creature commit records each victim, including friendly fire,
active KO and zero damage; standalone `CreatureDamageService.Apply` uses this
same commit. Zero damage still performs no health write or gameplay notification.
Rejected commands, failed preparation/payment, hunger, administrative damage,
regeneration and movement do not mark combat. Cancelling an accepted start keeps
that start's event. Registration, timestamp validity and deadline overflow are
checked before start acceptance or completion payment; same-lock stamp commits
cannot fail.

Every event requests rechecking, even when its timestamp is unchanged: a second
hit in the same tick can start KO. Timed melee completes at priority 315, death
runs at 470 and detached expiry at 950. Expiry sees the final health and combat
state of that tick. Combat deadlines use wall-based UnixMs. The scheduler uses
runtime `TimeState.Now`; wall-based hints are capped at one second before duration
conversion, so clock jumps cannot install a stale long runtime delay.

## Queue and final capture

`DetachedEntities` owns a typed indexed min-heap ordered by
`(NextCheckAt, EntityID, Handle)`. Setup prepares one permanent record per exact
Handle and reserves heap/pending-map capacity. Connected records have no heap
membership. Repeated detachment does not reset the original `ExpirationTime`.
Reattach removes pending membership only after successful client attachment.
Despawn and synchronous EntityHealth removal release preparation, including
conversion of the same Handle to a corpse.

Expiry first extracts at most 256 due candidates into a fixed buffer, then checks
each once. Blocked, stale and invalid candidates all consume that budget. There
is no detached-map traversal. Retry hints are scheduled no earlier than the next
tick. `RequestRecheck` advances a pending check but cannot bypass base delay or
`SaveRetryAt`. Periodic and final snapshot failures update retry through the same
scheduler API.

Immediately before final snapshot, expiry revalidates alive identity, detached
state and policies. Only accepted enqueue permits inventory, spatial, tracking
and entity cleanup. Capture/enqueue failure retains all state and sets
`SaveRetryAt = Now + 5s`, without changing the original base deadline or an older
valid pending snapshot. Deferred AOI cleanup flushes at most 128 IDs per call and
one call per tick. A partial batch becomes eligible after one runtime second;
the per-tick budget can add backlog delay. A shard-owned retirement token prevents
an old batch from unregistering a new AOI preparation,
including the interval before the replacement ECS body is spawned.

## Reconnect, transfer and restart

Client identity checks and binding changes follow `shard.mu -> ClientsMu`. A late
disconnect from an older connection cannot detach a new session. Disconnect keeps
the existing action cancellation, movement stop, UI, links and carry handling.

Runtime cache and transfer snapshots own a common `{Health, CombatState}` copy.
Priority is explicit runtime state, then cache, then DB. The selected state is
validated and restored before publication. Failed spawn does not consume the
cache. Transfer and rollback restore participant state before attaching a client
or queueing a closed client's body for logout; a socket closing after source
capture does not cancel restoration of that vulnerable body.

Combat timestamps and KO deadlines are deliberately runtime-only. Restart resets
combat activity. DB-only loading with SHP=0 starts a new 60-second KO; positive SHP
does not restore an earlier active KO. This is the approved restart behavior.
Reconnect, transfer and rollback within the running server preserve both values.

## Verification scope

Regression tests cover accepted starts, strike/sweep, miss, zero damage, multiple
victims, equal/backwards event times, rejected completion and payment; independent
base/combat/KO/noncombat blockers; deadline equality, overflow and invalid state;
queue ordering, deduplication, generation reuse, bounded checks and retry wakeups.
An actual timed-cycle test proves hit/death/logout priority ordering.

Lifecycle tests cover zero-delay disconnect, hits received after disconnect,
capture/enqueue failure and later single cleanup, stale sessions, failed/closed
reattach, runtime cache priority and AOI reuse. Transfer/rollback tests exercise
production source capture and compose shared setup, participant restore and
attachment stages, including closing the connection after capture. They do not
run the full DB-backed `executeTransfer` path end to end.

Prepared event validation/marking, policy evaluation, heap rescheduling and
bounded blocked checks have allocation assertions. Capture and cleanup are
measured separately and are outside that zero-allocation contract. No new ECS
component, definition, migration, SQL query, protobuf field or dependency is used.

Final checks passed with `CGO_ENABLED=0` and the configured cache:

- `go test ./internal/game ./internal/ecs ./internal/ecs/systems ./internal/playerstate -count=1`.
- `make test`, including protobuf and sqlc regeneration; generated outputs have
  no diff.
- Benchmark suites with `-benchmem -count=5`, including the sequential isolated
  regression check and 1,000/30,000-player policy/scheduler workloads.
- Read-only code review and `git diff --check`. Review found and verified fixes
  for deferred AOI cleanup during a new spawn and a closed connection after
  transfer capture.

## Performance procedure

Apple M3 Max, darwin/arm64. Go benchmarks use `CGO_ENABLED=0` and
`GOCACHE=/private/tmp/origin-combat-go-cache`, `-benchmem -count=5`; reported timings
are medians. Timing thresholds are diagnostic and are not CI assertions. The
baseline is the unchanged HEAD tree with the identical idle-expiry workload.

```sh
go test ./internal/game ./internal/ecs ./internal/ecs/systems -run '^$' \
  -bench 'Benchmark(CreatureDamageService|MeleeExecution|MeleeActionCosts|DetachedExpiryIdle|DetachedSchedule|DetachedPreparation|DetachedExpiryCleanup|PlayerLogoutPolicies|CombatActivityAndLogoutWake|CombatActivityPrepared|CharacterSaveCapture|CombatLogoutRegistryMemory)$' \
  -benchmem -benchtime=100ms -count=5
```

The initial broad comparison showed about 30% growth even in unchanged
cost-only code. Sequential isolated baseline/current runs were therefore used to
verify combat regressions, without concurrent builds or tests. The unchanged
baseline itself shifted by a similar amount between runs; initial timing growth
cannot be attributed to the change. Allocation counts stayed unchanged throughout.


## Measurements

The existing prepared combat operations in the following table retain
**0 B/op, 0 allocs/op**. Values use the full sequential isolated rerun;
populations are unrelated entities/colliders.

| Existing operation | Before ns/op | After ns/op | Change |
|---|---:|---:|---:|
| `CreatureDamageService/world_1000/empty/fresh` | 198.9 | 209.8 | +5.5% |
| `CreatureDamageService/world_1000/empty/pending` | 191.8 | 196.2 | +2.3% |
| `CreatureDamageService/world_1000/partial/fresh` | 229.1 | 226.3 | -1.2% |
| `CreatureDamageService/world_1000/partial/pending` | 216.6 | 214.3 | -1.1% |
| `CreatureDamageService/world_1000/full/fresh` | 361.7 | 362.6 | +0.2% |
| `CreatureDamageService/world_1000/full/pending` | 348.5 | 350.5 | +0.6% |
| `CreatureDamageService/world_1000/KO_fresh` | 289.3 | 298.2 | +3.1% |
| `CreatureDamageService/world_1000/active_KO_fresh` | 263.2 | 270 | +2.6% |
| `CreatureDamageService/world_1000/invalid_draw` | 55.85 | 57.91 | +3.7% |
| `CreatureDamageService/world_1000/invalid_health` | 42.63 | 42.96 | +0.8% |
| `CreatureDamageService/world_1000/dead_unavailable` | 42.75 | 42.59 | -0.4% |
| `CreatureDamageService/world_100000/empty/fresh` | 194.5 | 207.5 | +6.7% |
| `CreatureDamageService/world_100000/empty/pending` | 183.5 | 196.5 | +7.1% |
| `CreatureDamageService/world_100000/partial/fresh` | 228.2 | 227.3 | -0.4% |
| `CreatureDamageService/world_100000/partial/pending` | 217.4 | 215 | -1.1% |
| `CreatureDamageService/world_100000/full/fresh` | 354.4 | 367 | +3.6% |
| `CreatureDamageService/world_100000/full/pending` | 340.6 | 350 | +2.8% |
| `CreatureDamageService/world_100000/KO_fresh` | 295.1 | 295.3 | +0.1% |
| `CreatureDamageService/world_100000/active_KO_fresh` | 259.9 | 268.8 | +3.4% |
| `CreatureDamageService/world_100000/invalid_draw` | 55.95 | 57.39 | +2.6% |
| `CreatureDamageService/world_100000/invalid_health` | 43.05 | 42.68 | -0.9% |
| `CreatureDamageService/world_100000/dead_unavailable` | 42.76 | 42.6 | -0.4% |
| `MeleeExecution/world_1000/miss` | 610.4 | 600.8 | -1.6% |
| `MeleeExecution/world_1000/nearest` | 2799 | 2856 | +2.0% |
| `MeleeExecution/world_1000/sweep_1` | 964.7 | 946.4 | -1.9% |
| `MeleeExecution/world_1000/sweep_10` | 3949 | 3978 | +0.7% |
| `MeleeExecution/world_1000/sweep_512` | 176157 | 179353 | +1.8% |
| `MeleeExecution/world_1000/full_armor` | 5549 | 5681 | +2.4% |
| `MeleeExecution/world_1000/invalid_health` | 3785 | 3826 | +1.1% |
| `MeleeExecution/world_1000/admission_cancel` | 6013 | 6141 | +2.1% |
| `MeleeExecution/world_100000/miss` | 646 | 640.3 | -0.9% |
| `MeleeExecution/world_100000/nearest` | 2817 | 2913 | +3.4% |
| `MeleeExecution/world_100000/sweep_1` | 967.3 | 1011 | +4.5% |
| `MeleeExecution/world_100000/sweep_10` | 3986 | 4067 | +2.0% |
| `MeleeExecution/world_100000/sweep_512` | 191545 | 190068 | -0.8% |
| `MeleeExecution/world_100000/full_armor` | 5681 | 5715 | +0.6% |
| `MeleeExecution/world_100000/invalid_health` | 3852 | 3909 | +1.5% |
| `MeleeExecution/world_100000/admission_cancel` | 6122 | 6411 | +4.7% |
| `MeleeActionCosts` | 474.2 | 483.6 | +2.0% |

No combat case exceeded 10% in the isolated rerun. Cost-only code, unchanged by
this feature, measured +2.0%. The extra successful-hit work consists of prepared
combat timestamp validation/marking and one scheduler wakeup, without allocations
or additional world traversal.

| Idle expiry with pending bodies | Before ns/op | After ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| 1,000 | 8,359 | 36.62 | 0 | 0 |
| 30,000 | 259,151 | 34.90 | 0 | 0 |

The old workload visited the pending map each pass. The new idle pass checks the
heap minimum; work no longer grows with the number of retained bodies.

| Prepared operation (rotating player records) | 1,000 ns/op | 30,000 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| `PlayerLogoutPolicies/allow` | 114.2 | 153.9 | 0 | 0 |
| `PlayerLogoutPolicies/combat` | 114.6 | 154.8 | 0 | 0 |
| `PlayerLogoutPolicies/KO` | 114.9 | 163.4 | 0 | 0 |
| `PlayerLogoutPolicies/unknown` | 115.3 | 151.3 | 0 | 0 |
| `PlayerLogoutPolicies/error` | 115.3 | 158.2 | 0 | 0 |
| `CombatActivityAndLogoutWake/connected_fresh` | 43.49 | 51.74 | 0 | 0 |
| `CombatActivityAndLogoutWake/detached_fresh` | 80.82 | 110.7 | 0 | 0 |
| `CombatActivityAndLogoutWake/detached_equal_time` | 69.01 | 96.49 | 0 | 0 |
| `DetachedSchedule/idle` | 4.69 | 4.683 | 0 | 0 |
| `DetachedSchedule/reschedule` | 236.1 | 315.5 | 0 | 0 |
| `DetachedSchedule/due_blocked` | 54884 | 82643 | 0 | 0 |

`due_blocked` extracts and reschedules 256 candidates per operation. Policy and
event samples rotate through actual prepared player contexts; they include the
map/cache footprint instead of repeatedly reading one hot key.

| Stage outside prepared-operation allocation contract | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Snapshot capture, empty inventories | 3595 | 2666 | 44 |
| Snapshot capture, fixed inventories | 3571 | 2666 | 44 |
| Expiry cleanup, 256 bodies, stub hooks | 146023 | 448 | 1 |

Snapshot capture measures the existing serializer/enqueue without DB. Cleanup
measures scheduler checks, link cleanup and despawn with a stub external hook;
production inventory/AOI work is not included in that figure.

| Prepared combat and logout registries | Retained B/player | Approximate total retained | Cumulative setup B/op |
|---|---:|---:|---:|
| 1,000 players | 410.2 | 0.41 MB | 734544 |
| 30,000 players | 432.8 | 12.98 MB | 2.44965e+07 |

Resource-only retained memory excludes World, component storages, inventories,
notification queues and cached snapshots. Queue records are 48 bytes plus an
8-byte heap pointer per reserved slot on arm64; the combined measurement includes
identity indexes, the pending-map reservation and the shared combat registry.
Cumulative preparation includes discarded maps/slices during geometric growth; it
is not the retained footprint. Preparation may allocate; all measured operations
on prepared records, including errors, remain allocation-free.
