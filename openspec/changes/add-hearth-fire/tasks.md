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
death portion of 4.3a add synchronous permanent-death cleanup. Task 4.2 connects
REST deletion through the same cleanup. Tasks 4.4/4.5 add lazy repair, recovery/final-save checks and cost documentation.
Section 5 adds ordinary return, strict opt-in interruption and layer-aware login fallback.
Owner-death Hearth and permanent character-death FX now use the shared object-FX
path; remaining presentation and replacement FX stay separate.
This replaces the former durable-lifecycle plan:
construction changes runtime first, then returns success after its database commit.
Other independent saves may lose recent progress on a crash. Runtime
ownership, object state, action cancellation, and stale-handle safety remain
authoritative; no lifecycle coordinator, mutation-reservation framework,
finisher sidecar, body-retention system, or special shutdown barrier is required.

The database is assumed available. Actual Hearth database errors, including an unknown
commit outcome, must stop the entire server without SQL retries or database-error
recovery. Expected absent rows remain ordinary outcomes; invalid results are fatal.
All Hearth SQL runs synchronously under the surface shard lock with five-second
read/transaction contexts. REST cleanup follows the same policy; the preceding
account-scoped character DELETE preserves HTTP 500 on SQL failure. This does not
describe an already-installed universal database-error handler.

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
REST cleanup is tracked below; release tasks remain unchecked.

## 4. Death, Deletion, and Ordinary Persistence

- [x] 4.1 Register a narrow permanent-death notification that immediately marks PendingTerminal. At the end of the current Update or attachment call, detach that call's character IDs, release the source lock and synchronously complete cleanup under the surface lock before returning. Reuse live/raw conversion and chunk pins; test no-home, cross-layer and unloaded deaths, latest ownership, repeated notification and blocked-return ordering. Track login/transfer owning calls with the existing Game wait group and drain it after network shutdown, before stopping any shard. No next-tick queue, new ECS system, nested shard locks or corpse persistence changes. Gameplay/admin removal protection in 3.1 is explicitly deferred.
- [x] 4.2 Inject the required Game.DeleteCharacter callback into REST. Hold the existing surface lock across account-scoped DeleteCharacter :execrows and synchronous cleanup; refuse unavailable/stopping runtime before SQL. Preserve 204 no-ops without index changes and HTTP 500 on DELETE failure. Resolve only offline Unknown through the composite saved-association LEFT JOIN and reuse terminal conversion or exact association-only saving. Verify authorization, online/offline/no-home/invalid sources, cancellation, construction and shutdown ordering; preserve body/session policy without corpse or FX.
- [x] 4.3 Save completed construction synchronously after final-branch processing, transforms and index replacement. Serialize final objects, then use one existing WithTx with a five-second context for the new Hearth, previous campfire if present, character_hearth and durable ID watermark. Return success after commit; retain object/chunk dirty flags and the raw-chunk pin until saving returns. Serialization/SQL/commit errors and an association affected-row count other than one are fatal; no version acknowledgment, runtime rollback, SQL retry or save queue.
- [x] 4.3a Complete synchronous death/deletion persistence on the owning surface path.
  - [x] Death: save an optional campfire conversion and exact matching association removal in one five-second transaction through existing SQL. Retain the chunk pin until saving returns; deleted, missing, changed or quarantined objects receive association-only cleanup. Known NoHome requires no Hearth I/O; Unknown at actual death is fatal. Test atomicity, rollback, restart, identity checks, idempotence and fatal failures.
  - [x] REST deletion: reuse synchronous cleanup after confirmed authorized deletion and saved-association resolution for offline Unknown owners. Independent five-second Hearth contexts finish after HTTP cancellation; known NoHome/Terminal perform no Hearth I/O; hidden invalid links remain outside the lazy repair boundary in 4.4. No asynchronous saver, dirty revisions, retry or acknowledgment.
