# Design

## Context

See proposal.md for the objective. The outbound path is ECS publication -> asynchronous eventbus worker -> `NetworkVisibilityDispatcher` -> client FIFO -> `writeLoop`. The writer already coalesces writes, but each queued object message still becomes a separate WebSocket frame. This change targets WebSocket messages and application dispatch overhead; it does not promise a proportional reduction in TCP segments.

The sources have different traffic profiles:

| Source | Current messages per observing client | Treatment |
| --- | --- | --- |
| `VisionSystem` and reattach | One spawn per newly visible/snapshotted entity | One message per nonempty observer result/snapshot |
| `TransformUpdateSystem` | One move per visible entry, despite one ECS batch event per update | One message per observer's nonempty subset |
| `LiftCarryFollowSystem` through relocation | One independent event/message per carried object whose position changes | One separate follow event per update, then one message per observer's nonempty subset |
| Link creation, lift stop, pickup/drop/transfer | Occasional individual updates | Preserve immediate delivery; no common tick accumulator |
| Appearance recomputation on activation | Per-object events can dispatch after Vision has added observers and duplicate initial spawns | Suppress publication while there are no observers |

For a continuously moving set at 10 TPS, M ordinary visible movers and C visible carried objects currently produce about `10 * (M + C)` move messages per second per observer, excluding occasional transitions. Batching only transform movement gives `10 * (1 + C)` when M is nonzero. Batching periodic follow separately gives at most 20, or 10 if only one stream has entries. Thus 100 movers with 100 carried objects change from 2000 to at most 20 messages/s; these are analytical counts, not measured performance claims.

Visibility/spawn dispatch already rechecks current world state, and per-client enqueue order is FIFO. Concurrent eventbus jobs do not provide a total publication order across messages. Spawn entries have stream epochs; movement entries do not. `MoveController` sequence filtering does not currently prevent every subsequent store/carry update in `handlers.ts`, so this change must preserve the existing behavior rather than claim a stronger stale-entry guarantee.

## Goals / Non-Goals

**Goals:**
- Reduce the three bulk paths independently: visibility/reattach spawns, ordinary movement, and periodic carry-follow.
- Eliminate initial appearance-upsert duplicates at their source while keeping the final restored state in the initial spawn.
- Preserve payloads, existing client handling, snapshot/enqueue ordering, and send criticality; events own all published slices.
- Verify message reduction and payload completeness together, and measure server serialization cost under fanout.

**Non-Goals:**
- Exactly one movement packet across all sources per tick; removal of single-message senders; timers or a universal outbound accumulator.
- Despawn batching, generic transport envelopes, compression, tick/vision/gameplay changes, or queue-policy unification.
- Adding movement epochs, fixing existing stale-move store/carry behavior, or changing rendering semantics.
- Mandatory Prometheus expansion or manual protobuf wire encoding without measured need.

## Decisions

### Typed batches reuse existing entry messages

Add `S2C_ObjectMoveBatch { repeated S2C_ObjectMove moves = 1; }` and `S2C_ObjectSpawnBatch { repeated S2C_ObjectSpawn spawns = 1; }` on free `ServerMessage` oneof fields. There is no outer epoch. Stamp every spawn entry with the recipient's current `StreamEpoch` under `ClientsMu`, then reuse the existing per-entry client epoch check. Movement retains its existing fields, including the current sequence values of immediate relocations; add neither an epoch nor a new sequence policy.

Use the existing single message for one valid entry, a typed batch for two or more, and send nothing for zero entries. Keep occasional appearance upserts and single-entry movement publications. Removing single send sites would add work without reducing their message count. A generic envelope is unnecessary for the three selected paths.

### Aggregate visibility spawns where the target set already exists

`VisionSystem.publishResultEvents` publishes one `EntitySpawnBatchEvent` for each nonempty observer result; `ForceUpdateForObserver` shares this path. The event contains observer ID, layer, and an owned list of target handles/IDs. Reattach publishes one event for its currently known live targets. Chunk activation still restores objects into the spatial index, and Vision determines which are visible; chunks becoming ready in different visibility updates naturally produce separate batches.

The dispatcher uses one `WithWorldRead` scope through snapshot capture and enqueue, reuses `buildObjectSpawn` validation per target, and rechecks visibility. Dead targets, handle/ID mismatches, missing required components, and snapshot failures skip only that entry; optional components keep their existing fallbacks. Acquire `ClientsMu` once, require a live in-world recipient, stamp entry epochs, serialize, and enqueue before releasing the world read lock. Preserve the existing `PlayerEnterWorld`/epoch publication lock relationship and animation snapshot ordering. Reuse this building logic without forcing occasional appearance upserts into an accumulator.

### Suppress unobserved appearance notifications before enqueue

Apply the appearance/component mutation and behavior recomputation normally. Route the network appearance producers through a small shared publication guard: on the ECS/shard thread, check `VisibilityState.ObserversByVisibleTarget[targetHandle]` under its read lock, release that lock, and publish only if there are current observers. Apply it to behavior recomputation and the direct object-transform, tree, and burner producers. Do not change chunk activation's restoration or dirty-state preservation.

For a newly restored object with no observers, no appearance event enters the eventbus. A subsequent visibility spawn reads the final resource. For an already observed object, the existing appearance-upsert path remains active and its dispatcher still rechecks visibility. Checking only in the async dispatcher is insufficient because Vision can add observers before it runs. This is not general deduplication of changes to already observed objects.

