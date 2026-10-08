# Object health and fractional persistence

## Implemented boundary

Definition-backed world objects store `HP float64` and `HasHP bool` in the existing
`components.ObjectInternalState`, component ID 23. No separate health component
is registered. This prepares object state for the combat stage described in
[combat_final.md](combat_final.md). Damage, destruction and public combat handlers
are separate changes. HP zero is retained as zero; this component does not
automatically despawn an object.

Fresh objects receive `float64(def.HP)`. Database restoration uses `float64(def.HP)`
when `object.hp` is NULL; an explicitly saved HP must be finite and nonnegative,
including zero. Saved values have no rounding, maximum-HP clamp or integer ceiling.
Players retain `EntityHealth`; special dropped items and inventory container
entities have no object health.
`HasHP` distinguishes missing health from a valid zero pool. HP is a typed field
outside the replaceable behavior state; it is not stored in a map or an interface.
Definitions and their existing integer initial HP values are unchanged.

The common spawn helper accepts an optional borrowed `DefSpawnParams.HPOverride`
for restoration. It validates the value before spawn, writes the final HP once,
and retains no pointer. Component observers and behavior initialization see the
saved value from the first component creation.

A real `TypeID` transition through the shared transform helper receives the full
destination definition HP. Reapplying the current type or changing Quality
preserves current HP. Corpse conversion removes creature health and initializes
object HP from the corpse definition.

## Ownership and mutation

Call `world.SetObjectHP(world, handle, hp)` under the owning shard lock. It checks
the live generational handle, existing internal state, `HasHP` and finite,
nonnegative input before writing. On change, one observer-aware component mutation
writes HP and `ObjectInternalState.IsDirty` together; an equal value is a no-op.
Observers see both updated fields in that notification.
Failures return sentinel errors and leave health, dirty state and observers
unchanged. A valid write can repair an invalid current value.

`ValidateObjectHP` is shared by runtime, factory and portable snapshot paths.
Both validation and the setter allocate zero bytes, including missing/stale
targets, invalid input and no-op calls. No new observers, queues, world scans,
chunk lookups, SQL queries or goroutines are needed for HP mutation. Existing
chunk dirty filtering consumes the object's dirty flag. Players retain their
separate SHP/HHP, KO and pose model in `EntityHealth`.

Chunk activation and portable restore preserve HP while replacing behavior
state, flags and dirty intent. This also creates internal state for dropped items
without setting `HasHP`. Behavior recomputation cannot replace the typed HP fields.
The formerly assigned standalone component ID 38 remains reserved under the
stable-ID rule; it has no registered type or storage.

## Persistence and failure handling

`object.hp` is nullable `DOUBLE PRECISION` with
`CHECK (hp >= 0 AND hp < 'Infinity'::double precision)`. The nullable column
supports special dropped-item rows and definition defaults for ordinary objects.
Ordinary object restoration initializes NULL HP from the definition before
publishing the entity, without modifying the raw row or backfilling the database.
Generated sqlc records and parameters use `sql.NullFloat64`; map generation leaves
initial HP NULL. Chunk activation retains clean dirty intent, so loading or
deactivating an unchanged object does not schedule a database write. Subsequent
gameplay mutations use the existing save path and persist the current resolved HP.

Factory serialization validates health before JSON serialization, including
before the transient empty-build-site exclusion. Missing/invalid health returns
an error, never the nil result that the existing save path interprets as deletion.
Players and special dropped items retain their existing persistence exclusions.

`EmbeddedObjectSnapshotV1` includes `HP *float64` as JSON `hp,omitempty`.
Definition-backed object snapshots require that field, including a pointer to
zero. Dropped-item snapshots omit it. Capture owns an independent copy; later
ECS changes cannot change accepted HP. JSON decode and snapshot restore validate
the field before creating entities. Old snapshots without ordinary-object HP
are unsupported; there is no compatibility adapter.

Chunk deactivation captures all objects and inventories before despawning any.
A failed capture retains the active chunk, entities, inventories and spatial/ref
indexes. Existing deactivation retry backs off from one second to one minute.
Lift transfer captures before source removal; target restore and source rollback
retain the snapshot HP. Existing immediate-save and chunk-save paths forward the
same value.

Active entities remain authoritative over raw cache entries with the same ID.
`Chunk.SaveToDB` returns capture/write errors while saving other valid objects.
Per-row object write errors do not skip following valid rows. On any failure,
dirty/delete intent remains for retry; the save worker retains a failed chunk.
Shutdown reports separate `chunks_saved` and `chunks_failed` counts. Worker
counts, timeouts, persistence ordering and existing transaction boundaries stay
in place.