- [x] 4.4 Add lazy repair only after Unknown LoadHome receives a filtered miss: existing saved lookup, exact conditional DeleteInvalidCharacterHearth through the existing saver/WithTx, then NoHome. Preserve known-state zero-I/O and accept residual orphan-to-campfire SQL links without restoring ownership. Test invalid sources, conditional repair, independent checkpoints and ordinary shutdown with a real character saver. Reuse existing locks, gates, pins and final-save ordering; no new SQL/schema, core hook, world scan, dirty flush or barrier.
- [x] 4.5 Document accepted crash-loss boundaries and synchronous ordering; five seconds bounds each operation, not total owner-call latency or I/O-mutex waiting. Activation work scales with current chunk candidates without a fixed numeric Hearth cap; index memory follows encountered characters for process lifetime. Verify fatal Hearth read/repair/save errors and the REST DELETE HTTP 500 exception through existing suites plus new repair tests. Add no metrics, per-player labels, scan or persistence framework.

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
passed focused race tests with PostgreSQL. REST cleanup verification is recorded
separately below.

Task 4.2 and the REST portion of 4.3a verification: focused endpoint, runtime
and game tests passed with an isolated temporary PostgreSQL instance. Coverage
includes authenticated account scope, zero-row no-ops, real DELETE SQL failure
without runtime changes or Fatal, live/raw/unloaded homes, exact region/object
identity, invalid-source association-only cleanup and known NoHome without I/O.
Controlled save delays verify response waiting after request cancellation,
construction commit before DELETE, terminal refusal of later construction and
cleanup completion before surface shutdown. Actual-death Unknown remains fatal
without the new saved lookup. `GOCACHE=/private/tmp/origin-go-build-cache make test`
passed with `ORIGIN_OBJECT_HEALTH_TEST_DSN` set to that instance; regeneration
changed only the two expected sqlc files, with no schema/protocol changes. Full
`go test -race ./internal/game/hearth ./internal/game ./internal/restapi -count=1`
passed with PostgreSQL; the final real-DELETE-failure regression also passed
focused normal and race runs. Strict OpenSpec validation and `git diff --check`
passed. Independent code review returned Approve with no Critical or Important
findings and independently passed focused PostgreSQL race tests.

Tasks 4.4/4.5 verification: focused runtime/writer and PostgreSQL recovery tests
passed. Coverage includes exact conditional repair of invalid sources, separate
five-second contexts, commit-before-NoHome, retained PendingTerminal, cached-state
I/O counts and Fatal failures. Restart tests preserve independent character,
material and Hearth checkpoints, an incomplete saved build site and harmless
residual orphan associations. The ordinary shutdown regression uses a real
CharacterSaver, blocks its PostgreSQL write and verifies chunk final saves wait;
fresh loading checks character state, current ownership and live/raw conversions.
A temporary source overlay reversing CharacterSaver.Stop/ChunkManager.Stop made
this regression fail as expected, without changing production shutdown code.
`GOCACHE=/private/tmp/origin-go-build-cache make test` and
`go test -race ./internal/game/hearth ./internal/game ./internal/restapi -count=1`
passed with `ORIGIN_OBJECT_HEALTH_TEST_DSN` set to an isolated temporary PostgreSQL.
Protobuf/sqlc regeneration produced no diff. Strict OpenSpec validation and
`git diff --check` passed. Production changes are limited to the Hearth lookup
and its existing saver; SQL, schema, protocol and core lifecycle are unchanged.
Independent code review returned Approve with no material findings and
independently passed focused runtime/recovery tests with PostgreSQL.

## 5. Return, Interruption, and Normal Login

