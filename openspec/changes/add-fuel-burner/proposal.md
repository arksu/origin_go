# Proposal

## Why

Campfire currently consumes a hard-coded station resource on every ECS update and becomes unlit. It cannot be refuelled, its lifetime is not expressed in server runtime seconds, and the logic cannot be reused by furnaces or other objects with different acceptable fuels and burn rates.

## What Changes

- Introduce numeric item abilities, beginning with `branch.abilities.fuel = 1`.
- Introduce a reusable `burner` object behavior configured with accepted fuel ability keys, fuel capacity, seconds per fuel unit, initial fuel, and exhaustion outcome.
- Make campfire the first `burner`: it starts full at five fuel units, burns one unit per 1,440 server-runtime seconds, accepts only the `fuel` ability, and becomes one dropped `ash` item when empty.
- Consume a refuelling item even when accepted fuel would exceed capacity; clamp stored fuel to capacity.
- Sum every ability value on a consumed item whose key is accepted by the burner.
- Replace campfire's current per-update autonomous fuel-consumption rule and unlit exhaustion outcome.

## Capabilities

### New Capabilities
- `fuel-burner`: Defines reusable item fuel abilities, configurable burner behavior, persistent runtime-time burning, refuelling, and exhaustion conversion.

### Modified Capabilities
- None.

## Impact

- Item-definition schema, validation, and the `branch` data definition.
- Object-definition behavior configuration, station state persistence, and campfire data.
- Behavior registry, runtime scheduling, world-object persistence/despawn, dropped-item spawning, chunk spatial state, and visibility updates.
- Tests for item/object loading, refuelling, runtime-only duration, chunk reload catch-up, capacity clamping, and ash conversion.
