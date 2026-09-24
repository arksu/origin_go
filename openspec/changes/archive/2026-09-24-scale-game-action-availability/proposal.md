# Proposal: Remove Game Action Availability

## Why

The server currently recalculates every action's availability and reason for every player on every tick, then synchronizes changes to the menu. Keeping that preview correct as action requirements grow would require substantial work across skills, equipment, stamina, and handler state for little gameplay benefit.

## What Changes

- Send the server's action definitions on enter-world bootstrap without per-player availability or unavailable reasons. Do not recalculate or resend the action list when player requirements change.
- Show every listed action as selectable in the Actions panel and hotbar. Close the panel immediately after a selection. If the action cannot start, the server rejects it with the existing mini-alert and leaves no new action armed.
- Keep server-side requirement checks at activation, target acceptance, during active work, and before committing an effect. Check active actions each tick without scanning idle players for menu availability.
- **BREAKING:** Remove `available` and `unavailable_reason` from `ActionDefinition` in the wire protocol without reserving their field numbers. Update the maintained server and browser client together.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `game-actions`: Replace availability previews and refreshes with a static action list, selectable icons, and server rejection on activation.

## Impact

- Server action list construction and active-action validation in `internal/game/`.
- `ActionDefinition` in `api/proto/packets.proto` and generated Go and browser protocol bindings.
- Actions panel, hotbar activation, and related tests in `web_new/`.
- No changes to action definition files, map-click routing, or saved hotbar IDs.