- [x] 5.1 Register Travel to Hearth Fire as an ordinary no-target action with five-second progress compiled from TickRate before registry publication, zero stamina and the existing 360000 ms cooldown. Reuse an optional admission hook for prior interaction/movement cleanup. Use known runtime ownership from login/hydration; Unknown fails closed without a lookup. Test missing/unknown home, combat, swimming, active KO, post-KO lying, and standing shallow water through existing eligibility/cycle tests.
- [x] 5.2 Resolve the latest runtime home again on the target surface shard immediately before relocation, not only at queue admission; recheck exact source handle/action generation, connection, combat, swimming, and resolved KO state before and after source saving. Reuse stock deferred transfer with narrow generic source-policy/target-spawn/result hooks and one preparation timeout; complete pre-detach failure without leaving progress stuck, guard late results, and start cooldown only after successful target relocation. Reprepare changed homes after retiring unpublished AOI and use actual coordinates throughout.
- [x] 5.3 Reuse exact player-only destination clearance and the carry transfer participant for same-layer/cross-layer return. Use a standing target-runtime copy while preserving the lying source rollback snapshot; reload the full saved character under the source lock after SaveSync for both target and rollback. Test DB/ECS stats/profile divergence, blocked-home failure without nearby fallback, carried-footprint exclusion, rollback/carry integrity, and unchanged stairs/admin incapacity rules.
- [x] 5.4 Invoke feature-selected interruption through a small generic action capability before gameplay-attempt rejection and on fresh movement intent, damage, combat/swimming transition, and normal disconnect cleanup. Test rejected/cooldown-blocked attempts, blocked movement without displacement, damage without combat, and chat/personal-window exemptions; shared ingress must not check a Hearth action key.
- [x] 5.5 Preserve cancellation when another action attempt is rejected by client cooldown controls using existing `C2S_CancelAction`. Verify Actions/hotbar mouse, keyboard, and touch paths plus movement/map-click suites; add no new cancellation packet and do not start the blocked action.
- [x] 5.6 Keep ordinary first entry and body reattachment without automatic Hearth creation; add layer-aware normal login candidates in order: saved position, exact current home when present, near saved position, existing random fallback. Preserve original runtime KO/combat state, capacity/setup failure cleanup and authoritative surface-body reattachment despite stale saved layers. Reuse UpdateCharacterPositionAndLayer after successful cross-layer spawning because ordinary CharacterSaver omits Layer; keep its stock warning policy and retire offline caches only after attachment. Test no-home/reconnect, blocked-home, cancellation, final-save/reload and actual write failure; update runtime-KO notes.

Section 5 verification: PostgreSQL return tests passed through real RequestTransfer
and executeTransfer, including same/cross-layer carry, lying-to-standing success,
lying rollback, blocked destinations, replaced homes/AOI, full identity validation,
timeout, early failure cleanup, stale source/session/generation, disconnect during
SaveSync and after target spawn, actual position persistence and success-only
cooldown. Admission regressions retire old build phantom and personal craft,
including later return cancellation/failure. Login tests cover saved/home/nearby/
random ordering, no-home first entry, original KO/combat state, stale-row reattach,
capacity/setup errors, cancellation before attach, existing layer write and reload
after real character save/expiry; SQL failure retains the stock warning contract.
Login now reuses locked finalization for every candidate: spawn, character
registration and attachment/logout finish before the owning lock is released.
Cross-layer position persistence remains outside that lock with no later
attachment/logout. A PostgreSQL regression blocks this real SQL write, runs
lethal damage and PlayerDeathSystem, then checks ordinary DeathDialog/observer
mode and no EnterWorld after death; it failed before the login correction.
It also expires the spawn context without detaching the corpse. Immediate
attachment-death tests cover saved-position and home candidates, including
synchronous cleanup after unlock. Focused PostgreSQL tests, make test, the
four-package race run, strict OpenSpec validation and diff checks passed;
independent review approved the login correction.
Locked-attachment ordering coverage verifies old-layer caches retire before unlock
and a subsequent real expiry preserves fresh health/combat; a temporary source
overlay reverting to local-only retirement made the regression fail as expected.
Interruption tests cover valid/rejected intents, stale input/session, WASD release/
refresh/expiry, Stand Up epochs, swimming, damage/zero-damage combat, starvation
masked by KO resolution, admin damage, chat/windows and existing disconnect.
Opt-out/empty interruption paths allocate zero in AllocsPerRun checks.

DB/ECS divergence regressions verify current stamina, energy, heading and the full
profile on same/cross-layer return and rollback, then save the restored body again.
They reproduced stale-body state and durable data loss before the post-SaveSync
reload fix. A failed reload preserves the source body, client, carry and inventories
after successful source persistence; no destructive participant capture occurs.
The post-save reload uses a separate five-second timeout under the source lock;
a PostgreSQL blocked-read regression verifies deadline expiry with an unbounded
parent context and preserves the source before destructive capture.

