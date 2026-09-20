# Design

## Context

See [proposal.md](proposal.md) for motivation and the delta specs for the behavioral contract. The campfire definition already declares `unlit`. The original burner initialization nevertheless armed a seconds-based deadline, and a dedicated ECS system scanned objects to consume fuel. The revised implementation uses the existing scheduled-behavior mechanism. The context-action service provides target approach, one-cycle progress, cancellation, and completion through `ActiveCyclicAction`.

## Goals / Non-Goals

**Goals:**

- Keep the campfire's configured initial fuel but leave its deadline unarmed until ignition succeeds.
- Reuse the existing burner behavior and cyclic-action lifecycle so no network command or client protocol is introduced.
- Persist both unlit/unarmed and lit/armed states correctly across object reload and server restart.
- Notify station consumers after ignition so linked craft availability reflects the burning state.
- Make every applicable burner transition use one object-agnostic appearance convention so clients render its unlit and burning states correctly.

**Non-Goals:**

- Adding player-supplied tinder, tool requirements, new sounds, or a relighting flow after fuel exhaustion.
- Changing fuel capacity, refueling capacity rules, cooking recipes, or the ash outcome.
- Supporting or migrating seconds-based burner configuration or saves. There are no existing burners, per the user's deployment constraint.
- Adding per-object visual-resource fields to burner configuration, or allowing burner behavior to branch on a concrete object key such as `campfire`.

## Decisions

### Extend the existing burner behavior with ignition

The burner behavior will expose `Light my fire` only for an unlit burner with its configured initial fuel waiting to be activated. It will hide `Add fuel` in that state, ensuring that the menu has exactly the one required action. The same behavior will validate, start, and complete the cyclic action.

This keeps ignition adjacent to the stored fuel and its activation deadline. A separate campfire-only behavior would duplicate target, persistence, and burner-state checks and add another ordered behavior to the object definition.

### Represent an unarmed burner with no fuel deadline

For a newly spawned burner in the `unlit` station state, initialization persists the configured fuel amount with a zero `next_fuel_burn_at_tick` and schedules no callback. On successful ignition, the behavior sets the first deadline to `TimeState.Tick + ticksPerFuel` and schedules itself through `ScheduleBehaviorTick`.

Only the tick format is supported. Configuration values are counts of ticks without any seconds-to-ticks conversion; the campfire's configured value is 10 ticks per unit. New-format saves preserve the fuel reserve and absolute tick deadline. Restore catches up by computing elapsed fuel intervals arithmetically, then schedules the next boundary or an exhaustion retry. Inactive objects remain unscheduled.

### Keep fuel advancement inside scheduled burner behavior

Remove `BurnerSystem` and implement `OnScheduledTick` on `burnerBehavior`. A successful fuel callback schedules the next boundary without resetting progress when fuel is added. Exhaustion sets the station to `unlit`, updates appearance, and calls the existing durable exhaustion service through an injected execution dependency. A failed outcome is retried on the next tick; successful completion or despawn removes scheduled work. The service remains responsible for database/drop/despawn operations, while the behavior owns timing and fuel state.

`BehaviorTickSystem` continues using ticks, its existing queue, and its existing budget. Its priority becomes 313, ahead of cyclic action completion at 315, so due fuel transitions processed in that tick are visible to crafting. Other scheduled behaviors continue through the same dispatcher.

### Complete ignition through one normal target-linked cycle

Selecting `Light my fire` creates an `ActiveCyclicAction` targeting the campfire and sets the player to the normal interacting state. It uses the established single-cycle duration used by simple target actions (10 ticks), with no repeated cycle. At completion, revalidate that the campfire is still unlit and unarmed, then consume the exact 50-stamina ignition cost before changing any campfire state. Ignition deliberately does not use the shared long-action reserve rule: a player with exactly 50 stamina can pay this explicitly specified cost.

If the cycle is interrupted, the target changes state, or stamina is insufficient at completion, cancel without changing fuel, burner deadline, or station state. Successful completion arms the deadline, changes the station to `burning`, marks the object state dirty, and publishes the existing station-state-changed event so craft availability refreshes.

### Leave the campfire definition as the initial-state authority

The campfire station definition will declare `unlit` as its initial state while retaining the configured initial fuel. The burner initialization will derive whether to arm its deadline from the spawned station state rather than adding a campfire-specific persistence field or a new behavior configuration flag.

This is sufficient because the current burner-backed object is the campfire, and it avoids configuration that can disagree with station state. If a future burner needs delayed ignition with different rules, it can introduce an explicit configuration extension then.

### Derive burner appearance resources from common object state

Whenever burner behavior transitions a station-backed burner between the shared `unlit` and `burning` states, it will update `Appearance.Resource` by composing the immutable object-definition key with that state: `{object}/unlit` or `{object}/burning`. For example, a campfire's client resource data declares `campfire/unlit` and `campfire/burning`, but the behavior never contains either literal or receives either path through its configuration. It needs only the current object's definition key and the common target state.

The transition will publish the existing entity-appearance event only when the derived resource differs from the current appearance. That event already sends an object-spawn upsert to visible clients, so no protocol message or client state store is added. Client resource data remains responsible for mapping the conventional resource names to visuals; the generic burner behavior is responsible only for deriving and publishing the resource name.

The burner also returns this derived resource from its runtime recompute hook. The shared behavior result supports an optional appearance resource, with the first non-empty result in behavior priority order taking precedence over flag-based definition rules. This keeps subsequent dirty-queue recomputations from reverting an ignited object to its definition's default resource. Spawn and restore initialization derive the same resource before exposure; fuel exhaustion marks behavior state dirty so a surviving object awaiting its exhaustion outcome can render unlit.

## Risks / Trade-offs

- [An unarmed zero deadline could be treated as overdue] → Guard both runtime burning and restore catch-up so only armed deadlines can consume fuel or trigger exhaustion.
- [Station state changes may not refresh dependent craft UI] → Publish the existing station-state-changed event after a successful ignition.
- [A burner transition may select a missing client resource] → Require each burner-backed object that uses the shared states to register both `{object}/unlit` and `{object}/burning`; cover the generic derivation with a non-campfire fixture and the campfire resource registration with an integration assertion.
- [Generic behavior could acquire object-specific exceptions] → Keep the appearance helper limited to the immutable object key and the two shared state values; reject new resource-path fields or concrete object-key branches in burner configuration and tests.
- [An action may be requested from stale UI state] → Validate unlit/unarmed state at action execution, during cyclic validity checks, and at completion.
- [Unloaded objects or deferred callbacks could lose elapsed fuel intervals] → Catch up from the persisted tick deadline without per-unit loops, then schedule from the next original boundary.
- [Exhaustion persistence can fail] → Keep the exhausted burner unlit and schedule an outcome retry for the next tick; do not consume fuel again.

## Migration Plan

1. Deploy the tick-based definition and behavior changes together to the world with no existing burners.
2. Newly built campfires start unlit and require ignition. The new tick-based state survives subsequent unload/reload and restart.
3. No compatibility reader or migration is included. Rolling back after new burners are saved requires handling that new data explicitly; the old seconds-based implementation cannot read the new schedule correctly.
