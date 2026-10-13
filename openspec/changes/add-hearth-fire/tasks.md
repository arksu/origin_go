# Tasks

## Existing Preparation and Revised Baseline

Only object/art preparation remains in the source tree. The server catalog
contains static, indestructible `hearth_fire` (ID 36, resource `hearth_fire`,
12x12 collider) without Hearth behaviors or a station. The ordinary client
resource uses the approved seven green flame frames at 70% scale and 4 FPS.
Eight approved skull-smoke frames and their asset descriptor are also present;
the descriptor specifies 250 ms per frame and a 2000 ms one-shot animation.
Reuse these assets instead of regenerating them.

Previous feature persistence and lifecycle implementation was reverted. All
tasks below are therefore unchecked. This replaces the former durable-lifecycle
plan: gameplay changes finish in runtime without waiting for a database commit.
Ordinary asynchronous persistence may lose recent progress on a crash. Runtime
ownership, object state, action cancellation, and stale-handle safety remain
authoritative; no lifecycle coordinator, mutation-reservation framework,
finisher sidecar, body-retention system, or special shutdown barrier is required.

## 1. Ownership Data and Runtime State

- [ ] 1.1 Add the single undeployed migration and matching canonical schema for unpartitioned `character_hearth`: character PK/FK, unique `(object_region, object_id)`, and composite RESTRICT object FK. Verify PostgreSQL rejects duplicate or invalid associations; add no first-entry marker, compatibility migration, or backfill.
- [ ] 1.2 Add sqlc queries for indexed home/owner lookup, batched owner loading, latest association replacement/removal, and restart repair; run `make sqlc` and cover absent ownership, deleted characters, and invalid object references without hand-editing generated code.
- [ ] 1.3 Add the feature-owned runtime home index with explicit unknown/known-no-home/current-home states and dirty revisions. Keep mutation on the owning execution path and expose copied metadata to other layers; test first assignment, replacement, removal, stale revisions, and absence of permanent pending-operation records.
- [ ] 1.4 Document runtime authority, schema/index responsibilities, asynchronous save lag, and restart repair in the feature engineering notes. Verify ordinary character saves cannot overwrite home ownership and shared ECS/build/action state contains no Hearth-specific fields.

## 2. Manual Construction and Immediate Replacement

- [ ] 2.1 Add the universally available five-branch Hearth recipe using existing two-second and eight-stamina processing per branch. Verify solo ten-second/forty-stamina construction, cooperative acceleration, and no skill/discovery requirement.
- [ ] 2.2 Add small registered construction preflight/completion hooks and implement Hearth policy in its feature module. Before final-cycle costs, prepare any unknown old-home metadata through existing bounded read workers; if not ready, end that attempt with a loading message and no final-cycle costs. Verify ordinary recipes retain their current path and no SQL or blocking worker wait runs under the shard lock.
- [ ] 2.3 Complete Hearth synchronously on the surface owning shard using the actual successful final-branch actor. Prepare fallible runtime transforms before mutation, convert the previous home to an ordinary unlit campfire, transform the site, and replace ownership in one shard-locked completion; test exact identity/location preservation and unchanged player position without a saved finisher sidecar.
- [ ] 2.4 Apply an inactive old-home conversion to the prepared raw record without forcing activation; retain its latest runtime projection until ordinary save/activation consumes it. Make runtime raw updates advance the existing cache revision; test unloaded homes, stale read/load results, and activation before database catch-up cannot reveal a second active home.
- [ ] 2.5 Add cooperative and competing completion coverage and update construction notes. Verify unlimited sites, initiator independence, offline/dead initiators, first-home construction, repeated completion of an already transformed site, and latest runtime ownership without admission tickets or a retained mutable build site.

## 3. World Rules and Ownership Restoration

