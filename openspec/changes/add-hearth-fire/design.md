# Design

## Context

See `proposal.md` for motivation and `docs/features/hearth_fire.md`, Part 2, for the approved gameplay and database contract. This is a cross-cutting persistence, world-lifecycle, action, transfer, and rendering change; no implementation is implied by these artifacts.

Each world layer has its own shard and ECS world. Network commands and server jobs execute through the owning shard. Database work must not be introduced into that tick path, and the shared event bus is not a cross-layer mutation queue. The accepted unified-behavior ADR informs object integration; proposed time/stamina ADRs are not evidence of implemented behavior.

Relevant current boundaries:

- `object` is partitioned by region with primary key `(region, id)`. Character death is a soft delete. Generic object upsert can overwrite type/state, so a transaction alone does not prevent an older chunk save from resurrecting pre-conversion state.
- Build progress lives in the `build` behavior in `object.data`. Fully processed builds can be finalized again by a different actor; the current state does not record the original finisher.
- `spawnAndLogin` first attempts live-body reattachment, then normal spawn candidates. This distinction must remain intact.
- `RequestTransfer` admits an asynchronous transfer, not its successful outcome. Source detach clears transient actions and despawns the source handle. Target spawn and participant restoration precede client attachment; rollback uses the captured source runtime.
- Transfer currently rejects all KO/lying states. Shared action cooldowns commit on successful action completion and survive transfer/rollback and ordinary saves.
- `S2C_Fx` has only key and position. Existing dispatch supports local-player `exp_gain`; object-attached continuous particle effects have separate lifetime rules.

## Goals / Non-Goals

**Goals:**

- Make the active-hearth association and its object conversion durable as one operation, with deterministic ownership, bounded asynchronous work, and replay-safe reconciliation.
- Integrate return with ordinary action progress and transfer outcomes without charging cooldown on admission or failure.
- Keep movement collision, structural placement, and carried-object destination checks explicitly separate.
- Deliver finite event-position effects without making them persistent object/view state.

**Non-Goals:**

- A general transaction framework, new dependencies, replacement of unrelated action/transfer behavior, or broad death/corpse refactoring.
- Legacy backfill, inherited homes, new travel resources, minimap work, or a new persistent owner representation in object JSON.
- Changing existing REST authorization/deletion policy, reconnect body ownership, or post-capture transfer restoration on socket failure.

## Decisions

### 1. Separate the active association from the physical object

Retain the exact approved schema from feature section 2.11:

```sql
ALTER TABLE origin.character
	ADD COLUMN first_world_entry_at TIMESTAMPTZ NULL;

CREATE TABLE origin.character_hearth
(
	character_id BIGINT PRIMARY KEY
		REFERENCES origin.character (id) ON DELETE RESTRICT,
	object_region INT NOT NULL,
	object_id BIGINT NOT NULL,

	CONSTRAINT character_hearth_object_unique
		UNIQUE (object_region, object_id),
	CONSTRAINT character_hearth_object_fk
		FOREIGN KEY (object_region, object_id)
		REFERENCES origin.object (region, id) ON DELETE RESTRICT
);
```

The unpartitioned association's primary key enforces at most one active hearth per character across regions; its unique object key prevents two owners sharing one hearth. Coordinates, surface layer, type, quality, and state remain in `object`. Active-hearth `object.owner_id` stays NULL. Runtime ownership caches are derived from the association, not independently persisted.

FKs guarantee row existence, not an undeleted character, an undeleted hearth type, or layer zero. Lifecycle transactions validate those conditions. RESTRICT blocks accidental physical deletion while linked; soft delete requires explicit conversion and unlinking. Ordinary campfires have no active association after conversion.

Alternative: an owner-based partial unique index on `object` would mix generic attribution with Hearth ownership and cannot provide global owner uniqueness on the region-partitioned table without including the partition key. Duplicating target coordinates on the character would introduce another authority to reconcile.

### 2. Serialize bounded lifecycle operations in accepted gameplay order

Use a small Hearth lifecycle coordinator shared by the game's shards, with bounded per-character pending state. It owns operation identity and sequencing, not live ECS references. Each accepted operation contains immutable EntityIDs, source keys, expected transition, snapshots, and completion state.

