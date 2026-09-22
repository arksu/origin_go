# Design

## Context

The server already has a target-first action pipeline: behaviors provide context actions per entity (`ContextActionService`, `S2C_ContextMenu`), and execution rides move → link → validate → execute with `PendingContextAction` consumed on `LinkCreated` (`context_action_service.go:357`). `LiftService` owns lift carry: `StartLiftFromContextAction` as the entry, `HandleLiftPutDown` for placement, `S2C_LiftCarryState` as the broadcast. The client holds a local `liftPutDownModeActive` ref driving `LiftGhostController`, then sends `C2S_LiftPutDown`.

Def-loader and skill-gate precedents exist: `itemdefs`/`builddefs` (types + loader + registry under `internal/`, JSON under `data/`), and craft/build defs carry `RequiredSkills []string` checked via `containsAllStrings` against `CharacterProfile.Skills` (`crafting_service.go:448`). Enter-world list snapshots exist for crafts and builds, queued as jobs (`JobSendCraftListSnapshot`, `game_auth.go:867`). The 12 cursor PNGs in `web_new/public/assets/cursor/` are unused. The next feature, plow, will be a tile-target action with a skill requirement — see proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- One generic armed-action path: arm → server-acked cursor → target click → per-action handler; lift/lift_down are the first consumers.
- MapClick interception isolated to a single dispatch point so plow's future MapClick field changes don't collide.
- Cursor state fully server-owned; client renders only what the last packet said.
- Zero lift_down-specific routing in the dispatcher.

**Non-Goals:**
- No auto-arm of lift_down; no question.png; no skill def registry; no stamina/terrain fields in action defs (plow keeps those in its handler).
- No move-to-tile-then-execute pending state yet (plow may add it; `ArmedAction` is kept cheap to extend with a target position).
- No changes to other context actions (teach, take, open, chop…) or to the cyclic action machinery.

## Decisions

### D1: `internal/actiondefs` mirrors the existing def packages
`types.go` / `loader.go` / `registry.go` + `data/actions/*.json`. Def fields: `id`, `label`, `cursor`, `target_kind` (`object|tile|none`), `required_skills []string`. Duplicate id or malformed file fails startup (same `LoadError` style as itemdefs).
*Why not a hardcoded Go registry?* Handlers must be Go anyway, but label/cursor/skill tuning is content — keeping it in data matches items/builds/crafts and feeds `S2C_ActionList` without recompiling client catalogs. An action ships as def + handler, enforced by a startup check that every registered handler has a def and vice versa.

### D2: `ActionService` owns arming, armed state, and the handler registry
New `internal/game/action_service.go`. `ArmedAction{ActionID}` is a new ECS component. The handler contract:

```go
type ArmedActionHandler interface {
    // TargetKind comes from the def; the dispatcher fills exactly one of the two.
    HandleObjectTarget(w, playerID, playerHandle, targetID, targetHandle) (executed bool)
    HandleTileTarget(w, playerID, playerHandle, position) (executed bool)
}
```

- **lift handler** wraps `LiftService`: armed lift + object click reuses the pending pattern — the service sets `PendingContextAction{target, action_id}` and lets the ordinary link-intent movement proceed; on `LinkCreated` it validates and calls the existing lift entry (renamed `StartLift`). `lift_behavior.go` stops implementing `ContextActionProvider` so the context menu no longer offers lift; the behavior's validation logic moves into the handler (or the behavior stays as the validator-only collaborator — implementation detail, tests decide).
- **lift_down handler** wraps `HandleLiftPutDown` with the clicked position, preserving its placement validation; rejection maps to a mini-alert.
- Skill check is generic in the service (`containsAllStrings(profile.Skills, def.RequiredSkills)`), not per-handler.
- Arming clears any pending context action first — one pending thing per player.

*Alternative considered:* armed click sends `SelectContextAction` and keeps lift in the behavior system — rejected because it would leave lift enumerated in `ProvideActions` (context menu leakage) or need a second enumeration channel.

### D3: MapClick precedence is a single early branch
In `network_command.go`'s map-click handling the order stays: (1) admin pending command (existing code untouched), (2) `ActionService.HandleArmedClick(...)` returning *consumed*, (3) ordinary behavior. The dispatcher knows only def target kinds — no action names. This isolates the interception point so the plow change can extend MapClick parsing independently.

