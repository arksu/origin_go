# Design

## Context

See `proposal.md` and `docs/features/hearth_fire.md`, Part 2, for gameplay. On 2026-10-13 the owner reverted Hearth gameplay/persistence code and requested a simpler design: runtime integrity is required; asynchronous saves, independent checkpoints, and loss of recent progress after a crash are acceptable.

The prepared `hearth_fire` definition, seven green-flame frames at 70% scale, and eight approved skull-smoke frames still exist. Tasks 1.1-1.4 provide the ownership migration/schema, sqlc queries and isolated metadata index. The construction stage now connects that index through `hearth.Runtime` and the result's `hearth` behavior; it assigns the actual finisher and replaces their previous home in memory. Ownership saving, general chunk hydration and restart repair remain unimplemented. Finisher codecs and lifecycle coordinators are not introduced; previous reverted gameplay is not counted as delivered work.

The database is assumed available. Any database read, write or commit error must stop the entire server; Hearth adds no SQL retry, reconnect or database-error recovery path. An expected absent row, a runtime optimistic-version conflict and a full worker queue are ordinary outcomes, not database failures. Asynchronous saving and recovery of partial saved checkpoints remain unchanged. This is the integration policy, not a claim that the current server already has a universal fatal database-error handler.

Relevant existing mechanisms:

- One shard/World owns each layer. Hearths and their construction sites are surface-only, so both sides of a replacement belong to one surface World.
- Build cycles already process materials and stamina under the owning shard lock. `TransformObjectToDefInPlace` updates a live object's type, collider, behaviors, appearance and dirty intent without changing its ID or position.
- Chunk persistence has bounded workers, I/O gates and raw cache. `PinPersistenceBlocking` waits for the existing I/O mutex, increments its pin count and releases the mutex before loading; `core.Chunk.UpsertRawObject` marks the replacement row dirty for ordinary saving.
- `finalizeCompletedBuild` now propagates the transform result while keeping ordinary link/window cleanup before transformation.
- Player transfers are asynchronous; accepting `RequestTransfer` is not successful relocation. Action progress, cooldowns, carry snapshots, generation checks and rollback already exist.
- `S2C_Fx` currently carries a key and position. Existing dispatch is local-player-oriented; finite world effects need detached rendering, not a new persistence subsystem.

## Goals / Non-Goals

**Goals:** immediate, unambiguous runtime ownership; ordinary construction/action/transfer reuse; feature-owned policy; bounded background saving; safe restoration of potentially partial saved state.

**Non-Goals:** lossless crash recovery, a globally consistent world checkpoint, commit-before-gameplay publication, lifecycle operation journals/coordinators, generic mutation reservations, special corpse/character save barriers, or a new global shutdown protocol. No automatic first-entry hearth, inheritance, backfill, new dependencies, balance changes, or minimap work.

## Decisions

### 1. Keep ownership in the Hearth module

Use `internal/game/hearth` for home lookup, current ownership, conversion policy and association persistence. It owns a current `character -> home` cache and reverse object-owner index. Distinguish an unknown lookup from known absence; an absent home never creates one automatically. Store copied IDs, positions and versions, not pointers to live components.

Current runtime state wins over SQL results. `Version` supplies optimistic locking for home changes, lookup results and save acknowledgments. One `loading` flag admits at most one home read per character; accepted reads are never canceled and their submitter delivers each result exactly once. Protect shared index access across layers with short synchronization, never holding that lock across ECS callbacks, shard locks or I/O. Geometry checks use the cached owner, not SQL.

Cache entries are needed for connected characters, active/loading hearth objects and dirty ownership changes, not all historical characters. Do not evict dirty entries or revocation overrides. Retire clean idle entries only after their writes and referencing lookups finish. A read remains loading even after logout or a runtime home change; its sole result clears loading before checking the observed version, so stale data cannot strand the entry. No permanent per-character terminal registration is required.

The delivered index returns copied `Snapshot`/`Home` values. A single monotonic sequence supplies `Version uint64` values for entry creation and runtime changes; versions never repeat after eviction/recreation or wrap on exhaustion.

`BeginLookup(character)` sets loading and returns the observed version; `CompleteLookup(character, version, home)` consumes the read result. No token or lookup ID exists. Only the successful `BeginLookup` caller may invoke `RejectLookup(character)`, once and immediately after a rejected submission guarantees no job or result can follow. Rejection leaves the home version unchanged. An accepted read cannot be canceled through logout, timeout or deferred cleanup. The submitter must neither lose nor redeliver its result, including after a later read begins; the index does not deduplicate deliveries across attempts.

