# Proposal

## Why

Burning campfires render a static flame animation with no smoke, so fires feel inert and players get no ambient cue about which fires are lit at a distance. The client also has no reusable particle infrastructure — `FxManager` only plays one-shot animations — so every future visual effect (denser smoke variants, sparks, dust) would need bespoke code. Building a data-driven particle system now gives the campfire smoke today and a config-only path for future effects.

## What Changes

- Add a generic, config-driven particle emitter engine (`ParticleEmitter` + pooled particles) to the web client, integrated with the existing `FxManager` ticker and lifecycle contract.
- Add a smoke-specific preset layer that expands domain knobs (density, rise speed, tint, puff size, sway, wind response) into engine configs.
- Extend client object definitions (`structures.json`) so an object state can declaratively attach an fx emitter (first used by `campfire` → `burning`), attached on view build and detached on view destroy.
- Add a static ambient wind constant (left-to-right, +14 px/s) consumed by particle updates.
- Smoke appears only while the campfire's appearance resource is `campfire/burning`; it requires no server changes and no new network packets.

## Capabilities

### New Capabilities

- `client-particle-fx`: The web client renders procedural particle effects attached to world objects, driven by declarative per-state fx definitions; campfire smoke while burning is the first effect.

### Modified Capabilities

- None. `fuel-burner` covers server-side burning mechanics only; smoke is client-visual and driven by the already-broadcast appearance resource.

## Impact

- **Client code (`web_new/src/game/fx/`)**: new `ParticleEmitter`, `ParticlePool`, smoke preset; `FxManager` gains emitter attach/detach handling. `ObjectView` reads the new `fx` field and wires attach/detach. `ObjectManager`/`Render` pass-throughs unchanged.
- **Client data**: `web_new/src/game/objects/structures.json` gains an `fx` entry on `campfire.burning`; new constants file `web_new/src/constants/fx.ts` (wind); one smoke puff texture asset under `public/assets/game/fx/`.
- **Server**: none. The `campfire/unlit` ↔ `campfire/burning` appearance broadcast already exists.
- **Performance**: emitters skip spawning and updating for culled objects (owner container `visible === false`); particle counts capped per emitter. `ParticleContainer` upgrade deliberately deferred.
