# Design

## Context

See `proposal.md` for motivation and scope. The authority for gameplay is `docs/features/combat_final.md` §§4–6, 10–11, 13.6, 16–17.1; `docs/features/combat_phases.md` §4 limits this change to the test range.

Observed integration points:

| Current code | Consequence for this change |
|---|---|
| `internal/actiondefs/types.go` supports string action IDs, none/object/tile targets, and tick-based completion cost. `ActionService.Activate` cancels the previous action; `Recheck` rechecks affordability. | Add an explicit direction/combat contract; a paid axe must not enter legacy cancellation or completion-payment paths. Keep existing action IDs and hotbar storage. |
| `internal/ecs/systems/network_command.go` routes primary clicks, secondary cancellation, context work, crafting, and inventory operations. `directional_movement.go` cancels non-selecting actions. | Commitment must be checked before these entry points produce side effects, including deferred callbacks. |
| `data/items/tools.jsonc` already defines `stone_axe` (1002), tag `axe`, both hand slots. Inventory instances carry Quality. | Extend the existing weapon definition; do not create an unrelated starter weapon or item-quality system. |
| `internal/entitystats/movement.go` enforces stamina/carry limits; Crawl has no movement cost. Stamina is fractional, while long-action helpers impose a reserve. | Use normal stamina/regen state and exact payment, not the long-action reserve helper. Add a temporary effective movement cap. |
| `Collider` is an axis-aligned rectangle. `internal/core/spatial.go` indexes centers in cells. | Sector-center tests and radius-18 center queries alone miss large targets. |
| `internal/cyclicaction`, `ActionAnimationSystem`, and character spawn events provide public revisions, incarnation, snapshots, and tick timing. `data/action_animations/tree.json` has complete `chop_r`/`chop_l` clips. | Reuse synchronization and clips with a combat source and millisecond timeline; do not make animation impact authoritative. |
| `TimeState.Now` supplies runtime time and `UnixMs` supplies network sampling time. Movement/collision/transform priorities are 100/200/300; action validation is 314 and public animation push is 491. | Read time from the resource, resolve geometry after collision/transform, and publish after transitions. |
| Player health and persisted object HP use incompatible legacy health semantics/integer fields. | Register only test receivers now. No health migration or KO rewrite is required for phase 1. |

The existing `game-actions` and `map-click-input` specs explicitly require cancellation on inputs that cannot cancel a committed axe. Their delta requirements preserve complete existing scenarios, scope legacy behavior to noncombat, and add combat exceptions. `character-action-animations` also needs an explicit exception to effects occurring only at the end of a legacy cycle.

## Goals / Non-Goals

**Goals:** Keep one small reusable attack runtime, one precise geometry implementation, and one damage path; expose a complete client demonstration; make all bypass-sensitive decisions on the shard thread; leave integration seams for later receivers, effective attributes, and interruption sources.

**Non-Goals:** A universal effects engine, an AI framework, persistent test fixtures, new health schemas, action-speed modifiers, final animation production, or a new scene editor. Session authenticity and stale-message rejection are required now; durable combat timers and combat logout protection are phase 7.

## Decisions

### 1. Extend existing definitions with an explicit combat profile

Keep action IDs `axe_aoe` and `axe_single` in `data/actions/`. Each uses `target.kind: direction`, an equipped `axe` requirement in either existing hand slot, and `execution.combat` describing `selection: all|nearest`, `sectorAngleDegrees: 90`, `windupMs: 600`, `recoveryMs: 400`, `cooldownMs: 2000`, and `damageMultiplier: 1|1.5`. Keep `execution.stamina: 60` as the single authored cost. Combat profiles reject `execution.ticks`, `execution.repeat: true`, tile approach, and `isRepeatable: true`; omitted combat preserves the old policy.

Add optional weapon combat metadata to item definitions: `baseDamage: 6`, `range: 18` on `stone_axe`. Keep normalization values S0=1 and Q0=10 as named combat configuration constants. All numeric inputs must be finite; damage/cost nonnegative, Quality and normalization positive, geometry and durations positive, and milliseconds integral. Unknown or conflicting fields/references fail before activation. Status definitions and numeric status IDs are not introduced; current action string IDs remain the existing catalog identity.

