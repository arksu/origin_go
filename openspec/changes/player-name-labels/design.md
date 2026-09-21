# Design

## Context

See proposal.md — Why. Current state that shapes the approach:

- The nickname already exists server-side on the entity: `components.Appearance{Name *string, Resource string}` is added at player spawn (`internal/game/game_auth.go`). No color/role concept exists anywhere yet.
- `buildObjectSpawn` (`internal/game/events/game_events.go`) already reads `Appearance.Resource` into `S2C_ObjectSpawn.resource_path`; the name is dropped there. Spawns are rebuilt on appearance/equipment changes (`EntityAppearanceChangedEvent` → full `S2C_ObjectSpawn`), so a spawn-borne name is naturally refreshed.
- The client has a proven floating-text pattern: `ChatBalloonManager` (`web_new/src/game/ChatBalloonManager.ts`) — parented to `objectsContainer`, very high zIndex, counter-scaled each frame from camera zoom, anchored to the object's local bounds top, mirrors culling visibility, wired into `Render.ts` update/clear/destroy.
- `S2C_ObjectSpawn` uses fields 1–7; 8+ are free. Proto style is zero-based enums with semantic defaults (`CHAT_CHANNEL_LOCAL = 0`). Boundary-mapping precedent: `characterAttributeNameToProtoKey` (`internal/game/shard.go`).
- The local player's own entity arrives through the same `objectSpawn` path (`handlers.ts` special-cases it for the camera), so self needs no separate transport.

## Goals / Non-Goals

**Goals:**

- One wire change that carries name + role color for any named entity, leaving future NPC/admin roles additive-only.
- Client label rendering that reuses the chat-balloon mechanics rather than inventing a new overlay layer.

**Non-Goals:**

- Wiring actual ADMIN/NPC colors (enum values reserved only; nothing populates them yet).
- Mid-session renames (name treated as static per entity lifetime).
- A separate name-only packet or its lifecycle tracking.
- Per-player display toggles or UI settings.

## Decisions

### D1: Transport — fields on `S2C_ObjectSpawn` (not `EntityAppearance` revival, not a separate packet)

Add `string name = 8` + `NicknameColor name_color = 9` to `S2C_ObjectSpawn`, populated in `buildObjectSpawn` when `Appearance.Name` is non-nil.

- vs reviving `EntityAppearance` (proto has a dead message with `resource`+`name`): more structure, zero current benefit — nothing else would go in the message. It stays dead or is deleted during regen.
- vs a dedicated `S2C_EntityName` packet: names are static per entity lifetime; a spawn already fires exactly when a client (re)learns about an entity, and spawns are re-sent on appearance changes anyway. A separate packet would need its own lifecycle handling (per-entity "sent?" tracking, resend on reconnect) for no bandwidth win.
- Re-sending a short string on equipment-change respawns is negligible.

### D2: Color as proto enum `NicknameColor`, not packed RGB

`NICKNAME_COLOR_DEFAULT = 0`; ADMIN/NPC reserved for future. Rationale: the stated use is roles, not arbitrary colors; proto3 zero-default idiom means "absent = default" for free; visual style stays client-side (server says "admin", client decides admins are gold). Adding a role = one proto edit. Packed RGB `uint32` was the alternative — wins only if arbitrary server-chosen colors (guild/event dyes) become a need; 0-as-default would be an overloaded sentinel.

### D3: Server component — `components.Appearance` gains a color field, mapped at the boundary

`Appearance` gains e.g. `NameColor` whose zero value maps to `NICKNAME_COLOR_DEFAULT`; `buildObjectSpawn` maps it to proto via a small boundary function (same pattern as `characterAttributeNameToProtoKey`). ECS components stay proto-free. Today every entity maps to DEFAULT; future admin/NPC features just populate the component.

### D4: Client — new `NicknameManager` cloning ChatBalloonManager mechanics, minus bubble and expiry

New `web_new/src/game/NicknameManager.ts`: plain `Text` with dark outline (Pixi `Text.style`), parented to `objectsContainer`, very high zIndex, per-frame counter-scale from camera zoom, anchored at the object container's local bounds top, `visible` mirrored from the object's culling state, removed on `despawnObject`, cleared on world reset (mirror the ChatBalloonManager hooks in `Render.ts` update/clear/destroy). Keyed by entityId; `show(entityId, name, color)` on spawn, `setColor`/rebuild on respawn updates. Client owns the palette map (enum → fill). Known Pixi gotcha from the balloon work: y-negative coordinates for the tail/anchor base — bounds-top anchoring already handles this pattern.

### D5: Store + respawn early-return branch

Entity record gains `name`/`nameColor`. In `handlers.ts`, the early-return branch (same visual generation + same resource + same typeId) currently skips everything except equipment and carry visuals — it MUST still apply name/color updates, because a respawn is exactly when a re-rendered entity needs them (and the server re-spawns on appearance change). Spec: "Late name and color updates are applied on respawn".

### D6: Stacking — nickname owns the anchor, chat balloon floats above it

`ChatBalloonManager.update` anchors the balloon tail at the object's bounds top; when a nickname label exists for the entity, the balloon offsets up by the label height (one constant, since labels are fixed-size text). Nickname keeps `boundsTop`. Alternative (balloon keeps the anchor, label moves) rejected: the balloon is transient and its position would then depend on label existence in both directions.

## Risks / Trade-offs

- [Label overlaps nameplates of stacked/carried entities (item drops carried by players)] → Labels render only for entities with a name; carried items have none. Balloon-offset pattern reusable if carried-entity labels ever collide.
- [Spawn packets grow for every player in view] → Two short fields per spawn, only for named entities; spawns are infrequent relative to movement packets.
- [Early-return branch forgotten during implementation silently keeps stale names] → Spec requirement makes it testable; tasks call it out explicitly.
- [Unknown enum values from a newer server] → proto3 preserves unknown enums; client palette falls back to DEFAULT (spec'd).
- [Proto regen churn in `packets.pb.go` / `packets.js`] → Mechanical; generated files committed as usual.

## Migration Plan

Purely additive wire fields — no migration. Old clients ignore fields 8/9; new clients handle empty name as "no label". Deploy server and client independently in any order.

## Open Questions

None — remaining unknowns (exact palette hex values, label font size) are visual tuning inside the client palette/style constants, safely deferred to implementation.
