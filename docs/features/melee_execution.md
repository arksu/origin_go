# Melee execution: strike and sweep

Implementation and acceptance report, 2026-10-08.

## Runtime contract

`ShardManager` compiles the immutable equipment catalog and all existing combat
action selectors before creating shards. Each shard owns its equipment resolver,
creature/object receivers and one melee executor. Definitions select the weapon,
sector, hit mode and multiplier; the handler has no branches by action ID or weapon
kind. Login, transfer and rollback use the shared spawn preparation hook. Reattach
validates the surviving body before removing detached/reconnect state.

The existing `CyclicActionSystem` completes both presets on the sixth tick, at
priority 315, before `PlayerDeathSystem` at 470. The starting direction stays fixed;
position, base STR, equipment, Quality and armor are read at completion. Friendly
fire is enabled. Valid contacts are ordered by distance and then EntityID; nearest
does not prioritize creatures over objects. Invalid health, identity or required
preparation rejects the whole action, including for other geometric contacts of a
nearest attack.

Objects with `indestructible: true` are skipped before nearest/all selection and
do not appear in AttackResult. They do not prevent hitting another eligible
contact. An immune-only sector completes as the existing paid miss. The object
damage receiver independently rejects direct damage to these targets.

Completion performs validation and calculation for all selected targets, then
reserves every lethal object, allocates an EventID, and calls the existing stamina
and cooldown payment once. A failed preparation owns its rollback; a nested busy
attempt cannot discard the outer preparation. After payment, all health changes
and pending-destruction markers commit before action completion and quarantine.
Prepared values exist only within that synchronous shard-locked call. A miss uses
the same paid completion. Cancellation before completion has no damage, payment,
new cooldown or AttackResult.

Reservations use the existing 512-operation destruction queue, a fixed free list
and a ticket per slot. Reservation changes no HP, visibility, links, UI or carry
state; workers see only finalized operations. Cancellation releases its slot and
source pin exactly once. Each committed destruction retains its existing separate
transaction and retry behavior. A persistence failure never replays the attack.
`committedDroppedActivationBudget` remains 64.

## Bounds and memory

Each query permits 1,024 cells, 4,096 raw collider memberships (before
deduplication) and 512 selected hits. Exceeding a budget rejects the whole action;
sweep never returns a partial success. Candidate/contact buffers hold 4,096
records; hit plans and result buffers hold 512. Hit preparation has no I/O,
goroutines, inventory traversal or lazy target preparation.

Bounded fallback queries iterate a dense list of live cell keys. The existing
cell map stores an 8-byte reverse position, with a 16-byte dense key per occupied
slot on arm64. Removal swaps the tail in O(1); retained slice capacity does not
increase query work. No additional map is used. This protects against expensive
map iteration after a large occupied world becomes sparse.

The benchmark reports **274,608 bytes (268.17 KiB) per shard** for the fixed melee
executor, contact backing storage and bounded spatial candidate buffer. This
excludes ECS, shared definitions, existing persistence queues and variable spatial
index storage. No buffer is sized by full ECS capacity.

## Network contract

AttackResult is sent synchronously after commit to the attacker and the attacker's
current observers. Exact live identities deduplicate recipients; the current
client, layer and epoch are rechecked. One monotonic server-wide uint64 sequence
allocates IDs before payment; gaps are allowed, wraparound is rejected. Each shard
enqueues results in completion order, preserving increasing EventIDs for each
current-epoch listener. There is no cross-layer observation stream.

All recipients receive the full ordered hit list, including zero damage and
unknown target IDs; misses contain an empty list. Serialized bytes belong to the
message and may be reused by clients sharing an epoch. SendCritical failure does
not undo gameplay or repeat damage.

The client validates exact nonzero uint64 IDs, epoch, finite nonnegative damage
and target uniqueness. It accepts only increasing EventIDs, resetting its
watermark on connection/session or epoch changes. This receiver predicts no
health, spawns no targets and renders no new hit effects. Protocol details are in
`combat_protocol.md`. Recovery, ranged attacks, wounds, effective-attribute
modifiers remain separate work. Combat logout uses the shared read-only logout
policies and indexed due queue described in [player_logout.md](player_logout.md).
KO deadlines and combat event times remain runtime-only by design; DB-only restore
starts a new KO for zero SHP and does not restore an active KO for positive SHP.

