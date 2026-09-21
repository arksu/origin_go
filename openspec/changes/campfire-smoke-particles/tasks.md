# Tasks

## 1. Assets and constants

- [ ] 1.1 Add smoke puff texture asset at `web_new/public/assets/game/fx/smoke_puff.png` (small soft puff, flame-style palette-compatible) and confirm it loads via `ResourceLoader` path `fx/smoke_puff.png`
- [ ] 1.2 Create `web_new/src/constants/fx.ts` exporting `FX_WIND = { x: 14, y: 0 }` with a comment noting the emitter API receives the vector per update so future dynamism touches only this file; verify `npm run type-check` passes

## 2. Particle engine (fx layer)

- [ ] 2.1 Implement `web_new/src/game/fx/ParticlePool.ts` (pooled particle records + sprite reuse, single acquire/release path) and verify pool reuse with a temporary console assertion or unit test in `web_new/tests`
- [ ] 2.2 Implement `web_new/src/game/fx/ParticleEmitter.ts`: config-driven spawning (rate, spread, maxParticles cap), per-particle integration (speed/angle, buoyancy, drag, wind vector applied per update), over-life interpolation (`scale`, `alpha` with fadeIn/fadeAway, `tint`, `rotation`/`spin`), sine-wander turbulence, satisfying the FxManager contract `update(deltaMs): boolean` returning `false` only after stop + last particle death; verify `npm run type-check` passes
- [ ] 2.3 Implement emitter `stop()` semantics (stop spawning, finish live particles, self-remove) and `linger` behavior (reparent to scene and fade out when released by a destroyed view, default false); verify by manually stopping an emitter in a dev-session debug hook

## 3. FxManager integration

- [ ] 3.1 Extend `FxManager` with attach/detach: `attach(config, ownerContainer, zIndex)` resolves preset → engine config, creates the emitter's child container, randomizes start phase, registers in `activeFx`; `detach` stops/removes; verify attach + detach of a debug emitter leaves no orphan sprites in the scene graph
- [ ] 3.2 Add culling gate: emitter skips spawn and update while its owner container has `visible === false`, resumes when visible; verify by panning the viewport away from a burning campfire (CPU work stops — observable via a breakpoint or perf counter) and back

## 4. Smoke preset

- [ ] 4.1 Implement `web_new/src/game/fx/presets/smoke.ts` expanding `{ density, riseSpeed, tint, puffSize, sway, windResponse }` plus texture/zIndex/linger into a full engine config with the soft fade-in/fade-away alpha envelope; reject unknown params (fail fast); verify `npm run type-check` passes
- [ ] 4.2 Tune default smoke numbers (spawn point at flame top, density 1.0 baseline, tint ~0x9a9a9a) in-game against the existing flame animation and record final values in the preset file; verify visually in a dev session with a lit campfire

## 5. Declarative attachment

- [ ] 5.1 Extend the object-definition schema handling so a state may declare an `fx` entry (preset/texture/zIndex/linger/params) and update `scripts/validate-object-schema.mjs` to accept and validate it; verify `npm run validate-objects` passes with the new field and fails on a malformed `fx`
- [ ] 5.2 Wire `ObjectView`: on build, if the state definition declares `fx`, attach via FxManager into the view container with the configured zIndex; on `destroy()`, detach; verify `npm run type-check` passes
- [ ] 5.3 Add the `fx` entry to `campfire.burning` in `web_new/src/game/objects/structures.json`; verify in a dev session: lighting a campfire shows smoke above the flame, fuel-out flips to unlit and smoke vanishes, no smoke on unlit campfires

## 6. Regression and cleanup

- [ ] 6.1 Verify appearance-driven rebuild does not leak: light and extinguish a campfire repeatedly in a dev session and confirm no accumulating sprites/emitters (scene-graph count stable)
- [ ] 6.2 Run `npm run type-check`, `npm run lint`, `npm run validate-objects` and confirm all pass with no new findings attributable to this change
