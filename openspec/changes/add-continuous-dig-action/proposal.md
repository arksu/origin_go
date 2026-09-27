# Proposal

## Why

Players need a direct way to gather basic terrain resources without changing the ground. The existing tile-action path already handles targeting and approach, but its `isRepeatable` flag waits for another click instead of repeating work on one selected tile.

## What Changes

- Add a data-defined `dig` action with the existing `dig` cursor and tile-center approach. Grass yields soil, shallow water yields clay, mountain yields stone, and sand yields sand; every completed cycle yields one Q10 item while leaving terrain unchanged.
- Add `execution.repeat: true` for timed targeted actions, so one accepted click can run successive cycles on the same target. Keep `isRepeatable` as the existing return-to-selection behavior; `plow_tile` remains one attempt per click.
- Make each dig cycle take 20 ticks (two seconds at the default 10 Hz) and cost exactly 300 stamina on successful item delivery. Stop on cancellation, invalid target, insufficient stamina, full inventory and hand, or after a successful hand fallback.
- Register soil, clay, and sand as inventory items using the existing item images. Stone already has a definition. Player-bound generated items use the standard `GiveItem` placement path; failure to place an item produces no item or action stamina charge.

## Capabilities

### New Capabilities

- `dig-tile`: Terrain eligibility, item mapping, repeated digging, inventory delivery, and stop conditions.

### Modified Capabilities

- `game-actions`: Define and validate opt-in automatic execution repeat for timed targeted actions without changing `isRepeatable` semantics.

## Impact

- Action definition loader and server action cycle lifecycle; `data/actions/` and the action handler registry.
- Item definitions in `data/items/`, the existing `GiveItem` service, and inventory update notifications.
- Existing cursor and item PNG assets are reused; no new network message or terrain mutation is required.
