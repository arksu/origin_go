# Proposal

## Why

Initial chunk visibility sends one spawn message per visible object, while ordinary movement and carried-object following send one move message per visible updated entity; restoration can also generate redundant appearance-upsert spawns. Aggregate these high-volume paths and suppress redundant initial notifications to substantially reduce WebSocket message count, especially during chunk loading, without redesigning occasional gameplay messages.

## What Changes

- **Visibility spawn bursts:** add `S2C_ObjectSpawnBatch` and publish one owned spawn event per observer per visibility update, including forced updates and the reattach snapshot. Build and enqueue the observer's valid entries under one shard read-lock and one client-map lock acquisition.
- **No redundant initial appearance notifications:** apply appearance state changes normally, but do not publish a network appearance event when the object has no observers at publication time. Its later visibility spawn contains the final state. Appearance changes of already observed objects retain their upsert delivery.
- **Ordinary movement:** add `S2C_ObjectMoveBatch`; send each observer's visible subset of the existing transform movement event in one message.
- **Periodic carry-follow:** collect movements of carried objects into a separate event per follow-system update instead of publishing one relocation event per object. Keep immediate world/spatial updates and occasional pickup, put-down, forced-drop, transfer, and link notifications.
- **Preserve existing entry semantics:** reuse `S2C_ObjectSpawn` and `S2C_ObjectMove` inside the batches. Spawn epochs remain per entry; movement gains no epoch. Retain current sequence/teleport/carry handling and isolate client entry failures. All asynchronously published batch slices own their data.
- **Keep singles and send policies:** isolated updates remain valid single messages. A spawn batch is critical if any included entry carries action-animation state; movement batches remain droppable. There is no requirement to combine all sources into one message per tick.
- **BREAKING** (server↔client protocol only): new batch oneof fields require the matching web client. Server and client continue supporting single messages, but old clients cannot consume the new batches.
- **Verify the reduction:** count WebSocket messages, bytes, and delivered entity updates in representative chunk, movement, and carry scenarios; compare server CPU/allocations before and after. Full-content validation must not stop decoding after the load client's current first-100-message limit.
- Non-goals: universal batching, despawn batching, generic envelopes, compression, new movement ordering/epoch semantics, changed queue-overflow policy, and mandatory Prometheus expansion.

## Capabilities

### New Capabilities
- `s2c-packet-batching`: reduce messages in visibility-spawn, ordinary-movement, and periodic carry-follow bursts while preserving entity payloads and occasional single-message delivery.

### Modified Capabilities
- None; existing gameplay, character-animation, and name-label requirements remain unchanged.

## Impact

- `api/proto/packets.proto` and generated Go/JavaScript/TypeScript protocol files.
- Spawn/move dispatch and publication: `internal/game/events/game_events.go`, `internal/ecs/events.go`, `internal/ecs/systems/{vision,transform,lift_carry_follow}.go`, `internal/game/{game_auth,lift_service}.go`, and `internal/game/world/object_relocate.go`.
- Visibility-aware appearance publication shared by object behavior recomputation, object transformation, and tree/burner behavior producers; chunk activation keeps computing final state before visibility delivery.
- `web_new/src/network/{MessageDispatcher,handlers}.ts`, focused client/server regression tests, and load-test decoding/counters/reporting in `cmd/load_test/`.