Real-socket reconnect regressions pause return after source capture, invoke actual
disconnect/offline and token authentication, and verify auth waits for the existing
per-flight completion channel before repeating its transaction with fresh data.
Both layers and success/rollback retain exactly one live character across all
worlds, the replacement connection, the stock carry drop and success-only cooldown.
Waiting holds no shard/DB lock and uses one SpawnTimeout; timed-out auth makes no
online/session claim. Repeated return/rollback regressions preserve nested player
and carried-object inventories while retiring all previous container handles and
source refs, without growing the entity count. Existing source raw-cache removal
now runs at capture before despawn, preventing stale carry reactivation.
The carry snapshot pins its existing active source chunk before destructive
capture through transfer completion; busy/unavailable sources refuse capture.
Real PostgreSQL tests seed carry in SQL, pause after capture, force capacity
eviction and returning source AOI, and verify success/rollback retain one body,
one carry and nested inventories. Normal eviction resumes after cleanup. Partial
capture, failed rollback spawn, persistence fallback and repeated-save failure
tests verify pin release; direct tests cover busy admission and idempotent release.
The stock repeated-persistence-failure policy remains best effort: SQL may retain
the old source after both writes fail. The pin closes the in-flight reload window,
not that separate durable-write failure; the failure regression states this bound.
Pin correction verification: make test, the four-package server race run and focused
world pin/gate/eviction race tests passed. Adding the entire world package to the
race run exposed unchanged allocation-count assertions and Skull metadata's
time.Local write racing existing LRU timers; that expanded run did not pass.
The socket fixture publishes its deliberately paused disconnect boundary with a
mutex; this does not validate or fix the pre-existing unsynchronized Client.Layer
accesses observed without that fixture synchronization.

Atomic finalization regressions cover both layers and target/rollback: a competing
real Shard.Update with lethal damage and PlayerDeathSystem cannot enter between
spawn and attachment. Its first observation includes the living bound body and
restored carry; death then drops carry and sends the ordinary observer/dialog flow
without a later ordinary EnterWorld. Immediate attachment death also completes
Hearth cleanup only after owning-shard unlock. Existing transfer/disconnect tests
forward the locked spawn finalizer, and direct lifecycle tests own the restore lock.
All eight PostgreSQL death cases passed normally and with -race; make test and
the four-package server race run passed after the finalizer correction.

Client actions, movement-input, map-click, knockout and direction-aim suites passed;
the GameView keyboard harness passed all six tests. Mouse/touch/hotbar tests verify
cancel-only cooldown attempts and drag/long-press/clearing exemptions. Type-check
and lint passed; lint's diff was reviewed. No manual browser/server E2E is claimed.
`GOCACHE=/private/tmp/origin-go-build-cache make test` passed with isolated temporary
PostgreSQL via ORIGIN_OBJECT_HEALTH_TEST_DSN; protobuf/sqlc regeneration produced
no diff. Full `go test -race ./internal/game ./internal/game/hearth
./internal/ecs/systems ./internal/restapi -count=1` passed with the same fixture.
One aggregate race run hit the unchanged lift-health fixture's single-Update
chunk-activation assertion; no data race was reported. Thirty focused repetitions
and the repeated full race run passed without changing that fixture or chunk code.
Strict OpenSpec validation and git diff --check passed. Independent review returned
Approve after resolving admission cleanup, durable layer persistence and cache
retirement ordering findings; the final cache regression also passed independent
PostgreSQL race review.
No new SQL/schema/protocol, dependency, worker, queue or global lifecycle barrier
was introduced. Section 6 presentation/KO-window/FX work remains unchecked.

## 6. Prepared Assets, UI, and Shared Skull Smoke

