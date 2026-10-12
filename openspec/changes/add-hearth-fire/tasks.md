# Tasks

## 1. Persistence Contracts

- [ ] 1.1 Add the deployment migration and matching `migrations/schema.sql` changes for nullable `character.first_world_entry_at` and unpartitioned `character_hearth` with character PK, unique object reference, and composite RESTRICT FK; verify PostgreSQL tests reject duplicate ownership and invalid references and do not require legacy backfill.
- [ ] 1.2 Add sqlc queries for indexed home/owner joins, character-row locking, guarded initialization, replacement, and terminal unlinking using `(region, id)`; run `make sqlc` and verify transactional tests cover an absent association and soft-deleted characters without hand-editing generated files.
- [ ] 1.3 Add optional `hearth_finisher_id` to the existing build behavior state and serialization only for pending completed Hearth sites; verify round-trip/recovery tests preserve the original finisher and ordinary build states omit the field.
- [ ] 1.4 Reconcile the implemented persistence contract with `docs/features/hearth_fire.md` section 2.11; verify schema/query paths, no duplicated home coordinates or cooldown columns, and `git diff --check`.

## 2. Ordered Durable Hearth Lifecycle

- [ ] 2.1 Add bounded per-character lifecycle admission, immutable operation records, and terminal latching shared across shards; verify tests preserve accepted completion/death order regardless of worker scheduling and queue saturation, without consuming final-branch costs on rejected admission.
- [ ] 2.2 Prepare and pin affected surface chunks through their owning shard and submit work through the existing bounded persistence workers; verify inactive/unloaded chunk cases and instrumentation show no SQL, blocking DB waits, or cross-world mutation in the tick path.
- [ ] 2.3 Implement atomic initialization/replacement transactions with character locks, expected-source guards, current association checks, and allocator high-watermark advancement for first durable IDs; verify rollback, conflicting source, and unsaved-build-object PostgreSQL cases.
- [ ] 2.4 Reconcile confirmed commits into raw cache, derived ownership, and ECS through existing in-place lifecycle helpers before releasing pins; verify stale handle, old save/load snapshot, duplicate callback, and ordinary-campfire restore tests cannot resurrect a prior hearth or site.
- [ ] 2.5 Implement uncertain-commit retry, replay detection, definitive-refusal cleanup, and bounded shutdown handling; verify lost-ack and delayed-old-completion tests neither allocate another automatic hearth nor overwrite a newer home, and all definitive paths release reservations.
- [ ] 2.6 Document coordinator ordering, worker/pin lifetime, and recovery boundaries in the feature's engineering notes; verify they match the delivered tests and do not claim DB transactions alone prevent stale chunk saves.

## 3. World Definition and Placement

- [ ] 3.1 Add the `hearth_fire` object/behavior definition using existing indestructibility and non-liftability mechanisms, fixed green appearance, and exact 1x1-tile bounds without fuel runtime; verify definition/restore tests deny damage, removal, and lift for owner and non-owner and keep ordinary campfires unchanged.
- [ ] 3.2 Add owner/animal-specific hearth candidate filtering with cached ownership and reuse it in authoritative spawn checks; verify owner passage, other-player blocking, animal passage, and unrelated-obstacle blocking without global `IgnoreObjectCollision` or tick-time SQL.
- [ ] 3.3 Validate Hearth placement over every overlapped surface tile against existing terrain identifiers for passable dry land, excluding water, shallow water, swamp, and bog; verify edge-overlap, underground, and allowed dry-soil tests using the same bounds as collision.
- [ ] 3.4 Enforce independent hearth-footprint exclusion for construction and carried-object put-down at admission and final application, including owner and pending reservations; verify anchor-outside/footprint-inside and stale-placement tests reject overlap while preserving valid carry state.
- [ ] 3.5 Hydrate derived hearth ownership with batched joins/lookup on chunk activation and clear it on conversion; verify chunk unload/reload and conversion tests restore the correct collision policy before visibility and document actual terrain mapping and placement predicates.

## 4. First Entry and Login Recovery

- [ ] 4.1 Integrate first-entry detection into normal world-entry preparation rather than REST character creation, reserving a spawn that fits player and hearth; verify player-only-fit rejection and exact shared-position tests.
- [ ] 4.2 Commit first object, association, player position, marker, and allocator watermark before world exposure; verify failure before commit leaves retry eligibility and failure after commit, disconnect, or restart reuses the existing initialization without duplicate objects.
- [ ] 4.3 Implement staged normal login candidates: saved position, exact active home with its layer, near the saved position, then existing random fallback on the saved layer; verify same-layer and underground-to-surface cases and that a blocked home does not move the nearby search around the home.
- [ ] 4.4 Preserve `tryReattachPlayer` as the first unchanged live-body path, plus existing capacity/error cleanup; verify reconnect keeps the same entity/position and never runs first-home creation or fallback for that body.
- [ ] 4.5 Update the feature's entry/recovery documentation to match actual marker and candidate-stage behavior; verify examples distinguish normal login fallback from voluntary exact-home return and ordinary saves cannot rewrite initialization.

