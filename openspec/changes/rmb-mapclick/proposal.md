# Proposal

## Why

Map input is split between `MapClick` for primary clicks and `Interact` for secondary object clicks, so secondary ground clicks never reach the server. A single input message should preserve existing interactions while making RMB consistently cancel an active gameplay action and place a carried world object at the clicked position.

## What Changes

- Extend `MapClick` with an explicit primary/secondary button. Send secondary clicks on both objects and ground; touch long-press uses the same secondary path, including coordinates and modifiers.
- **BREAKING**: Remove `sendInteract`, the `Interact` protocol message/action, and the separate server command route. Reserve the retired action tag/name and move its pickup and context-interaction behavior into secondary `MapClick` routing. Keep context-menu selection as `SelectContextAction`.
- Process secondary clicks independently of existing armed targeting: cancel any selecting, approaching, or executing gameplay action before processing the same click as RMB input. Cancel incomplete effects and action-owned movement without an action stamina charge.
- While carrying a world object, secondary-clicking an object or ground requests put-down at the click coordinates, ahead of pickup or context interaction. Reuse existing approach, placement, and cancellation rules. A failed RMB put-down keeps the object carried, reports an error, and returns to idle without arming lift_down; explicit Actions/hotbar lift_down keeps its existing retry selection.
- Without a carried object, a secondary ground or stale-target click starts no movement. It still cancels an active gameplay action; a secondary object click preserves the former `Interact` behavior.
- Preserve primary-click routing, hand-item dropping rules, and administrator click selection. Pending administrator commands consume primary clicks only; secondary clicks leave them pending.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `map-click-input`: Describe both mouse buttons in `MapClick`, retire `Interact`, preserve primary routing, and define secondary carry/context/ground precedence.
- `game-actions`: Restrict armed target dispatch to primary clicks, make secondary clicks cancel active actions, and allow RMB put-down in addition to the Actions menu and hotbar.
- `admin-object-inspection`: Replace the separate `Interact` exemption with a secondary-`MapClick` exemption while retaining primary-only inspection selection.

## Impact

- Protocol: `api/proto/packets.proto` and generated Go/browser bindings; maintained browser and load-test senders. Missing button defaults to primary for existing `MapClick` producers; legacy `Interact` is no longer supported.
- Client: `web_new/src/game/Render.ts`, `PlayerCommandController.ts`, input integration, and existing map-click/action regression suites.
- Server: player-action decoding and command IDs, `NetworkCommandSystem`, shared pickup/context helpers, `ActionService`, lift action handlers, and their integration tests. Remove the obsolete protocol interaction enum dependency from internal pickup state when retiring that enum.
- Documentation: affected input/context-flow references and the three capability delta specs. No database, asset, or new dependency changes.
