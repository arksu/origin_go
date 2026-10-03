# Tasks

## 1. Combat definitions and weapon inputs

- [x] 1.1 Extend `internal/actiondefs` with direction targeting and the explicit combat profile, preserving legacy defaults; add loader tests proving existing definitions still load and invalid timings, numbers, references, approach/repeat combinations, and unknown fields fail with file-specific errors.
- [x] 1.2 Extend `internal/itemdefs` with optional weapon damage/range metadata and add `axe_aoe`/`axe_single` definitions plus the existing `stone_axe` preset; verify loaded values are B=6, range=18, angle=90, multipliers=1/1.5, times=600/400/2000 ms, and stamina=60 using registry tests.
- [x] 1.3 Implement deterministic right-hand-then-left-hand weapon resolution and a float64 STR input adapter; verify different-quality dual axes, missing/invalid weapons, and fractional supplied effective STR in focused tests without adding a status runtime.
- [x] 1.4 Document the new action and weapon fields beside existing definition documentation, including normalization and unsupported combinations; verify examples through the loaders and `go test ./internal/actiondefs ./internal/itemdefs`.

## 2. Shared targeting and damage

- [x] 2.1 Implement pure sector/AABB contact and intersection-distance calculation in `internal/combat` with the documented geometry tolerance; test angular/range tangency, just-outside misses, rectangle containment, degenerate intersections, centers outside range, and nearest collider points outside the cone.
- [x] 2.2 Implement shared candidate filtering, self exclusion, deduplication, AoE selection, and nearest-distance/ascending-ID selection; verify shuffled input order, duplicate handles, stale incarnations, depleted receivers, self-only misses, allied receivers, and absence of category priority.
- [x] 2.3 Implement float64 raw damage and armor helpers with explicit invalid-input handling; verify 6/9 baseline damage, STR/Quality scaling, multiplier-before-armor, 3.6 and 81/13 at A=4, positive sub-unit damage, and Draw=A=0 with `go test ./internal/combat`.
- [x] 2.4 Add the minimal damage-receiver registration/dispatch contract and collider-extent-aware broad phase across loaded chunk grids; verify a large receiver centered outside range/across a chunk edge is found and duplicate discovery still produces one hit.
- [x] 2.5 Document receiver eligibility, stable ordering, numerical tolerance, and the distinction between movement blockers and damage receivers beside the combat package; verify no player-session dependency, wall-occlusion check, or permanent test-type whitelist is required by the shared API.

## 3. Wire contract and request validation

- [x] 3.1 Extend `api/proto/packets.proto` with additive combat support, profile/selection state, directed-attempt identity, public execution/results, fractional target HP, and millisecond animation timing; regenerate Go and browser bindings with `make proto` and `npm --prefix web_new run proto`, verifying existing field numbers and retired reservations are preserved.
- [x] 3.2 Route combat activation and tagged direction clicks through session/world/epoch and high-water revision checks before effects; add tests for wrong connection, wrong epoch, zero/old revisions, stale selection, repeated accepted/rejected attempts, and replay after cooldown expiry with no ordinary-click fallback.
- [x] 3.3 Add protocol round-trip fixtures covering float64 HP, full-width identities, locked direction, phase timing, and absent optional combat fields; verify legacy map clicks and noncombat activation remain compatible and combat requests require advertised support.
- [x] 3.4 Document the activation/click sequence and replay scope in protocol documentation; verify each documented packet field exists in generated bindings and owner-only cooldown state is separate from public state.

## 4. Authoritative combat lifecycle

- [x] 4.1 Add entity-owned execution/cooldown/activity state and register the combat service with the existing action catalog/handlers; verify selecting is free, conflicting execution is rejected, and a valid direction commit atomically enters windup exactly once.
- [x] 4.2 Implement start payment using existing stamina, regen-delay, dirty/save paths; test stamina 59 rejection, exact 60-to-zero acceptance, no later affordability cancellation, and no repayment/refund on hit, miss, or interruption.
- [x] 4.3 Implement runtime-time strike/recovery/cooldown deadlines and shard scheduling after collision/transform; use controlled-time tests for impact at 600 ms, next-action availability at 1000 ms, same-action cooldown at 2000 ms, alternate actions, another tick rate, and a step crossing multiple deadlines.
- [x] 4.4 Connect strike resolution to current actor/receiver positions and shared damage dispatch; verify moved/escaped targets, moving attacker with locked direction, STR changed during windup, wall non-occlusion, and no delayed target lock.
- [x] 4.5 Implement stable accepted-start/strike/hit event identities and deterministic application order; test repeated updates, multiple due strikes, depleted targets after an earlier event, and exactly one hit per target/execution.
- [x] 4.6 Add the explicit external interruption path and actor teardown cleanup, wiring existing invalid/dead/KO transitions without redesigning health; test interruption before/after strike, canceled recovery, retained cooldown/payment, no resurrection from late callbacks, and ordinary damage not interrupting.
- [x] 4.7 Update runtime LastCombatEventAt only for accepted combat events using maximum timestamps; verify start t=0/miss t=0.6 and that movement, aim, rejection, recovery, and noncombat health updates leave it unchanged.
- [x] 4.8 Document state transitions and the boundary between session input validation and deferred timer persistence; verify a non-player test actor can use the shared runtime without a socket or equipped inventory item, and run the focused combat service tests.

## 5. Movement, cancellation, and inventory compatibility

