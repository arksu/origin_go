# Proposal: Game Actions System

## Why

Gameplay actions need one discoverable entry point and one server-owned execution path. Today lift is a context-menu action, put-down is a client-local mode, and the HUD Actions button is a stub. Future actions may target an object, a tile, or no target, and may have skill, equipment, and other requirements. Adding each case directly to map-click or HUD code would keep fragmenting the system.

## What Changes

- Add internal/actiondefs and data/actions/*.json. Each definition separates identity/presentation (including a menu icon), target selection (object, tile, none), requirements, and optional execution duration/stamina cost. Targeted actions may declare a cursor and isRepeatable.
- Add a server-side ActionService with registered handlers. The server validates requirements and owns selecting, approaching, executing, completion, cancellation, and active cursor state. Adding an action for a supported target kind requires a definition and handler, without an action-specific map-click branch.
- Provide action definitions and current availability (including a reason for unavailability) to the client on enter world and when relevant state changes. Clicking the Actions button on the left opens a one-row horizontal icon panel in server-list order. Unavailable icons are disabled; their label and reason appear on hover/focus. The panel toggles closed from the button, closes on outside click, and marks the active action.
- For object and tile, activating an action enters target selection; an optional cursor appears, and a matching map click chooses the target. For none, activation starts one execution immediately. Without a tick duration, the handler starts without a timed cycle and may complete through an existing deferred domain transition; with a duration, it runs through the existing cyclic-action flow. Optional stamina is charged only on successful completion.
- isRepeatable may be set only on targeted actions. After success, a repeatable action returns to target selection; a non-repeatable action ends. A none action always runs once.
- Escape cancels the active action before closing an open window, stops movement initiated to reach its target, and resets the cursor. Activating a different action also cancels the current selection, approach, or cycle without a stamina charge, stops action-initiated movement, then starts the new action. Losing a requirement cancels the action in the same way.
- Move lift and lift_down fully into Actions, including lift of objects without colliders. Remove the lift context-menu and implicit interact paths and the client-local put-down mode. Both definitions are non-repeatable and have no separate tick duration or stamina cost; existing approach, carry, and placement transitions remain their execution mechanics.
- Allow gameplay action icons to be dragged from the panel into the existing hotbar. Persisted IDs are checked against the server action list only after that list arrives, so loading does not erase valid pins.

The first menu contains lift and lift_down. The tile and none paths are covered with test definitions and handlers; this change does not invent a new player-facing effect for either kind.

## Capabilities

### New Capabilities

- game-actions: Data-driven action definitions, requirements, target selection, optional cyclic execution, server-owned action/cursor state, Actions menu, hotbar pinning, and complete migration of lift/put-down.

### Modified Capabilities

- map-click-input: Pending administrator clicks remain highest priority; an armed action gets the next chance to consume a map click before ordinary movement, linking, or pickup.

## Impact

- **Protocol** (api/proto/packets.proto): activate/cancel requests, action list with availability, and authoritative action-state updates. C2S_LiftPutDown is retired and its field reserved. Go and client bindings are regenerated together.
- **Server**: new action definitions, loader, registry, service, ECS state, handler integration, map-click interception, enter-world snapshot, availability refresh, and cancellation hooks. Lift's old context/interact entry points are removed.
- **Client** (web_new): Actions menu, server-driven cursor and action state, Escape cancellation, gameplay hotbar entries, and removal of local put-down mode.
- **Data**: lift and lift_down definitions in data/actions/.

Server and client ship together. Other context actions and their content remain outside this migration. Plow and other new gameplay effects are follow-up changes.