`ReplaceHome` changes both indexes together and requires the expected version; `AcknowledgeSaved` only clears that exact dirty version. Callers retain metadata through `Retain`/release-once `Reference` while bodies, objects or writes refer to it. Reference release state and counts share the index mutex; live references prevent eviction without separate generation tracking or atomics. No ECS hooks or saver jobs are installed in this stage.

Terminal eligibility is distinct from `NoHome`. `MarkTerminalPending` blocks new acquisition/return without mutating the current association from a notifying thread. An already admitted owning completion may finish before `ApplyTerminal` revokes the latest home. Applied terminal rejects rebinding and remains cached only while dirty or referenced. These contracts do not yet connect to death or REST and add no terminal operation queue.

### 2. Finalize construction immediately, in one surface critical section

Integrate through one optional construction-result wrapper capability using the existing behavior registry. The result behavior prepares its policy before invoking the ordinary completion callback. Shared construction does not check a Hearth key or carry a finisher field.

Preparation runs before the last material and stamina costs. Unknown ownership uses a direct `GetCharacterHearth` with the existing five-second timeout under the surface shard lock, then hydrates the index immediately. A missing saved row establishes known absence. An unloaded old chunk is also awaited directly through `WaitPreloaded` with its own five-second context. Successful preparation completes construction in the same call without an extra action cycle, retry result or another click. The wrapper returns ordinary `BehaviorResult`; there is no Hearth preparation queue, completion channel or `Shard.Update` apply hook.

This explicitly chosen blocking tradeoff delays the whole game loop during a slow query, busy I/O gate or chunk load: `ShardManager` waits for all layers before the next tick, and serial wait latency adds up. Movement/cancel commands arriving during the wait remain queued until the call returns. Expected chunk unavailability during shutdown refuses completion before costs; genuine database/I/O errors remain fatal. Existing chunk workers perform the load, while the owning call waits and then applies gameplay. Do not introduce polling, a special pending-finalization phase or remote-area activation.

When ready, revalidate the exact actor, site handle, current home revision and old source. Under the single surface shard lock, the wrapper invokes ordinary final-branch processing and site completion, converts the previous home to an ordinary unlit campfire, and switches the feature-owned association to the actual finisher. Preserve ordinary completion order: close windows/break links before transforming the site. Propagate the transform's actual `bool` result instead of reporting unconditional success. Prepare the concrete Hearth/campfire definitions and source data before costs; no generic transform transaction or rollback framework is needed.

For a non-active old home, call `PinPersistenceBlocking` before `WaitPreloaded`, then hold the existing pin through the synchronous callback and raw update with deferred unpin. The small blocking helper reuses the current gate and pin counter: it waits for `ioMu`, increments the counter and releases `ioMu`. The nonblocking pin would return busy; holding `WithPersistence` while waiting would prevent the loader from acquiring that mutex. Apply the prepared campfire row with `UpsertRawObject`, then release the pin; do not spawn or activate the old object. There is no new gate, cache-revision change, polling loop or pin held until SQL commit.

The game sees the final result immediately. Later save snapshots see either previous build progress or the final object, not a paid completed site waiting for a DB commit. The actual finisher is available in this synchronous call and becomes the current owner there. Neither `BuildBehaviorState.HearthFinisherID` nor `behaviors.hearth_construction` is needed. No feature state is added to shared ECS/build contracts.

This construction stage reads saved ownership when needed and marks the runtime association dirty. Ownership saving, batch chunk hydration, restart repair, owner-death cleanup, collision/terrain rules, return and FX remain separate stages. Ordinary object saves do not persist the new association; this intermediate stage does not preserve newly assigned ownership across a server restart.

### 3. Save current state, not a history of operations

Keep the simple active-only unpartitioned table:

```sql
CREATE TABLE origin.character_hearth
(
	character_id BIGINT PRIMARY KEY
		REFERENCES origin.character (id) ON DELETE RESTRICT,
	object_region INT NOT NULL,
	object_id BIGINT NOT NULL,
	CONSTRAINT character_hearth_object_unique UNIQUE (object_region, object_id),
	CONSTRAINT character_hearth_object_fk FOREIGN KEY (object_region, object_id)
		REFERENCES origin.object (region, id) ON DELETE RESTRICT
);
```