## 5. Manual Construction and Replacement

- [ ] 5.1 Add the accessible five-branch build recipe with effective two-second and eight-stamina processing per branch through existing construction mechanics; verify solo ten-second/forty-stamina and split-worker cost tests with no skill/discovery requirement.
- [ ] 5.2 Capture the actual final-branch actor once after lifecycle admission and preserve `hearth_finisher_id` through pending completion and recovery; verify another caller, offline/dead site initiator, restart, and retry cannot change the finisher.
- [ ] 5.3 Route Hearth-only finalization through the durable coordinator and remove build state only on committed conversion; verify the finisher's old home becomes an ordinary unlit campfire at the same identity/location while the initiator's home and ordinary build finalization stay unchanged.
- [ ] 5.4 Add cooperative, competing-completion, and delayed-replay integration coverage and reconcile construction documentation; verify unlimited sites coexist while DB ownership remains unique and a newer home is never replaced by an old acknowledgment.

## 6. Permanent Death and Terminal Cleanup

- [ ] 6.1 Integrate home conversion/unlink with character soft-delete, status, and auth-token updates through the ordered worker lifecycle; preserve terminal commit before durable corpse writes by pinning/gating source chunk saves while retaining immediate runtime corpse/observer behavior, and verify cross-layer, unloaded-home, worker-delay, and restart injection tests leave no live SQL character alongside its durable corpse.
- [ ] 6.2 Route existing REST terminal character deletion through the same Hearth cleanup without changing its authorization/deletion policy; verify authorized offline deletion converts/unlinks its home, unauthorized deletion remains rejected, and no nonexistent player-body death effect is fabricated.
- [ ] 6.3 Emit one character-position death event at actual permanent death and one observed hearth-conversion event after its committed runtime transition; verify event-time observer snapshots, separate locations, duplicates, and unobserved/unloaded homes before adding client rendering.
- [ ] 6.4 Add completion-before-death, terminal-before-completion, worker-outage, and restart tests; verify deterministic terminal ownership and document the terminal-lifecycle and existing REST boundary rather than widening unrelated corpse persistence behavior.

## 7. Ordinary Return and Transfer Outcomes

- [ ] 7.1 Register the no-target `Travel to Hearth Fire` definition/handler with five-second existing progress, zero stamina, and 360000 ms shared cooldown; verify eligibility/action-cycle tests cover missing home, combat, swimming, active KO, and standing shallow water.
- [ ] 7.2 Resolve the latest active home at authoritative handoff and validate source identity, generation, action token, connection, combat, swimming, and resolved KO state before detach; verify mid-progress replacement and late-canceled/stale-transfer tests cannot use an old home or transfer after cancellation.
- [ ] 7.3 Add a Hearth transfer cause with deferred completion and an idempotent successful-target cooldown hook after ordinary participant restoration/failure resolution and before initial target snapshots; verify request admission starts no cooldown, successful body relocation starts it once even after a carry-restoration warning, and source/target rejection, timeout, and rollback start none.
- [ ] 7.4 Reuse exact-target player-only clearance and the existing carry participant for same-layer/cross-layer return; verify blocked-home failure has no nearby fallback, large carried footprint is excluded from clearance, and failed transfers preserve body/carry without duplication.
- [ ] 7.5 Apply the post-KO lying allowance only to Hearth and use a separate standing target snapshot while retaining the untouched lying rollback snapshot; verify successful return stands, failed return remains lying, stairs/admin rules are unchanged, and post-capture socket failure follows ordinary transfer restoration.
- [ ] 7.6 Update `docs/features/runtime_ko.md` and related feature notes with the Hearth-only post-KO exception and success boundary; verify existing transfer/health tests plus the new cause-specific tests agree with the documentation.

## 8. Interruption at Gameplay Intent

