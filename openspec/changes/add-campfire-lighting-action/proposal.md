# Proposal

## Why

Newly constructed campfires currently become burning immediately and begin consuming their supplied fuel before a player performs any interaction. Campfires need an explicit ignition step so the player decides when their initial fuel and cooking capability become active.

## What Changes

- Create an unlit campfire state after construction that preserves its initial fuel reserve without scheduling fuel consumption.
- Add the single context action `Light my fire` for an unlit campfire. Selecting it uses the normal target-link and cyclic-action flow.
- Make ignition complete after one cycle, consume exactly 50 player stamina at successful cycle completion, transition the campfire to `burning`, and start its fuel timer only then.
- Render station states through the existing appearance-update flow using the client resources `campfire/unlit` and `campfire/burning`.
- Keep existing refueling, burning, persistence, cooking-station, and ash-exhaustion behavior unchanged after ignition.

## Capabilities

### New Capabilities

- `campfire-ignition`: Player-triggered, single-cycle ignition of a newly built unlit campfire.

### Modified Capabilities

- `fuel-burner`: Delay a campfire's initial fuel schedule until successful ignition rather than construction.

## Impact

- Object definition data for the campfire's initial station and burner state.
- Burner behavior initialization and the context-action/cyclic-action integration used by object behaviors.
- Campfire, burner, cyclic-action, persistence/reload, and cooking-station regression tests; client-visible action/progress updates reuse existing protocols.