- [ ] 6.1 Reuse the prepared green flame resource and skull-smoke assets with existing projection/anchor/alpha contracts; integrate the Hearth icon and always-green presentation. Verify label `Hearth Fire`, seven approved flame frames at 70% scale, no ownership/online indicator, no minimap marker, and unchanged campfire rendering.
- [ ] 6.2 Add `Travel to Hearth Fire` beside `Stand Up` only after active KO ends, using the same action, progress, and shared cooldown UI. Verify knockout/action UI suites cover active KO, lying, cancellation, success, and cooldown.
- [ ] 6.3 Add only `stream_epoch = 3` to existing `S2C_Fx`, preserving fields 1/2, and publish one position-based skull-smoke event at runtime character death or Hearth conversion to current observers. Regenerate server/client protobuf and test session/layer isolation, separate event positions, no historical replay, and legacy local-player `exp_gain` behavior.
- [x] 6.4 Render a reusable detached one-shot effect from the eight preloaded frames at 250 ms per frame, with a 2000 ms local monotonic lifetime, bounded concurrent effects, and owner-container cleanup. Drop events before asset readiness instead of retaining callbacks for old events. Test source-view removal, suspended rendering, capacity limits, late loading after reset, and world/session destruction without an event-ID deduplication or server-clock freshness subsystem.
- [ ] 6.5 Add visual/client asset regressions and gameplay-scale desktop/mobile screenshots for green fire, skull smoke, and post-KO controls; document delivered contracts and continuous-versus-transient smoke. Run relevant asset tests and, only if atlas/tools change, the three required pipeline checks; generate no replacement art and add no explanatory in-app copy.
- [x] 6.6 Add the minimal Hearth owner window through ordinary RMB approach/link, reusable text snapshots, owner metadata and existing close/reset paths. Keep the owner-contact exception feature-owned and use one optional LinkSystem callback; break links before live conversion. No window registry, instance IDs, client close command or push updates.

Task 6.3 character-death increment (2026-10-17):
Permanent death now emits the existing skull_smoke at the body's Transform,
through one call in HandlePlayerPermanentDeath after corpse conversion. A local
helper in player_death_visual.go uses unchanged SendObjectFx and supplies only
the missing self-recipient via existing SendFx before first visibility. An indexed
self-observer is not sent twice. Shared sending, client code, protocol, assets,
persistence and observer-mode ordering are unchanged. No new state or subsystem
is introduced. Task 6.3 remains unchecked only for replacement FX and its coverage.

`GOCACHE=/tmp/origin-go-build make test` and targeted PlayerDeathFx/HearthDeathFx/
PlayerDeathSystem tests passed. Real socket regressions verify one self-send
with/without self visibility, current versus replaced/ineligible sessions,
observer positions/epochs, and separate body/Hearth positions. Existing death
lifecycle tests cover one-time processing and knockout separation. The PostgreSQL
attachment/transfer death packet helper now checks body-position FX too; these
integration scenarios were explicitly skipped because ORIGIN_OBJECT_HEALTH_TEST_DSN
is unset. No claim of a live database or full-game death E2E run is made.
Client test:world-fx passed (5 tests). Independent code-reviewer2 review returned
Approve without significant findings. A temporary copy of the existing hybrid
character preview exercised the unchanged real lying actor and FxManager at
game scale; smoke placement over the body was visually checked. This was a
renderer preview, not a full-game network E2E run. The temporary files were
removed. Strict OpenSpec validation and standard whitespace checks passed.

Tasks 6.3 (partial), 6.4 and smoke-only 6.5 verification (2026-10-17):
The approved implementation scope is only the converted Hearth at permanent owner
death. Character-location, replacement and REST-deletion triggers are not added;
6.3 remains unchecked for its remaining triggers and 6.5 for its other visuals.
`CompleteTerminal` returns a saved live-conversion handle; the existing surface
completion uses `SendObjectFx(handle, key)`, the visibility reverse index and
`SendFx`. The only wire addition is `S2C_Fx.stream_epoch = 3`. No extra ECS system,
queue, event-bus event, callback registry, SQL or dependency is introduced.
The existing client FxManager owns detached world effects through activeFx and
its ticker: shared preload, drop-before-ready, 64 effects, monotonic 2000 ms
expiry and per-container reset/destroy cleanup. No pending event loading callback
exists, so late asset completion cannot revive reset-world presentation.