Resolve a weapon deterministically at start: compatible right hand first, then left hand. Store item identity, B, range, and Quality in the execution; both weapon-hand slots are locked until recovery ends. Presentation variants follow the same preference. Read effective STR as float64 at impact, initially adapting the existing integer base-attribute helper. Later modifiers replace that provider, not the formula.

Alternative: create an independent combat catalog and duplicate action requirements. Rejected because the existing catalog, hotbar, item instances, and equipment selectors already cover those concerns.

### 2. Selection plus a directional click is the activation gesture

Selecting either icon or its pinned hotbar slot arms the action on the server. The pointer provides a local sector preview independently of movement direction. A primary map click commits one direction from the server's current actor position toward the clicked world coordinates. Target entity ID is irrelevant to victim choice. A zero vector is rejected; no fallback to movement or a guessed direction occurs. No target approach or continuous aim network stream is needed for an axe.

Use the existing pointer-to-world conversion and keyboard handoff: activation/click releases an existing keyboard hold; a fresh WASD press can move during selection or the attack. Preserve this established input behavior rather than introducing a hidden alternate click mode. After acceptance, show the locked sector separately from the movable pointer preview. During windup/recovery an ordinary primary click requests only coordinate movement, even when an object is under the pointer; it cannot link, pick up, drop the cursor item, or queue another attack. Secondary clicks are consumed while committed. After recovery return to idle, requiring another explicit activation/click for the next attempt.

A selecting action may be canceled or replaced for free. Starting combat while crafting/building/context execution, another noncombat execution, or world-object carry is active returns busy; do not cancel work merely to discover combat cannot start. Replacing another selection is allowed. Successful commit clears any remaining noncombat approach/pickup/link intent through its existing lifecycle so it cannot complete during the attack. Plain movement can continue subject to the input handoff and movement cap.

Alternative: activate immediately toward the last pointer position. Rejected because menu activation, touch, and missing/stale pointer coordinates would require a second gesture contract.

### 3. Separate combat execution from legacy cyclic work

Add a small entity-owned combat runtime and service under `internal/game`/`internal/ecs`, with geometry and arithmetic in a pure `internal/combat` package. `ActiveGameAction` remains the catalog/selection adapter; accepted combat delegates to the combat service rather than `ActiveCyclicAction`. Route voluntary cancellation through a policy-aware method and keep a distinct external interruption method. Extend public action phase handling to `windup` and `recovery`.

Minimal runtime state:

- Execution identity: actor incarnation, monotonically increasing execution sequence, action ID, locked direction, weapon parameters.
- Deadlines: start, strike, recovery end; a resolved-strike marker ensures once-only processing.
- Actor runtime: per-action cooldown deadlines, last accepted directed-input revision, current selection generation, and runtime LastCombatEventAt.
- Test receiver: float64 current/max HP, actual collider, fixture identity/incarnation, and depletion state.

Use `TimeState.Now` for runtime deadlines and elapsed durations, plus `UnixMs` when publishing anchors/events. Do not call wall time independently in systems or convert definitions to a fixed number of 10 Hz ticks. Resolve at the first server step at or after each deadline. The minimum temporal resolution remains one server step; no historical target rewind is introduced.

| Transition | Atomic effects |
|---|---|
| Selection -> windup | Validate session/request, weapon, state, cooldown, and stamina; charge 60 once; update regen delay/dirty state; allocate execution; lock direction and weapon; set strike=start+600 ms, recoveryEnd=strike+400 ms, cooldown=start+2000 ms; emit accepted-start event. |
| Windup -> recovery | After collision-resolved movement, if still active, resolve one strike against current positions; mark resolved before callbacks; emit strike and hit events; remove only the attack movement cap. |
| Recovery -> idle | Clear committed execution and presentation; retain cooldowns. No automatic rearm/repeat. |
| External interruption -> idle | Invalidate unresolved strike/callback identity and discard recovery; retain payment, cooldown, and already-applied damage. |

