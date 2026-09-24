# Design: Remove Game Action Availability

## Context

See `proposal.md` for the motivation and `specs/game-actions/spec.md` for the new behavior. Today `ActionValidationSystem` queries every attached character each tick. `ActionService.Recheck` validates any active action, builds a signature by checking every definition, and may call `SendList`, which checks every definition again. The browser reads `ActionDefinition.available` and `unavailable_reason` to disable menu icons and gate hotbar activation. `ActionService.Activate` and the later target, execution, and commit paths already validate requirements directly.

## Goals / Non-Goals

**Goals:**

- Send a player-independent action catalog on enter-world bootstrap without recomputing per-player action availability.
- Keep rejection reasons and active-action cancellation authoritative on the server.
- Make idle validation ticks independent of player count and action count by visiting only active actions.

**Non-Goals:**

- Adding new actions or changing target selection, stamina charging, action definitions, or map-click routing.
- Building a dirty queue, requirement dependency graph, availability benchmarks, or replacement menu preview.
- Supporting mixed old/new browser and server versions during this protocol change.

## Decisions

### D1: Make the action list a static catalog

`SendList` builds definitions from the startup registry without calling requirement or handler validation. It runs for enter-world bootstrap, including re-entry, and no longer runs after a requirement change or rejected activation. Remove the per-player availability signature and its lifecycle cleanup. Keep the catalog in server order. The alternative dirty-player invalidation plan would require every current and future requirement mutation to mark the right players; that cost is the reason for this change.

### D2: Keep validation at execution boundaries and for active actions

The existing requirement and handler checks remain at activation, target acceptance, handler start, and effect commit. `ActionValidationSystem` uses a prepared query containing `ActiveGameAction` and rechecks only those players each tick for requirement loss, target loss, and approach timeout. This preserves prompt cancellation and no-stamina-on-cancel behavior without listening to every skill, equipment, stamina, and carry mutation. The alternative of removing active rechecks would break the existing cancellation contract.

### D3: Treat every catalog entry as a selectable client action

`ActionsMenu` renders all received entries with their label, icon, drag behavior, and active marker. It removes the disabled style, availability tooltip, and menu-only reason lookup. `GameView` sends activation for any ID present in the loaded catalog, whether selected in the panel or pinned to the hotbar; a missing or not-yet-loaded ID still sends nothing. Panel selection closes the panel immediately, regardless of the server result. Existing `S2C_MiniAlert` handles rejection feedback, so no activation acknowledgment or new packet is needed. Waiting for an acknowledgment would add state and protocol complexity for a panel that now closes on every selection.

### D4: Remove the two protocol fields

Delete `ActionDefinition.available` (field 11) and `unavailable_reason` (field 12) without `reserved` entries, as requested. Regenerate the Go and browser protocol bindings from `api/proto/packets.proto`. Keep `S2C_ActionList`, `S2C_ActionStateChanged`, activation requests, and mini-alerts. An old browser would decode absent `available` as false and disable every action, so the server and maintained browser must be deployed together. Retaining unused fields would leave a misleading availability contract.

## Risks / Trade-offs

- **No up-front requirement preview** → Players can choose an action they cannot perform; the server returns the existing mini-alert with the reason. This is the intended UX trade-off.
- **An active player is missed by the new query** → Add focused tests for selection, timed execution, approach timeout, requirement loss, and late callbacks. Keep direct commit validation as a final guard.
- **A stale catalog ID remains in local storage** → Keep the existing post-load catalog membership check, so the slot behaves as empty and sends nothing.
- **Old and new protocol builds overlap** → Coordinate browser and server deployment or revert both together. Field numbers 11 and 12 are intentionally not reserved.

## Migration Plan

Update the protocol and generated bindings together with the server and browser. No persisted action definitions or hotbar assignments need migration. Replace tests that expect availability refreshes with catalog, rejection, and active-action tests; run the Go suite and browser action tests/build. Roll back the server and browser together if needed.
