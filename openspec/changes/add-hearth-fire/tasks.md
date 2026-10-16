# Tasks

## Existing Preparation and Revised Baseline

The server catalog contains the prepared static, indestructible
`hearth_fire` (ID 36, resource `hearth_fire`,
12x12 collider), now with the `hearth` construction behavior and no station. The ordinary client
resource uses the approved seven green flame frames at 70% scale and 4 FPS.
Eight approved skull-smoke frames and their asset descriptor are also present;
the descriptor specifies 250 ms per frame and a 2000 ms one-shot animation.
Reuse these assets instead of regenerating them.

Previous feature persistence and lifecycle implementation was reverted.
The foundation contracts in section 1 provide migration/schema, sqlc queries
and a runtime metadata index with tests. Task 2.1 adds the ordinary build recipe.
The construction stage now assigns its actual finisher in runtime and replaces
their previous home, using synchronous ownership lookup, old-chunk preparation
and construction saving.
Its verification is tracked below. Task 3.5 delivers synchronous chunk hydration and
orphan normalization; its completed verification is recorded below. Task 4.1 and the
death portion of 4.3a add synchronous permanent-death cleanup. REST deletion,
broader restart repair and the remaining gameplay remain separate stages.
This replaces the former durable-lifecycle plan:
construction changes runtime first, then returns success after its database commit.
Other independent saves may lose recent progress on a crash. Runtime
ownership, object state, action cancellation, and stale-handle safety remain
authoritative; no lifecycle coordinator, mutation-reservation framework,
finisher sidecar, body-retention system, or special shutdown barrier is required.

The database is assumed available. Actual database errors, including an unknown
commit outcome, must stop the entire server without SQL retries or database-error
recovery. Expected absent rows remain ordinary outcomes; invalid results are fatal.
All Hearth SQL runs synchronously under the surface shard lock with five-second
read/transaction contexts. Future REST cleanup must follow the same policy;
it does not describe an already-installed universal database-error handler.

## 1. Ownership Data and Runtime State

- [x] 1.1 Add the single undeployed migration and matching canonical schema for unpartitioned `character_hearth`: character PK/FK, unique `(object_region, object_id)`, and composite RESTRICT object FK. Verify PostgreSQL rejects duplicate or invalid associations; add no first-entry marker, compatibility migration, or backfill.
- [x] 1.2 Add sqlc queries for indexed home/owner lookup, batched owner loading, latest association replacement/removal, and restart repair; run `make sqlc` and cover absent ownership, deleted characters, and invalid object references without hand-editing generated code.
- [x] 1.3 Add the feature-owned runtime home index with explicit unknown/known-no-home/current-home states and `LoadIfUnknown`. Preserve known runtime state and copied metadata, retain `PendingTerminal`/`Terminal` with a short mutex, and keep records for the process lifetime. Remove loading/version/dirty/acknowledgment and retain/eviction machinery; test assignment, replacement, removal, terminal notification during SQL and copied reverse snapshots that include pending owners.
- [x] 1.4 Document runtime authority, schema/index responsibilities, synchronous owner-path SQL, accepted whole-loop blocking and process-lifetime index growth. Verify ordinary character saves cannot overwrite home ownership and shared ECS/build/action state contains no Hearth-specific fields.

## 2. Manual Construction and Immediate Replacement

- [x] 2.1 Add the universally available Hearth recipe using existing two-second and eight-stamina processing per branch. Verify solo cycle count and total costs from the recipe configuration, cooperative acceleration, and no skill/discovery requirement.
- [x] 2.2 Add one optional registered construction-result wrapper returning ordinary `BehaviorResult` and keep Hearth policy in its feature module. Before final-cycle costs, resolve unknown ownership through direct `GetCharacterHearth` and await an unloaded old chunk through `WaitPreloaded`, each with a five-second context under the surface shard lock. Hydrate the index and complete in the same call without extra cycles, retry results or Hearth preparation queues/channels. Accept whole-game-loop blocking, cumulative serial latency and queued movement/cancel commands. Expected shutdown unavailability refuses before costs; genuine database/I/O errors remain fatal.
- [x] 2.3 Complete Hearth synchronously on the surface owning shard using the actual successful final-branch actor. Prepare concrete source/definition checks, invoke ordinary completion with its existing link-before-transform order, propagate the transform result, convert the previous home to an ordinary unlit campfire, and replace ownership; test exact identity/location preservation and unchanged player position without a saved finisher sidecar.
- [x] 2.4 Apply an inactive old-home conversion without forcing activation. Use `PinPersistenceBlocking` to wait for the existing gate, increment its existing pins and release the mutex before `WaitPreloaded`; defer unpin until completion, `UpsertRawObject` and synchronous construction saving return. Add no new gate or `cacheRevision` changes. Test unloaded/busy preparation, raw identity preservation and activation of the converted campfire.
- [x] 2.5 Add cooperative and competing completion coverage and update construction notes. Verify first-home construction, helper-finisher ownership, replacement of only the finisher's home, repeated completion rejection, stale handles/results and unchanged ordinary recipe costs without admission tickets or a retained mutable build site.

