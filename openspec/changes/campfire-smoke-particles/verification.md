# Verification — 2026-09-22

- `npm run type-check`: passed. The user approved four pre-existing Spine 4.3 compatibility fixes: `AnimationState.getCurrent(0)` → `getTrack(0)` (three calls), and `Skeleton.setSkinByName(name)` → `setSkin(name)` (one call).
- `npm run validate-objects`: passed for all seven object-definition files. Temporary fixture runs also confirmed the CLI rejects an unknown smoke parameter and a non-boolean `linger` value.
- ESLint on every changed JavaScript/TypeScript file: passed.
- `npm run lint`: still fails with 413 pre-existing errors. Comparison against original source files found zero new findings and removed one `no-explicit-any` error from `FxManager`. No repository-wide lint cleanup was included.
- `openspec validate campfire-smoke-particles --strict`: passed.
- `git diff --check`: passed.

## Browser verification

Open `/tests/campfire-smoke.html` on the web client Vite server. This harness uses the actual Pixi renderer, ResourceLoader, ObjectView, ObjectManager, viewport culling controller, and FxManager shared ticker.

All 34 assertions passed, covering asset loading and alpha, pool reuse and double-release protection, malformed definitions and offsets, per-effect source offsets, particle caps, paused/resumed simulation, changing wind, alpha envelopes, stop completion, linger reparenting with preserved transforms, destruction while texture loading, burning/unlit rebuilds, 20 repeated appearance cycles, and world cleanup.

Manual checks confirmed that stop finishes the live particles and that releasing an owner with `linger` preserves the tail until the emitter count returns from two to one. Smoke now defaults to `linger: true`: when a campfire or future kiln changes to an unlit state, spawning stops immediately while its current puffs drift and fade out. An individual effect may still explicitly set `linger: false` for instant removal. The preview shows two independently phased burning campfires and one unlit campfire. Transparent edges were inspected over dark green; no rectangular matte or checkerboard remnants were visible. The spawn point was lowered from `(0, -54)` to `(0, -42)` after observing a gap above the animated flame.

Appearance changes are replayed through the production renderer in this harness. Actual fuel depletion on a live authenticated game-server session has not been exercised. Acceptance of the existing lint baseline remains pending the user's verification-scope decision.

## Asset provenance

The built-in imagegen tool generated `web_new/public/assets/game/fx/smoke_puff.png`. Its transparent output was resized to 64×64 with alpha preserved. The final generation prompt was:

> Use case: stylized-concept. Asset type: small game smoke particle PNG sprite for a medieval isometric pixel-art game. Generate one single soft irregular round smoke puff, isolated and centered, with a genuinely transparent background and generous transparent padding. Neutral near-white grayscale so engine tinting to gray works. Subtle clustered pixel-art shading, soft translucent edge, simple readable shape at 48x48 pixels, no hard outline, no fire, no trail, no ground, no shadow, no text. Any directional shading must be lit from upper-left only. This is a texture repeated as small puffs, not a whole plume. Output a compact square image ideally 64x64 pixels.