`GOCACHE=/tmp/origin-go-build make test` passed, including protobuf/sqlc
regeneration and real localhost socket regressions. Targeted new object-FX and
Hearth death tests passed outside the socket-restricted sandbox. They cover
position/observer/session/epoch selection, stale handles, no send before save,
source-unlock completion from another layer, terminal repeats, raw/no-home,
REST deletion and save failure. Updated terminal tests assert which conversions
return a live handle. Initial ad-hoc CGO-enabled tests failed at the host macOS
SDK linker; the required make target uses CGO_ENABLED=0 and passed.
Client proto generation, type-check, lint, test:world-fx (5 tests), test:sounds,
test:render-init and test:asset-pipeline passed. Lint did not rewrite existing
files. The render-init failure fixtures log expected missing browser texture
loader warnings while testing deliberately incomplete renderers.
Independent code-reviewer2 review returned Approve without significant findings.
A temporary browser harness exercised the real ObjectManager/FxManager conversion
and smoke at game scale on desktop/mobile-sized canvases; the approved art and
placement were visually inspected. This is not a full-game network E2E test or
an actual mobile-device run. The harness is removed after verification.
PostgreSQL integration tests were skipped because ORIGIN_OBJECT_HEALTH_TEST_DSN
is unset; live database restart and full-game owner death were not exercised.
Strict OpenSpec validation and `git diff --check` with standard whitespace rules
passed. The repository's `indent-with-non-tab` setting conflicts with existing
space-indented TypeScript; no indentation rewrite was introduced.

Task 6.6 verification (2026-10-17): `make test` with `GOCACHE=/tmp/origin-go-build`
passed, including protobuf/sqlc regeneration and real RMB/FIFO regressions.
The sender and its test use platform-neutral filenames `custom_window.go` and
`custom_window_test.go`; `go list` confirms both are included on Darwin and Linux.
The real sender test passes explicitly after correcting the accidental `_windows`
suffix, which had excluded both files from earlier non-Windows checks.
Client `npm run proto`, `type-check`, `lint`, `test:map-click` and
`test:custom-windows` passed; lint left existing files unchanged.
`tests/custom-windows.html` was checked in a browser for literal HTML-like names,
long-text wrapping/scrolling, a second form and local close/reopen without commands.
Strict OpenSpec validation passed using the installed Homebrew Node runtime.
`git diff --check` with standard whitespace rules passed; the user's
`indent-with-non-tab` setting flags the repository's existing space indentation.
PostgreSQL integration tests were skipped because `ORIGIN_OBJECT_HEALTH_TEST_DSN`
was unset; live database restart and full-game browser scenarios were not exercised.

## 7. Integration and Release

- [ ] 7.1 Run make test and PostgreSQL Hearth integration coverage using the existing fixture. Verify regeneration, schema uniqueness, synchronous construction, activation and death transactions, REST cleanup, and restart repair; report an unavailable fixture rather than claiming skipped database tests passed.
- [ ] 7.2 Run client type-check, lint, and relevant action/execution/knockout/movement/map-click/chunk/minimap/FX/asset suites; review lint's automatic diff and verify unrelated presentation/protocol regressions remain covered.
- [ ] 7.3 Exercise no-home entry, first construction/replacement, underground carry return, post-KO return, owner death/deletion, and blocked-position login recovery end to end. Verify runtime invariants during synchronous I/O, full-server stop on actual Hearth database/invalid-result errors, HTTP 500 on the preceding REST DELETE failure, accepted loop delay and process-lifetime memory growth, and bounded collider/metadata/FX work without tick scans or N+1 SQL.
- [ ] 7.4 Run strict OpenSpec validation and `git diff --check`, review generated/assets/protocol diffs, and deliver clean-database deployment notes. Verify no automatic entry, backfill, database clearing, new dependency, or unrelated balance/`life_death_factor` change, and explicitly distinguish accepted crash rollback from runtime corruption.
