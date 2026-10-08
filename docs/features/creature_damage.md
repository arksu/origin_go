# Creature combat damage receiver

## Implemented boundary

`internal/game/creature_damage_service.go` applies caller-calculated raw damage
(`Draw`) to real `EntityHealth` components, using the target's current equipment
armor. It has no weapon, action, skill or attacker-specific rules. Callers can
later reuse it for melee, projectiles and AI.

This is a dependency of the first working combat stage described in
[combat_final.md](combat_final.md). Combat handlers remain unregistered. Object
HP/destruction, hit geometry, effects, event ordering, combat logout deadlines
and durable KO timers are separate work. Public attacks require both creature
and ordinary-object recipients to be ready.

## Ownership and preparation

Construct `NewCreatureDamageService(world, equipment)` under exclusive world
access. The equipment resolver must belong to that World. The constructor
prepares component storages and captures resource references; recreate the
service when replacing the World, resources, storages or definitions.

Before exposing a target to attacks, call `PrepareTarget(handle)` under its
owning shard lock. Preparation validates its identity and finite nonnegative
health, registers the exact generational handle, and allocates notification
records/capacity. Repeating preparation is safe. Admission allocations are
allowed; `Apply` never prepares a target lazily.

The service is not yet installed in shard construction or spawn. Future combat
integration must prepare after successful spawn/setup and before publishing
the target, including transfer/rollback and reattachment. Detach retains the
registration. Synchronous despawn and health-removal callbacks retire it; no
world scan is needed. Queue cleanup also covers legacy visual producers on
every World despawn.

## Applying damage

Call `Apply(target, rawDamage)` under the owning shard lock, before
`PlayerDeathSystem` (priority 470). Calls are sequential and use the current
`TimeState.UnixMs`; the caller must supply deterministic hit ordering. The
receiver does not create an event queue or synchronize different shards.

Each call validates the live identity, preparation and health, performs one
bounded equipment read (at most ten slots), and calculates on a local copy:

1. `DamageAfterArmor(Draw, armor)`.
2. `SplitCreatureDamage(D, SHP, activeKO)`.
3. `entityhealth.ApplyDamage` using the current HHP as the upper bound. Damage
   only decreases pools, so no CON/MHP fallback or profile read is needed.
4. Death or a new 60-second KO, followed by one observer-aware health commit.

`CreatureDamageResult` owns its before/after snapshots. Armor, damage and split
amounts are `float64`, without rounding. The split amounts precede limiting
deductions to available pools; use the snapshots to inspect actual pool changes.
The target's armor and health are read again on every call.

All failures return sentinel errors and a zero result without changing health,
movement or notifications. Validation rejects nonfinite/negative input and
health, `SHP > HHP`, negative KO deadlines, invalid identities and equipment,
unprepared/dead targets, new KO deadline overflow, and lying-revision overflow.
A validated zero post-armor damage is a complete no-op, including an expired KO;
the existing health pass still handles that deadline.

## KO, death and notifications

An unfinished KO remains active for hits at or before its deadline. Hits exactly
at that deadline all execute before KO completion. A positive hit strictly after
the deadline first completes KO on its local copy with the existing minimum-SHP
grant, then uses ordinary SHP/HHP splitting. Lying alone never selects full HHP
damage. An active KO is not extended; a fresh depletion after expiry starts a new
60 seconds. HHP zero clears the deadline and immediately rejects later hits.

KO, lying and death stop movement through the existing helper, preserving
independent stun and stop publication. The existing health system remains
responsible for interaction cleanup and permanent death/corpse/persistence. The
receiver performs no I/O. Hunger, administrative damage and regeneration keep
their existing paths.

Ordinary pool changes use the configured stats TTL. KO/pose transitions and
death schedule an urgent stats update; a pose change also schedules character
visuals. Stats coalescing retains the earliest pending deadline, so ordinary
updates cannot postpone an urgent one.

## Allocation and memory contract

After `PrepareTarget`, the complete `Apply` path, including numerical work,
observer-aware ECS commit, movement stop and notification enqueue, allocates
zero bytes. Errors and the first KO also satisfy this contract. Network snapshot
construction/encoding/delivery and deferred death processing are outside it.

Player stats use a typed indexed min-heap of persistent records, ordered by
`(DueUnixMs, EntityID)`, with one actual pending entry per player and no stale
entries. Preparation reserves capacity for every registered record. Drain
retains registrations. `ForgetPlayer` removes pending/sent state while retaining
a prepared live body's binding; release/despawn removes the record.

