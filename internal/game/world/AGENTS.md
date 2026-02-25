# World Package Notes

`internal/game/world` owns chunk/object lifecycle and object persistence.

## Chunk Activation/Deactivation Contracts

- On chunk activation, objects with behaviors must receive **immediate** behavior recompute.
  - No lazy defer to later ticks.
  - This guarantees correct flags/appearance before normal gameplay/network flow.
- On chunk activation, restored objects must run behavior lifecycle init with `restore` reason before recompute.
  - Behaviors may schedule future ticks and apply bounded catch-up during this init.
- On chunk deactivation, serialize persistent object data and owned inventories, then despawn runtime entities safely.

## Object Behavior Integration

- Use `ecssystems.RecomputeObjectBehaviorsNow(...)` after activation for newly built behavior-bearing objects.
- Do not rely on debug fallback sweeps for initial state correctness.
- Keep behavior scheduler cleanup on despawn (`ecs.CancelBehaviorTicksByEntityID`) intact.
- On restore, rehydrate runtime-derived components implied by persisted behavior state when required (for example build-site collider restored from build behavior state).

## Persistence

- `ObjectInternalState.State` persists as object data.
- Runtime flags are computed and should not be persisted directly.
- `ObjectInternalState.IsDirty` is used for save filtering.
- Empty behavior state persists as `NULL` (not empty JSON payload).
- Runtime despawns of chunk-owned objects must preserve delete intent until chunk save (tombstone/deferred delete), otherwise objects can reappear after restart.

## Runtime Transform / Despawn Contracts

- Prefer shared in-place transform helper (`TransformObjectToDefInPlace(...)`) for runtime object type changes.
- In-place transforms must keep collider/appearance/entity info/behavior init/dirty-mark updates consistent in one path.
- Container result defs must ensure object inventories exist immediately after transform (same-tick openability).
- Use shared despawn-persistence abstraction for gameplay-driven despawns instead of feature-specific DB delete logic.