Schedule strike resolution after transform updates and before public visibility/state publication. Existing invalid/dead/KO actor state must be checked before impact; hook existing external health/teardown transitions into interruption without changing their health semantics. Resolve due strikes in deadline then execution-sequence order; target application order is stable ID order. Assign actual event timestamps at processing and a monotonic event sequence to break ties. Finish each damage application before the next event. If a step crosses strike and recovery deadlines, resolve once then finish. A busy check may finish already-resolved expired recovery before accepting a new command; it must never skip an unresolved strike to accept that command.

The core functions accept actor inputs and receiver interfaces, not a Player session or a fictitious inventory weapon requirement. The player adapter validates equipment; a later NPC adapter can provide its own attack parameters without duplicating the runtime.

Alternative: use a legacy cyclic action and refund/restore state on cancellation. Rejected because its affordability rechecks and completion payment conflict with commitment, and it would entangle crafting with combat.

### 4. Centralize commitment guards and temporary movement limits

Expose a common combat-busy/interruption policy through ECS/service interfaces usable by Actions, network dispatch, craft/build/context entry points, and inventory mutation validation. Guard before cancellation or any effect. Rejected work is never stored for later. Check deferred interaction/completion paths as well as direct requests.

Inventory validation must inspect the resolved source, destination, and swap participants before changing containers. Any mutation affecting either equipped hand fails while committed, including same-container slot moves, swaps from a grid, transfers, and drops. Do not block unrelated grid rearrangements. Weapon validation failure from an external administrative mutation uses an explicit invalidation/interruption, not a silent free restart.

Compute the effective movement mode from existing stamina/carry/state rules and then apply the attack's Crawl cap while windup is unresolved, including the movement step immediately before impact. Do not overwrite `Movement.Mode`: a player's selected mode must remain available after the cap ends. Use the effective mode consistently for speed, movement stamina, and network state in both point and directional movement. Zero stamina can leave a paid attack stationary but cannot invalidate it. Expired/rejected directional holds remain retired; removing a cap does not resurrect them.

Alternative: guard only the client or `ActionService.Cancel`. Rejected because raw craft/context/inventory commands and deferred callbacks bypass that boundary.

### 5. One exact sector/rectangle calculation also supplies ranking

Use the target's real AABB from Transform and Collider, not its render bounds, center, movement collision mask, or phantom placement preview. Exclude the attacker's stable ID and invalid/depleted receivers before geometry. Do not use party/friendship filters or raycast walls.

For the convex 90-degree cone, clip the rectangle against the two cone half-planes. Find the closest point to the origin on that clipped polygon (distance zero when it contains the origin). If that distance is within range, the rectangle intersects the finite sector, and the same distance is exactly `distance(origin, collider ∩ sector)`. This avoids a sampled arc polygon and the incorrect nearest point outside the cone. Handle degenerate line/point intersections. Use a documented linear geometry tolerance of 1e-7 world units; keep target ranking by actual distance and then ascending stable ID, without a non-transitive epsilon sort comparator.

Use the existing spatial grids only as broad phase. Maintain a conservative per-world maximum receiver half-extent when receivers are registered/changed; query all loaded chunks intersecting the sector AABB expanded by that bound (floor/ceil conversion), then deduplicate valid handles and run exact geometry. Increasing extents updates the bound before a target is hittable; a retained larger bound after removal is safe. This covers centers outside range and chunk edges without a hardcoded test-target size or scanning every entity per swing. Range fixtures register through the same receiver registration path later adapters will use.

AoE applies full damage once to each candidate. Single-target selects the minimum distance then ID from the same candidate set. Revalidate receiver liveness/incarnation when applying an event. Geometry selects candidates without requiring a player session or hardcoded object category.

Alternative: center-radius query plus center-angle test. Rejected because it cannot satisfy large-collider or intersection-distance acceptance cases.

### 6. Shared arithmetic and receiver dispatch, with range-only integration

`internal/combat` computes raw damage and armor reduction from validated float64 inputs. Zero raw damage returns zero before division. No integer conversion, minimum damage, hard cap, or per-target AoE division is allowed. B/Quality are the committed weapon parameters; effective STR and each receiver's armor are sampled at impact. Range receivers provide A=0; formula tests also use A=4/12 so equipment armor can be added without replacing the calculation.