### D4: Protocol — three new messages, one removal
`packets.proto`: `C2S_ActivateAction{action_id}`, `S2C_ActiveCursorChanged{cursor}` (empty = reset), `S2C_ActionList{repeated ActionDefInfo}` with `ActionDefInfo{id, label, cursor, target_kind, required_skills}`. `C2S_LiftPutDown` and its server wiring are removed (superseded by armed tile click); load-test does not use it. Regenerate with `make proto` and `npm run proto` (pbjs/pbts) in `web_new`.

### D5: ActionList rides the enter-world job queue
Mirror `JobSendCraftListSnapshot`: add `JobSendActionListSnapshot` queued in the auth sequence next to the craft list, delivered through the same snapshot-sender interface pattern (`SendActionListSnapshot`). The server also sends `S2C_ActiveCursorChanged{""}` in the snapshot sequence — always-correct default, and the groundwork for restoring a cursor on mid-session reconnect if armed state ever persists.

### D6: Client — cursor store + `CursorManager`, catalog with hotspots
`gameStore.activeCursor: string` set by the new `S2C_ActiveCursorChanged` handler; `CursorManager` watches it and applies `cursor: url(/assets/cursor/<id>.png) <x> <y>, help` on the game-window container. `help` as the CSS-level fallback satisfies the unknown-id → question-mark requirement even when the browser rejects an image. `cursorCatalog.ts` is a static map `id → {path, hotspot}` (hotspots tuned per PNG at implementation; atlas frame dimensions verified then — see Risks). HandOverlay is untouched; it already renders above the cursor. Empty id restores the default cursor.

### D7: Client action ids — widen, don't merge
`actionCatalog.ts` keeps the closed `ActionId` union for window openers. Gameplay ids are plain strings validated against the server list (`gameStore.actionList`). `HotbarAssignment = ActionId | GameplayActionId | null`; hotbar parsing tolerates stale ids (→ empty slot, per spec). The stub `actions` case in `GameView.vue` opens the new `ActionsMenu.vue`, which lists `gameStore.actionList` entries (label + cursor PNG as icon), renders `lift_down` unavailable while `!liftCarryActive`, and dispatches `sendActivateAction(id)`. Hotbar slot activation and menu selection share that sender. The local `liftPutDownModeActive` ref, its toggle, and the direct `sendLiftPutDown` call are deleted; `LiftGhostController` is driven by "lift_down is armed" store state instead.

### D8: Disarm and state-loss wiring
`ActionService.Disarm(playerID)` clears the component and sends the cursor reset. Triggered by: successful execution, toggle re-activation, replacement, and carry-state loss for lift_down (`LiftService.clearCarryStateForPlayer` calls an injected disarm hook — interface, not import). Player death/leave-world removes the component with the entity; no packet is needed once the socket is gone, and the next enter-world sends the default cursor.

## Risks / Trade-offs

- **Lift regression surface** — lift's entry point moves; its core (`LiftService`) must not change. → Only entry wiring changes; existing lift tests are updated to the armed path, and lift context-menu tests are removed with the feature.
- **MapClick interception regressing ordinary clicks** — link intent, pickup, and admin pending are spec'd behaviors. → Interception is one early-return branch after the admin check; the existing map-click and admin test suites must pass unchanged for the no-armed case.
- **Cursor PNG size/hotspot quirks** — browsers are picky about cursor image sizes (>32px may be ignored on some platforms). → Verify frame dimensions during implementation; the CSS fallback chain (`..., help`) degrades gracefully if an image is rejected.
- **Shared `PendingContextAction` between armed and menu flows** — a context-menu selection could be pending when the player arms an action. → Arming clears any pending context action (D2); only one pending intent per player.
- **Manual put-down costs a menu/hotbar visit per placement** — accepted product decision; the hotbar pin is the fast path.

## Migration Plan

Server and client ship together (single repo, dev-stage). No persisted state migration: hotbar localStorage values are either window-opener ids (still valid) or gameplay ids (validated against the live list). Rollback = revert; the removed `C2S_LiftPutDown` is the only wire incompatibility and mixed versions are not a supported deployment.

## Open Questions

- Esc-to-disarm keybinding (client): Esc already closes windows; arming toggles via re-activation covers the spec. Defer the keybinding decision to implementation if it's free; otherwise ship without it.
