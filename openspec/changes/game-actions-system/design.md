# Design

## Context

The existing lift entry goes through ContextActionService and a lift behavior provider. The provider must disappear from context menus, and ContextActionService.ExecuteAction resolves only actions still provided by a behavior. Therefore a pending armed lift cannot reuse PendingContextAction as its completion route. LiftService also has a separate implicit Interact path for objects without colliders and an asynchronous PendingLiftTransition for put-down. Both paths must report their final outcome to Actions.

The server already has CharacterProfile.Skills, an equipment InventoryContainer indexed by InventoryRefIndex, item keys and tags in itemdefs, and ActiveCyclicAction for tick-based work. The client has an Actions HUD stub, a local put-down mode, a hotbar persisted in localStorage, and cursor PNG assets. S2C_CharacterProfile does not expose skills, so the server must provide menu availability and reasons.

## Goals and non-goals

- One server-owned action lifecycle: activate, optionally select a target, execute immediately or for ticks, then finish, repeat, or cancel.
- Separate definition sections for target, requirements, and execution. Support object, tile, and none now, with an explicit place to extend the target and requirement schemas later.
- Make lift and lift_down the only player-facing v1 actions. Test tile and none with test definitions/handlers.
- Preserve existing lift approach, carry, and placement rules while removing every old lift/put-down entry point.
- Keep other context actions and existing craft/build actions outside this migration. Plow and new gameplay effects follow later.

## D1: Action definitions and registry