Reserve admission before accepting a final construction cycle or first-entry preparation. Do not consume the last branch or stamina and then discover that the bounded operation budget cannot accept ownership activation. Permit at most one pending initialization/replacement per character; ordinary busy admission can retry without changing the accepted finisher. A later terminal deletion is latched behind that operation, not lost because the worker queue is full. Terminal cleanup for an already accepted/registered character has reserved pending capacity and retries submission through existing bounded workers.

The coordinator orders replacement and terminal operations independently of worker scheduling and layer-update order. A replacement accepted before death finishes its durable transition before the queued terminal operation converts its result. Terminal acceptance prohibits later replacements. REST deletion of a character without existing pending state uses bounded service admission and the same terminal lifecycle.

Every transaction first locks the character row with `SELECT ... FOR UPDATE`, even when no association exists. It then rechecks the character's terminal state, expected object source, and current association. Row locks protect durable concurrency; the coordinator, not lock acquisition races, preserves accepted gameplay order.

Alternative: submitting independent workers and relying only on association uniqueness or character locks prevents some duplicate rows but leaves completion/death order dependent on I/O scheduling.

### 3. Reuse chunk persistence gates and durable in-place conversion

Stage object preparation under the owning surface shard, using bounded server jobs where the initiating character is on another layer. Capture immutable snapshots and pin affected chunks. Old and new hearths are both on the surface; unloaded objects are prepared from durable/raw state without activating unrelated world areas or retaining live mutable references.

Worker operations reuse `SubmitPersistenceJob` and `WithPersistence` to serialize loads and old saves for all affected chunks. Under the character-row lock, atomically perform one of these transitions:

- First initialization: create the hearth, create the association, save the prepared player position, and set the first-entry marker.
- Replacement: durably transform the finished site into the new hearth, transform the old hearth into an ordinary unlit campfire, and insert/update the association.
- Terminal deletion: transform the active hearth into an ordinary unlit campfire, delete the association, and write the character's soft-delete/status/token changes.

Whenever the transaction first persists an allocated ID, advance the existing allocator high watermark using `UpsertGlobalVarLongMax` in that transaction. This includes an unsaved build object, not just an automatically allocated hearth.

After a confirmed commit, use owning-shard completion jobs and the common in-place transform helper to reconcile raw cache, ECS type, collider, appearance, behaviors, dirty intent, and derived ownership. Keep pins until this reconciliation completes. Apply bounded work; a stale handle must pass `World.Alive` and source-ID/type checks before mutation. FX are emitted through this owning-shard transition, never directly by a persistence worker.

Unknown commit outcomes retain the exact accepted record and pins until durable state is checked/retried. Definite refusal releases reservations without publishing an active hearth. Completed source keys are never blindly upserted back into build state. A source already transformed to hearth, later converted campfire, or deleted is not a fresh completion; delayed replay must not switch a newer association. Runtime completion acknowledgment and effect publication are idempotent within the retained operation record.

Alternative: transforming runtime first and relying on periodic saves exposes ownership that can disappear on restart, while an isolated transaction without gates still admits stale type/state overwrites.

### 4. Persist the actual finisher while activation is pending

Add optional `hearth_finisher_id` to the existing build behavior JSON only for the Hearth recipe. Set it once when the last branch is successfully processed and its ordinary stamina charge is accepted. Any durable snapshot of a fully processed, pending site includes this ID. Retried finalization and restart recovery use it rather than the calling helper, placer, or current online player.

The accepted immutable persistence record carries the same ID. Successful durable transformation removes the entire build behavior state, including finisher metadata; active ownership then exists only in `character_hearth`.

Alternative: keeping the ID solely in a worker closure loses it when a completed site is saved and restored. Gates and source-type guards alone do not recover the original actor while the object remains a completed site.

### 5. Prepare first entry durably before exposure

Keep character creation as record creation; do not create a hearth there. On normal first spawn, prepare a surface dry-ground candidate that fits both complete footprints. Reserve the initial placement against conflicting structural placements/initializations and retain the needed chunk/capacity lifecycle while the worker commits. Revalidate relevant runtime conditions before exposing the prepared world entry.