- [x] 5.1 Add policy-aware cancellation and busy guards to action activation/Escape, secondary routing, craft/build/context services, lift shortcuts, and deferred interaction paths; test raw requests in windup/recovery reject without cancellation, side effects, or queued work while ordinary noncombat cancellation tests still pass.
- [x] 5.2 Route primary clicks during committed combat to coordinate movement only and preserve direction-selection precedence over held-item drops; verify object/drop clicks cannot initiate link/pickup/put-down, stale tagged clicks cannot become movement, and primary administrator precedence remains intact.
- [x] 5.3 Apply an effective Crawl cap to both directional and point movement through impact without overwriting selected mode; test speed and movement cost, recovery restoration, zero-stamina immobility with attack completion, collision, and unchanged hold revision/expiry behavior.
- [x] 5.4 Guard combat-equipment mutations in inventory validation before transaction changes, including source/destination/swap participants, same-container hand moves, transfer, and drop paths; test both hand slots stay unchanged during windup/recovery and grid-only rearrangements still succeed.
- [x] 5.5 Add compatibility documentation for legacy payment/cancellation versus combat commitment; verify existing `action_service`, `action_repeat`, `move_direction_actions`, dig/plow, inventory, and context/crafting regressions with `go test ./internal/game/... ./internal/ecs/systems ./internal/entitystats`.

## 6. Reproducible server test range

- [x] 6.1 Add validated default-off `game.combat_test_enabled` configuration and public-player range commands for create/reset/remove and bounded presets; test disabled requests and successful access by ordinary players, limits, collision with existing objects, and no modification of unrelated world entities.
- [x] 6.2 Implement transient test receivers with float64 HP/revisions, actual colliders, depletion at zero, reset, and complete spatial/entity cleanup; verify HP 1 minus 0.4 remains fractional, depleted receivers are excluded, and recreated fixtures reject old-incarnation hits.
- [x] 6.3 Add data-defined small/large/equidistant/moving/blocker fixtures and server motion before strike resolution; verify every preset exercises the common hit path and the blocker proves no axe occlusion without connecting real-object HP.
- [x] 6.4 Provide Q=10 starter-axe setup through the existing item-grant path and test-only stamina/STR/interruption controls; verify normal equip flow, exact 59/60 setup, and no automatic alteration of real-character health or new status runtime.
- [x] 6.5 Write `docs/features/combat_phase1_test_range.md` with exact configuration/player commands, fixture layout, input gesture, expected damage/timing, reset/cleanup, and missing phase-2–7 capabilities; verify commands against the server harness and state explicitly that target HP/cooldowns/activity do not survive restart.
- [x] 6.6 Verify combat expenditure marks existing persistence state and a save/load round trip preserves spent stamina and item Quality under controlled regeneration; assert fixtures are excluded from persistence and document that no new DB migration is introduced.

## 7. Public state, animation, and client controls

- [x] 7.1 Publish owner combat/cooldown state, visibility-scoped public execution/results, and target HP snapshots using existing critical-delivery conventions; test visibility-entry races, late observer snapshots, duplicate results, queue failure, fixture removal, and target visibility filtering.
- [x] 7.2 Extend server animation source resolution/snapshot building and asset-pipeline validation for generic combat sources, direction-facing, and millisecond timing; add data-only bindings using `chop_r`/`chop_l`, verify hand preference, reject invalid bindings, and preserve existing tree/craft animation behavior.
- [x] 7.3 Extend client action catalog/state, menu/hotbar activation, and map input to arm then commit a direction, preserve selection identity, and send no continuous aim stream; test keyboard handoff/fresh WASD, touch primary selection, Escape, repeated input, no held-item drop, and support-absent behavior.
- [x] 7.4 Add generic aim/locked-sector and phase presentation plus authoritative hit/miss/target-HP feedback; test a moving attack, hit deduplication, one-decimal and `<0.1` formatting, and no health/payment mutation from client collision or animation callbacks.
- [x] 7.5 Add informational independent cooldown indicators while retaining selectable icons and server rejection reasons; verify cooldown and recovery are visibly distinct and rejected activation does not change the catalog or invent local availability rules.
- [x] 7.6 Adapt common actor animation playback to authoritative millisecond combat timelines and locked facing; test delayed packets, mid-cycle visibility, asset-load completion, culling resume, interruption, stale epoch/incarnation, and uniform full-clip fitting with the strike independent of clip frames.
- [x] 7.7 Publish the updated action-animation catalog and extend the range guide with client/observer controls; verify `npm --prefix web_new run test:actions`, `test:map-click`, `test:movement-input`, `test:action-animations`, `test:character-visual`, and the relevant asset-pipeline definition/publication tests, then run the client type-check/build.

## 8. End-to-end acceptance

- [x] 8.1 Run the documented two-client demonstration at STR=1/Q=10/A=0: AoE 6 HP to every contact, single hit 9 HP to the nearest intersection, misses on escape, attacker movement with locked aim, and visible phases/stamina/cooldowns; record observed results and late-observer behavior in the range guide.
- [x] 8.2 Exercise the combined bypass/boundary matrix through client input and the packet harness: stamina 59/60, both sector edges/range, large targets, ties, self exclusion, repeated/late requests, Escape/secondary clicks, craft/lift/context entry, equipment swaps, and external interruption; verify no duplicate costs/hits or queued actions and record results.
- [x] 8.3 Run `go test ./...` with the project's documented integration-test prerequisites and the client checks from group 7; record any environmental blockers accurately and confirm ordinary movement, inventory, dig/plow, crafting, and existing animation behavior pass without requiring deferred combat phases.
- [x] 8.4 Validate the completed change with `openspec validate add-axe-combat --strict`, confirm all implementation tasks and demonstration evidence are complete, and prepare the accepted spec sync/archive handoff without claiming PvP or persistent-world combat readiness.

Acceptance evidence and reproducible commands: [phase-1 range guide](../../../docs/features/combat_phase1_test_range.md).
