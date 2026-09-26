# Design

## Context

See `proposal.md` for motivation and the three delta specs for the behavior contract. This change spans protocol, browser input, ECS command routing, and the action/lift lifecycle, so a design is required.

Observed integration points:

- `Render.setupInputController` routes mouse RMB and touch long-press through `handleContextInteraction`. That helper sends `sendInteract` only when an object is hit and currently loses modifiers; primary clicks already use `sendMapClick`, with separate hand-drop and placement priorities.
- `Game.handlePlayerAction` converts the protocol oneof into inbox commands. `NetworkCommandSystem.handleMapClick` currently runs administrator selection, armed targeting, pickup, then link/movement. `handleInteract` contains the existing context-action dispatch, including the single-action menu override.
- `ActionService.Cancel` removes the active action before handler cleanup, clears action-owned movement/cycles, and sends authoritative idle state. Deferred lift transitions already carry an action generation and check it before committing.
- `liftDownActionHandler` delegates to `LiftService.StartPutDownAt`; placement approaches the exact requested coordinates, uses a phantom collider, and finalizes through `LiftPlacementSystem`. `ActionService.Complete` currently returns failed targeted attempts to selection, even for non-repeatable actions. Tests explicitly require that retry behavior for menu-activated lift_down.
- `InteractionType` is also used by `PendingInteraction` and `AutoInteractSystem`, but the only currently produced pending type is pickup.

The main specs currently describe primary-only MapClick, a separate Interact exemption for administrators, and put-down exclusively through Actions/hotbar. The deltas intentionally change those contracts. The existing `remove-interact-rmb-mapclick` change is separate and is not a dependency of this design.

## Goals / Non-Goals

**Goals:**

- Make button routing explicit at the input boundary and keep interpretation on the ECS thread.
- Reuse context and lift behavior, including generation-based cancellation, without introducing another placement state machine.
- Keep the confirmed failure policies distinct: RMB put-down ends idle on failure; explicitly armed lift_down retains retry selection.

**Non-Goals:**

- Changing placement geometry, collision acceptance, movement stamina, or teleporting the carried object immediately.
- Removing lift_down from Actions/hotbar, auto-lifting through RMB, or changing primary build placement and inventory hand dropping.
- Changing client build-placement state into a server gameplay action. Here “armed action” means the authoritative gameplay action state; RMB does not start build placement.
- Supporting old Interact senders, changing database state, adding dependencies, or implementing this plan during proposal creation.

## Decisions

### 1. One MapClick message with a button enum

Add `MapClickButton button = 4` to MapClick, with `MAP_CLICK_BUTTON_PRIMARY = 0` and `MAP_CLICK_BUTTON_SECONDARY = 2`. Keep x/y/target field numbers and `C2S_PlayerAction.map_click = 5` unchanged. Zero preserves existing MapClick senders, including load-test input. Reject all unsupported button values before administrator consumption or cancellation. Middle mouse remains camera pan input and emits no gameplay MapClick.

Remove the Interact message, `C2S_PlayerAction.interact`, and InteractionType enum. Reserve action tag 3 and name `interact`; retain retired movement tags/names and ensure they cannot be reused. Keep `select_context_action = 4` unchanged. Replace CmdInteract with a vacant constant slot so later command IDs do not shift.

Because pending interaction currently means pickup only, remove its obsolete protocol type field and dispatch pickup directly in AutoInteractSystem. Keep the existing pending component and pickup flow; avoid introducing another internal enum or renaming unrelated interaction services.

Alternatives: a boolean secondary flag is smaller but less explicit at call sites; transmitting browser button numbers as an unrestricted integer makes the supported input contract unclear. Adding a new RMB packet preserves the split this change removes.

### 2. Normalize secondary input once on the client

Extend `sendMapClick` to accept a typed button with primary as its default, preserving existing primary call sites. Remove `sendInteract`. Rework the secondary Render helper to accept modifiers and always send MapClick after genuine input consumption checks, including ground with target zero. Close a stale context menu on both ground and object secondary clicks, and preserve the screen pointer update used for menu placement.

Mouse RMB and touch long-press call that same helper with secondary button, exact screen-to-world coordinates rounded by the sender, hit-tested target, and modifiers. Long-press must retain the existing suppression of its release tap. RMB bypasses primary dropped-item shortcuts, hand dropping, and build/lift placement callbacks that consume primary targeting. A gameplay action or its visual cursor never consumes the secondary click locally.

Alternative: sending client-side CancelAction followed by MapClick creates two independently queued commands and depends on potentially stale client state. One MapClick lets the server cancel and route against its current state.

### 3. Branch on button before administrator or armed dispatch

Use one entry point with small primary/secondary routing helpers and shared pickup/context helpers:

```text
MapClick --> validate payload and supported button
              |
              +--> PRIMARY --> pending admin --> armed target
              |                                  |
              |                                  +--> pickup / link / movement
              |
              +--> SECONDARY --> cancel current gameplay action
                                   |
                                   +--> carrying --> direct put-down attempt
                                   +--> dropped target --> pickup
                                   +--> collider target --> context actions
                                   +--> ground/stale/no collider --> finish
```

Secondary routing does not call the administrator consumer or pass the click to the previously active armed action. It inspects authoritative carry state after cancellation; if carrying, consume the click as placement even when the supplied target is a dropped item or no longer exists. A rejected placement never falls through.

Extract the body of handleInteract into a helper that takes the already resolved live target, rather than synthesizing an Interact payload. Preserve zero/single/multiple context-action behavior and the move-to-link execution path. Ordinary secondary ground clicks start no movement and do not clear unrelated primary movement or pending context work. Only cancellation of an active action stops its owned approach.

Alternative: changing the existing order to “admin, armed, RMB” allows either prior state to intercept RMB, violating its cancellation and routing requirements.

### 4. Reuse the action lifecycle for one direct put-down attempt

Add a server-only targeted-once entry point to ActionService and its command interface. Secondary carry routing supplies the existing `lift_down` definition and a coordinate target with no object target. The entry point validates definition, requirements, and target using the existing helpers, allocates a fresh generation, and starts the registered handler without entering or publishing selecting state. Share target validation/execution logic with the armed path; do not change the action definition or add individual action-ID branches to generic armed targeting.

Represent the direct-attempt policy in transient ActiveGameAction state with a small flag, for example `DirectAttempt`. Ordinary activation leaves it false. Completion for a direct attempt always ends idle after cleanup, including rejection, failure, and timeout; ordinary activation retains its current retry/repeat behavior. Accepted put-down still publishes the stable approaching state with an empty cursor and owns its movement, enabling Escape, requirement-loss cancellation, and ordinary-click interruption to work through the existing lifecycle.

Reuse liftDownActionHandler and StartPutDownAt with the new nonzero generation. Cancellation removes the old pending transition and phantom before a new attempt is started. Preserve the current generation checks so an old finalization cannot place the object or clear a newer request. Placement stays at the requested click coordinates; no snapping to object or tile center.

Alternatives: activating lift_down and feeding it the RMB click through normal armed selection emits a selection state and re-arms after failure. Calling StartPutDownAt with generation zero bypasses action commit checks and makes cancellation and late completion harder to distinguish. A new standalone placement service duplicates the existing lifecycle.

### 5. Focus verification on input and deferred outcomes

Extend the existing Render map-click suite for object/ground RMB, long-press, modifiers, all action phases, hand-item preservation, unsupported/middle input, and single-packet behavior. Keep primary placement/drop cases. Extend protocol and game routing tests for both buttons, default primary, invalid enum values, and raw retired Interact field input. Migrate context interaction tests to secondary MapClick without weakening single-action rules.

Add integration coverage with the real action/lift services for direct placement success, rejection and timeout ending idle, exact coordinates when clicking an object, replacement by another RMB, Escape cancellation, and late callbacks. Keep the existing explicit lift_down retry tests unchanged in their expected behavior. Preserve primary administrator and armed-target regression coverage, and add secondary clicks leaving all pending administrator command kinds intact.

## Risks / Trade-offs

- [An old server ignores the new button and treats RMB as primary] -> Deploy generated protocol, server, and browser together; do not claim bidirectional mixed-version compatibility.
- [Removing InteractionType breaks delayed pickup rather than just networking] -> Remove its internal dependency and verify the existing AutoInteract pickup behavior.
- [A direct-attempt flag changes explicit action retries] -> Default it off and retain explicit lift_down rejection/timeout regression tests.
- [Cancellation leaves movement or phantom state behind] -> Check active action, pending transition, phantom, cycle, movement ownership, and stale callback outcomes in integration tests.
- [An object-click placement point is obstructed] -> Preserve the current placement validity rules and failure reason; RMB does not promise placement through obstacles.
- [Touch produces both a secondary request and release tap] -> Exercise long-press suppression along with the Render secondary sender.

## Migration Plan

1. Update the protocol and regenerate Go and browser JS/TS bindings through the existing generation commands. No persisted schema changes are required.
2. Update maintained senders, server routing, and lifecycle handling; migrate tests and input/context documentation together.
3. Run focused Go protocol/game/ECS/load-test suites, browser map-click/actions suites, and browser type/build checks before coordinated deployment.
4. Roll back server and browser together if needed. Retired Interact remains unsupported by the new release; missing-button primary MapClick remains supported.