`first_world_entry_at` records durable first-entry preparation, not socket acknowledgment. Its transaction commits the hearth, association, chosen player position, and marker before publishing the new hearth/player entry. A rollback leaves the marker NULL. After commit, disconnect, capacity retry, or callback failure retains the prepared hearth and non-NULL marker; recovery installs/reuses the existing object rather than allocating another one. Ordinary character snapshots cannot overwrite the marker or association.

On later normal spawn, try the saved position, exact current hearth position, existing nearby candidates around the saved position, then existing random candidates. Carry the candidate's layer through AOI preparation and attachment: a saved underground position may fall back to a surface hearth, while nearby/random fallback remains on the saved layer according to existing rules.

Attempt live-body reattachment first and unchanged. An existing detached body is not respawned at a hearth or duplicated. Preparation failure before any accepted initialization produces the ordinary spawn error; no client-visible fake success is sent.

Alternative: treating successful packet delivery as initialization would require impossible database/socket atomicity and permits duplicate hearth creation on ambiguous connection failure.

### 6. Keep movement pass-through independent from placement exclusion

Use one shared Hearth bounds definition for the 1x1-tile object and all structural overlap tests. Placement validates the whole footprint on layer zero against passable dry-land terrain, excluding water, shallow water, swamp, and bog. The eternal hearth uses its own definition/behavior, not an infinite synthetic fuel balance in the ordinary burner.

Movement candidate filtering allows only the owning character and animals through an active hearth; other players collide normally. Reuse the same ownership exception in player spawn and return collision checks. Do not implement it with global `IgnoreObjectCollision`, which would suppress unrelated obstacles.

Construction and lifted-object put-down use structural exclusion regardless of mover ownership or creature pass-through. Include reserved/pending structural transitions and existing sites where applicable, so neither placement path can overlap an active hearth. Restore initializes derived ownership/collision before ordinary world exposure, using batched association joins/lookups rather than per-object SQL or tick scans.

Alternative: a single passability predicate would wrongly let owners put objects on their hearth or make animals structural blockers.

### 7. Use a deferred, cause-scoped transfer completion

Register a no-target `Travel to Hearth Fire` action with the existing five-second cyclic progress and six-minute cooldown definition, zero stamina, no repeat, and no new client action protocol. After the progress completes, its handler returns `ActionDeferred`; `RequestTransfer` returning nil is only admission.

Add a Hearth transfer cause carrying the accepted action token/generation, source identity, and outcome context. Resolve the current active hearth at the authoritative transfer handoff, after preceding accepted ownership changes. Coordinate that short handoff with lifecycle replacement so a mid-progress change selects the new hearth instead of canceling or retaining the channel-start destination.

Before detach, under the source shard, recheck the live generational handle, current action token, connection/progress cancellation, health, resolved KO state, combat, swimming, and transfer reservations. Target spawn checks the exact hearth position, current ownership/type, terrain, and unrelated obstacle overlap. There is no nearby/random return fallback. Destination clearance considers only the player's footprint; the existing carry participant transports the lifted object without making its footprint part of target clearance.

After successful target body spawn and ordinary participant restoration or its existing failure-resolution path, use an explicit transfer-success commit hook under the target shard to start the shared action cooldown exactly once before initial action/stat snapshots. Successful authoritative body relocation is the success boundary: a participant warning or ordinary force-drop recovery does not retroactively make the transfer fail or suppress cooldown/standing. It must not call completion on the despawned source handle. All source rejection, target rejection, rollback, and pre-handoff cancellation paths finish unsuccessfully without charging cooldown. Duplicate outcomes cannot commit a second cooldown.

Keep the established post-capture rule: a successful body transfer is authoritative even if the socket closes before attachment. Restore body/carry and queue ordinary logout; do not create an extra rollback solely for network attachment failure. A disconnect while progress is pending cancels before authorized detach through ordinary action cleanup.

Alternative: returning `ActionSucceeded` on async admission charges cooldown before a possibly failing transfer. Copying all transient active actions to the target would broaden unrelated transfer behavior.

### 8. Limit the lying exception to this transfer cause

Hearth eligibility rejects an active knockout but allows lying after the existing KO resolver has cleared that state. Do not infer eligibility only from a raw deadline or change stairs/admin incapacity rules.

