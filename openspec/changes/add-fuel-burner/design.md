# Design

## Context

See proposal.md for motivation. Today `StationSystem` subtracts `amountPerTick` from station resources on every ECS update, so its behavior depends on tick frequency and ends by changing campfire to `unlit`. Item definitions have no generic ability map. The existing `tree` behavior can spawn persistent world objects such as logs, but it is tied to chopping stages; craft output is the existing path that creates durable dropped items.

World-object state can persist mutable station data and behavior-owned state. `TimeState.RuntimeSecondsTotal` is a durable server-runtime clock: it advances only while the server process runs and is restored at boot. Chunks may unload objects, so a burning object's persisted schedule must be reconciled when it is restored.

## Goals / Non-Goals

**Goals:**

- Add one generic definition-level item ability map and validated numeric values.
- Add a reusable, data-driven burner contract usable by campfires, furnaces, and future objects with different fuels and durations.
- Preserve fuel state and the next burn boundary against restart and chunk reload.
- Make campfire fuel-compatible with branch now, without hard-coding branch into runtime code.
- Convert exhausted campfires into durable dropped ash without leaving duplicate or invisible world state.

**Non-Goals:**

- Adding coal, furnaces, lighting visuals, player-visible fuel meters, manual extinguishing, or partial-item consumption.
- Giving generic item abilities any meaning outside the behaviors that explicitly consume them.
- Reusing tree chopping or log-spawn code for combustion outcomes.

## Decisions

### 1. Item abilities are a map of positive integer values

Add `abilities` to item definitions as a key-to-positive-integer map. A burner calculates fuel by summing entries whose keys occur in its configured `fuelAbilities` set.

This permits `fuel`, `coal`, and later types without a global item-class enum or per-object item-key lists. A matching item is consumed in full even when capacity discards some or all of the result; partial consumption would require stack/instance splitting and would violate the agreed interaction rule.

Alternative: one `fuelValue` field with an object-specific allowlist of item keys. This does not represent mutually incompatible fuels or compose multiple abilities.

### 2. `burner` is a reusable object behavior, with a runtime system for time advancement

Register a `burner` behavior that validates and owns the context action for adding a compatible hand item. Its per-object configuration includes `fuelAbilities`, `fuelCapacity`, `secondsPerFuel`, `initialFuel`, and an exhaustion outcome.

Use a generic burner runtime system to compare persisted burn boundaries with `TimeState.RuntimeSecondsTotal`, rather than decrementing once per ECS update or scheduling solely by tick count. The behavior configuration provides object-specific policy; the system provides clock-driven advancement and completion. This keeps a furnace's duration and allowed abilities data-defined while avoiding a campfire-specific system.

Alternative: extend `StationSystem` with fuel abilities. That makes every future burner a station and retains its tick-rate-dependent consumption. Alternative: use only behavior-tick scheduling. Tick numbers are unsuitable as the durable source of a wall-independent runtime schedule across process restart.

### 3. Persist discrete reserve and next runtime boundary

Store the remaining whole fuel reserve and the next runtime second at which one unit burns. The initial boundary is the construction runtime second plus `secondsPerFuel`; each accepted item increases the reserve after clamping but does not reset progress toward the current boundary. The runtime system may consume multiple overdue units during catch-up.

This preserves the existing partial unit's elapsed time and makes refuelling deterministic. On chunk activation, restore logic calculates overdue boundaries before exposing the object; an already exhausted burner proceeds directly to its configured outcome.

Alternative: persist only a single expiration timestamp. It cannot represent capacity as whole stored units or correctly retain partial progress while accepting new fuel.

### 4. Exhaustion is a durable replacement operation

For the campfire outcome, allocate and persist one dropped ash record at the former object's position, then remove the campfire's durable chunk object and its ECS/spatial/visibility state. If persistence fails, leave the burner intact for retry so the world cannot lose both the campfire and ash. Coordinate the operation with the existing dropped-item persistence helpers and chunk object-despawn persistence.

The generic behavior expresses the outcome as data; the runtime operation owns ordering, cleanup, and visibility. Tree's `spawnChopObject` path is not used because ash is an inventory-backed dropped item, not a static object definition.

### 5. Campfire is the first configuration

Replace campfire's `autonomousConsumption` with a `burner` behavior accepting `["fuel"]`, capacity five, 1,440 seconds per unit, five initial units, and an ash-on-exhaustion outcome. Keep its cooking station capability and `burning` state while it holds fuel; the runtime system must ensure crafts see the updated state before an exhausted campfire can complete a fuel-dependent craft.

## Risks / Trade-offs

- [A persistence failure during replacement can create a missing or duplicate object] -> Persist ash before removing campfire, use idempotent/retry-aware state handling, and test failures at each boundary.
- [A chunk remains inactive longer than a burn duration] -> Reconcile overdue burner state during restore before it is inserted into interaction and spatial visibility.
- [Fuel is silently lost at capacity] -> This is intentional per the agreed rule; expose only a normal successful consumption result and cover it with regression tests.
- [Existing campfire saves contain old station fuel/state] -> Define a compatibility migration/default that converts active legacy campfires to the new bounded initial/runtime representation without overwriting unrelated station resources.

## Migration Plan

1. Add definition schemas, behavior registration, persistent burner state, runtime advancement, and test fixtures behind the new campfire configuration.
2. Migrate or normalize loaded legacy campfire state to a valid burner reserve and next boundary; mark it dirty for persistence.
3. Deploy with campfire definitions switched from autonomous fuel consumption to `burner`.
4. Roll back by retaining the state decoder's compatibility path; do not delete persisted burner fields before all active worlds have been migrated.
