# World Package Notes

`internal/game/world` owns chunk/object lifecycle and object persistence.

## Chunk Activation/Deactivation Contracts

- On chunk activation, objects with behaviors must receive **immediate** behavior recompute.
  - No lazy defer to later ticks.
  - This guarantees correct flags/appearance before normal gameplay/network flow.
- On chunk deactivation, serialize persistent object data and owned inventories, then despawn runtime entities safely.

## Object Behavior Integration

- Use `ecssystems.RecomputeObjectBehaviorsNow(...)` after activation for newly built behavior-bearing objects.
- Do not rely on debug fallback sweeps for initial state correctness.

## Persistence

- `ObjectInternalState.State` persists as object data.
- Runtime flags are computed and should not be persisted directly.
- `ObjectInternalState.IsDirty` is used for save filtering.
