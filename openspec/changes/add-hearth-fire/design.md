# Design

## Context

See `proposal.md` and `docs/features/hearth_fire.md`, Part 2, for gameplay. On 2026-10-13 the owner reverted Hearth gameplay/persistence code and requested a simpler design: runtime integrity is required; asynchronous saves, independent checkpoints, and loss of recent progress after a crash are acceptable.

The prepared `hearth_fire` definition, seven green-flame frames at 70% scale, and eight approved skull-smoke frames still exist. Tasks 1.1-1.4 provide the ownership migration/schema, sqlc queries and isolated metadata index. The construction stage now connects that index through `hearth.Runtime` and the result's `hearth` behavior; it assigns the actual finisher, replaces their previous home in memory and saves construction synchronously. Login also restores existing home metadata before reattachment or spawning. Terminal persistence, general chunk hydration and restart repair remain unimplemented. Finisher codecs and lifecycle coordinators are not introduced; previous reverted gameplay is not counted as delivered work.

The database is assumed available. Any database read, write or commit error must stop the entire server; Hearth adds no SQL retry, reconnect or database-error recovery path. An expected absent row, a runtime optimistic-version conflict and a full worker queue are ordinary outcomes, not database failures. Construction now waits for its save; independent checkpoints and future asynchronous terminal persistence remain acceptable. This is the integration policy, not a claim that the current server already has a universal fatal database-error handler.

Relevant existing mechanisms:

- One shard/World owns each layer. Hearth construction can complete only on the surface, so both sides of a successful replacement belong to one surface World. Ordinary build-site admission does not filter the layer.
- Build cycles already process materials and stamina under the owning shard lock. `TransformObjectToDefInPlace` updates a live object's type, collider, behaviors, appearance and dirty intent without changing its ID or position.
- Chunk persistence has bounded workers, I/O gates and raw cache. `PinPersistenceBlocking` waits for the existing I/O mutex, increments its pin count and releases the mutex before loading; `core.Chunk.UpsertRawObject` marks the replacement row dirty for ordinary saving.
- `finalizeCompletedBuild` now propagates the transform result while keeping ordinary link/window cleanup before transformation.
- Player transfers are asynchronous; accepting `RequestTransfer` is not successful relocation. Action progress, cooldowns, carry snapshots, generation checks and rollback already exist.
- `S2C_Fx` currently carries a key and position. Existing dispatch is local-player-oriented; finite world effects need detached rendering, not a new persistence subsystem.

## Goals / Non-Goals

**Goals:** immediate, unambiguous runtime ownership; ordinary construction/action/transfer reuse; feature-owned policy; simple synchronous construction saving; bounded future terminal saving; safe restoration of potentially partial saved state.

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

`Runtime.LoadHome(character, hearthType) error` is shared by construction and login. In `spawnAndLogin`, after validating the character's layer and before `tryReattachPlayer`, resolve the surface runtime and catalog `hearth_fire` definition and load under the surface shard lock, including for underground login. Known runtime metadata skips SQL; `Unknown` uses the existing `GetCharacterHearth` with an independent five-second context, then `CompleteLookup` fills both index directions or establishes `NoHome` for `sql.ErrNoRows`. Keep the existing single-read/version protection. Missing dependencies or remaining `Unknown` refuse entry; genuine database errors and invalid results remain fatal. An accepted read finishes despite client cancellation, then login checks disconnect/timeout again before reattachment. Release the surface lock before reattachment or spawning, without holding two shard locks together. This restores metadata only: it does not create or activate objects, repair chunks, change spawn candidates, or connect terminal cleanup/return.

### 2. Finalize construction immediately, in one surface critical section

Integrate through one optional construction-result wrapper capability using the existing behavior registry. The result behavior prepares its policy before invoking the ordinary completion callback. Shared construction does not check a Hearth key or carry a finisher field.

Preparation runs before the last material and stamina costs. Unknown ownership uses a direct `GetCharacterHearth` with the existing five-second timeout under the surface shard lock, then hydrates the index immediately. A missing saved row establishes known absence. An unloaded old chunk is also awaited directly through `WaitPreloaded` with its own five-second context. Successful preparation completes construction in the same call without an extra action cycle, retry result or another click. The wrapper returns ordinary `BehaviorResult`; there is no Hearth preparation queue, completion channel or `Shard.Update` apply hook.

This explicitly chosen blocking tradeoff delays the whole game loop during a slow query, busy I/O gate, chunk load or construction save: `ShardManager` waits for all layers before the next tick, and serial wait latency adds up. Movement/cancel commands arriving during the wait remain queued until the call returns. Expected chunk unavailability during shutdown refuses completion before costs; genuine database/I/O errors remain fatal. Existing chunk workers perform the load, while the owning call waits and then applies gameplay. Do not introduce polling, a special pending-finalization phase or remote-area activation.