Keep the captured source health/runtime snapshot immutable for rollback. Prepare a distinct successful-target runtime copy that stands the post-KO character and advances the existing lying visual revision. Apply that standing state only to successful target publication; a failed transfer restores the original lying state and old cooldowns. Reuse existing stats/pose messages and the same ordinary action behind the knockout-window button.

Alternative: clearing lying on the source or mutating the shared transfer snapshot also makes a failed return stand the character.

### 9. Interrupt at input/health admission without changing unrelated actions

Use the existing progress/action cleanup paths with a narrow Hearth-return check. Any movement command and any attempt to start another gameplay action cancel Hearth return at ingress, before later cooldown/requirement/target rejection. Damage, combat/swimming transitions, and ordinary disconnect/KO/death cleanup likewise invalidate it. Chat and personal-window opening do not.

Recheck the same authoritative conditions at delayed handoff; a queued transfer cannot bypass a cancellation that happened after its handler started. Preserve unrelated actions' admission/cleanup rules. The cooldown belongs to shared successful completion, not the five-second timer.

Alternative: canceling only after displacement or successful other-action admission would miss the approved command-attempt interruption rule.

### 10. Include every terminal soft-delete route without fabricating death events

Actual permanent death retains existing corpse/observer/transient cleanup and emits character-position skull smoke once at that gameplay event, capturing its current observers before visibility changes. Hearth conversion/unlink joins its durable terminal transaction and reconciles on the surface shard, including when the dead owner was underground or disconnected.

Preserve the existing durable ordering of character terminal commit before corpse persistence. Immediate runtime corpse/observer/FX handling can remain, but pin the source corpse chunk and defer both its immediate durable write and ordinary chunk-save exposure until the terminal transaction is confirmed. Capture the corpse record under its owning shard and persist it through workers, never by passing a live world to a worker. If the corpse write needs retry after terminal commit, the character remains terminal; never save a corpse while its owner is still live in SQL. This is a scoped sequencing change, not a general corpse/decomposition redesign.

The REST character-delete route currently writes `deleted_at` directly. Route its terminal persistence through the same ordered Hearth cleanup so the live-owner association invariant cannot be bypassed; preserve its existing authorization and deletion policy. This produces conversion smoke only for an observed hearth that actually changes. It does not invent a character-location death effect or a corpse for an offline REST deletion.

Alternative: integrating only `HandlePlayerPermanentDeath` leaves active immortal hearths linked to soft-deleted characters from the other existing route.

### 11. Extend the existing FX message additively for finite world events

Preserve `S2C_Fx.fx_key = 1` and `position = 2`; add the following unused field numbers:

```proto
uint32 stream_epoch = 3;
uint64 event_id = 4;
int64 server_time_ms = 5;
```

World events use a nonzero monotonic server-instance FX event sequence, never wrap/reuse an ID, and reuse the same ID on duplicate delivery. Allocate and serialize publication synchronously through the owning shard, so a recipient's current world stream sees monotonic IDs. Stamp each recipient's current nonzero epoch and the event's authored wall timestamp. Use owned encoded bytes; no async worker retains mutable scratch buffers or live observers.

Capture event-time observer/session identities from the existing reverse visibility index, plus the dying player's own current session when applicable. Send only to still-matching sessions/epochs; a later viewer or a reattached session is not added retroactively. A conversion in an unloaded/unobserved area emits no future replay. Character death and hearth conversion are separate events at their own locations, each with its own identity.

`skull_smoke` renders at the packet's world position in the current world FX layer, independent of a surviving owner sprite. Require valid current epoch, nonzero exact uint64 event ID, finite position, and valid server timestamp. Maintain/reset the per-stream event highwater using the existing exact-ID decoding pattern. Reject duplicate/stale packets and expire the effect at authored time plus 2,000 ms using existing server-time synchronization; delivery or asset-loading delay does not grant a fresh two seconds. Freshness uses an initialized current-session clock sample; without one, cosmetic events are dropped rather than guessing from an unrelated local clock. Reject future skew over 250 ms, tolerate smaller synchronization skew with age clamped to zero, and retain the original absolute expiry rather than restarting from receipt time.

