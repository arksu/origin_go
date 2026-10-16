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
Its verification is tracked below. Terminal persistence, general chunk hydration,
restart repair, death cleanup and the remaining gameplay remain separate stages.
This replaces the former durable-lifecycle plan:
construction changes runtime first, then returns success after its database commit.
Other independent saves may lose recent progress on a crash. Runtime
ownership, object state, action cancellation, and stale-handle safety remain
authoritative; no lifecycle coordinator, mutation-reservation framework,
finisher sidecar, body-retention system, or special shutdown barrier is required.

The database is assumed available. Actual database errors, including an unknown
commit outcome, must stop the entire server without SQL retries or database-error
recovery. Expected absent rows, optimistic-version conflicts and queue saturation
remain ordinary outcomes. This policy is to be wired with the gameplay/saver work;
it does not describe an already-installed universal database-error handler.

## 1. Ownership Data and Runtime State

- [x] 1.1 Add the single undeployed migration and matching canonical schema for unpartitioned `character_hearth`: character PK/FK, unique `(object_region, object_id)`, and composite RESTRICT object FK. Verify PostgreSQL rejects duplicate or invalid associations; add no first-entry marker, compatibility migration, or backfill.
- [x] 1.2 Add sqlc queries for indexed home/owner lookup, batched owner loading, latest association replacement/removal, and restart repair; run `make sqlc` and cover absent ownership, deleted characters, and invalid object references without hand-editing generated code.
- [x] 1.3 Add the feature-owned runtime home index with explicit unknown/known-no-home/current-home states, optimistic versions and one loading read per character. Accepted reads complete exactly once without cancellation; only rejected submissions release loading before execution. Keep mutation on the owning execution path and expose copied metadata to other layers; test first assignment, replacement, removal, stale versions, exclusive reads and absence of permanent pending-operation records.
- [x] 1.4 Document runtime authority, schema/index responsibilities, asynchronous save lag, and restart repair in the feature engineering notes. Verify ordinary character saves cannot overwrite home ownership and shared ECS/build/action state contains no Hearth-specific fields.

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
does not claim a live-database integration check. Terminal persistence and the
later stages remain unchecked.

## 3. World Rules and Ownership Restoration

- [ ] 3.1 Extend the prepared Hearth definition with feature behavior, fixed green appearance, and the approved exact 1x1 footprint. Reuse indestructibility/non-liftability; add a small registered removal policy checked before gameplay/admin deletion changes memory or SQL. Test damage, removal, and lift rejection while ordinary campfires remain unchanged, without a Hearth key check in shared deletion code.
- [x] 3.2 Only the owner passes through the hearth. Other players and animals retain ordinary collision. One optional callback shared by movement and authoritative player spawn keeps policy in `hearth.Runtime`, without a behavior capability/dispatcher, global collision bypass or tick-time SQL. Verify owner passage/spawn, non-owner blocking, unrelated obstacles, unknown ownership and home replacement.
- [x] 3.3 Configure Hearth `disallowedTiles: [1, 3, 50, 53, 56, 255]` and reuse standard build validation at placement admission over the result collider footprint. Test allowed tiles, every forbidden ID, neighboring footprint overlap, exact boundary contact and missing chunk data. Preserve ordinary build timing without a new final terrain recheck or layer field; existing Hearth completion remains surface-only.
- [x] 3.4 Complete structural-footprint exclusion, including owner attempts, through existing spatial queries shared by manual put-down and construction placement.
  - [x] 3.4.1 Reject manual carried-object placement (`lift_down` and RMB) at request and final application through the shared generic `colliderPlacementAllowed` helper on every layer. Use saved collider dimensions/masks or optional lift placementSize (log_x 24x6, log_y 6x24; default 2x2 with Layer/Mask 1). Query loaded colliders with ordinary masks; exclude carrier, carried object and phantom candidates without Hearth-specific filtering or neighbor readiness checks. Verify exact contact, cross-chunk colliders, bounded query refusal, stale attempts and preserved carry; forced lifecycle drops remain outside this step.
  - [x] 3.4.2 Reuse the shared collision helper at construction request and final placement with the current result collider, before new pending/phantom or site/ID creation. Cover ordinary construction sites, owner/non-owner attempts, the complete footprint, late obstacles, exact contact and replaced previews; retain admission-only terrain checks and existing `BUILD_PENDING_FAILED` cleanup.