## Verification

Passed checks:

- Targeted game/world/inventory/core/combat/playerstate/ECS/system tests, including
  actual timed cycles, fractional damage, sequential KO/death, live weapon/armor
  changes, cancellation, stale completion, reentry and rollback of admission.
- Exact/overflow cell, raw-visit and selected-hit budgets; deterministic geometry;
  zero allocations for prepared nonlethal hits, miss, invalid state and limits.
- Spawn-before-publication, detached reattach/retry, transfer/rollback shared
  preparation hooks and generational cleanup. Transfer tests exercise production
  source capture, shared spawn/attachment and participant restore; they do not
  constitute a complete DB-backed `executeTransfer` test.
- Real PostgreSQL object-health/destruction tests via
  `ORIGIN_OBJECT_HEALTH_TEST_DSN` in isolated temporary schemas.
  `TestMeleePostgresSweepDestructionRetryAndRestart` passed five repeats: one sweep
  destroys two sources, one commits while the other rolls back after its first SQL
  batch, retry preserves IDs/coordinates/time/payload, six drops preserve nested
  bag contents, and restart restores no duplicates. Stamina/result happen once.
- Server-wide `make test`, including sqlc/protobuf generation; generated SQL and
  protobuf outputs have no diff.
- Client `type-check`, `test:actions`, `test:direction-aim` (10 tests),
  `test:combat-protocol` (9 tests), `test:knockout` (5 tests) and ESLint on changed
  client files.
- Diff review and `git diff --check`. Review identified the sparse-map churn cost;
  the dense-key fix was subsequently reviewed and tested.

The required global client `npm run lint` was run and failed with **479 pre-existing
errors** in generated protobuf declarations, the vendored Basis decoder and
unrelated client/test files. `--fix` changed none of the 230 checked client files.
Those errors were not folded into this feature.

## Performance procedure

Apple M3 Max, darwin/arm64, Go benchmark parallelism 16. All tables use the median
of five runs with `-benchmem -benchtime=100ms -count=5`. Timing is diagnostic and
has no CI threshold; functional, allocation and work-budget assertions remain in
tests. The baseline was captured before receiver/queue refactoring. Target resets
and cancellation are included where needed to make repeated samples comparable.
Network samples include actual WebSocket writing/draining and are not estimates
of shard-lock duration. Quarantine samples have no interested UI/carry clients or
active chunk and include reinstating a collider for the next iteration.

```sh
export CGO_ENABLED=0
export GOCACHE=/private/tmp/origin-combat-go-cache
go test ./internal/game -run '^$' \
  -bench 'Benchmark(SectorResolver|CreatureDamageService|ObjectDamageApply|CombatEquipment)$' \
  -benchmem -benchtime=100ms -count=5
go test ./internal/game -run '^$' \
  -bench '^Benchmark(MeleeExecution|MeleeActionCosts|MeleeActionCompletion|MeleeObjectQuarantine|ObjectDestructionReservation|SectorResolverBounded)$' \
  -benchmem -benchtime=100ms -count=5
go test ./internal/game -run '^$' -bench '^BenchmarkAttackResultNetworkFanout$' \
  -benchmem -benchtime=100ms -count=5
go test ./internal/core -run '^$' \
  -bench 'BenchmarkColliderSpatial(Locality|BoundedFallbackChurn|CellTransitions)$' \
  -benchmem -benchtime=100ms -count=5
```

## Measurements

Final baseline comparisons and executor/fanout tables follow below.

All foundation cases retained **0 B/op and 0 allocs/op**. Asterisked rows use
the required isolated rerun; broad-run values and the reason for rerun are recorded
after the table.