- [ ] 8.1 Add narrowly scoped Hearth cancellation before availability checks at all shared gameplay execution entry points, including menu/hotbar, context, craft/build, carry/item gameplay, and Stand Up attempts; verify successful and rejected attempts, including cooldown rejection, cancel return without changing unrelated action admission.
- [ ] 8.2 Cancel Hearth on fresh valid WASD and ordinary ground-movement requests before gameplay movement restrictions or displacement checks; verify collision, lying, stun, affordability, and unchanged-target cases cancel, while stale/malformed/expired/wrong-session input, release/refresh, and administrator-consumed clicks retain existing isolation.
- [ ] 8.3 Wire any damage and combat/swimming transitions to ordinary return cancellation and retain generic disconnect cleanup; verify damage-without-combat, disconnect during progress, and stale completion tests leave source position and cooldown unchanged, while chat/personal-window opening preserves progress.
- [ ] 8.4 Convey cancellation intent for other-action attempts blocked locally by existing client cooldown controls without starting the blocked action; verify Actions menu, hotbar keyboard/touch/mouse, and post-KO movement-intent tests, plus `npm run test:actions`, `npm run test:movement-input`, and `npm run test:map-click` regressions.

## 9. Hearth Presentation and Post-Knockout UI

- [ ] 9.1 Before generating/editing Origin art, read the approved style guide and visually inspect its reference; create the hearth/flame and skull-smoke assets with original sources/provenance and existing projection, scale, anchor, frame-key, and alpha contracts, and verify them at gameplay scale rather than substituting concept art.
- [ ] 9.2 Integrate the hearth resource/icon and always-green world appearance through existing loaders and Actions presentation; verify label `Hearth Fire`, no owner/online indicator, no minimap changes, and unaffected ordinary campfire rendering.
- [ ] 9.3 Add `Travel to Hearth Fire` beside `Stand Up` only after active KO ends using the same activation, progress, and shared cooldown presentation; verify `npm run test:knockout` and action UI tests for active KO, lying, success, cancellation, and cooldown.
- [ ] 9.4 Verify asset/visual integration with relevant client asset tests and desktop/mobile screenshots; if atlas/tools are changed, run `python3 tools/tests/roundtrip_test.py --regression`, `node tools/tests/pixi_semantics_test.mjs`, and `python3 tools/tests/render_sim.py`, and document the delivered asset contracts without adding explanatory in-app copy.

## 10. Shared World-Event Skull Smoke

- [ ] 10.1 Add `stream_epoch = 3`, `event_id = 4`, and `server_time_ms = 5` to existing `S2C_Fx` while preserving fields 1/2; run `make proto` and `npm run proto` in `web_new/`, and verify round-trip tests preserve legacy `exp_gain` decoding and exact event identity.
- [ ] 10.2 Publish world-position skull events through current reverse visibility with immutable event/session snapshots and recipient epochs; verify all current source observers receive one event, old sessions/other layers/later viewers receive none, and unloaded conversion does not queue historical replay.
- [ ] 10.3 Extend FX dispatch/manager with a reusable detached one-shot preset and author-time 2000 ms total expiry, exact-ID dedup, current-entry freshness, and existing session-clock validation; verify death/conversion positions, duplicate/stale/expired/malformed events, delayed delivery/loading, permitted clock jitter, and unchanged owner-local experience FX.
- [ ] 10.4 Bound active events, particles, and pending asset work; clear them and invalidate asynchronous callbacks on world/session/layer/reset/destroy paths; verify hidden/tab-suspended expiry, capacity exhaustion, view rebuild independence, and late-load-after-reset tests without an unbounded cosmetic queue.
- [ ] 10.5 Document the event fields, freshness/lifetime boundary, and continuous-versus-transient smoke distinction; verify the resulting ordinary unlit campfire emits no continuous smoke and visual smoke is fully gone at the original two-second expiry.

## 11. Integration and Release Gates

- [ ] 11.1 Run `make test` at the repository root and PostgreSQL Hearth lifecycle integration coverage using the existing test fixture; verify protobuf/sqlc regeneration is coherent and report any unavailable DB fixture rather than claiming skipped coverage passed.
- [ ] 11.2 In `web_new/`, run `npm run type-check`, `npm run lint`, and the relevant action, execution, knockout, movement, map-click, chunks, minimap, combat-protocol, visual, asset, and new FX suites; verify unrelated regression coverage and review lint's automatic diff for scope.
- [ ] 11.3 Exercise the complete first-entry/build-replacement/underground-carry-return/post-KO/death/login lifecycle with server-authoritative state assertions and screenshots; verify failure injection and bounded queue/collider/FX work under representative load without new tick scans or N+1 SQL.
- [ ] 11.4 Run strict OpenSpec validation and `git diff --check`, review generated/assets/protocol diffs, and deliver clean-database deployment and controlled rollback notes; verify no backfill, automatic DB clearing, unrelated balance or `life_death_factor` changes, or new dependencies were introduced.