The game layer dispatches a hit record containing execution/event identity, attacker/target IDs and incarnations, raw/final damage, and authoritative time. A minimal receiver adapter answers eligibility/armor and applies damage. In this phase only test receivers are registered. They subtract float64 HP, clamp to zero, publish a revision, and mark depleted without invoking player health or ordinary object destruction. Keep a clear post-damage integration point for later effects, but no fake wound, permanent stun, or placeholder effect engine.

Accepted start, actual strike (including miss), and applied hit update the attacker's LastCombatEventAt with `max(old,eventTime)`. Test targets are object-like, so they need no character logout timer. Future creature adapters update recipient activity when they are added. Ordinary damage notification is distinct from the interruption entry point.

Alternative: decrement HP in a test command or client overlay. Rejected because it would not exercise the path future objects and creatures must share.

### 7. Additive protocol, explicit request identity, and observer convergence

Extend `api/proto/packets.proto` and regenerate Go/TypeScript bindings with additive fields/messages; preserve retired reservations. Advertise combat protocol support in enter-world bootstrap. Updated clients do not send combat-only fields without that support. Updated servers reject untagged direction execution; existing noncombat commands retain their meanings.

Proposed wire additions:

| Message area | Fields and behavior |
|---|---|
| Action definition/state | Target kind direction; combat profile summary; current selection generation; committed phase/execution identity; owner cooldown deadlines and server-time sample. Catalog remains static and contains no per-player availability flags. |
| Activation and directed MapClick | Combat activation has stream epoch and monotonic request revision. A direction-selection click carries action ID, selection generation, request revision, and stream epoch in an optional action-attempt envelope. Coordinates remain the existing MapClick coordinates. |
| Public combat state | Actor ID/incarnation, execution ID, monotonically increasing state revision, action ID, locked direction, phase, elapsed/duration, phase deadlines, and server-time sample; include current state in visibility entry. |
| Combat result | Event ID/sequence, execution ID, source identity, visible receiver identities, damage, authoritative receiver HP/revision, and strike hit-count/miss. Scope target information to recipients allowed to see that target. |
| Target snapshot | Fixture incarnation, current/max float64 HP, and revision; depleted state included on late visibility entry. |

For request acceptance, verify the queued command's connection still owns the entity and its world/epoch, as the directional-input path already does. Consume each well-formed new attempt revision once, including a gameplay rejection; retire a consumed selection generation after commit. A tagged attempt is always handled as such even if selection has since ended, so it cannot become an ordinary movement click. The existing server-assigned queue CommandID alone is insufficient to distinguish a retransmitted client attempt. Keep replay state bounded to high-water revisions/current generation, scoped by actor incarnation and stream epoch. No client timestamp can backdate impact.

Use the existing critical-delivery convention for action transitions and authoritative results: enqueue failure closes the affected connection rather than leaving connected clients with silently stale state. Current snapshots reconstruct state; old events do not need replay. Client caches key by epoch/incarnation/execution/revision and deduplicate event feedback. Keep uint64 identities lossless using existing protobuf-number conventions, not JavaScript floating-point ID arithmetic.

Alternative: infer phase from receipt time or rely solely on network ordering. Rejected because visibility snapshots, asset loading, retransmission, and actor replacement still create stale-state races.

### 8. Reuse animation synchronization and existing art

Add generic source kind `combat` and a `direction` facing policy in action-animation definitions/validation/publication. Extend animation state with optional authoritative elapsed/duration milliseconds and locked direction; legacy tick fields remain supported. Combat takes the millisecond timeline; noncombat retains `totalTicks × tickDuration`. The complete existing hand-specific chop clip traverses the 1000 ms windup-plus-recovery timeline, with no clipping to force its visible impact onto 600 ms. Server strike/result visualization identifies the actual hit moment.

Create bindings for both axe actions using the existing right/left axe variants, a movement-compatible eligibility rule (no stationary-only restriction), and locked-direction facing. Do not copy the tree's target-sourced sound cue: an axe direction has no selected target. New combat audio is optional and unnecessary for acceptance. Put action names, clip names, and variant choices only in definitions, not renderer branches. Use an ordinary Pixi sector/phase overlay and server-result feedback for the prototype; no raster art or tile-atlas change is needed.