- [ ] 3.5 Batch ownership preparation during chunk loading and apply metadata on the owning shard before object visibility; guard load generations and normalize orphan Hearth records to ordinary unlit campfires. Verify unload/reload, stale snapshots, dead owners, and conversions; document terrain predicates and the actual metadata hook without a new deferred-restore framework.

## 4. Death, Deletion, and Ordinary Persistence

- [ ] 4.1 Register a narrow permanent-death hook that immediately revokes runtime ownership and schedules any surface conversion through the existing owning-shard execution path. Retain only coalesced conversion intent when an inbox is full; test queue pressure, death with/without a home, cross-layer death, unloaded homes, and completion/death runtime ordering without retaining bodies or changing corpse save/decomposition rules.
- [ ] 4.2 Connect REST character deletion to the same feature cleanup while preserving account-scoped authorization, deletion responses, and current online/offline policy. Test absent/already-deleted characters and no-home cleanup; emit no character-position death effect or fabricated corpse for REST deletion.
- [x] 4.3 Save completed construction synchronously after final-branch processing, transforms and index replacement. Serialize the final objects, then use one existing `WithTx` with a five-second context for the new Hearth, previous campfire if present, `character_hearth` and durable ID watermark. Return success after commit and acknowledge only the saved index version; retain object/chunk dirty flags and the raw-chunk pin until saving returns. Serialization/SQL/commit errors and an association affected-row count other than one are fatal; add no runtime rollback, SQL retry or save queue.
- [ ] 4.3a Implement future death/deletion latest-projection persistence through existing bounded workers, with one in-flight write per character and coalesced dirty versions. Use existing chunk I/O gates and modest object/association cleanup transactions. Test queue saturation, stale acknowledgments and fatal database failures without introducing a lifecycle operation queue or changing ordinary deletion ordering.
- [ ] 4.4 Add restart repair and ordinary shutdown coverage. Treat a valid saved association as the surviving home and normalize orphan or dead-owner Hearth objects; verify partial save lag yields a usable single-home runtime rather than a journal replay, and reuse existing shutdown/final-save ordering instead of a new global barrier.
- [ ] 4.5 Document the accepted crash-loss boundary, latest-projection save ordering, restart normalization, and bounded dirty work. Verify death and home changes remain immediate while saving is queued or in flight, database errors stop the entire server, and instrumentation reveals pending save age without per-player metric labels or a full-world tick scan.

## 5. Return, Interruption, and Normal Login

- [ ] 5.1 Register `Travel to Hearth Fire` as an ordinary no-target action with five-second progress, zero stamina, and the existing 360000 ms cooldown. Test missing home, combat, swimming, active KO, post-KO lying, and standing shallow water through existing eligibility/cycle tests.
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

- [ ] 7.1 Run `make test` and PostgreSQL Hearth integration coverage using the existing fixture. Verify regeneration, schema uniqueness, synchronous construction saving, future asynchronous terminal saving, and restart repair; report an unavailable fixture rather than claiming skipped database tests passed.
- [ ] 7.2 Run client type-check, lint, and relevant action/execution/knockout/movement/map-click/chunk/minimap/FX/asset suites; review lint's automatic diff and verify unrelated presentation/protocol regressions remain covered.
- [ ] 7.3 Exercise no-home entry, first construction/replacement, underground carry return, post-KO return, owner death/deletion, and blocked-position login recovery end to end. Verify runtime invariants during queued/in-flight saves, full-server stop on actual database errors, and bounded collider/metadata/FX work under representative load, without new tick scans or N+1 SQL.
- [ ] 7.4 Run strict OpenSpec validation and `git diff --check`, review generated/assets/protocol diffs, and deliver clean-database deployment notes. Verify no automatic entry, backfill, database clearing, new dependency, or unrelated balance/`life_death_factor` change, and explicitly distinguish accepted crash rollback from runtime corruption.
