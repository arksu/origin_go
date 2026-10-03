# Proposal

## Why

The approved combat design has no playable attack implementation yet. Phase 1 of `docs/features/combat_phases.md` establishes a complete, server-authoritative axe loop that can be exercised through the ordinary client against isolated test targets, providing the foundation for later world-object and creature damage.

## What Changes

- Add two data-defined axe actions using the existing stone axe: a 90-degree area attack and a stronger attack against the nearest collider intersection. Use the approved preset from `docs/features/combat_final.md` §17.1: base damage 6, multipliers 1/1.5, range 18, windup 600 ms, recovery 400 ms, independent cooldowns 2000 ms, and 60 stamina paid at accepted start.
- Add a minimal combat execution path: validate, pay once, lock direction, wind up, resolve one strike, recover. Movement is capped at Crawl through impact and follows normal rules during recovery. Voluntary cancellation, other gameplay actions, and combat-equipment changes cannot bypass the cycle; external interruption has an explicit entry point.
- Resolve actual collider/sector intersections at impact, exclude the attacker, ignore occlusion, and preserve fractional damage through a shared damage-receiver path. The armor formula accepts armor points now; test targets use zero armor.
- Expose direction selection, authoritative phase, hit/miss, target HP, stamina, and individual cooldown readiness through Actions/hotbar and existing rendering facilities. Publish stable execution/event identities and observer snapshots; presentation never authorizes damage.
- Add an opt-in, public-player test range with resettable server-owned HP, different collider sizes, and moving targets. Keep real player health and ordinary world-object damage disconnected until their respective phases.
- Revise existing action, map-click, movement, and animation contracts to distinguish committed combat from cancelable noncombat work. Keep existing noncombat behavior and catalog conventions.

## Capabilities

### New Capabilities

- `combat-actions`: Data-defined axe execution, directed input, costs, independent timers, action/equipment exclusion, interruption, combat events, and authoritative client presentation.
- `combat-hit-resolution`: Sector/collider targeting, deterministic target choice, shared fractional damage calculation/application, and the isolated test range used to verify them.

### Modified Capabilities

- `game-actions`: Add direction targeting and combat execution policy; qualify cancellation, payment, revalidation, repeatability, input, and HUD requirements without changing existing noncombat semantics.
- `map-click-input`: Route directed attack selection and committed-cycle clicks without triggering cancellation, pickup, linking, or equipment-drop bypasses.
- `directional-movement`: Apply the axe's temporary Crawl cap while preserving input validation, hold expiry, normal restrictions, and independent attack direction.
- `character-action-animations`: Support a combat execution source and locked-direction facing on the authoritative timeline, independent of the strike deadline.

## Impact

Affected areas include action/item definitions and loaders, ECS action and movement state, shard system ordering, command dispatch, inventory mutation validation, public visibility snapshots, protobuf messages and generated bindings, Actions/hotbar/input state, actor presentation, and focused Go/client tests. Reuse `stone_axe`, existing hand slots and clips, stamina/regen helpers, and the existing slash-command mechanism; no new runtime dependency is planned.

Dependencies: none of the later combat phases. This change creates planning for phase 1 only.

Demonstration: two clients in a disposable test world, one equipping a Quality-10 stone axe at STR 1 and repeatedly hitting/resetting stationary and moving targets; both clients observe the same attack phase and results. AoE deals 6 HP and the single-target action deals 9 HP at armor 0.

Out of scope: persisted world-object HP/destruction, PvP/SHP/HHP/KO redesign, equipment armor, wounds/status runtime, bows/projectiles, NPC controllers, delayed logout, and persistence/transfer/restart recovery of combat timers. Cooldowns and `LastCombatEventAt` are explicitly runtime-only in this phase. This is a test-world milestone, not combat v0 readiness for a persistent world.