| Existing benchmark | Before ns/op | After ns/op | Change |
|---|---:|---:|---:|
| `CombatEquipment/world_1000/empty/armor` * | 27.95 | 27.86 | -0.3% |
| `CombatEquipment/world_1000/empty/weapon` * | 37.24 | 37.79 | +1.5% |
| `CombatEquipment/world_1000/partial/armor` | 41.1 | 40.66 | -1.1% |
| `CombatEquipment/world_1000/partial/weapon` | 70.77 | 74.85 | +5.8% |
| `CombatEquipment/world_1000/full/armor` | 119.2 | 117.4 | -1.5% |
| `CombatEquipment/world_1000/full/weapon` | 145.6 | 152.7 | +4.9% |
| `CombatEquipment/world_1000/two_weapons/armor` * | 45.03 | 45.09 | +0.1% |
| `CombatEquipment/world_1000/two_weapons/weapon` * | 86 | 82.97 | -3.5% |
| `CombatEquipment/world_100000/empty/armor` * | 28.33 | 27.86 | -1.7% |
| `CombatEquipment/world_100000/empty/weapon` * | 38.55 | 38.33 | -0.6% |
| `CombatEquipment/world_100000/partial/armor` | 40.34 | 40.78 | +1.1% |
| `CombatEquipment/world_100000/partial/weapon` | 69.88 | 72.78 | +4.1% |
| `CombatEquipment/world_100000/full/armor` | 118.5 | 118.1 | -0.3% |
| `CombatEquipment/world_100000/full/weapon` | 147 | 151.4 | +3.0% |
| `CombatEquipment/world_100000/two_weapons/armor` * | 45.05 | 44.92 | -0.3% |
| `CombatEquipment/world_100000/two_weapons/weapon` * | 85.9 | 84.15 | -2.0% |
| `CreatureDamageService/world_1000/empty/fresh` | 139.6 | 148.5 | +6.4% |
| `CreatureDamageService/world_1000/empty/pending` | 130.7 | 140.8 | +7.7% |
| `CreatureDamageService/world_1000/partial/fresh` | 165 | 168.8 | +2.3% |
| `CreatureDamageService/world_1000/partial/pending` | 158.1 | 160.1 | +1.3% |
| `CreatureDamageService/world_1000/full/fresh` | 259.8 | 257.1 | -1.0% |
| `CreatureDamageService/world_1000/full/pending` | 253.3 | 250.1 | -1.3% |
| `CreatureDamageService/world_1000/KO_fresh` | 208.1 | 206.1 | -1.0% |
| `CreatureDamageService/world_1000/active_KO_fresh` | 188.7 | 187.1 | -0.8% |
| `CreatureDamageService/world_1000/invalid_draw` | 39.58 | 41.2 | +4.1% |
| `CreatureDamageService/world_1000/invalid_health` | 31.5 | 31.47 | -0.1% |
| `CreatureDamageService/world_1000/dead_unavailable` | 31.81 | 31.43 | -1.2% |
| `CreatureDamageService/world_100000/empty/fresh` | 138.7 | 143.6 | +3.5% |
| `CreatureDamageService/world_100000/empty/pending` | 130.3 | 136.7 | +4.9% |
| `CreatureDamageService/world_100000/partial/fresh` | 164 | 170.7 | +4.1% |
| `CreatureDamageService/world_100000/partial/pending` | 156.3 | 163.8 | +4.8% |
| `CreatureDamageService/world_100000/full/fresh` | 259.9 | 262.1 | +0.8% |
| `CreatureDamageService/world_100000/full/pending` | 250.3 | 255.3 | +2.0% |
| `CreatureDamageService/world_100000/KO_fresh` | 212.6 | 208.3 | -2.0% |
| `CreatureDamageService/world_100000/active_KO_fresh` | 189 | 188.4 | -0.3% |
| `CreatureDamageService/world_100000/invalid_draw` | 39.69 | 41.35 | +4.2% |
| `CreatureDamageService/world_100000/invalid_health` | 31.07 | 31.24 | +0.5% |
| `CreatureDamageService/world_100000/dead_unavailable` | 31.31 | 31.23 | -0.3% |
| `ObjectDamageApply/1000/Nonfatal` | 65.42 | 69.37 | +6.0% |
| `ObjectDamageApply/1000/Zero` | 27.36 | 29.08 | +6.3% |
| `ObjectDamageApply/1000/Invalid` | 27.89 | 26.4 | -5.3% |
| `ObjectDamageApply/100000/Nonfatal` | 68.51 | 68.31 | -0.3% |
| `ObjectDamageApply/100000/Zero` * | 27.34 | 29.46 | +7.8% |
| `ObjectDamageApply/100000/Invalid` | 27.73 | 27.96 | +0.8% |
| `SectorResolver/1000` | 407.6 | 435.5 | +6.8% |
| `SectorResolver/100000` | 426 | 436.4 | +2.4% |

