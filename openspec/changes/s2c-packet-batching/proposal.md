# Proposal

## Why

Server-to-client object updates are sent one entity per WebSocket frame: a moving player receives one `S2C_ObjectMove` frame per visible moving entity per tick (10 tps), and a client entering the world receives one `S2C_ObjectSpawn` frame per entity in its area of interest — each frame paying its own WebSocket header, `ServerMessage` oneof wrapper, eventbus dispatch, and client decode/dispatch. The eventbus hop and shard read-lock are also taken per spawn pair. Verified by reading the outbound path (`internal/game/events/game_events.go`, `internal/ecs/systems/vision.go`, `internal/network/server.go`). Batching the per-client stream reduces frame count, lock churn on the login burst, and client-side decode overhead without changing any gameplay semantics.

## What Changes

- **Batched move transport**: add `S2C_ObjectMoveBatch` (repeated per-entity move entries) to the server→client proto. `NetworkVisibilityDispatcher` already receives one movement batch event per tick; it now sends each visible observer one batch message instead of one message per entity.
- **Batched spawn transport**: add `S2C_ObjectSpawnBatch` (repeated per-entity spawn entries) to the proto. `VisionSystem` publishes one spawn-batch event per observer per vision update (instead of one event per observer×target pair); the dispatcher builds all of that observer's spawns under a single shard read-lock and `ClientsMu` acquisition and sends one message. The reattach path in `game_auth.go`, the `ForceUpdateForObserver` path, and the appearance-change upsert path all deliver through the same batch transport, so the server stops emitting single `object_spawn` frames entirely.
- **Batch events own their data**: the new spawn batch event carries its own slice (no cross-tick reuse), and the move batch publication stops aliasing the reused `moveBatch` scratch buffer (currently a latent data race if an eventbus worker reads a previous tick's batch into the next tick).
- **Per-entry semantics preserved**: every entry keeps its own `stream_epoch`, `move_seq`, `is_teleport`, carried-by relation, and visibility re-checks; per-entry build failures skip the entry, not the batch. The client keeps per-entity `MoveSeq` ordering and epoch gating.
- **Critical-send policy unchanged in effect**: a spawn batch is sent via `SendCritical` when any entry would have been sent critically today (spawn carrying an action animation), otherwise via plain `Send`.
- **BREAKING** (server↔client protocol only): new oneof fields in `ServerMessage`; server and web client ship together, single-message `object_spawn`/`object_move` become unused by the server (the client keeps handling them for compatibility with in-flight messages).
- **Load-test client updated** for the new message types so packet/position metrics keep working.
- Non-goals: despawn batching (same pattern can follow later), transport-level envelope batching of unrelated message types, compression, and changes to tick rate, vision, or movement logic.

## Capabilities

### New Capabilities
- `s2c-packet-batching`: server→client object-state transport is batched per client — movement updates arrive as one batch message per tick and spawn updates as one batch message per visibility update; per-entry ordering, epoch, sequence, and error-isolation semantics are preserved for the client.

### Modified Capabilities
- (none — no existing spec covers the server→client protocol)

## Impact

- `api/proto/packets.proto` (new messages + oneof fields) → regenerate `internal/network/proto/packets.pb.go` (Makefile protoc) and `web_new/src/network/proto/` (npm run proto).
- `internal/game/events/game_events.go` (dispatcher handlers for move/spawn batches), `internal/ecs/systems/vision.go` (batch event publication), `internal/game/game_auth.go` (reattach spawn burst), `internal/ecs/events.go` (new batch event type).
- `web_new/src/network/`: `MessageDispatcher.ts` (new message types), `handlers.ts` (per-entry handlers reused by single and batch messages).
- `cmd/load_test/virtual_client.go` (new message cases; metrics semantics shift: fewer packets, same entity updates — add per-entry counters).
- Tests: `internal/game/events/*_test.go` assert single-message sends and need batch equivalents; `internal/network` tests unaffected.
- Monitoring: packet-rate metrics drop in meaning without per-entry counters.