Use the existing particle engine with a one-shot skull preset and bounded active-effect/particle limits. Preload through existing resource loading; asynchronous loads carry a world/session token and are discarded after reset, expiry, or container destruction. Emission plus every particle's tail ends within the two-second total lifetime. Disconnect, leave/entry, layer change, and renderer reset clear event effects and dedup state. View rebuilds and late object spawns never recreate this event.

Keep `exp_gain`'s existing local-player presentation and continuous object smoke's attach/detach contract; the new world-event validation branch must not reject legacy owner-local FX merely because new fields are absent. An unlit campfire has no continuous smoke; a finite conversion event is not a burner state. No new dependencies are required.

Alternative: only adding a skull texture leaves effects tied to the local player and admits late/duplicate rendering across world transitions. Persisting an FX appearance flag would replay the effect on view rebuild.

### 12. Create assets only during implementation under the approved art contract

Hearth rendering and skull particles will use production assets/presets consistent with existing object/resource integration. Before any Origin art generation/editing, read `docs/art/STYLE_GUIDE.md` and visually inspect `docs/art/style/origin-forest-camp-v1.png`; verify the target projection, scale, direction/frame keys, ground anchor, and transparency rules at that time. These planning artifacts generate no art. Asset pipeline changes must run the repository's corresponding regression checks.

## Risks / Trade-offs

- [Worker saturation or DB outage] -> Admission is bounded before consuming the final build cycle; accepted terminal work remains latched and retryable rather than dropped. Pins and operation state have bounded budgets and are released only after definitive refusal or reconciled completion.
- [Commit succeeds but acknowledgment is lost] -> Retain immutable source/finisher/operation identity and check durable transition state; do not allocate another first hearth, rebuild a transformed site, or overwrite a newer association.
- [Old snapshot overwrites a conversion] -> Gate all affected chunk persistence and reconcile committed raw/runtime state before releasing pins or admitting the next same-character lifecycle operation.
- [Death originates on a different layer] -> Sequence by character, stage surface object work through its shard, and never assume worker-pool layer order or mutate another ECS from the shared event bus.
- [Corpse save overtakes asynchronous death] -> Pin/gate the source corpse chunk and defer its durable saves until terminal character commit; failure-injection tests must prevent simultaneous durable live-character and corpse state.
- [Generic FK permits a wrong type or soft-deleted owner] -> Lifecycle row locks and explicit checks enforce these semantic conditions; restoration validates data and fails safely rather than silently assigning ownership.
- [Spawn reservation survives a callback/connection failure] -> Distinguish pre-commit rollback from committed preparation; recover the existing object/marker and release transient capacity/reservation state without erasing durable initialization.
- [Transfer callback acts on a stale handle/action] -> Validate generation and source identity immediately before detach, commit once on the actual target handle, and retain an untouched rollback snapshot.
- [Network/texture delay or event flood] -> Epoch/ID/timestamp validation, authored-time expiry, bounded caps, and reset tokens prevent replay and unbounded particle work. Visual delivery is best effort and never rolls back gameplay.
- [Extra association lookups at scale] -> Batch chunk ownership hydration and indexed character lookup; keep movement filtering O(local candidates) with cached association data, not SQL or world scans in the tick.

## Migration Plan

1. Implement deployment migration, matching `migrations/schema.sql`, and sqlc lifecycle/lookup queries together. Regenerate with `make sqlc`; never manually edit generated repository code. No legacy-character backfill is included.
2. Regenerate server/client protobuf outputs after the additive FX fields, and deploy matching client assets and server functionality. Preserve the old FX field numbers and local `exp_gain` behavior.
3. Validate schema constraints, first-init replay, pending finisher recovery, completion/death order, REST terminal cleanup, chunk save races, transfer success/failure/lying rollback, and stale FX/session behavior. Run required server/client/asset checks from AGENTS.md for the implemented scope.
4. The project owner performs the planned database/world cleanup before feature release. Neither these artifacts nor the feature implementation automatically erase existing data.
5. Roll back by stopping/draining feature-enabled services before restoring the prior client/server build. Do not remove tables/columns while a running server can write them. Hearth-linked data must first be removed through an explicit coordinated cleanup or restored together from a compatible backup; an old server cannot safely operate a feature-populated world. Fresh-world rollback may use the owner's clean database plan. Schema rollback is a separate controlled step, not a code-only hot rollback.
