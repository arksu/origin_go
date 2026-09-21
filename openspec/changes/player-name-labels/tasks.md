# Tasks

## 1. Proto and server

- [x] 1.1 Add `NicknameColor` enum (`NICKNAME_COLOR_DEFAULT = 0`; comments reserving ADMIN/NPC) and fields `string name = 8`, `NicknameColor name_color = 9` to `S2C_ObjectSpawn` in `api/proto/packets.proto`; regenerate with `make proto` and confirm `internal/network/proto/packets.pb.go` compiles (`go build ./...`)
- [x] 1.2 Add name-color field to `components.Appearance` (`internal/ecs/components/appearance.go`) whose zero value means default; `go build ./...` passes
- [x] 1.3 In `buildObjectSpawn` (`internal/game/events/game_events.go`), populate `Name`/`NameColor` from `components.Appearance` when `Name` is non-nil, mapping color via a boundary function (pattern of `characterAttributeNameToProtoKey`); unit test in the style of `internal/game/events/burner_appearance_test.go` asserts: player spawn carries nickname + `NICKNAME_COLOR_DEFAULT`, a tree/object spawn carries empty name; `go test ./internal/game/...` passes

## 2. Client data path

- [x] 2.1 Regenerate client proto (`npm run proto` in `web_new`) and confirm `packets.js`/`packets.d.ts` expose the new enum and fields
- [x] 2.2 Extend the game store entity record with `name`/`nameColor`; `spawnEntity` stores them; `npm run type-check` passes
- [x] 2.3 In `handlers.ts` `objectSpawn`, pass `msg.name`/`msg.nameColor` through on fresh spawns AND in the early-return branch (same visual generation) so respawns update name/color on the existing entity; `npm run type-check` passes

## 3. Client rendering

- [x] 3.1 Create `web_new/src/game/NicknameManager.ts` cloning ChatBalloonManager mechanics minus bubble/expiry: plain outlined `Text`, entity-keyed map in `objectsContainer`, very high zIndex, per-frame counter-scale, anchored at object bounds top, `visible` mirrored from the object's culling state, `show`/`update`/`clear`/`destroy`; client palette maps `NicknameColor` → fill (DEFAULT first, unknown falls back to DEFAULT); `npm run type-check` and `npm run lint` pass
- [x] 3.2 Wire `NicknameManager` into `Render.ts`: per-frame `update(objectManager)` next to `chatBalloonManager.update`, label creation/update on spawn via `GameFacade`, removal on `despawnObject`, `clear()` in the world-reset path (mirror lines ~692) and `destroy()` (~713); `npm run type-check` passes
- [x] 3.3 Offset `ChatBalloonManager.update` anchoring up by the nickname label height when the labeled entity has a visible label (nickname keeps bounds top); `npm run type-check` passes

## 4. Verification

- [x] 4.1 Full server suite: `make test` (or `go test ./...`) passes
- [ ] 4.2 Full client checks: `npm run type-check` and `npm run lint` pass in `web_new`
- [ ] 4.3 Manual two-client session: each client sees the other's nickname and its own over every player head; labels track movement and keep constant size on zoom; label hides with culled entity; chat balloon stacks above nickname without overlap; label disappears on despawn (walk apart) and reconnect clears all labels; tree/campfire/item drops show no label
