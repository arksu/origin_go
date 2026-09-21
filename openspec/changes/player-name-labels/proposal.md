# Proposal

## Why

Players cannot identify each other in the world: the server stores each character's nickname on the entity (`components.Appearance.Name`), but the spawn packet drops it, so no name for a remote player ever reaches the client — only your own name (`S2C_PlayerEnterWorld.name`) and chat sender names (`S2C_ChatMessage.from_name`). With chat balloons now shipped, players see others talking with no way to tell who is who. The proto's unused `EntityAppearance` message confirms this was anticipated but never wired.

## What Changes

- `S2C_ObjectSpawn` gains `string name = 8` and a new `NicknameColor name_color = 9` field; new proto enum `NicknameColor` with `NICKNAME_COLOR_DEFAULT = 0` (ADMIN/NPC values reserved for future roles).
- `buildObjectSpawn` populates both from `components.Appearance` whenever `Name` is non-nil (players today; future NPCs pass through the same gate). The dead `EntityAppearance` message is not revived.
- `components.Appearance` gains a name-color field (zero value = default), mapped to the proto enum at the network boundary (same pattern as `characterAttributeNameToProtoKey`).
- Client: new persistent `NicknameManager` renders every named entity's nickname — including the local player's — as plain outlined text above the head, counter-scaled per frame, mirroring culling, removed on despawn, cleared on world reset.
- Store entity record gains `name`/`nameColor`; the `objectSpawn` handler's early-return branch (same visual generation) still applies name/color updates so respawns stay correct.
- Chat balloons stack one nickname-height above the nickname, which owns the anchor point at the object's visual top.
- Protobuf regeneration for both sides (`packets.pb.go`, `web_new` `packets.js`/`packets.d.ts`).

Non-goals: mid-session renames, wiring actual ADMIN/NPC colors (enum reserved only), per-player display toggles.

## Capabilities

### New Capabilities
- `player-name-labels`: Spawn-borne display names with role colors for named entities; persistent client-side nickname rendering incl. the local player; stacking with chat balloons; cleanup on despawn and world reset.

### Modified Capabilities

(none — no existing spec covers entity naming or appearance streaming)

## Impact

- **Server**: `api/proto/packets.proto` (+ regen of `internal/network/proto/packets.pb.go`), `internal/ecs/components/appearance.go`, `internal/game/events/game_events.go` (`buildObjectSpawn`).
- **Client**: `web_new/src/network/proto/packets.js`/`.d.ts` (regen), `web_new/src/network/handlers.ts` (`objectSpawn` handler incl. early-return branch), game store entity record, new `web_new/src/game/NicknameManager.ts`, `web_new/src/game/Render.ts` (update/clear/destroy wiring), `ChatBalloonManager` anchor offset.
- **No DB or migration changes**; the nickname already exists on `character.Name`.