Character visuals use persistent records in an intrusive FIFO. Prepared Mark
and Drain only relink records; Forget removes them in O(1). Legacy producers
retain lazy registration. Object behavior/action animation and regeneration
queues are unchanged.

Stats enqueue/removal costs O(log P); visual enqueue/removal costs O(1).
Registration memory depends on targets that use the queues, independently of
unrelated world entities. Heap capacity retains the observed high-water mark.
Drain callers must supply sufficient reusable buffers to avoid buffer growth.

## Validation

Receiver and queue tests cover real equipment, fractional results, KO boundaries,
sequential hits, observer writes, health pass/death, atomic errors, identity and
generation reuse, detached preparation, retirement, queue ordering and bounded
pending counts. Allocation assertions cover fresh and pending notifications,
movement stop, errors and repeated prepared enqueue/drain cycles.

Run with `CGO_ENABLED=0` and
`GOCACHE=/private/tmp/origin-combat-go-cache`: targeted package tests, `make test`,
benchmarks with `-benchmem -count=5`, and `git diff --check`. Review generated
outputs after `make test`; this feature changes no protocol, SQL or definitions.

## Measured results (2026-10-07)

Apple M3 Max, darwin/arm64, Go 1.27.1, `-benchmem -count=5`. The tables report medians
of five samples per case. Queue benchmarks use identical fixtures before/after;
preparation and buffer allocation are outside timing. Receiver samples include
a fixed health-storage reset; fresh samples also drain into reusable buffers.
The receiver is new, so it has no earlier receiver baseline.

| Notification scenario | Before ns/op | After ns/op | Before B/allocs per op | After B/allocs per op |
| --- | ---: | ---: | ---: | ---: |
| Stats, 1 player, fresh mark/drain | 51.18 | 11.60 | 48 / 2 | 0 / 0 |
| Stats, 1 player, already pending | 5.748 | 4.321 | 0 / 0 | 0 / 0 |
| Stats, 128 players, fresh mark/drain | 105.1 | 28.87 | 48 / 2 | 0 / 0 |
| Stats, 128 players, already pending | 5.780 | 4.367 | 0 / 0 | 0 / 0 |
| Visual, fresh mark/drain | 24.28 | 8.900 | 0 / 0 | 0 / 0 |
| Visual, already pending | 3.990 | 3.946 | 0 / 0 | 0 / 0 |
| Visual, 64 fresh marks/drain | 2422 | 502.1 | 0 / 0 | 0 / 0 |

Initial duplicate-mark paths regressed by more than 10%; isolated reruns
confirmed it. Keeping the existing-record fast paths inline and avoiding the
stats sent-state lookup for an already-due entry removed those regressions.
No time thresholds were added to tests.

| Receiver scenario | 1,000 unrelated entities ns/op | 100,000 unrelated entities ns/op |
| --- | ---: | ---: |
| Empty equipment, fresh notifications | 141.3 | 141.3 |
| Empty equipment, already pending | 131.6 | 131.5 |
| Partial equipment, fresh notifications | 161.0 | 165.2 |
| Partial equipment, already pending | 155.1 | 156.4 |
| Full equipment (10 entries), fresh notifications | 256.8 | 259.4 |
| Full equipment (10 entries), already pending | 248.2 | 248.0 |
| New KO with movement stop, fresh notifications | 206.5 | 212.1 |
| Active KO, fresh notifications | 188.8 | 193.8 |
| Invalid Draw | 40.10 | 40.39 |
| Invalid health | 32.09 | 32.17 |
| Dead target | 32.15 | 32.07 |

All 110 receiver samples report **0 B/op, 0 allocs/op**. The full-equipment
benchmark reports ten entries per operation; unrelated entities do not enter
the receiver's work. Targeted tests and `make test` passed. Generated SQL and
protobuf outputs remained unchanged; read-only code review approved the diff.

Raw local output is in `/private/tmp/origin-creature-stats-{before,after}.txt`,
`/private/tmp/origin-creature-visual-{before,after}.txt` and
`/private/tmp/origin-creature-damage-bench-after.txt`. Reproduce with:

```sh
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache \
  go test ./internal/ecs -run '^$' \
  -bench 'Benchmark(PlayerStatsDirty|CharacterVisualDirtyQueue)$' -benchmem -count=5
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache \
  go test ./internal/game -run '^$' \
  -bench '^BenchmarkCreatureDamageService$' -benchmem -count=5
```
