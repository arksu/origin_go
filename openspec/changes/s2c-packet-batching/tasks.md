# Tasks

## 1. Protocol

- [ ] 1.1 Add `S2C_ObjectMoveBatch` (repeated per-entity move entries, `repeated S2C_ObjectMove moves`) and `S2C_ObjectSpawnBatch` (repeated `S2C_ObjectSpawn spawns`, `uint32 stream_epoch`) to `api/proto/packets.proto` in the `ServerMessage` oneof on free field numbers; verify `make` protoc step regenerates `internal/network/proto/packets.pb.go` and `cd web_new && npm run proto` regenerates `src/network/proto/packets.{js,d.ts}` without errors
- [ ] 1.2 Regenerate both protocol stacks and confirm `go build ./...` and `web_new` `npm run type-check` compile against the new messages

## 2. Server: batch events own their data

- [ ] 2.1 Make the move-batch publication stop aliasing the reused scratch buffer: `TransformUpdateSystem` publishes a copy (or a slice that is not truncated next tick) of the movement batch before `PublishAsync`; add a test that fails when the published event's entries are mutated by the next tick (reuse/overwrite of the scratch buffer) and passes with the copy — `go test ./internal/ecs/systems/ -run Transform`
- [ ] 2.2 Add `EntitySpawnBatchEvent` (observer, target list with handles and entity ids, layer) to `internal/ecs/events.go` that owns its slice; verify `go build ./...`

## 3. Server: spawn publication aggregates per observer

- [ ] 3.1 `VisionSystem.publishResultEvents` publishes one `EntitySpawnBatchEvent` per observer result (and one despawn path unchanged); same for `ForceUpdateForObserver`; verify `go test ./internal/ecs/systems/ -run Vision`
- [ ] 3.2 Reattach burst in `internal/game/game_auth.go` replaces its per-entity `NewEntitySpawnEvent` loop with one `EntitySpawnBatchEvent`; verify `go build ./...`

## 4. Server: dispatcher sends batch messages

- [ ] 4.1 `handleObjectMoveBatch` in `internal/game/events/game_events.go` sends each visible observer one `S2C_ObjectMoveBatch` (per-observer marshal with the client's stream epoch where required) instead of N `Send` calls; per-entry single serialization reuse preserved; verify updated `go test ./internal/game/events/`
- [ ] 4.2 New `handleEntitySpawnBatch`: under one `shard.WithWorldRead`, build entries per target with the existing `buildObjectSpawn` guards (alive, external id match, component presence, snapshot errors skip the entry), visibility re-check per entry, one `ClientsMu` acquisition, `SendCritical` iff any entry carries an action animation else `Send`; verify a new unit test covers entry skip on dead target and the critical/droppable split
- [ ] 4.3 Appearance-change upsert path (`handleEntityAppearanceChanged`) delivers through the same batch message per observer; verify updated `go test ./internal/game/events/ -run Appearance`
- [ ] 4.4 Remove single-message `object_spawn`/`object_move` send sites server-side (single spawn handler becomes unused; single-entry move publications keep using the batch event); verify `grep` finds no `ServerMessage_ObjectSpawn{`/`ServerMessage_ObjectMove{` construction outside tests and `go build ./...`

## 5. Client: batch-aware dispatch

- [ ] 5.1 Extract per-entry logic of `objectSpawn`/`objectMove` handlers in `web_new/src/network/handlers.ts` into reusable per-entry functions with per-entry error containment; single-message handlers delegate to them; verify `npm run type-check`
- [ ] 5.2 Register `objectSpawnBatch`/`objectMoveBatch` in `MessageDispatcher.ts` and add batch handlers: whole-message stream-epoch gate for spawn batches, entry loop calling the per-entry functions, `move_seq`/carry-drop snap semantics per entry; verify `npm run type-check` and a manual dev-run smoke (spawn burst renders, movement smooth, equipment upsert applies)

## 6. Load test client

- [ ] 6.1 Add `ObjectSpawnBatch`/`ObjectMoveBatch` cases to `cmd/load_test/virtual_client.go`: per-message and per-entry counters, player position tracking from batch entries; verify `go build ./...` and a short local load run records nonzero batch counters

## 7. Regression and full verification

- [ ] 7.1 Update existing single-message assertions in `internal/game/events/*_test.go` (character visual, nickname, burner appearance, action animation ordering) to batch equivalents preserving their original invariants (late-observer equipment snapshot, delayed visibility re-check, animation ordering); verify `go test ./internal/...`
- [ ] 7.2 Add a dispatcher test that one tick with several movers yields exactly one message per observer, and a vision test that one update yields one spawn batch event per observer with one entry per newly visible target; verify `go test ./internal/game/events/ ./internal/ecs/systems/`
- [ ] 7.3 Run the full verification suite: `go test ./internal/...`, `cd web_new && npm run type-check && npm run lint`, and confirm `openspec validate s2c-packet-batching --strict` passes