Extend the public snapshot builder beyond its current cyclic-only source and reuse dirty queues/visibility rules. Late snapshots and completed asset loads seek to the current phase; missing bindings preserve base rendering while the combat overlay remains usable. Stamina uses existing stat updates; HP/damage use the fractional formatter. Cooldown countdowns are informative and do not disable server-authoritative activation.

Alternative: build a second renderer or drive damage from the chop frame. Rejected because it duplicates synchronization and makes gameplay depend on visual sampling.

### 9. A bounded public-player test range is the demonstration tool

Add a default-off `game.combat_test_enabled` setting and player commands `/combat-range create`, `reset`, and `remove`, plus a bounded fixture preset selector for the documented cases. Require the enabled setting and an active authenticated player; no administrator role is required during public testing. Administrator roles are deferred. Combat starts in this milestone require that enabled test context. Store fixture layout/HP/motion presets in a small data definition, not inline command branches.

Create a bounded patch around the requesting player on a disposable world, rejecting overlap with existing ordinary objects rather than deleting or moving them. Use existing object visuals plus collider/HP labels. Fixtures include small targets, a wide rectangle whose center is outside range, a symmetric nearest-distance pair, a server-moved target that crosses the sector during windup, and a wall-like movement blocker between attacker and target. The blocker has no damage receiver in phase 1; geometry still proves there is no occlusion. Registration/spatial position updates must follow fixture movement before strike resolution. Limit one range per player and clean up its entity/spatial/client state on removal.

The setup command provides a Q=10 `stone_axe` through the existing item grant path if needed; the user equips it normally. Reset only tagged range fixtures, never unrelated entities, inventory, or real-character health. Expose test-only interruption and stamina/attribute setup through the range service/command harness for reproducible boundary checks, without fabricating status runtime. Provide explicit normal and low-stamina presets in the demonstration instructions.

Fixtures, HP, execution state, cooldowns, and LastCombatEventAt are not persisted in this phase. Existing item Quality and player stamina retain existing save/load behavior and need regression coverage. Actor/fixture teardown invalidates callbacks and client state; this cleanup is not an implementation of combat disconnect protection. No schema migration is required.

Alternative: a permanent dummy-only targeting list or a new scene editor. Rejected because the former becomes a gameplay restriction and the latter adds unrelated infrastructure.

## Risks / Trade-offs

- [Cancellation bypass in another command path] → Guard service entry points and mutation transactions as well as packet dispatch; regression-test raw commands and late callbacks.
- [Large targets missed by center indexing] → Expand broad-phase bounds using registered receiver extents and test cross-chunk contacts; exact geometry remains the final authority.
- [Legacy affordability check cancels a paid attack] → Keep combat execution outside legacy cyclic completion and test exact-zero stamina across several updates.
- [Reused chop clip has a visually different impact frame] → Show the server strike with a sector/result cue; full animation fitting remains deterministic and final art is deferred.
- [New protocol state races with visibility/asset loading] → Preserve epoch/incarnation/revision guards, critical delivery, current snapshots, and lossless IDs.
- [Phase-1 timers can reset across lifecycle boundaries] → Default-off test context and explicit documentation; no persistent-world readiness claim until phase 7.
- [Receiver bound becomes conservative over time] → This only increases candidates; measure if needed later. Do not introduce a second spatial index for a small range prematurely.

## Migration Plan

1. Implement and validate definitions, runtime, protocol, and client together; regenerate protocol artifacts and publish the action-animation catalog through the existing asset pipeline.
2. Run focused server/client regressions and the scripted two-client range demonstration with the default-off setting enabled only in a disposable test environment.
3. Record completed checks, exact setup commands, and remaining phase boundaries in a range guide. Exercise existing save/load for stamina and item Quality; do not imply test HP persistence.
4. Keep production configuration disabled. Rollback consists of disabling the test setting and deploying the prior paired server/client/catalog; no DB rollback is needed. Clean up transient fixtures before switching builds.
5. When implementation is accepted, sync/archive through the project's OpenSpec workflow. Later proposals attach world-object and creature receivers to this path and own their migrations.