The keys enforce one saved home per character and one saved owner per object. Coordinates/type/state remain in `object`; Hearth ownership does not use `object.owner_id` or object JSON. There is no first-entry marker, finisher table, pending operation table or cooldown column.

After runtime changes, mark the latest ownership projection dirty. Reuse existing bounded persistence workers and the current saver pattern: coalesce changes per character, allow one in-flight association write per character, and resubmit dirty state when a full queue rejects admission. A database read, write or commit error stops the entire server instead of retrying SQL. This is a dirty-state saver, not a completion/death operation queue. An immutable worker snapshot carries its version; only a matching acknowledgment clears dirty intent. Workers never change runtime ownership, transforms or FX.

A modest association transaction ensures the current target object exists for its FK, converts a previously linked saved Hearth if necessary, and sets/deletes the link. A deleted saved character cannot acquire a link. For newly persisted IDs reuse allocator watermark advancement. Normal chunk objects, character state and corpses continue their ordinary saving; they do not join an all-or-nothing Hearth checkpoint. Reuse `WithPersistence` for affected object I/O, outside shard locks. Do not pin gameplay objects until commit.

An unknown commit outcome is a fatal database error: stop the entire server without retry, replay or commit probing. Replacements occurring while a write is in flight remain authoritative in memory and produce a newer dirty projection. Queue pressure does not roll runtime back or assign a different owner.

Ordinary shutdown makes a bounded best-effort flush using existing worker lifetimes before their closure. Report remaining dirty state on failure; do not introduce shared participant/drain infrastructure or retain dead bodies to make flushing lossless.

### 4. Hydrate and normalize before exposure

Prepare owners in one batch while a chunk loads, then apply copied metadata through the owning shard before object activation/visibility. Add only the narrow metadata preparation/apply hook needed by this responsibility. Preserve existing burner/corpse restoration composition; no new deferred-restore framework or generic codec registry is required.

In the running process, known runtime ownership and retired-object overrides supersede stale SQL/load results. A current home cannot be replaced by an old hydration callback. Reuse chunk load/raw revisions and the feature's single-read/optimistic-version contract; do not perform per-object SQL or scan the world each tick.

After restart, accept only a saved association whose character is alive and whose object is undeleted, on the surface and of Hearth type. An invalid association means no home and can be cleaned asynchronously. A restored Hearth without a valid current owner becomes an ordinary unlit campfire before visibility. Do not infer ownership from JSON/`owner_id`, upgrade a saved campfire from the link, replay construction, or recreate missing objects. Partial saves can lose a home or restore an earlier saved result; they cannot publish two runtime homes.

### 5. Death uses ordinary lifecycle paths

Actual permanent death immediately revokes the current home in the feature-owned index. If the source is another layer, enqueue ordinary surface work; only the surface owner transforms its live/raw Hearth. Until that work applies, the old object is already unavailable as an active home. Retain a coalesced feature conversion intent if an existing inbox is full, not the dead body or a heavyweight terminal operation. Process only changed homes with bounded work, not a full-world sweep.

Use a small registered terminal notification at actual death and the existing REST deletion boundary. Preserve REST authorization, response and online/offline policy. Do not manufacture an offline body or death FX. A delayed conversion must match the revoked object's identity and cannot affect a newer home. Character/corpse persistence and decomposition retain existing semantics; this feature does not promise atomic status/home/corpse persistence or terminal-commit-before-corpse ordering.

### 6. Keep object policy separate from geometry

Reuse the prepared definition and existing indestructibility; add the Hearth behavior without burner/fuel runtime or lift capability. Protect removal through a small registered object policy before gameplay/admin deletion, without adding a Hearth branch to the deletion core.

Use identical 1x1 bounds for terrain, collision and structural overlap. Placement checks every overlapped surface dry-land tile, excluding water, shallow water, swamp and bog. Movement/spawn/return filtering allows the owner and animals through this object only; unrelated obstacles still block. Construction and put-down separately reject footprint overlap, including owner attempts, at admission and final application. Reuse spatial queries and bounded buffers.

### 7. Return is an ordinary action with an asynchronous outcome