Construction verification, including task 4.3: focused Hearth/construction and
writer unit tests, plus `GOCACHE=/tmp/origin-hearth-go-cache make test`, passed.
Regeneration produced no changes. Tests use real catalogs and stubbed
ownership-read, chunk-worker and persistence boundaries. PostgreSQL fixture
tests were skipped because `ORIGIN_OBJECT_HEALTH_TEST_DSN` was unset; this stage
does not claim a live-database integration check. This earlier result predates the task 3.5 index simplification and activation work;
current checks are reported separately.

## 3. World Rules and Ownership Restoration

- [ ] 3.1 Extend the prepared Hearth definition with feature behavior, fixed green appearance, and the approved exact 1x1 footprint. Reuse indestructibility/non-liftability; add a small registered removal policy checked before gameplay/admin deletion changes memory or SQL. Test damage, removal, and lift rejection while ordinary campfires remain unchanged, without a Hearth key check in shared deletion code.
- [x] 3.2 Only the owner passes through the hearth. Other players and animals retain ordinary collision. One optional callback shared by movement and authoritative player spawn keeps policy in `hearth.Runtime`, without a behavior capability/dispatcher, global collision bypass or tick-time SQL. Verify owner passage/spawn, non-owner blocking, unrelated obstacles, unknown ownership and home replacement.
- [x] 3.3 Configure Hearth `disallowedTiles: [1, 3, 50, 53, 56, 255]` and reuse standard build validation at placement admission over the result collider footprint. Test allowed tiles, every forbidden ID, neighboring footprint overlap, exact boundary contact and missing chunk data. Preserve ordinary build timing without a new final terrain recheck or layer field; existing Hearth completion remains surface-only.
- [x] 3.4 Complete structural-footprint exclusion, including owner attempts, through existing spatial queries shared by manual put-down and construction placement.
  - [x] 3.4.1 Reject manual carried-object placement (`lift_down` and RMB) at request and final application through the shared generic `colliderPlacementAllowed` helper on every layer. Use saved collider dimensions/masks or optional lift placementSize (log_x 24x6, log_y 6x24; default 2x2 with Layer/Mask 1). Query loaded colliders with ordinary masks; exclude carrier, carried object and phantom candidates without Hearth-specific filtering or neighbor readiness checks. Verify exact contact, cross-chunk colliders, bounded query refusal, stale attempts and preserved carry; forced lifecycle drops remain outside this step.
  - [x] 3.4.2 Reuse the shared collision helper at construction request and final placement with the current result collider, before new pending/phantom or site/ID creation. Cover ordinary construction sites, owner/non-owner attempts, the complete footprint, late obstacles, exact contact and replaced previews; retain admission-only terrain checks and existing `BUILD_PENDING_FAILED` cleanup.
- [x] 3.5 Before raw activation, use one generic optional `ChunkManager.SetActivationPreparer` callback after existing gate/state guards. `Runtime.PrepareChunkActivation` filters current raw Hearth candidates, uses `SnapshotByObject` including PendingTerminal, and runs one existing `GetCharacterHearthOwners` batch for unresolved keys with a five-second context. Hydrate offline owners via LoadIfUnknown; known state wins. Prepare true-orphan campfires with `rawCampfire` and commit all copies via `ConstructionSaverDB.SaveOrphans` through existing `UpsertObject` in one synchronous five-second transaction before UpsertRawObject/activation. Preserve ordinary gates/generations/dirty flags; move dead-observer UnregisterEntityAOI under the existing shard lock. Verify no-query paths, batch counts, pending/dead/offline owners, stale data, skipped deleted/live duplicates and fatal failures. No new SQL/schema or worker framework.

Task 3.5 verification: focused Hearth/login/activation tests and race checks for
Index/Hearth/world/game passed. The final `make test` passed with
`ORIGIN_OBJECT_HEALTH_TEST_DSN` pointing to an isolated temporary PostgreSQL
instance and `GOCACHE=/private/tmp/origin-go-build-cache`; regenerated files were
unchanged. New PostgreSQL tests ran successfully for mixed live/deleted/unowned
Hearths, restart restoration and multirow transaction rollback. Regression
coverage also verifies activation is a no-op when the catalog has no Hearth
definition. `BenchmarkIgnoreObjectCollision`: 98.16 ns/op, 0 B/op, 0 allocs/op;
this is a local microbenchmark, not a 30,000-CCU result. Strict OpenSpec validation
and `git diff --check` passed. Independent code review returned Approve with no
outstanding Critical or Important findings. Death cleanup is tracked below;
REST cleanup and release tasks remain unchecked.

