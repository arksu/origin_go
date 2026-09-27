# Shared character action animations

Timed actions expose presentation independently of owner progress UI and gameplay
effects. The server, protocol and actor do not recognize concrete action, clip or
tool names. Canonical policy lives in `data/action_animations/*.json`; asset
recipes/manifests own the referenced clips and equipment attachments.

## Add a binding

1. Choose an existing timed execution source: `context` uses behavior namespace
   and action ID; `menu`, `craft` and `build` use their definition ID/key and an
   empty namespace. All production cycle creation sites use `cyclicaction.Start`
   or `StartContext`. Untimed actions need separate gameplay work.
2. Add a version 1 binding following `data/action_animations/README.md`. Declare
   actor, ordered variants/equipment predicates, eligibility, facing, blend and
   frame. Optional `unbind_equipment_slots` visually detaches the listed slot models
   while an action layer is displayed, including its blend-out and terminal hold.
   Optional preview metadata supplies label, equipment and example cycle
   duration; it never controls server timing.
3. Run `tools/assets publish-action-animations`. This uses existing immutable
   assets, validates the merged catalog and atomically publishes a client
   projection without Blender. Full and partial builds validate the same
   projection before replacing the catalog. Invalid publication leaves the old
   catalog usable. A source selector is not included in the client projection.
4. Restart the server with the updated defs and reload clients with the matching
   published catalog. The registry and each client catalog snapshot are immutable
   for that process/session. No protocol or runtime edit is needed for another
   supported timed source.

Prefer variant order explicitly; the runtime has no preferred hand. An empty
variant equipment array is unrestricted. Listed predicates must all hold, and
required equipment must have finished loading. Unknown bindings or missing
required facing context retain authoritative state, report once and use ordinary
presentation. Broken assets for a known binding fail validation explicitly.

## Timing and lifecycle

`CharacterActionAnimationState` carries generation, uint64 revision (kept as a
lossless string in TypeScript), binding key, actual elapsed/total ticks, fractional
milliseconds per tick, the sampling timestamp and an optional target position.
An empty key means idle; idle retains the last revision. The incarnation matches
the character visual generation. `S2C_CharacterActionAnimation` adds entity ID
and the recipient's current stream epoch. Player spawn/appearance snapshots
include the same state. Other messages and owner progress fields are unchanged.

```text
durationMs = totalTicks * tickDurationMs
elapsedMs = elapsedTicks * tickDurationMs + estimatedServerNowMs - serverTimeMs
phase = clamp(elapsedMs / durationMs, 0, 1)
clipTimeSeconds = phase * loadedClip.duration
```

A three-second source in a two-second cycle runs at 1.5x; a one-second source runs
at 0.5x. Render FPS, distance interpolation and visual blend time do not advance
the action clock. `baked8` quantizes displayed samples but preserves phase one.
The endpoint holds until a confirmed successor or stop; only the local preview
loops. TimeSync corrections and fresh consistent same-revision snapshots correct
phase without restarting blending. Before time calibration and during server
stalls, extrapolation is provisional; exact real-time agreement is not guaranteed.

The common lifecycle installs mapped state at start, publishes successors only
after gameplay resets the cycle, and clears on completion/removal. Unmapped
replacement also clears presentation. Ordinary elapsed-tick increments neither
increment revision nor dirty the queue. Transition delivery runs after cyclic
work, visibility and death cleanup, targeting the performer and current visible
observers once each. Spawn capture and enqueue remain under the world's read lock.
Critical queue overflow disconnects only the slow recipient asynchronously;
ordinary noncritical send behavior is unchanged.

Client handlers reject wrong epochs/incarnations, unknown entities, lower
revisions, stale samples and contradictory equal-revision timing. Equipment and
animation revisions are compared independently during appearance refresh. State
lives in the canonical Pinia entity and latest render-side object. Despawn,
leave-world and reconnect discard it. Async actor/equipment completion cannot
replay an old network snapshot.

Movement, carry, equipment readiness and def predicates may suppress presentation
while retaining the authoritative cycle. Resumption uses current phase. KO is a
global terminal pose priority. No source clip trimming, impact markers, sound
rescheduling or visible-impact/result alignment is performed.

## Rendering and review

Frame dimensions/origin extend the orthographic view at the existing native
pixel density and ground anchor. Bounds include incoming/outgoing frames until
blend-out finishes, plus the last rendered texture while throttled. Picking and
pool accounting use actual dimensions. Output slots are limited to 128 and
allocated RGBA output bytes to 128 MiB (separate from the asset cache estimate).
Context restoration recreates shared render targets and preserves the current
phase/frame. GPU resources associated with a lost context are disposed before
Three initializes its replacement maps.

`/tests/hybrid-character.html` populates binding/variant choices from the catalog.
Its local duration, loop checkbox and normalized phase slider use the same actor
API. A second def needs no executable preview change. `/tests/hybrid-integration.html`
checks real assets, every declared variant in eight directions, complete clip
samples, alpha edges, picking, frame return and context restoration.

Run focused checks:

```sh
go test ./internal/actionanimationdefs ./internal/cyclicaction ./internal/charactervisual ./internal/ecs/systems ./internal/game ./internal/game/behaviors ./internal/game/events ./internal/network
npm --prefix web_new run test:action-animations
npm --prefix web_new run test:character-visual
npm --prefix web_new run test:actions
npm --prefix web_new run test:chunks
npm --prefix web_new run test:asset-pipeline
npm --prefix web_new run build
npm --prefix tools/asset_pipeline test
npm --prefix tools/asset_pipeline run type-check
```

Shared synthetic defs and negative cases are in
`tests/fixtures/action_animations/`. `browser-extra.json` is an optional second
real-asset def for temporary publication during browser acceptance; it is not a
production binding. Live acceptance uses `session.json` and
`node web_new/scripts/verify-action-animation-session.mjs` against a disposable
local server on port 8081 only. It creates test accounts/objects and uses admin
commands in that isolated world. See the change's `evidence.md` for actual runs.

## Compatibility and rollout

Deploy matching server defs, additive protobuf/server code, client bundle and
published catalog together. Old clients ignore the extra protobuf fields. A new
client with an old server treats missing spawn animation state as ordinary idle;
a missing optional catalog projection also leaves normal presentation available.
Unknown future binding keys fall back without changing gameplay.

For rollback, restore the previous server/client synchronization code and generated
protocol bindings together, or remove the binding from defs and republish/restart
to disable presentation. Preserve the authored animation sources and immutable
clip assets. Never replace hashed artifacts in place or restore specialized
per-action APIs as an extension mechanism.

## Equipment unbind verification

Shared definition fixtures cover omission, empty lists, hand and non-hand slots,
and invalid slot lists in both Go and TypeScript. Actor tests cover object/lease
preservation, selection against actual equipment, repeats, overlapping blends,
asynchronous replacement, cancellation, carry, knockout, LOD and destruction.

For a published binding with unrestricted variants and preview equipment wholly
covered by its unbind list, open `/tests/equipment-unbind.html?binding=<key>`.
The test supports both facing policies and reads the chosen clip and equipment
from the catalog. For a review with rigid and skinned equipment together,
temporarily publish `tests/fixtures/action_animations/browser-extra.json` alongside the local defs,
then open `/tests/equipment-unbind.html`. It compares an equipped actor with a
bare reference at the same action phase in all eight directions, covering rigid
attachments and a skinned garment, render throttling, picking and context
restoration. Restore the previous catalog and remove the temporary def after
review. The generic runtime never reads the fixture directly.