The new internal/actiondefs package loads data/actions/*.json using the established loader/registry pattern. A definition has:

- id, label, and a menu icon asset path for identity and presentation;
- target.kind: object, tile, or none; target.cursor is optional for object/tile and forbidden for none;
- requirements.skills: skill IDs, all required;
- requirements.equipment: entries with a non-empty list of known slot names and exactly one itemKey or itemTag. Every entry must match; within one entry, a matching item in any listed slot suffices;
- execution.ticks and execution.stamina: independent optional non-negative values. Missing/zero ticks means no timed cycle; missing/zero stamina means no stamina charge;
- isRepeatable: may be set only on object/tile definitions, default false; the loader rejects it on a none action, which always executes once.

The loader trims and validates identifiers, skill IDs, slots, item keys/tags, numeric values, and incompatible combinations. Menu icon paths must be local assets under /assets/ and cannot contain traversal segments or external URLs. Duplicate IDs or malformed definitions fail startup with the filename. Unknown item keys fail startup; tags are validated as non-empty strings. The startup registry check requires a handler for each definition and a definition for each handler. It does not require protocol or map-click-dispatch changes for a new action using an existing target kind. A new target kind or requirement type may need an explicit schema/protocol extension; the design does not promise otherwise.

The v1 files define lift as object-target with cursor lift and lift_down as tile-target with cursor lift_down. Both have isRepeatable false and omit execution ticks/stamina. Lift_down's carry prerequisite is a handler state condition, not a special field in every action definition.

## D2: Authoritative action state and protocol

ActionService owns an ECS action state with action ID, phase, and selected target where applicable. Phases are idle, selecting, approaching, and executing. A target action enters selecting on activation; a none action enters executing immediately or completes in the same server turn if untimed. The active state remains authoritative while a deferred lift or timed cycle completes.

Protocol messages:

- C2S_ActivateAction {action_id} and C2S_CancelAction;
- S2C_ActionList with each definition's presentation (including menu icon asset path), target, requirement, execution, and repeatability fields plus current availability and a reason code when unavailable;
- S2C_ActionStateChanged {action_id, phase, cursor}. Idle uses empty action_id and cursor. The client never infers the action ID from a cursor ID.

The server sends ActionList and the actual current ActionStateChanged in the enter-world snapshot. It resends ActionList when skills, equipment, carry state, or another supported requirement changes. Activation and execution always revalidate on the server; the menu status is informational. The cursor is non-empty only when the definition supplies one; an unknown ID renders CSS help on the client.

For untimed actions, ActionService records executing internally before calling the handler but sends only the handler's resulting phase when that call transitions synchronously. Timed actions send executing when the cycle starts. This avoids a transient executing packet immediately followed by approaching, selecting, or idle in one server turn.

C2S_LiftPutDown and its ClientMessage field are retired and reserved only after the lift migration removes the message's last server reference; the protocol task adds the new messages without touching it. Server and client protocol bindings are regenerated and shipped together.

## D3: Selection, execution, and cancellation

Activation validates the definition, requirements, handler state condition, and enough stamina for one execution. Selecting the same already armed action toggles it off. Selecting a different action cancels the previous selection, approach, or cycle without a stamina charge, stops movement started for its target, then attempts the new activation. If the new action is unavailable, the server reports the reason and remains idle. This also applies when the old action was already executing.

For object/tile actions, a valid target click is consumed. The handler validates the target before starting its domain operation. A rejected target produces a mini-alert and leaves the action in selecting, without starting ordinary movement or pickup. A live object click for an object-target action is consumed even when the object is not liftable; empty ground or a stale target falls through to ordinary movement while the action stays armed. A tile-target action consumes every map click and receives the click coordinates. This version does not add generic move-to-tile behavior; the handler validates whether a position is usable.

An untimed action starts its handler after target acceptance, or immediately on activation for none; a domain handler may complete asynchronously. A timed action creates ActiveCyclicAction after target acceptance (self-target for none) and completes after execution.ticks. CyclicActionSystem must dispatch these actions to ActionService before context-behavior lookup; a none action must not depend on a ContextActionProvider. Requirements, handler conditions, and available stamina are checked at activation, target acceptance/start, during an active action when relevant state changes, and immediately before completion. The completion contract prechecks stamina and lets the handler validate its effect before mutation. Handlers must not change player stamina and must roll back any partial effect on failure. In the same ECS turn, a successful handler commit is followed by one execution.stamina charge. A rejection, cancellation, timeout, target loss, or lost requirement consumes no stamina or leaves a partial effect.

On success, a non-repeatable action becomes idle and resets its cursor. A repeatable target action returns to selecting with its cursor if its requirements still hold; otherwise it becomes idle. A none action always becomes idle after one execution. A target-level failure during approach or execution returns to selecting only while the requirements still hold; otherwise the action is canceled.

Escape sends C2S_CancelAction before any open-window Escape handling. The server clears the action, any action-owned pending target/cycle, and action-initiated movement, and sends idle ActionStateChanged. A further Escape can close a window. State loss or death cancels active Actions; leave-world cleanup removes transient ECS state, and the next snapshot reports idle. The client changes its cursor and ghost only from server state.

The action handler contract must distinguish rejected target, accepted/deferred work, success, and failure. A single executed bool cannot represent lift's asynchronous completion or a rejected placement. ActionService owns the generic lifecycle, while handlers own target-specific validation and effects.

## D4: MapClick precedence

NetworkCommandSystem handles MapClick in this order: pending administrator command, ActionService.HandleArmedClick, cancellation of any approach superseded by an ordinary MapClick, then ordinary pickup/link/movement. Canceling before ordinary routing clears action-owned pending work and movement before the new click creates its own intent; an administrator-consumed click does not cancel the approach. The map-click dispatcher is generic over target.kind and has no lift_down branch. Explicit Interact, build placement, and UI-consumed clicks retain their existing routes; they do not produce a duplicate MapClick. While any object/tile action is selecting, Render sends MapClick even with an item in hand. If object selection does not consume the click, ordinary server map-click routing runs; the item is not dropped. Outside target selection, item-in-hand drop keeps its existing route.

## D5: Complete lift migration

Collider lift uses a new action-owned pending target rather than PendingContextAction. ActionService starts the existing move-to-link behavior, then handles LinkCreated directly, revalidates the target, and calls a new LiftService.StartLift entry point; the context-coupled StartLiftFromContextAction is removed with the context path. It receives a completion/failure outcome and updates the action state. Removing liftBehavior.ProvideActions cannot silently break this route. A click on a non-liftable live object is rejected before movement.

No-collider lift moves the existing phantom-collider and PendingLiftTransition path behind the armed lift handler. The generic Interact route no longer invokes TryStartNoColliderLiftInteract. LiftService reports final success/failure/timeout to ActionService, so the action resets only after success and remains selectable after a target-level failure.

Lift_down's handler passes the clicked position to LiftService without fabricating a C2S_LiftPutDown packet. LiftService keeps its placement validation and deferred transition. Immediate rejection keeps lift_down armed; final successful placement ends it. A later failure or timeout leaves it armed while carry remains valid. Forced carry loss cancels lift_down and resets its cursor. The completion callback and carry-loss callback must produce one terminal state update, even when success clears carry state.

Neither lift definition adds a timed cycle or stamina charge. Their existing approach and placement transitions are still asynchronous domain work, and switching or Escape cancels those transitions and their movement.

## D6: Client menu, cursor, and hotbar

Clicking the Actions button in the left HUD rail toggles a compact, non-modal panel beside the button. The panel presents one horizontal row of action icons in the order received from the server; it does not wrap, and it can scroll horizontally if the row exceeds the available width. Each icon is an action button with the definition's accessible label. Hovering or keyboard focus shows the label and, when unavailable, the reason. Unavailable icons use aria-disabled, remain keyboard-focusable so the reason can be read, and cannot be activated. The active action's icon is visibly and accessibly selected whether the panel was already open or opens while an action is active.

Selecting an available icon sends sendActivateAction and closes the panel so the next map click can choose a target. Dragging an icon to a hotbar slot pins that action. Clicking the Actions button again or clicking outside the panel closes it. Clicking inside the panel does not count as an outside click. The menu and hotbar use the same activation path; the server still validates every request.

gameStore holds ActionList and the last ActionStateChanged. CursorManager applies the declared cursor image on the game window, uses CSS help for unknown IDs, and restores default on an empty ID. LiftGhostController is visible while lift_down is in selecting phase and carry is active; it does not infer armed state from cursor ID. The local liftPutDownModeActive and direct sendLiftPutDown route are removed.

The hotbar accepts window-opener IDs and server action IDs. Persisted gameplay IDs are preserved while ActionList is pending; after the authoritative list arrives, missing IDs render as empty/inert slots. Normalization must not rewrite localStorage before the list arrives.

## Risks and verification

- Lift has two approach paths and deferred placement: tests must prove success, rejection, timeout, cancellation, and forced carry loss each produce exactly one terminal action state.
- Action-list availability must refresh when equipment, skills, and carry change; tests must cover a requirement lost while selecting and while a cycle is active.
- Timed actions need synthetic none, tile, and object handlers to verify cycle completion, optional stamina, cancellation, and repeatability without adding a new gameplay effect.
- Admin pending MapClick must still outrank Actions, and ordinary clicks must remain unchanged when no action is armed.