## 4. Death, Deletion, and Ordinary Persistence

- [x] 4.1 Register a narrow permanent-death notification that immediately marks PendingTerminal. At the end of the current Update or attachment call, detach that call's character IDs, release the source lock and synchronously complete cleanup under the surface lock before returning. Reuse live/raw conversion and chunk pins; test no-home, cross-layer and unloaded deaths, latest ownership, repeated notification and blocked-return ordering. Track login/transfer owning calls with the existing Game wait group and drain it after network shutdown, before stopping any shard. No next-tick queue, new ECS system, nested shard locks or corpse persistence changes. Gameplay/admin removal protection in 3.1 is explicitly deferred.
- [ ] 4.2 Connect REST character deletion to the same feature cleanup while preserving account-scoped authorization, deletion responses, and current online/offline policy. Test absent/already-deleted characters and no-home cleanup; emit no character-position death effect or fabricated corpse for REST deletion.
- [x] 4.3 Save completed construction synchronously after final-branch processing, transforms and index replacement. Serialize final objects, then use one existing WithTx with a five-second context for the new Hearth, previous campfire if present, character_hearth and durable ID watermark. Return success after commit; retain object/chunk dirty flags and the raw-chunk pin until saving returns. Serialization/SQL/commit errors and an association affected-row count other than one are fatal; no version acknowledgment, runtime rollback, SQL retry or save queue.
- [ ] 4.3a Complete synchronous death/deletion persistence on the owning surface path.
  - [x] Death: save an optional campfire conversion and exact matching association removal in one five-second transaction through existing SQL. Retain the chunk pin until saving returns; deleted, missing, changed or quarantined objects receive association-only cleanup. Known NoHome requires no Hearth I/O; Unknown at actual death is fatal. Test atomicity, rollback, restart, identity checks, idempotence and fatal failures.
  - [ ] REST deletion: reuse synchronous cleanup after authorized deletion and add saved-association resolution for offline Unknown owners. No asynchronous saver, dirty revisions, retry or acknowledgment.
- [ ] 4.4 Add broader restart repair and ordinary shutdown coverage around task 3.5 normalization. Verify independent checkpoints restore a usable single-home runtime without journal replay; execute any Hearth repair SQL synchronously on the owner path and retain existing shutdown/final-save ordering without a feature dirty flush or new global barrier.
- [ ] 4.5 Document accepted crash-loss boundaries, synchronous SQL ordering, restart normalization, whole-game-loop blocking and process-lifetime index growth. Verify database/invalid-result errors stop the server and work is limited to current candidates, without save-lag queue metrics, per-player labels or a world scan.

Task 4.1 and the death portion of 4.3a verification: focused runtime/writer tests
and PostgreSQL integration tests passed, including real death through Update,
login attachment, reattachment and immediate teleport attachment. Controlled
save delays verified source unlock, synchronous outer-call/manager waiting and
detached-batch ownership across overlapping calls. Live/raw/unloaded conversion,
NoHome without I/O, latest admitted construction, idempotence, deleted sources,
fatal failures, transaction rollback and restart behavior are covered.
The shutdown regression enters through real auth/transfer launches, holds
terminal completion before the surface lock and again before saving, and verifies
that an unloaded home remains loadable and cleanup commits before shutdown.
It passes with PostgreSQL and under the race detector; temporary source overlays
fail with the original shutdown ordering, either launch registration removed,
or only the wait moved back after shard shutdown. No production test seam or new
shutdown barrier was added.
`GOCACHE=/private/tmp/origin-go-build-cache make test` passed with
`ORIGIN_OBJECT_HEALTH_TEST_DSN` pointing to an isolated temporary PostgreSQL;
protobuf/sqlc regeneration produced no diff. Full
`go test -race ./internal/game/hearth ./internal/game -count=1` also passed with
that fixture. Strict OpenSpec validation and `git diff --check` passed.
Independent code review returned Approve with no findings and independently
passed focused race tests with PostgreSQL. REST cleanup remains unimplemented,
so the parent 4.3a and task 4.2 remain unchecked.

## 5. Return, Interruption, and Normal Login