When ready, revalidate the exact actor, site handle, current home revision and old source. Under the single surface shard lock, the wrapper invokes ordinary final-branch processing and site completion, converts the previous home to an ordinary unlit campfire, and switches the feature-owned association to the actual finisher. Preserve ordinary completion order: close windows/break links before transforming the site. Propagate the transform's actual `bool` result instead of reporting unconditional success. Prepare the concrete Hearth/campfire definitions and source data before costs; no generic transform transaction or rollback framework is needed.

For a non-active old home, call `PinPersistenceBlocking` before `WaitPreloaded`, then hold the existing pin through the synchronous callback, raw update and construction save with deferred unpin. The small blocking helper reuses the current gate and pin counter: it waits for `ioMu`, increments the counter and releases `ioMu`. The nonblocking pin would return busy; holding `WithPersistence` while waiting would prevent the loader from acquiring that mutex. Apply the prepared campfire row with `UpsertRawObject`; do not spawn or activate the old object. Release the pin after the save returns. There is no new gate, cache-revision change or polling loop.

Runtime changes precede serialization and SQL; the owning call reports success only after commit. Later save snapshots see either previous build progress or the final object, not a paid completed site waiting for a DB commit. The actual finisher is available in this synchronous call and becomes the current owner there. Neither `BuildBehaviorState.HearthFinisherID` nor `behaviors.hearth_construction` is needed. No feature state is added to shared ECS/build contracts.

This construction stage reads saved ownership through the same `Runtime.LoadHome` used by login and saves the completed objects and association in the same call. Batch chunk hydration, restart repair, owner-death/REST cleanup and persistence, collision/terrain rules, return and FX remain separate stages.

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

Construction first applies final-branch costs, object transforms and `ReplaceHome` under the surface shard lock, then serializes the final objects. One existing `WithTx` call with a five-second context saves the new Hearth, the previous unlit campfire if present, `character_hearth` and the allocator's durable ID watermark through `UpsertGlobalVarLongMax`. Success is returned only after commit; `AcknowledgeSaved` clears the association's dirty state only for the saved version. Keep ordinary object/chunk dirty flags so their normal persistence paths remain intact. Hold an old raw chunk's pin until this save returns.

Serialization, SQL or commit failure, including an unknown commit outcome, stops the entire server. A `SetCharacterHearth` affected-row count other than one is also fatal: no successful completion may omit its association. There is no runtime rollback, SQL retry, completion job queue or commit probing. Normal character, inventory, material, stamina and corpse saving remains independent; this transaction is not an all-or-nothing world checkpoint. Schema, protocol and recipe do not change.

Future death/REST persistence remains a separate latest-projection saver using existing bounded workers, at most one in-flight write per character and coalesced dirty state. Rejected queue admission keeps dirty intent. Copy snapshots with their version; a matching acknowledgment alone clears dirty, and workers never change runtime ownership, transforms or FX. Use existing chunk I/O gates outside shard locks for that future saver. Its modest transaction converts affected saved objects and removes the association without changing ordinary destruction ordering or reviving deleted objects.

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

Use identical 1x1 bounds for terrain, collision and structural overlap. Hearth's recipe uses `disallowedTiles: [1, 3, 50, 53, 56, 255]` with existing `BuildService.validateTileRules` over the result footprint at build admission; other known tile IDs remain allowed. There is no additional final terrain recheck, layer field or build placement behavior policy. Existing Hearth completion remains surface-only. Movement/spawn/return filtering allows only the owner through this object; other players, animals and unrelated obstacles retain ordinary collisions. Manual put-down separately rejects footprint overlap at admission and final application; construction exclusion remains future work.

Owner-only passage is implemented through one optional callback bound to the surface `hearth.Runtime`, shared by collision candidate filtering and authoritative player spawn. `IgnoreObjectCollision(actorID, targetHandle)` validates the live target's current Hearth behavior and composite identity against `Index.Owner`; unknown or ineligible ownership gives no exemption. No behavior capability/context/dispatcher is needed for this single rule. Existing terrain, phantom and other-object checks remain authoritative. Animals receive no exemption; task 3.2 is complete.

Manual `lift_down` and RMB placement use a generic check in `LiftService` on every layer. Check before creating pending/phantom and again before restoring the carried object's runtime; refusal preserves carry and uses existing attempt cleanup and `LIFT_PUTDOWN_INVALID`. The shape and masks come from the saved `OriginalCollider`, or the current definition's optional `lift.placementSize` for colliderless objects: log_x 24x6, log_y 6x24, default 2x2 with Layer/Mask 1. Size config is validated at catalog loading; ordinary pickup, movement and approach phantom remain unchanged.

Reuse `WorldColliderSpatial.Index.QueryBoundedInto` with a LiftService-owned 4096-handle buffer, 1024-cell and 4096-visit budgets. Strict positive-area overlap with any live collider using ordinary movement mask compatibility blocks placement; exact edge/corner contact is allowed. Skip the carrier, carried object and candidates with a phantom, matching existing movement filtering. Hearth ownership and behavior are not consulted, so an ordinary campfire remains an obstacle after conversion. Retain the existing active-anchor-chunk check and query loaded colliders only, including those extending across chunk boundaries; do not add neighbor readiness or deferred activation checks. A missing spatial index or query error rejects the attempt without I/O. Forced lifecycle drops are outside this step; task 3.4 remains partial until construction exclusion is implemented.

