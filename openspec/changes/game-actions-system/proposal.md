# Proposal: Game Actions System

## Why

Gameplay actions are fragmented across three unrelated mechanisms: `lift` exists only as a per-target context-menu entry, put-down is a client-local UI mode the server never sees, and the HUD "actions" rail holds only window openers (the Actions button itself is a stub). The next feature on the roadmap — the "plow tile" action, which arms a cursor and targets a tile — needs a generic, server-authoritative action framework to plug into, so the structure must exist before more actions are added ad hoc.

## What Changes

- **Data-driven action definitions** — new `internal/actiondefs` loader + `data/actions/*.json` (mirroring `itemdefs`/`builddefs`): id, label, cursor, target kind (`object` | `tile` | `none`), `required_skills`.
- **Server-authoritative armed actions** — new `C2S_ActivateAction` request; the server validates skill and state, sets an `ArmedAction` component on the player, and replies with `S2C_ActiveCursorChanged {cursor}`. An empty cursor id is a server-initiated reset and may be sent at any time (execution, cancel, state loss).
- **Armed target clicks via MapClick interception** — the server's MapClick handler gives a player's armed action precedence over ordinary click behavior and dispatches to a per-action handler registry (no new target packet). `C2S_LiftPutDown` is superseded and removed (**BREAKING** protocol change; server and client ship together).
- **Lift becomes mode-first only** — `lift` is removed from object context menus; lifting is armed exclusively from the actions menu or hotbar. Put-down (`lift_down`) is armed manually; the server never auto-arms it.
- **Action list at login** — new `S2C_ActionList` snapshot on enter world (same pattern as `S2C_CraftList`/`S2C_BuildList`).
- **Skill gating** — `required_skills` on action defs checked against `CharacterProfile.Skills` using the existing `containsAllStrings` pattern from crafts/builds; the server rejects activation of locked actions. Skills remain plain string ids (no skill def registry).
- **Client cursor system** — new `CursorManager` owning the game-window cursor, driven only by `S2C_ActiveCursorChanged`; a client `cursorCatalog` maps cursor ids to `/assets/cursor/<id>.png` (first consumer of those assets); unknown ids fall back to CSS `cursor: help`, empty id resets to default.
- **Actions menu + hotbar pinning** — the stub Actions button opens the actions menu listing server-provided actions; the hotbar widens to accept gameplay actions alongside window openers (drag-and-drop, localStorage persistence as today).
- **Client cleanup** — the local `liftPutDownModeActive` special case in `GameView.vue` is absorbed into the generic armed flow; the placement ghost is shown while `lift_down` is armed.

Non-goals: no auto-arm of `lift_down` after lifting; no `question.png` asset; no skill def registry; no migration of dig/mine/fish or plow (plow arrives as a follow-up change consuming this framework).

## Capabilities

### New Capabilities
- `game-actions`: Data-driven gameplay actions — server-side def loader, arming protocol with server-owned cursor state, MapClick interception dispatch, skill gating, the actions menu, and hotbar pinning, with lift/lift_down as the first two actions.

### Modified Capabilities
- `map-click-input`: Ordinary map clicks gain a new precedence layer — a player's armed action consumes map clicks before ordinary movement/pickup/linking behavior (below admin pending commands, which keep top precedence).

## Impact

- **Protocol** (`api/proto/packets.proto`): new `C2S_ActivateAction`, `S2C_ActiveCursorChanged`, `S2C_ActionList` (+ action info message); `C2S_LiftPutDown` removed. Regenerated via `make proto` (Go) and `npm run proto` (client pbjs/pbts).
- **Server**: new `internal/game/action_service.go` (+ `internal/actiondefs`, `data/actions/`); `internal/ecs/components` gains `ArmedAction`; `internal/ecs/systems/network_command.go` routes the new C2S and intercepts MapClick; `lift_behavior.go` stops providing a context action; `LiftService` gains handler wrappers; enter-world snapshot job for `S2C_ActionList`.
- **Client** (`web_new`): `game/hud/actionCatalog.ts` widened; new `CursorManager`, `cursorCatalog`, actions menu UI; `ActionsRail.vue`/`GameView.vue`/`useHotbarAssignments.ts` extended; network layer (`index.ts`, `handlers.ts`, `MessageDispatcher.ts`) gains three message types; local lift put-down mode removed.
- **Data**: new `data/actions/` directory with `lift.json` and `lift_down.json`.
- **Load test**: unaffected (does not use `C2S_LiftPutDown`).
