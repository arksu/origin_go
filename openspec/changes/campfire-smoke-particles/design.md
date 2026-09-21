# Design

## Context

The web client (`web_new`, Pixi 8.16 + Vue) renders world objects as data-driven `ObjectView`s built from `web_new/src/game/objects/structures.json`, keyed by `<def>/<state>`. A campfire's burning state already flows end-to-end with no gaps: the server flips `Appearance.Resource` between `campfire/unlit` and `campfire/burning` and rebroadcasts `S2C_ObjectSpawn`; the client's `objectSpawn` handler despawns and rebuilds the view (`ObjectManager.spawnObject`), so the `burning` state's 7-frame flame layer appears and disappears automatically.

The pieces this design builds on:

- `FxManager` (`web_new/src/game/fx/FxManager.ts`): singleton on `Ticker.shared`, holds `activeFx` entries with the contract `update(delta): boolean` — return `false` to self-remove. Currently only one-shot effects.
- `ObjectView.destroy()` (`ObjectView.ts:1107`) is the single teardown point on every rebuild/unload.
- Viewport culling sets `container.visible = false` on culled objects (`culling/ViewportCullingController.ts:218`), a ready-made gate for simulation skipping.
- Layers carry per-frame offsets, `fps`, `loop`, `z`; looping layers randomize their start frame (`ObjectView.ts:359`), the established desync mechanism.

Motivation is in proposal.md; behavior requirements are in the `client-particle-fx` spec delta.

## Goals / Non-Goals

**Goals:**

- A small generic particle engine (emitter + pooled particles) living inside `fx/`, reusable by any future effect.
- A smoke preset exposing domain knobs (density, rise speed, tint, puff size, sway, wind response) so smoke variants are pure data.
- Declarative attachment: an `fx` field on object state definitions; campfire `burning` is the first consumer.
- Deterministic cleanup: emitters die with their object view; culled views don't simulate.

**Non-Goals:**

- Pixi `ParticleContainer` adoption (deferred until particle counts justify it).
- Server changes or new network packets — the appearance resource is sufficient.
- Dynamic/server-synced wind, gusts, or per-tile wind fields.
- Sound, lighting, or collision interaction for particles.
- A general curve/gradient editor or library-grade feature set (Shuriken parity).

## Decisions

### D1: Custom minimal engine, not `@pixi/particle-emitter`

The Pixi ecosystem library works on v8 but is a heavyweight dependency with config-format churn (v2 vs v5 formats are a known migration pain), and its feature surface far exceeds one smoke family. A ~200-line emitter matching the existing `update(delta): boolean` FxManager contract is simpler and fully owned. *Alternatives*: `@pixi/particle-emitter` (rejected: dep weight, API churn, harder to gate culling per owner); per-effect bespoke code (rejected: exactly the duplication this change exists to prevent).

### D2: Three layers, one responsibility each

1. **Engine** — `fx/ParticleEmitter.ts`, `fx/ParticlePool.ts`: config-driven, zero smoke knowledge. Owns spawn timing, per-particle integration, over-life interpolation, pooling, and the FxManager lifecycle contract.
2. **Smoke preset** — `fx/presets/smoke.ts`: expands domain knobs into an engine config. Encodes the smoke-specific envelope (soft fade-in, soft fade-out) and cheap sine-wander turbulence with a per-particle phase.
3. **Attachment** — ObjectView reads an `fx` field from the state definition and hands it to FxManager; FxManager resolves preset → config → emitter inside a child container of the view.

### D3: Attachment lives in `structures.json` next to `layers`

The `fx` field sits on the state definition, symmetric with the existing `layers`/`shadow`/`interactive` schema — one place holds the full visual definition of a state, and no object-type-specific code exists anywhere. *Alternative*: a separate `resourcePath → fx` registry file (rejected: splits the definition of one state across two files).

### D4: Lifecycle via the existing FxManager contract

