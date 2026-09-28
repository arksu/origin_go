# Design

## Context

Outbound path today (see proposal.md — Why): ECS tick → `PublishAsync` → eventbus workers → `NetworkVisibilityDispatcher` (`internal/game/events/game_events.go`) → `client.sendCh` (FIFO, buffered 132k) → `writeLoop` coalesces queued frames into one TCP flush. Every object update is still one WebSocket frame / one `ServerMessage` per entity per observer.

Key existing mechanics the design must build on, not fight:

- Movement is already aggregated per tick into one `ObjectMoveBatchEvent` (`internal/ecs/systems/transform.go`); the dispatcher pre-serializes each entry once and fans bytes out per observer. Only the "N frames per observer" step remains.
- Spawns are produced per observer×target pair by `VisionSystem.publishResultEvents` (`internal/ecs/systems/vision.go`), by the reattach loop (`internal/game/game_auth.go`), and by `ForceUpdateForObserver`. The per-pair event handler takes a shard read-lock per pair and re-checks visibility at dispatch time.
- Spawn/despawn/move arrive at the client already unordered across topics (concurrent eventbus workers); correctness rests on per-entry guards, not on ordering: despawn re-checks visibility, spawn re-checks visibility under lock, client drops stale moves by `MoveSeq` and gates everything by `stream_epoch`.
- `S2C_EventBatch` (field 45) was considered before and left commented in the proto — no committed prior design exists.
- `eventbus.BatchPublisher`/`SubscribeBulk*` exist but are unused and group by topic globally, not per observer — not suitable as-is.

## Goals / Non-Goals

**Goals:**
- One message per observer per tick for moves; one message per observer per vision update for spawns — fewer frames, fewer per-message wrappers, fewer eventbus hops and shard read-lock acquisitions (login burst: hundreds of RLocks → one).
- Strictly preserve per-entry client-observable semantics (epoch, seq, teleport, carried-by, error isolation).
- Batch events own their data — no aliasing of tick-reused scratch buffers.

**Non-Goals:**
- Despawn batching (follow-up change, same pattern), envelope batching of unrelated message types, compression.
- Changing vision, movement, tick, or drop/overflow behavior of `sendCh`.
- Any change to client rendering logic; handlers get per-entry functions, nothing else.

## Decisions

- **Typed batch messages, not a generic envelope.** `S2C_ObjectMoveBatch` / `S2C_ObjectSpawnBatch` with `repeated` entries inside the existing `ServerMessage` oneof, instead of the commented-out `S2C_EventBatch`-style generic wrapper or raw frame concatenation (impossible: protobufs are not self-delimiting). Typed messages keep the existing dispatch-by-field model, keep per-entry proto definitions untouched, and give the client a narrow change. Alternative rejected: generic envelope would force every consumer (client dispatcher, load test) to loop over nested `ServerMessage`s and gain nothing else.
- **Aggregate spawns at the event level, one event per observer per vision update.** The vision system already has per-observer results with a `spawns` slice (`observerResult`); publishing one batch event mirrors the move-batch idiom exactly. Alternative rejected: accumulating per-pair events in the dispatcher with a flush timer (needs latency-bearing buffering, reorders spawn vs move within a flush window, and duplicates state that vision already holds atomically).
- **Keep single-message handlers on the client as the per-entry implementation.** `handlers.ts` refactors `objectSpawn`/`objectMove` logic into per-entry functions used by both single and batch messages, so batch entries get per-message error isolation (`try/catch` per entry) — the `objectSpawn` handler can throw on animation-incarnation mismatch and must not lose the rest of the batch.
- **Critical-send policy: batch takes the max of its entries' current policies.** Today a spawn is sent `SendCritical` iff it carries an action animation; chunk load/unload are always critical. A spawn batch goes via `SendCritical` when any entry would be critical today, else `Send`. Move batches stay plain `Send` (moves are self-healing: next tick's update supersedes). Alternative rejected: making all spawns critical — an improvement in theory (a dropped spawn permanently desyncs the client), but it changes disconnect-on-overflow behavior; keep for a separate decision.
- **Batch events own their slices.** The spawn batch event is built from a copied slice at publish time. The move batch publication is fixed to publish a copy (or a freshly accumulated slice) instead of the reused `moveBatch` buffer — today `transform.go` publishes a reference to a scratch buffer it truncates next tick, a latent data race whenever an eventbus worker runs longer than one tick (100 ms). Single-entry move publications (`link.go`, `lift_service.go`, `object_relocate.go`) already allocate fresh slices and are unaffected.
- **Regenerate both protocol stacks from one proto edit.** Server via Makefile protoc, client via `npm run proto` (pbjs/pbts). Server and web client ship from one repo; single-message senders are removed server-side in the same change, client single-message handlers kept for in-flight frames.

## Risks / Trade-offs

- [Latent moveBatch race fixed mid-change] → fix is mechanical (copy before publish) and gets a dedicated test; without it, the batching change would inherit and widen the race.
- [Client batch handler drops whole batch on a malformed entry] → per-entry try/catch in the batch handler; the spawn incarnation-mismatch throw stays scoped to one entry.
- [Tests assert single-message sends] → `internal/game/events/*_test.go` updated to batch equivalents; they are the regression net for per-entry guards (late-observer equipment snapshot, delayed despawn visibility re-check, animation ordering).
- [Packet-count metrics lose meaning] → load-test client and prometheus counters get per-entry counters alongside per-message counters.
- [Slightly larger per-frame payloads on wire] → one batch message ≈ sum of entries minus per-message overhead; only degenerate case (single-entry batches) is byte-identical overhead-wise. No meaningful regression: entries were already serialized per entity.
- [Old open tabs receive unknown message types] → client logs unknown types and continues; stale-tab breakage is limited to that tab's session and matches existing unknown-message behavior.

## Migration Plan

Single deploy (server + client from one repo build). Server stops emitting single `object_spawn`/`object_move` in the same release that adds batch emission; client handles both during rollout. Rollback = redeploy previous build; no data or persistence involvement.

## Open Questions

- None blocking. (Despawn batching deliberately deferred; critical-send unification deliberately deferred — both noted as follow-ups.)