- [ ] 5.1 Register Travel to Hearth Fire as an ordinary no-target action with five-second progress, zero stamina, and the existing 360000 ms cooldown. Use known runtime ownership from login/hydration; Unknown fails closed without an asynchronous lookup. Test missing/unknown home, combat, swimming, active KO, post-KO lying, and standing shallow water through existing eligibility/cycle tests.
- [ ] 5.2 Resolve the latest runtime home again on the target surface shard immediately before relocation, not only at queue admission; recheck exact source handle/action generation, connection, combat, swimming, and resolved KO state. Reuse deferred transfer with narrow generic source-validation/target-resolution/success hooks and existing preparation timeout; test cancellation, replacement during progress or queued transfer, stale callbacks, and cooldown only after successful target relocation.
- [ ] 5.3 Reuse exact player-only destination clearance and the carry transfer participant for same-layer/cross-layer return. Use a standing target-runtime copy while preserving the lying source rollback snapshot; test blocked-home failure without nearby fallback, carried-footprint exclusion, rollback/carry integrity, and unchanged stairs/admin incapacity rules.
- [ ] 5.4 Invoke feature-selected interruption through a small generic action capability before gameplay-attempt rejection and on fresh movement intent, damage, combat/swimming transition, and normal disconnect cleanup. Test rejected/cooldown-blocked attempts, blocked movement without displacement, damage without combat, and chat/personal-window exemptions; shared ingress must not check a Hearth action key.
- [ ] 5.5 Preserve cancellation when another action attempt is rejected by client cooldown controls using existing `C2S_CancelAction`. Verify Actions/hotbar mouse, keyboard, and touch paths plus movement/map-click suites; add no new cancellation packet and do not start the blocked action.
- [ ] 5.6 Keep ordinary first entry and `tryReattachPlayer` without automatic Hearth creation; add normal login candidates in order: saved position, exact current home when present, near saved position, existing random fallback. Test no-home/reconnect and blocked-home cases, update entry and runtime-KO notes, and preserve capacity cleanup.

## 6. Prepared Assets, UI, and Shared Skull Smoke

- [ ] 6.1 Reuse the prepared green flame resource and skull-smoke assets with existing projection/anchor/alpha contracts; integrate the Hearth icon and always-green presentation. Verify label `Hearth Fire`, seven approved flame frames at 70% scale, no ownership/online indicator, no minimap marker, and unchanged campfire rendering.
- [ ] 6.2 Add `Travel to Hearth Fire` beside `Stand Up` only after active KO ends, using the same action, progress, and shared cooldown UI. Verify knockout/action UI suites cover active KO, lying, cancellation, success, and cooldown.
- [ ] 6.3 Add only `stream_epoch = 3` to existing `S2C_Fx`, preserving fields 1/2, and publish one position-based skull-smoke event at runtime character death or Hearth conversion to current observers. Regenerate server/client protobuf and test session/layer isolation, separate event positions, no historical replay, and legacy local-player `exp_gain` behavior.
- [ ] 6.4 Render a reusable detached one-shot effect from the eight preloaded frames at 250 ms per frame, with a 2000 ms local monotonic lifetime, bounded concurrent effects, and reset-generation cleanup. Test source-view removal, suspended rendering, capacity limits, late loading after reset, and world/session destruction without an event-ID deduplication or server-clock freshness subsystem.
- [ ] 6.5 Add visual/client asset regressions and gameplay-scale desktop/mobile screenshots for green fire, skull smoke, and post-KO controls; document delivered contracts and continuous-versus-transient smoke. Run relevant asset tests and, only if atlas/tools change, the three required pipeline checks; generate no replacement art and add no explanatory in-app copy.

## 7. Integration and Release

- [ ] 7.1 Run make test and PostgreSQL Hearth integration coverage using the existing fixture. Verify regeneration, schema uniqueness, synchronous construction, activation and death transactions, future REST cleanup, and restart repair; report an unavailable fixture rather than claiming skipped database tests passed.
- [ ] 7.2 Run client type-check, lint, and relevant action/execution/knockout/movement/map-click/chunk/minimap/FX/asset suites; review lint's automatic diff and verify unrelated presentation/protocol regressions remain covered.
- [ ] 7.3 Exercise no-home entry, first construction/replacement, underground carry return, post-KO return, owner death/deletion, and blocked-position login recovery end to end. Verify runtime invariants during synchronous I/O, full-server stop on actual database/invalid-result errors, accepted loop delay and process-lifetime memory growth, and bounded collider/metadata/FX work without tick scans or N+1 SQL.
- [ ] 7.4 Run strict OpenSpec validation and `git diff --check`, review generated/assets/protocol diffs, and deliver clean-database deployment notes. Verify no automatic entry, backfill, database clearing, new dependency, or unrelated balance/`life_death_factor` change, and explicitly distinguish accepted crash rollback from runtime corruption.