Register the no-target action with existing five-second progress, zero stamina and 360000 ms cooldown. Read the latest runtime home, not SQL, and resolve it again on the target surface shard immediately before authoritative relocation. Coordinates captured at progress completion or queue admission are preparation hints, not a fixed destination. If a newer home needs preparation, reuse ordinary bounded transfer preparation/timeout and revalidate afterward; never spawn at a retired home. Add only neutral source-validation, target-resolution and successful-target completion hooks to existing transfer; Hearth supplies its policy through registration. Return `ActionDeferred` on accepted transfer work, then start cooldown exactly once after successful body relocation, before initial target snapshots. Failure/rollback starts none.

Revalidate the action generation, connection, health/KO, combat and swimming before detach. Destination checks exact home identity/location and player footprint only; no nearby fallback. Use the existing carry participant. Allow post-KO lying only by Hearth transfer policy: use a separate standing target snapshot and preserve the lying rollback snapshot. Other transfer causes retain their rules and ordinary post-capture disconnect handling.

Interruption is an optional active-handler policy invoked through existing cancellation paths at valid gameplay intent, before another action's eligibility/cooldown rejection. Include fresh movement attempts, context/craft/build/carry/Stand Up attempts, any damage, combat/swimming and ordinary disconnect cleanup. Invalid/stale input is not gameplay intent. Chat and personal windows do not interrupt. The client sends existing `C2S_CancelAction` when its cooldown UI blocks another attempted action during return; no new command protocol is needed.

Login keeps `tryReattachPlayer` first. Normal spawning tries saved position, exact current home if present, nearby saved-position candidates, then existing random fallback. Carry each candidate's layer. Missing ownership never creates an object or changes ordinary first-entry spawn eligibility.

### 8. Reuse approved assets and a small transient FX path

Keep the existing green fire and skull-smoke assets; do not generate replacements. Play the eight skull frames once over two seconds at the event position, independent of the surviving object/player view. The renderer is reusable for other death events; animal event wiring is not added by this change.

Reuse `S2C_Fx` fields 1/2; add only `stream_epoch = 3` for world-event freshness. Publish once from real runtime death/conversion to event-time observers and matching sessions. No persistence worker sends effects, and restore/view rebuild never synthesizes old events. No event-ID sequence, retransmission/dedup service, server-authored expiry or new clock protocol is required.

Preload through existing asset loading, bound concurrent one-shot effects, and use a local monotonic two-second deadline starting at playback. Clear effects on world/layer/session/reset/destroy and invalidate late callbacks with the existing reset-generation pattern. Suspended rendering does not extend an already started effect. Legacy local-player `exp_gain` and continuous burning-campfire smoke retain their behavior.

## Risks / Trade-offs

- [Crash or independent save snapshots] -> Recent construction, home changes or death can roll back or lose a home. Conservative restore removes invalid runtime ownership; no lossless or cross-feature checkpoint is claimed.
- [Unknown ownership or unloaded old-home chunk] -> Read/wait synchronously before final costs with separate five-second contexts; accept whole-loop delay, cumulative latency and queued cancellation during the call.
- [Stale load or write completion] -> Existing raw/load revisions plus feature lookup/dirty revisions prevent callbacks from overriding known current runtime state.
- [Database error] -> Stop the entire server, including on unknown commit outcome; do not add SQL retries or database-error recovery.
- [Queue pressure] -> Coalesce dirty projections and resubmit rejected admissions through bounded workers. Observe dirty count/save age without per-entity metric labels or a new persistence platform.
- [Cross-layer terminal work] -> Revoke the logical home immediately, apply physical conversion through the surface owner, and validate identity on delayed work.
- [Transfer admission confused with success] -> Use existing deferred action/rollback and a target-success callback; do not charge cooldown at queue admission.
- [Cosmetic delay] -> FX is best effort and locally short-lived. It has no durable exactly-once delivery guarantee and does not roll back gameplay.

## Migration Plan

1. Add the Hearth association migration, canonical schema and sqlc queries together; regenerate rather than hand-edit generated code. The feature is undeployed, so no corrective migration or legacy backfill is needed.
2. Implement and test runtime construction/ownership before adding background save and return. Prepared art remains input; previous reverted code is not counted as delivered work.
3. Regenerate server/client protobuf only for the additive FX epoch field. Run server, PostgreSQL schema/projection, client and relevant asset checks for implemented scope.
4. The owner prepares the approved clean database/world before release. The application performs no automatic database cleanup. Rollback of a populated feature world requires a compatible backup or explicit cleanup; dropping the table under a running writer is not supported.