### Batch ordinary movement and periodic carry-follow independently

`handleObjectMoveBatch` groups an event's entries by current observing client, preserving entry order within each subset, and enqueues one message for each nonempty subset. Transform movement keeps its existing one-event-per-update producer.

For carry-follow, separate relocation's world mutation and movement-entry construction from its immediate-publication wrapper. `RelocateWorldObjectImmediate` retains its current success/no-op behavior and immediate publication for existing pickup, put-down, forced-drop, and transfer callers. Periodic `SyncLiftCarryFollow` uses the shared relocation logic and returns an optional movement entry without publishing it; `LiftCarryFollowSystem` collects these entries and publishes one owned nonempty event at the end of its pass. Keep spatial/chunk/transform/dirty-state changes synchronous, as well as invalid-carry cleanup and cached-handle refresh.

A successful unchanged-position relocation yields no entry; it must not be confused with a real relocation merely because the existing API returns `true`. Preserve every emitted field, including carrier ID, timestamp, teleport flag, and current default sequence values. Forced reindex transitions still publish as before. Do not emit both an immediate event and a collected entry for periodic following.

The two periodic producers need no shared flush phase or timer. They can produce two messages per observing client per tick, plus occasional independent link/lift/relocation messages. This preserves the bulk reduction without making unrelated action paths wait for aggregation.

### Preserve ownership, client handling, and criticality

Copy the transform scratch slice before async publication, or transfer a fresh slice that will never be reused. Apply the same ownership rule to spawn and carry-follow batches; do not return published slices to scratch storage while workers can read them. Preserve immutable pointed-to entry values as well.

Extract reusable per-entry client handlers for both single and batch messages. Spawn entries retain their epoch check. Each batch iteration has its own error boundary so an animation-incarnation mismatch cannot prevent other entries from applying. Preserve existing movement sequence filtering and carry-drop snapping without changing downstream store updates. Per-client FIFO is unchanged; no new ordering across asynchronous events is promised.

For a spawn batch, use `SendCritical` if any included entry carries action-animation state; otherwise use `Send`. Movement remains `Send`, and existing singles, despawns, and chunk load/unload policies remain unchanged. Do not infer reliable delivery or self-healing of every isolated transition from this choice.

### Use normal serialization and verify the actual benefit

Start with standard generated protobuf messages and `proto.Marshal` per recipient. Reusing prepared immutable entry objects is possible, but retaining once-per-entity serialization across all observers is not a requirement. Measurement found a material dense-fanout regression, so the implemented encoder also reuses one standard protobuf result for recipients seeing the complete event. Different partial subsets still use per-recipient serialization; measured costs and this limitation are recorded in `validation.md`. Benchmark CPU and allocations against the current per-entry serialization/fanout baseline before introducing custom wire construction.

Verification must include the complete chunk activation -> Vision -> dispatcher -> outbound message path, not only the count of ECS events. With N valid visible objects restored in one visibility update and no unrelated live appearance changes, assert one spawn message, exactly N distinct entries, final resources, and no later restoration-triggered upserts. Deliberately let queued work dispatch after observers have been added. Also cover a chunk arriving after login, forced visibility, and reattach.

Test ordinary and carried movement with several entities and different observer subsets, stationary carries, and mixed periodic/occasional updates. Compare message count and delivered entries under identical inputs. For analytical examples at 10 TPS: 100 ordinary movers imply 1000 -> 10 messages/s; 100 ordinary movers plus 100 moving carried objects imply 2000 -> at most 20, excluding independent transitions.

Update load-test single/batch decoding and report messages, bytes, and decoded entity entries separately. Provide an explicit full-decode validation mode that bypasses the current 100-message cap and tracks position/content for the entire validation run; keep the existing drain-oriented mode available and label its partial entry counters accordingly. A nonzero batch counter alone is not acceptance evidence. Prometheus changes are optional. Record CPU/allocations and timing alongside the packet-count comparison so a serialization regression is visible.

## Risks / Trade-offs

- [Restoration events recreate per-object traffic] -> Test publication-time suppression with delayed dispatch; keep visible-object updates covered.
- [Carry aggregation emits stationary or duplicate updates] -> Distinguish no-op success from an emitted relocation entry and verify the immediate path is not also used by periodic follow.
- [Async workers read overwritten slices] -> Add ownership regression tests that reuse producer scratch after publication.
- [One entry fails on the client] -> Catch per entry and verify later entries still apply.
- [Per-recipient serialization increases server cost] -> Record CPU/allocations under fanout before accepting performance claims; optimize only from measured evidence.
- [Droppable spawn batches can lose multiple objects on overflow] -> Retain the approved existing policy and test its critical/noncritical split; reliability changes are outside this scope.
- [Old clients cannot decode new batches] -> Deploy the matching client/server together and require old tabs to reload; retaining singles is not backward compatibility for new batches.

## Migration Plan

Regenerate Go and web protocol stacks from the same proto edit, then deploy matching server and web-client builds. Both single and batch messages remain supported by the new client and server paths. Existing tabs running old code need a reload to consume batches. Roll back by redeploying the previous matching builds; no persistence or data migration is involved.