The broad final run showed +26.5/+29.5% for unchanged empty-equipment armor,
+12.3/+11.8% for unchanged empty-equipment weapon selection, +10.4/+10.9% for
two-weapon selection and +12.1% for 100k-world zero object damage. Isolated runs
of those same cases returned equipment differences from -3.5% to +1.5% and
zero object damage +7.8% (29.46 ns versus 27.34 ns). The >10% regression was not
confirmed. Earlier receiver measurements also eliminated larger value-copy
overhead by sharing pointer-filled calculations with standalone Apply; formulas
and validation remain shared with the by-value coordinator plans.

| Prepared executor scenario | 1k ns/op | 100k ns/op | Cells | Raw visits | Unique candidates | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| miss | 424.5 | 461.3 | 16 | 4 | 1 | 0 | 0 |
| nearest | 2041 | 2078 | 16 | 44 | 11 | 0 | 0 |
| sweep_1 | 697.9 | 711.2 | 16 | 8 | 2 | 0 | 0 |
| sweep_10 | 2900 | 2972 | 16 | 44 | 11 | 0 | 0 |
| sweep_512 | 129834 | 136813 | 16 | 2052 | 513 | 0 | 0 |
| full_armor | 4166 | 4187 | 16 | 44 | 11 | 0 | 0 |
| invalid_health | 2818 | 2811 | 16 | 44 | 11 | 0 | 0 |
| admission_cancel | 4411 | 4454 | 16 | 44 | 11 | 0 | 0 |

Each population adds 1,000 or 100,000 unrelated colliders outside the query.
Nearest validates ten eligible contacts but calculates damage for only one.
The full-armor case uses ten armor slots on each of ten selected creatures.
Admission/cancel reserves and returns ten fatal-object slots with fixture pins;
no worker, DB or quarantine cost is included.

| Other stage | Median ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `MeleeActionCosts` | 349.8 | 0 | 0 |
| `MeleeActionCompletion` | 2141 | 281 | 5 |
| `MeleeObjectQuarantine/1000` | 754.7 | 264 | 4 |
| `MeleeObjectQuarantine/100000` | 786.3 | 264 | 4 |
| `ObjectDestructionReservation/world_1000/occupied_0` | 179 | 0 | 0 |
| `ObjectDestructionReservation/world_1000/occupied_511` | 196.7 | 0 | 0 |
| `ObjectDestructionReservation/world_1000/occupied_512` | 2.4 | 0 | 0 |
| `ObjectDestructionReservation/world_100000/occupied_0` | 181.8 | 0 | 0 |
| `ObjectDestructionReservation/world_100000/occupied_511` | 199.4 | 0 | 0 |
| `ObjectDestructionReservation/world_100000/occupied_512` | 2.39 | 0 | 0 |
| `SectorResolverBounded/1000` | 438 | 0 | 0 |
| `SectorResolverBounded/100000` | 440.3 | 0 | 0 |
| `AttackResultNetworkFanout/1` | 14702 | 984 | 9 |
| `AttackResultNetworkFanout/10` | 43065 | 7704 | 46 |
| `AttackResultNetworkFanout/100` | 464720 | 74759 | 406 |

Cost-only baseline was 359.3 ns/op, 0 B/op, 0 allocs/op; final cost-only was
349.8 ns/op (-2.6%) with the same allocations. Full completion includes
reinstating the active action and its existing state-message construction.
Quarantine includes collider reinstatement. The completion/quarantine/network
allocations are outside the prepared-hit zero-allocation contract. Bounded sector
samples visit 20 cells, 32 raw memberships and 15 unique candidates in both world
sizes.

| Spatial churn/mutation control | Before ns/op | After ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| One-cell bounded fallback, fresh | 35.88 | 16.47 | 0 | 0 |
| Same fallback after removing 100,000 cells | 42651 | 16.63 | 0 | 0 |
| Existing local query, 1k | 217.7 | 215.5 | 0 | 0 |
| Existing local query, 100k | 227.8 | 229.3 | 0 | 0 |
| Four-cell membership transition | 212.9 | 214.9 | 0 | 0 |

The churn query reports one cell, one raw membership and one candidate in both
cases. Dense-key bookkeeping removes the historical-map-capacity scan. Legacy
query and movement-control costs stay within 1%; retained index memory and hit
buffers do not change the iteration bound.
