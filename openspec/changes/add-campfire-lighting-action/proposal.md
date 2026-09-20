# Proposal

## Why

Newly constructed campfires currently become burning immediately and begin consuming their supplied fuel before a player performs any interaction. Campfires need an explicit ignition step so the player decides when their initial fuel and cooking capability become active.

## What Changes

- Create an unlit campfire state after construction that preserves its initial fuel reserve without scheduling fuel consumption.
- Add the single context action `Light my fire` for an unlit campfire. Selecting it uses the normal target-link and cyclic-action flow.
- Make ignition complete after one cycle, consume exactly 50 player stamina at successful cycle completion, transition the campfire to `burning`, and start its fuel timer only then.
- **BREAKING**: Configure fuel duration with `ticksPerFuel` and persist `next_fuel_burn_at_tick`. Each fuel unit grants that many server ticks; seconds-based configuration and saves are unsupported, as there are no existing burners to migrate.
- Remove the dedicated burner ECS system and run fuel consumption, catch-up, and exhaustion retries through the existing `BehaviorTickSystem`.
- Have burner-driven `unlit` and `burning` transitions update appearance through the existing flow using the common client-resource convention `{object}/unlit` and `{object}/burning`. The burner derives `{object}` from the immutable object-definition key; it carries no object-specific resource paths, type checks, or configuration.
- Keep refueling capacity, cooking-station requirements, and durable ash-exhaustion behavior; persist the new tick deadline across reload and restart.

## Capabilities

### New Capabilities

- `campfire-ignition`: Player-triggered, single-cycle ignition of a newly built unlit campfire.

### Modified Capabilities

- `fuel-burner`: Delay the initial schedule until ignition, measure fuel duration in server ticks, use scheduled behavior callbacks, and define generic appearance updates for the common unlit/burning resource states.

## Impact

- Campfire object/client resource data for its initial station state and the `campfire/unlit` and `campfire/burning` renderer resources.
- Burner behavior initialization, its generic object-key/state appearance transition, and the context-action/cyclic-action integration used by object behaviors.
- Campfire, generic burner appearance, cyclic-action, persistence/reload, and cooking-station regression tests; client-visible action/progress and appearance updates reuse existing protocols.