- [ ] 3.1 Extend the prepared Hearth definition with feature behavior, fixed green appearance, and the approved exact 1x1 footprint. Reuse indestructibility/non-liftability; add a small registered removal policy checked before gameplay/admin deletion changes memory or SQL. Test damage, removal, and lift rejection while ordinary campfires remain unchanged, without a Hearth key check in shared deletion code.
- [ ] 3.2 Add the small behavior collision-filter capability and implement owner/non-player passage in Hearth behavior. Reuse it for movement and authoritative player spawn; test owner passage, foreign-player blocking, animals, and unrelated obstacles without global collision bypass or tick-time SQL.
- [ ] 3.3 Validate every overlapped surface tile against passable dry-land terrain using the result collider footprint. Test allowed dry ground, full-footprint edge cases, underground, water/shallow water, swamp, and bog rejection.
- [ ] 3.4 Add independent structural-footprint exclusion through existing spatial queries for construction and carried-object placement, at request and final application. Test owner placement, anchor-outside/footprint-inside overlaps, ordinary construction-site footprints, stale previews, and preserved carry state on refusal.
- [ ] 3.5 Batch ownership preparation during chunk loading and apply metadata on the owning shard before object visibility; guard load generations and normalize orphan Hearth records to ordinary unlit campfires. Verify unload/reload, stale snapshots, dead owners, and conversions; document terrain predicates and the actual metadata hook without a new deferred-restore framework.

## 4. Death, Deletion, and Ordinary Persistence

- [ ] 4.1 Register a narrow permanent-death hook that immediately revokes runtime ownership and schedules any surface conversion through the existing owning-shard execution path. Retain only coalesced conversion intent when an inbox is full; test queue pressure, death with/without a home, cross-layer death, unloaded homes, and completion/death runtime ordering without retaining bodies or changing corpse save/decomposition rules.
- [ ] 4.2 Connect REST character deletion to the same feature cleanup while preserving account-scoped authorization, deletion responses, and current online/offline policy. Test absent/already-deleted characters and no-home cleanup; emit no character-position death effect or fabricated corpse for REST deletion.
- [ ] 4.3 Persist copied latest runtime projections through existing bounded persistence workers, with at most one in-flight write per character and coalesced dirty revisions. Use modest transactions for affected object rows and association changes plus existing chunk I/O gates; test queue saturation, retries, and stale acknowledgments without reverting runtime state or holding a gameplay reservation until commit.
- [ ] 4.4 Add restart repair and ordinary shutdown coverage. Treat a valid saved association as the surviving home and normalize orphan or dead-owner Hearth objects; verify partial save lag yields a usable single-home runtime rather than a journal replay, and reuse existing shutdown/final-save ordering instead of a new global barrier.
- [ ] 4.5 Document the accepted crash-loss boundary, latest-projection save ordering, repair policy, and bounded dirty work. Verify death and home changes remain immediate during a worker/database outage and instrumentation reveals pending save age without per-player metric labels or a full-world tick scan.

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

- [ ] 7.1 Run `make test` and PostgreSQL Hearth integration coverage using the existing fixture. Verify regeneration, schema uniqueness, asynchronous latest-state saving, and restart repair; report an unavailable fixture rather than claiming skipped database tests passed.
- [ ] 7.2 Run client type-check, lint, and relevant action/execution/knockout/movement/map-click/chunk/minimap/FX/asset suites; review lint's automatic diff and verify unrelated presentation/protocol regressions remain covered.
- [ ] 7.3 Exercise no-home entry, first construction/replacement, underground carry return, post-KO return, owner death/deletion, and blocked-position login recovery end to end. Verify runtime invariants during delayed saves/outage and bounded collider/metadata/FX work under representative load, without new tick scans or N+1 SQL.
- [ ] 7.4 Run strict OpenSpec validation and `git diff --check`, review generated/assets/protocol diffs, and deliver clean-database deployment notes. Verify no automatic entry, backfill, database clearing, new dependency, or unrelated balance/`life_death_factor` change, and explicitly distinguish accepted crash rollback from runtime corruption.