### 7. Return is an ordinary action with an asynchronous outcome

Register the no-target action with existing five-second progress, zero stamina and 360000 ms cooldown. Read the latest runtime home, not SQL, and resolve it again on the target surface shard immediately before authoritative relocation. Coordinates captured at progress completion or queue admission are preparation hints, not a fixed destination. If a newer home needs preparation, reuse ordinary bounded transfer preparation/timeout and revalidate afterward; never spawn at a retired home. Add only neutral source-validation, target-resolution and successful-target completion hooks to existing transfer; Hearth supplies its policy through registration. Return `ActionDeferred` on accepted transfer work, then start cooldown exactly once after successful body relocation, before initial target snapshots. Failure/rollback starts none.

Revalidate the action generation, connection, health/KO, combat and swimming before detach. Destination checks exact home identity/location and player footprint only; no nearby fallback. Use the existing carry participant. Allow post-KO lying only by Hearth transfer policy: use a separate standing target snapshot and preserve the lying rollback snapshot. Other transfer causes retain their rules and ordinary post-capture disconnect handling.

Interruption is an optional active-handler policy invoked through existing cancellation paths at valid gameplay intent, before another action's eligibility/cooldown rejection. Include fresh movement attempts, context/craft/build/carry/Stand Up attempts, any damage, combat/swimming and ordinary disconnect cleanup. Invalid/stale input is not gameplay intent. Chat and personal windows do not interrupt. The client sends existing `C2S_CancelAction` when its cooldown UI blocks another attempted action during return; no new command protocol is needed.

After restoring home metadata, login keeps `tryReattachPlayer` before normal spawning. The future spawn fallback tries saved position, exact current home if present, nearby saved-position candidates, then existing random fallback. Carry each candidate's layer. Missing ownership never creates an object or changes ordinary first-entry spawn eligibility; metadata loading does not implement this fallback.

### 8. Reuse approved assets and a small transient FX path

Keep the existing green fire and skull-smoke assets; do not generate replacements. Play the eight skull frames once over two seconds at the event position, independent of the surviving object/player view. The renderer is reusable for other death events; animal event wiring is not added by this change.

Reuse `S2C_Fx` fields 1/2; add only `stream_epoch = 3` for world-event freshness. Publish once from real runtime death/conversion to event-time observers and matching sessions. No persistence worker sends effects, and restore/view rebuild never synthesizes old events. No event-ID sequence, retransmission/dedup service, server-authored expiry or new clock protocol is required.

Preload through existing asset loading, bound concurrent one-shot effects, and use a local monotonic two-second deadline starting at playback. Clear effects on world/layer/session/reset/destroy and invalidate late callbacks with the existing reset-generation pattern. Suspended rendering does not extend an already started effect. Legacy local-player `exp_gain` and continuous burning-campfire smoke retain their behavior.

## Risks / Trade-offs

- [Crash or independent save snapshots] -> Recent construction, home changes or death can roll back or lose a home. Conservative restore removes invalid runtime ownership; no lossless or cross-feature checkpoint is claimed.
- [Synchronous construction I/O] -> Read/wait before final costs with separate five-second contexts, then save with a five-second transaction context after runtime completion; accept whole-loop delay, cumulative latency and queued cancellation during the call.
- [Synchronous login lookup] -> Unknown home metadata is read under the surface shard lock with a five-second timeout; accept whole-loop delay as for construction. Client cancellation does not abandon an accepted lookup and is checked again before entry.
- [Stale load or write completion] -> Existing raw/load revisions plus feature lookup/dirty revisions prevent callbacks from overriding known current runtime state.
- [Database error] -> Stop the entire server, including on unknown commit outcome; do not add SQL retries or database-error recovery.
- [Future terminal queue pressure] -> Coalesce dirty projections and resubmit rejected admissions through bounded workers. Observe dirty count/save age without per-entity metric labels or a new persistence platform.
- [Cross-layer terminal work] -> Revoke the logical home immediately, apply physical conversion through the surface owner, and validate identity on delayed work.
- [Transfer admission confused with success] -> Use existing deferred action/rollback and a target-success callback; do not charge cooldown at queue admission.
- [Cosmetic delay] -> FX is best effort and locally short-lived. It has no durable exactly-once delivery guarantee and does not roll back gameplay.

## Migration Plan

1. Add the Hearth association migration, canonical schema and sqlc queries together; regenerate rather than hand-edit generated code. The feature is undeployed, so no corrective migration or legacy backfill is needed.
2. Implement and test runtime construction/ownership with synchronous construction saving before adding terminal persistence and return. Prepared art remains input; previous reverted code is not counted as delivered work.
3. Regenerate server/client protobuf only for the additive FX epoch field. Run server, PostgreSQL schema/projection, client and relevant asset checks for implemented scope.
4. The owner prepares the approved clean database/world before release. The application performs no automatic database cleanup. Rollback of a populated feature world requires a compatible backup or explicit cleanup; dropping the table under a running writer is not supported.