## Verification and performance

PostgreSQL tests use `ORIGIN_OBJECT_HEALTH_TEST_DSN` and a fresh temporary schema
per test, with cleanup limited to that schema. They verify fractional/zero values,
values above MaxInt32, SQL CHECK rejection, dropped NULL, repeated migration,
real chunk save/load, lift transfer/rollback, capture/write failures and shutdown.
Skipping PostgreSQL tests does not meet acceptance.

```sh
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache \
  go test ./internal/game/world ./internal/game ./internal/core ./internal/ecs/... ./cmd/mapgen
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache make test
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache \
  go test ./internal/game/world -run '^$' -bench '^BenchmarkObjectPersistence$' -benchmem -count=5 -benchtime=200ms
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-combat-go-cache \
  go test ./internal/game/world -run '^$' -bench '^BenchmarkObjectHP$' -benchmem -count=5 -benchtime=200ms
```

Measured on 2026-10-08, macOS/arm64, Apple M3 Max; medians of five runs.
The baseline is the completed standalone `ObjectHealth` implementation, recorded
immediately before moving HP into `ObjectInternalState`. Both versions have the
same fractional PostgreSQL and portable snapshot formats. Simple objects have no
inventory; containers have two fixed grid roots with two items each. Build
includes bounded teardown to reuse a warmed World and entity ID; setup is outside
measurement. Before and after use the same operations and fixture values.

| Operation | Before ns/op | After ns/op | Before/after B/op | Before/after allocs/op |
|---|---:|---:|---:|---:|
| Simple Build | 863.9 | 906.7 | 0 / 0 | 0 / 0 |
| Simple Serialize | 347.2 | 402.9 | 256 / 256 | 1 / 1 |
| Simple Capture | 516.7 | 593.3 | 320 / 320 | 4 / 4 |
| Container Build | 8239 | 8273 | 1009 / 1009 | 15 / 15 |
| Container Serialize | 348.2 | 397.6 | 256 / 256 | 1 / 1 |
| Container Capture | 3068 | 3154 | 2011 / 2011 | 20 / 20 |

Cases above 10% were checked serially with isolated `GOMAXPROCS=1`, five samples
and 300ms per sample against a retained baseline source snapshot. Simple Serialize
was 350.6 to 444.3 ns (+26.7%); Simple Capture was 525.7 to 635.4 ns (+20.9%);
Container Serialize was 374.6 to 435.1 ns (+16.2%). Simple Build was 860.8 to
916.7 ns (+6.5%), Container Build was 8682 to 8707 ns (+0.3%), and Container Capture
was 3462 to 3627 ns (+4.8%). Allocation counts and bytes were unchanged in each
before/after pair. Capture still owns its nullable HP copy; this refactor adds no
allocation. No timing threshold is enforced in CI.

| HP operation | Before/after ns/op, 1,000 other entities | Before/after ns/op, 100,000 other entities |
|---|---:|---:|
| Read | 26.36 / 55.84 | 26.17 / 56.33 |
| Validate | 1.075 / 1.072 | 1.073 / 1.075 |
| Validate invalid | 2.330 / 2.328 | 2.327 / 2.329 |
| Set changed | 158.1 / 124.1 | 158.0 / 124.4 |
| Set no-op | 61.66 / 46.44 | 61.84 / 46.39 |
| Set invalid | 68.41 / 47.22 | 68.45 / 47.44 |
| Set stale | 6.807 / 10.04 | 8.951 / 6.796 |

All HP scenarios measured **0 B/op, 0 allocs/op**. The changed setter is about
21% faster and no-op/invalid writes are about 25%/31% faster in the primary run;
health and dirty intent now use one component mutation. Isolated reads were
26.44 to 43.50 ns at 1,000 unrelated entities and 26.64 to 43.53 ns at 100,000.
The generic value API copies the entire 56-byte `ObjectInternalState`, rather
than the former 8-byte health component. This is the cost of using the existing
component and must not be presented as a read optimization. Serialize reuses one
internal-state lookup for health, build state and JSON serialization. Stale-handle
timings varied between about 6.8 and 10 ns, with zero allocations throughout.
Unrelated entity count does not increase the amount of work; functional tests
enforce the allocation contract.

## Development deployment

Use a fresh or explicitly reset development database. Apply
`20261007_fractional_object_health.sql` with the server stopped, then start the
updated binary. The migration is transactional, uses explicit `USING`, has a
five-second lock timeout and can be applied again without rounding fractional
values. It does not backfill NULL from definitions. No automatic data reset,
integer rollback or mixed old/new binary deployment is provided.