An emitter is an `activeFx` entry. `stop()` stops spawning and lets live particles finish before self-removal (the same semantic Factorio gives smoke). The `linger` flag on the effect definition decides what happens when the owning view is destroyed: default `false` — emitter and particles vanish with the view (a destroyed sprite can't render anyway); `true` — the emitter is reparented to the scene and fades out its live particles. ObjectView `destroy()` calls FxManager detach for any emitter it owns.

### D5: Wind is a static constant

`web_new/src/constants/fx.ts` exports `FX_WIND = { x: 14, y: 0 }` (px/s, left-to-right) following the per-feature constants convention (`chat.ts`, `moveMarker.ts`). Variation comes from the preset's per-particle `windResponse: [0.6, 1.4]`, so wind itself stays a single number while puffs drift 8–20 px/s, further damped by drag. The engine receives the vector at update time, so later making wind dynamic touches only the constant file. *Alternatives considered*: per-effect wind multiplier baked into configs (rejected: one global knob is the point), time-varying noise wind (non-goal for now).

### D6: Rendering and desync

Particles render as pooled `Sprite`s inside a child container added to the object's `Container` with a `zIndex` above the flame layer, keeping the campfire's existing y-sort placement intact. Each emitter starts with a randomized spawn-phase offset (the same spirit as the looping-layer desync) so multiple burning campfires don't pulse in lockstep.

### D7: Simulation gating

The emitter checks its owner container's `visible` flag each tick (culling already maintains it) and skips both spawning and updating when culled. Without this, every burning campfire on the shard burns CPU while off-screen — unlike frame animations, emitters run on the shared ticker outside `ObjectManager.update()`.

### D8: Puff texture via the existing asset pipeline

One small soft puff PNG loaded through `ResourceLoader` (`fx/smoke_puff.png`, same path family as the existing `fx/stick.svg`), restylable by an artist later. *Alternative*: runtime canvas-generated radial gradient (rejected: off-style with the pixel-art pipeline and less tweakable).

### Effect config schema

Ranges are `[min, max]` (Phaser-style EmitterOp convention); over-life properties use `{start, end}`; alpha uses Factorio-style fade-in/fade-away semantics.

```jsonc
// inside structures.json, on the burning state:
"fx": {
  "preset": "smoke",
  "texture": "fx/smoke_puff.png",
  "zIndex": 2,
  "linger": false,
  "params": {                       // preset knobs → expanded to engine config
    "density": 1.0,
    "riseSpeed": 1.0,
    "tint": 0x9a9a9a,
    "puffSize": 1.0,
    "sway": 1.0,
    "windResponse": [0.6, 1.4]
  }
}
```

Engine config (produced by the preset, never hand-written in defs) covers: spawn position/shape + spread, emission rate, `maxParticles` cap, particle `lifetime` range, `speed` + `angle` ranges, buoyancy acceleration, drag, wind response, `scale {start, end}`, `alpha {start, peak, end, fadeInMs}`, `tint` (single or random pick), `rotation`/`spin` ranges.

## Risks / Trade-offs

- **Visual tuning is empirical** → the preset's default numbers (density, rise, sway) need an in-game eyeball pass during apply; schema is fixed, numbers are tweakable constants in one file.
- **Overdraw cost with many visible fires** (alpha-blended sprites) → per-emitter `maxParticles` cap, culling gate, and the deferred `ParticleContainer` upgrade path if measurement demands it.
- **Pool bugs leak or flicker sprites** → pool is internal to the engine, single acquire/release path, release on particle death and emitter teardown only.
- **State-swap rebuild churn** (view rebuilt on every appearance change) is pre-existing behavior; emitters are rebuilt with the view and start-phase randomization makes the rebuild visually seamless rather than a visible restart.
- **FxManager grows responsibilities** (one-shot + attached emitters) → attach/detach is a thin registry over the same `activeFx` set; no second ticker, no second loop.

## Migration Plan

Purely additive client change; no data migration, no server deploy. Rollback is removing the `fx` entry from `campfire.burning` in `structures.json` — the engine stays inert without consumers.

## Open Questions

None blocking. The concrete default smoke numbers are tuned during apply (see Risks); that cannot change the specs, the approach, or the task breakdown.
