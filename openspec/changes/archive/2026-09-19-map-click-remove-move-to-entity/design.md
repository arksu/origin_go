# Design

## Context

See proposal.md for motivation. `Render.ts` currently routes primary clicks to `MoveTo`, except dropped items, placement callbacks, item-in-hand drops, and the special `adminObjectInfoSelectionArmed` branch. `GameView.vue` recognizes `/info` and `/destroy`; `GameFacade.ts` exposes arm/cancel methods. Context clicks and long presses send `Interact`.

`PlayerCommandController.sendMoveToEntity` has no callers in `web_new`; its server handler still resolves moving targets, auto-interaction, and pending spawn/teleport. `cmd/load_test/virtual_client.go` actively sends `MoveTo`. Internal target-following movement is also used by interaction flows and must remain even when the wire command is removed.

The existing inspection spec requires a one-shot primary-click selection, including consuming empty-ground attempts. The delta tightens its transport and ownership while retaining those semantics. The workspace already contains uncommitted `/destroy` implementation; preserve it and adapt only its selection entry point.

## Goals / Non-Goals

**Goals:** One input description per ordinary map click, server-only interpretation of administrator pending state, fixed-coordinate ordinary movement, and complete removal of dead movement protocol routes.

**Non-Goals:** Redesigning permanent deletion, changing right-click interactions, removing internal entity-following movement, introducing a universal input-event bus, or redesigning placement and hand-drop workflows.

## Decisions

### Neutral click action replaces both legacy wire movement actions

Define `MapClick` with `int32 x = 1`, `int32 y = 2`, and `uint64 target_entity_id = 3`. Use a fresh `C2S_PlayerAction.map_click` field number (5 is currently unused). Reserve numbers 1 and 2 and names `move_to` and `move_to_entity` in the enclosing message, remove their oneof members and message definitions, and retain modifiers at field 10. Regenerate bindings using existing project tools.

This avoids reinterpreting old payloads under new input semantics. Adding object metadata to `MoveTo` obscures its intent; reusing `MoveToEntity` changes fixed-point movement into entity following. A separate supplementary target packet creates unnecessary ordering and correlation problems.

### Client hit testing is independent of administrator state

Remove command parsing from `handleChatSend`, the render flag and branch, facade arm/cancel methods, and the Escape call. Add `sendMapClick` using the same coordinate rounding and modifier handling as current movement. Reuse existing object hit testing to populate the target ID or zero.

Replace the primary dropped-item `Interact(PICKUP)` send with `MapClick` and keep that branch's existing priority before placement/hand-drop handling. The server decides pickup after administrator interception, so `/destroy` can select a dropped item without picking it up. Ordinary unconsumed clicks also send `MapClick`. Placement and hand-drop actions keep their existing protocols and consume their own clicks; they do not also emit `MapClick`. A pending administrator action waits for the next actual `MapClick` while those explicit tools are active. Right click and long press retain `Interact`.

### Server dispatch has an explicit consumed result

Route the new player action through `CmdMapClick` and a map-click handler. Resolve administrator pending intent first using a small shared dispatch function returning whether the click was consumed. Keep existing pending resources unless their consolidation is necessary; they must be mutually exclusive and cleared by existing death/transfer/disconnect lifecycle paths.

Coordinate commands consume `x,y` even when an object ID is present. Object commands consume the supplied ID, including zero for a failed one-shot selection. They resolve live targets and enforce existing eligibility rules on the server. Preserve current command availability: this project has no administrator-role model; introducing authorization is outside this change (explicitly approved during apply).

If consumed, return without movement, pickup, or context action. Otherwise route live dropped-item targets through the existing pickup handler, preserving its validation/range rules. All other clicks use existing fixed-coordinate movement logic including stun, stamina, intent cleanup, and modifiers. Missing ordinary targets do not prevent coordinate movement. Remove pending `/info` and `/destroy` interception from `handleInteract`.

### Remove the transport, preserve internal movement mechanics

Delete the unused sender, protobuf types, queue command and game/network dispatch cases, and `handleMoveToEntity`. Keep `Movement.SetTargetHandle`, auto-interaction, and link logic wherever existing interaction paths need them. Update comments that describe the removed wire command and migrate load-test payload construction and mutation. Tests must detect accidental loss of pickup/context-interaction behavior.

## Risks / Trade-offs

- [Wire compatibility break] → Deploy server, browser assets, and load-test binary together; legacy clients must reload. Retired tags decode as unknown rather than as another action. No mixed-version compatibility layer is included.
- [Pickup bypasses pending commands] → Make ordinary primary pickup a map-click interpretation and test interception before pickup.
- [Special input tools consume clicks] → Preserve their normal routing explicitly; administrator commands await an ordinary map click, with no hidden client override.
- [Stale or forged target] → Resolve target in the current server world; preserve eligibility checks and never substitute a nearby object for failed administrator selection.
- [Pending mode cleanup regression] → Test replacement, one-shot failure, per-player isolation, death and departure; Escape remains a local UI action and does not secretly cancel server pending commands.
- [Removal damages ordinary interaction] → Remove only wire-specific handlers and test existing target-following interactions independently.

## Migration Plan

1. Update schema, reserve retired action tags/names, and regenerate both bindings.
2. Migrate browser, load-test, server routing, and focused tests in the same change.
3. Run Go tests, frontend type checks/build and focused input tests; verify no production references to retired routes or client administrator selection state remain.
4. Deploy coordinated versions and refresh clients. Roll back the protocol and its senders/handlers together if needed; no database migration is required.
