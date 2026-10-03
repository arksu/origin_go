# Gameplay action definitions

Version 1 files contain an `actions` array. IDs are lowercase string identifiers
and must have exactly one registered handler. Definitions require
`presentation` (label and local menuIcon under /assets/), `target`,
`requirements`, and `execution`. Unknown fields fail loading with the filename.

Legacy targets are `none`, `object`, or `tile`. Optional `execution.ticks` and
`execution.stamina` are nonnegative and default to zero; legacy stamina is paid
on success. `execution.repeat: true` requires a timed object/tile action.
`isRepeatable` instead returns an object/tile action to selection after an attempt.
`target.approach: "tile_center"` is allowed only on tiles.

## Directed combat

See [axe_aoe.json](axe_aoe.json) and [axe_single.json](axe_single.json) for loaded
examples. `target.kind: "direction"` requires `execution.combat`:

| Field | Meaning |
| --- | --- |
| selection | `all` contacts or `nearest` collider intersection |
| sectorAngleDegrees | Finite convex sector angle in (0, 180] |
| windupMs | Positive integral milliseconds from accepted start to strike |
| recoveryMs | Positive integral milliseconds from strike deadline to idle |
| cooldownMs | Positive integral milliseconds from accepted start to action reuse |
| damageMultiplier | Finite nonnegative multiplier applied before armor |

The axe presets use 90°, 600/400/2000 ms, multipliers 1/1.5, and
`execution.stamina: 60`. This is the only authored stamina cost; combat pays at
accepted start. Timing must fit Go's duration range, including windup + recovery.
Combat rejects authored `ticks` (even zero), repeat, automatic rearming, and
approach. An optional target cursor is allowed.

Each equipment selector names exactly one itemKey/itemTag and one or more valid
slots. All entries must be satisfied. Combat also requires a hand selector
matching a known item with weapon metadata. Items load before actions.
The compatible right-hand weapon wins, with left-hand fallback; its identity,
base damage, range, and instance Quality are locked for the cycle.

Raw damage is B × (effective STR / 1)^0.25 × (Quality / 10)^0.25 × multiplier.
Normalization constants live in `internal/combat`. STR is sampled at impact as
float64; Quality must be positive. Attributes do not speed up axe timings.
