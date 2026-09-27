# Implementation evidence — 2026-09-27

All 21 tasks are implemented. No commit or deployment was performed. Existing
source models, recipes, Mixamo files and standalone animation artifacts were
preserved. Comparing the old and new public catalogs confirmed all three asset
entries are identical; only the optional action projection reference was added.
Production projection SHA-256:
`195cfdd579751ba9840da422898e8abbef4a6c9d92c6071f58057fa53ee85afe`.

## Automated checks

Passed:

- `make proto`; `npm --prefix web_new run proto` (also regenerated during build).
- `go test ./internal/actionanimationdefs ./internal/cyclicaction ./internal/charactervisual ./internal/ecs/systems ./internal/game ./internal/game/behaviors ./internal/game/events ./internal/network ./cmd/gameserver`.
- `npm --prefix web_new run test:action-animations` — 7 groups covering lossless
  protobuf round trips, malformed state, phase/rates/endpoint, actual Three clip
  sampling, store/dispatcher/handler ordering, generic selection and async view
  lifetime. A 3s clip and a 1s clip both cover the complete 2s authoritative cycle.
- `npm --prefix web_new run test:character-visual` — 21 existing character,
  equipment, movement, async asset ownership and water tests.
- `npm --prefix web_new run test:actions`; `test:chunks` — 9 groups;
  `test:asset-pipeline` — 23 groups.
- `npm --prefix web_new run type-check`; `npm --prefix web_new run build`.
- `npm --prefix tools/asset_pipeline test` — 78 tests including the shared
  positive/negative def fixtures, publication failure atomicity, partial catalog
  validation, CLI and existing publishing/locking checks.
- `npm --prefix tools/asset_pipeline run type-check`.
- `python3 tools/tests/roundtrip_test.py --regression` — 1899 extracted sprites
  match source; `node tools/tests/pixi_semantics_test.mjs` passes;
  `python3 tools/tests/render_sim.py` — zero differing pixels.
- `git diff --check`.

The client build retains existing dependency warnings about protobuf eval,
mixed static/dynamic imports and large chunks. No atlas, source asset or gameplay
result/progress/sound assertion was changed to make checks pass.

## Real-asset browser checks

The Codex in-app browser ran `/tests/hybrid-integration.html` to
`ALL CHECKS PASSED`, including the existing character/equipment/movement/water
regressions and the new catalog-driven action harness:

- Every initial equipment variant, eight directions, 33 normalized samples over
  the full clip including its endpoint; no silhouette touches an output edge.
- Ground origin, actual texture dimensions, opaque/transparent picking and
  allocated output byte accounting.
- Outgoing bounds retained through blend-out; cancellation restores 128² output.
- Actual WebGL context loss/restoration during an expanded action frame restores
  identical pixels and keeps `glError = 0` on later frame resizing.

The check exposed a stale Three render-target disposal listener after context
loss. Disposing shared pass resources while the context is lost, then recreating
them after restoration, fixed the invalid GL handle on the next size change.
The final browser run includes that regression.

Temporarily copying `tests/fixtures/action_animations/browser-extra.json` into
the canonical def directory and running `tools/assets publish-action-animations`
added a second distinct source/clip binding and a 160×192 frame, alongside the
192×224 production frame. No executable preview/render/protocol change was made.
The identical integration harness passed both definitions. The preview enumerated
the extra choice, used its 1300ms duration, played it for eight actors and reported
GL error zero. The retained production animation was then selected and visually
inspected over the dark green backdrop in eight directions.
The temporary canonical def and projection were removed, and production defs
were republished. The additional fixture remains test data only.

## Live two-client session

Built the modified gameserver and ran it at `127.0.0.1:8081` against a new local
Postgres database `origin_animation_review_20260927`. Applied `migrations/schema.sql`
and seeded 8×8 chunks on two layers with 128² grass tile payloads. The existing
server on port 8080 and its database were not restarted or modified.

`node web_new/scripts/verify-action-animation-session.mjs` opened two real
WebSocket clients through the real registration, character-enter and auth paths.
It used existing inventory, context actions, visibility and admin transfer APIs.
The production action and equipment were chosen from data fixtures/defs, not
from a synchronization-code branch. Results:

| Scenario | Observed result |
| --- | --- |
| Late visibility | Spawn snapshot at elapsed tick 6 of 20, 100ms/tick, 2000ms cycle; reconstructed phase difference 0 at a shared clock |
| Cancellation via ordinary movement/link break | Both clients received the new idle revision with their own current stream epochs |
| Simultaneous visibility | Both clients received byte-equivalent decoded authoritative action state |
| Equipment switch during action | Observer received the new character equipment snapshot |
| Repeated cycle | Only the confirmed successor published a new revision, elapsed counter reset to zero |
| Exit/re-entry | Fresh appearance snapshot restored revision 4 at elapsed tick 6 |
| Target destruction | Active performer received idle after the test target was destroyed |
| Transfer to layer 1 and back | Incarnation changed; the old observer received despawn; both new spawns were idle |

This is a live server/protobuf session with two scripted WebSocket clients,
combined with the separate real-asset browser renderer checks. It is not a
populated production-world or mobile performance test. The temporary server was
stopped and its test database removed after verification.

To reproduce, use a fresh disposable database with that schema, seed grass chunks
as above, and start the built server with `SERVER_HOST=127.0.0.1`, `SERVER_PORT=8081`,
`DATABASE_DATABASE=<isolated database>`, `GAME_WORLD_WIDTH_CHUNKS=8`,
`GAME_WORLD_HEIGHT_CHUNKS=8`, both `GAME_WORLD_MIN_*_CHUNKS=0`,
`GAME_WORLD_MARGIN_TILES=2`, and `GAME_MAX_LAYERS=2`. Run the session script once the
server is ready. The script intentionally refuses other hosts/ports and creates
only disposable review accounts/objects. It waits for the test target's normal
growth before exercising repeated cycles.

## Generic-path audit

No concrete action/clip/tool literal or specialized identifier remains in the
new synchronization types/decoder, lifecycle/snapshot/dirty sender, actor action
player/sampler, frame renderer, or migrated action preview/harness. Existing
provider edits mechanically delegate start/continue/clear using their existing
source identities. A search of production `ActiveCyclicAction` removals finds only
the common helper; every production start is routed through it. The existing
unrelated equipment integration test still contains its original equipment
fixture. Existing gameplay effect implementations were preserved.

The synthetic shared fixtures exercise two different source/clip mappings,
reversed variant order, different frame/blend settings and the same compiled
runtime/protobuf schema. No impact marker, sound adjustment or source trimming
was introduced. Exact phase before initial time calibration and during server
stalls remains subject to the documented extrapolation limits.
