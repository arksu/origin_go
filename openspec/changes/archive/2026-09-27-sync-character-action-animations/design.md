# Design

## Context

See [proposal.md](proposal.md) for motivation and [the capability delta](specs/character-action-animations/spec.md) for the behavior contract.

The authored Mixamo source and hand-specific clip declarations remain available. The client/preview still has specialized remnants that must be replaced by generic playback and frame handling; this plan does not assume those remnants constitute a working common API. The previous experimental server/protocol implementation was removed.

Relevant existing seams:

- Context behavior executors, `ActionService`, `CraftingService`, and build progress create `ActiveCyclicAction`. `CyclicActionSystem` increments `CycleElapsedTicks`, completes or cancels through `ContextActionService`, and resets elapsed ticks when a repeated cycle starts. Some actions can start after the cyclic system has run in that world update.
- `Shard.SendCyclicActionProgress` and `SendCyclicActionFinished` send only to the performer. Client handlers use them for the progress UI. `ActionService` selection/approach state is also performer UI rather than a public animation contract.
- `TimeState.TickPeriod` provides the actual fixed-step duration and `UnixMs` anchors network time. An action created before the cyclic system can already have one elapsed tick by the end of that same world update. `Tick - StartedTick` is therefore not interchangeable with `CycleElapsedTicks`.
- `CharacterVisualSystem` already drains a deduplicated dirty queue and broadcasts through `VisibilityState.ObserversByVisibleTarget`. Character visual snapshots use a `layer:handle` incarnation string and uint64 revisions.
- `NetworkVisibilityDispatcher.handleEntitySpawn` builds and enqueues snapshots under the same world read lock. This prevents an ECS action transition from interleaving between snapshot capture and enqueue. Appearance-refresh spawns follow the same pattern.
- `Client.Send` drops messages on a full outbound queue. `SendChunkVisibility` already has a nonblocking enqueue-or-close policy suitable for critical state transitions.
- The client follows network -> plain `gameStore` state -> `GameFacade` -> `Render` -> `ObjectManager`/`ObjectView` -> `ActorInstance`. Actors and equipment load asynchronously. `TimeSync` estimates server time; movement independently uses an interpolation delay.
- `data/` contains versioned action, item, object, craft, and build definitions. `tools/asset_pipeline` already publishes immutable manifests and replaces `/assets/game/asset-catalog.json` atomically; `ActorAssetCatalog.ts` validates the catalog and loads referenced resources.

## Goals / Non-Goals

**Goals:**

- Define one extensible public presentation contract for supported timed action cycles, independent of the Actions menu, context-action implementation, and clip file names.
- Make defs the sole authored source of every concrete action/animation binding and its presentation rules. Adding a binding for a supported execution source must not require a new runtime branch or lifecycle hook.
- Make authoritative tick progress the timing reference, fit a complete loaded clip to each cycle, and recover phase after delayed delivery, loading, or culling.
- Reuse existing lifecycle, visibility, incarnation, and asset mechanisms with explicit cleanup and bounded server work.

**Non-Goals:**

- No impact markers, impact-frame timing, sound rescheduling, gameplay completion changes, client-driven effects, or action prediction.
- No new animation assets, and no bindings beyond the single first production binding. Other timed sources that already route through the shared cycle lifecycle — for example crafting, building, or gathering actions such as chipping stone or taking branches — gain public presentation later purely through def entries. The contract describes one timed cycle; repeating gameplay emits consecutive cycles. Untimed actions can be designed later.
- No changes to the movement interpolation algorithm, database schema, gameplay action semantics, server tick scheduling, or source animation geometry. Existing clips remain available; preview controls migrate to the common catalog and API.
- No rewrite of pre-existing action-specific gameplay effects. The prohibition on concrete action/clip identifiers applies to the synchronization mechanism and the client/preview presentation being migrated, including its code symbols and configuration constants. Existing gameplay providers merely delegate cycle lifecycle to shared helpers.

## Decisions

### 1. Add a presentation contract alongside the existing progress UI

Add `CharacterActionAnimationState`, a server update envelope `S2C_CharacterActionAnimation`, and an optional state field on `S2C_ObjectSpawn`. Use new protobuf field numbers without changing existing fields. The update envelope contains `entity_id`, `stream_epoch`, and `state`.

Proposed state fields:

| Field | Meaning |
| --- | --- |
| `generation` (string) | Same performer incarnation identity used by character visual snapshots |
| `revision` (uint64) | Monotonically increases within that incarnation on start, each next cycle, replacement, and stop; independent of equipment revision |
| `animation_key` (string) | Opaque def binding key; an empty key is explicit idle |
| `total_ticks` (uint32) | Positive authoritative duration for this cycle |
| `elapsed_ticks` (uint32) | Actual current `CycleElapsedTicks` at the sampling point, bounded by total ticks |
| `tick_duration_ms` (double) | Actual `TimeState.TickPeriod` converted to milliseconds, preserving fractional millisecond periods |
| `server_time_ms` (int64) | `TimeState.UnixMs` at the sampling point |
| `target_position` (optional position) | Generic world-space facing point; whether it is needed is declared by the binding's facing policy |

Idle states retain generation, revision, and server timestamp; active timing fields are not interpreted while idle. A never-animated character can use revision zero. Active states use a positive revision. No actor model, clip URL, bone transforms, inventory details, or new C2S animation request is transmitted.

Do not introduce protobuf fields, message routes, or enums for concrete actions, equipment, clips, handedness, or impact frames. A string binding key is resolved through loaded defs; it has no interpretation in packet routing or decoding.

This separates public animation data from private UI progress. Reusing progress messages would need a performer identity, public visibility semantics, incarnation guards, and spawn recovery, while risking updates to the observer's own progress bar. Putting animation state into equipment revisioning would couple two independent lifecycles. Both alternatives are rejected.

### 2. Author all bindings and presentation rules in one def catalog

Add versioned JSON definitions under `data/action_animations/`, following the existing strict def-loader conventions. Each file has a version and a list of bindings. An entry contains:

| Declaration | Generic interpretation |
| --- | --- |
| `key` | Opaque unique animation binding identity |
| `source` | Execution kind, optional namespace, and identifier; exact-match lookup with no wildcard or file-order precedence |
| `actor` | Actor asset identifier whose manifest supplies compatible clips |
| `variants` | Ordered clip references and declarative equipment predicates using slots and available visual keys |
| `eligibility` | Supported presentation predicates, initially stationary, not carrying, and not knocked out |
| `facing` | Preserve facing or use the cycle's target position |
| `blend_ms` | Finite nonnegative blend duration, separate from the authoritative cycle clock |
| `frame` | Positive width/height and origin/top padding, within generic renderer limits and preserving pixel density and ground anchor |
| `preview` | Optional display label, example equipment, and local cycle duration for the common preview controls |

Equipment keys, hand preference, source IDs, clip IDs, actor IDs, frame sizes, and eligibility combinations appear only in these defs or existing asset recipes/manifests. Variants are tried in declared order; a matching variant must have ready compatible equipment and clip resources. There is no hard-coded preferred hand. Unsupported predicates are errors, not executable expressions or scripts embedded in data. Use fixed generic predicate implementations rather than an expression engine.

The first production entry declares a preserved complete hand-specific clip pair, compatible equipment visual keys, preferred-hand-first variant ordering, stationary/non-carry/non-KO eligibility, target facing, and the enlarged frame. These are data choices, not reserved code values. No code symbol or runtime mapping contains a concrete action, equipment, or clip identifier. The source clip's loaded duration controls sampling; do not put a substitute duration or fixed frame count in the binding.

Build a server lookup keyed by normalized source kind/namespace/identifier. The generic source kinds correspond to existing context actions, menu actions, crafting, and building. Their existing executors provide the identity from the behavior/action definition, craft key, or build key when creating the cycle. The resolver does not infer kind by comparing against a concrete action name or parsing a binding key. A new binding for any of these already supported sources needs no new execution hook. First-version production data maps a single initially agreed source; a second fixture proves the general route.

Reject unsupported versions, unknown fields, duplicate keys, conflicting selectors, unknown predicate primitives, invalid numeric settings, invalid slot names, and malformed references with the source file and field identified. Load the server registry at startup rather than introduce hot reload. Publication validates actor IDs, clip existence and rig compatibility, equipment references, and frame limits against the effective merged asset catalog. Maintain shared positive/negative def fixtures for server and tooling validation; do not silently ignore broken entries.

Generate a client projection containing binding keys and presentation declarations from the same canonical defs. Extend the existing asset catalog with an optional immutable `actionAnimations` artifact reference and load/validate it through `ActorAssetCatalog.ts`. Install the projection and validate all referenced manifests before atomically replacing the public catalog. Def-only updates reuse already published assets and need no Blender re-export; partial asset builds validate against the merged catalog. An absent projection means no supported public binding, preserving ordinary presentation. Handwritten TypeScript tables and a separate authored client catalog are rejected because they duplicate action policy. A new protobuf configuration-delivery API is unnecessary when the existing asset publication path already serves immutable data.

Expose `tools/assets publish-action-animations` for def-only publication against the existing published manifest set, using the same publication lock and atomic replacement. Full and partial asset publication also regenerate/validate this projection so an asset change cannot invalidate a retained binding. Keep actor asset references and the projection in one effective catalog snapshot; failed publication never replaces the last usable snapshot.

### 3. Centralize cycle lifecycle and retain transient revision state

Keep public animation identity/revision in a transient ECS component that survives an idle transition but is destroyed with the character. `ActiveCyclicAction` remains the timing source; snapshot construction reads its current elapsed and total ticks. Idle retains the last revision so a delayed start cannot revive a canceled action. No persistence or transfer participant is introduced.

Introduce a shared cycle-start helper accepting the existing cycle payload and its generic execution-source descriptor. Route all production creation sites through it, including unmapped sources. It installs the gameplay cycle and resolves its def binding under the same world write lock; it must not alter duration, costs, validations, or completion handlers. This initializes public state even for a cycle created after `CyclicActionSystem`, before a visibility snapshot can observe it. Place the helper in a shared package that behavior contracts and game services can use without an import cycle.

Shared continuation publishes the next cycle after elapsed ticks reset; shared finish/clear removes the cycle's public state. Use the same removal helper at existing direct cycle-removal points in `ActionService`, `PlayerDeathSystem`, shard death cleanup, and player transfer. These helpers deduplicate redundant calls and mark a deduplicated animation dirty queue. Unmapped starts have no public animation, and replacing a mapped action with an unmapped one clears the prior state. Snapshot-facing target resolution uses generic cycle target metadata and the def policy; it has no action-specific coordinate rule.

Do not add a provider-specific animation-start callback or switch on an action ID. Existing provider edits are mechanical delegation to the common cycle lifecycle; once that route is installed, another def alone enables public presentation for the source. Existing gameplay behavior is outside the presentation registry.

Ordinary elapsed-tick increments do not increment revision or mark dirty. The snapshot reads fresh progress when needed. This avoids querying the whole world or broadcasting progress every server tick. Polling every `ActiveCyclicAction` and caching its signature is the rejected alternative: explicit lifecycle hooks are easier to audit, and align with existing dirty-queue rules.

### 4. Flush only changed state to current observers, and capture fresh spawn state

Register the dirty-queue sender after cyclic work, visibility, and death cleanup; a priority alongside `CharacterVisualSystem` is appropriate. Snapshot state under the world lock, then enqueue to the performer and current visible observers with deduplication when the performer is also in the observer set. Recipient validation includes a live world connection and its current stream epoch. A despawned handle is discarded; it must not address a newly reused handle.

Add the same snapshot builder to player object-spawn and appearance-refresh snapshots. Continue capturing and enqueueing under `Shard.WithWorldRead`, preserving the existing lock boundary. If an update was enqueued before the initial spawn, the later spawn reads at least that current state; if a transition occurs after capture, the queued spawn precedes that newer transition. Clients need no unbounded pre-spawn animation backlog.

Extract the existing nonblocking critical enqueue behavior behind a shared helper in `internal/network/server.go`, preserving the `SendChunkVisibility` wrapper and existing chunk behavior. Use it for action-animation updates and player spawn packets carrying animation state. On overflow, close only the affected connection asynchronously, so held shard/client locks cannot deadlock disconnect callbacks. Do not globally change ordinary `Client.Send` behavior. Silent dropping is rejected because a lost final idle state has no guaranteed later transition to repair it.

Repeated appearance spawns of the same incarnation must compare animation state independently of equipment revision. A snapshot with a newer animation revision applies even when the equipment snapshot is unchanged. A same-revision active snapshot with a fresher sampling timestamp corrects its clock without restarting blending.

### 5. Scale time by authoritative phase, not accumulated animation frames

For an accepted active snapshot, compute:

```text
actionDurationMs = totalTicks * tickDurationMs
elapsedAtSampleMs = elapsedTicks * tickDurationMs
cycleStartMs = serverTimeMs - elapsedAtSampleMs
phase = clamp((estimatedServerNowMs - cycleStartMs) / actionDurationMs, 0, 1)
clipSampleSeconds = phase * loadedClipDurationSeconds
speedMultiplier = loadedClipDurationSeconds / (actionDurationMs / 1000)
```

Use current `TimeSync.estimateServerNowMs()` for both performer and observers. Compute the estimate once per client render update and pass it into the action-animation path. Do not use receipt time or hard-code ten ticks per second. The source clip's actual loaded duration is authoritative for sampling; its original frames and recovery motion remain intact.

Sample time explicitly, rather than integrating `mixer.update(frameDelta * speed)`: explicit sampling cannot drift with frame rate, culling, or pauses. `hybrid3d` samples continuous phase; `baked8` quantizes visual samples while the underlying phase/duration remain unchanged. Preserve phase one as the terminal sample rather than mapping it back to zero. No modulo wrap is used for network-controlled cycles.

Re-evaluate phase when the server-clock estimate changes. Before the first valid pong, the existing time estimator provides a provisional phase; the first valid estimate corrects it without replaying the cycle or blend. Perfect phase accuracy before time calibration is not guaranteed. The movement interpolation clock remains unchanged; def eligibility can suppress the action while a walk finishes, then reveal its current phase without restarting it.

A newer cycle replaces timing and phase; the same animation binding does not re-trigger blend-in at every cycle boundary. At the inferred end, hold the terminal pose until the next server cycle or idle. Extrapolating motion past the published cycle is rejected because cancellation, exhaustion, or target destruction can end an action without another successful cycle.

### 6. Guard authoritative state before passing it to rendering

Add a plain client action-animation type/decoder. Validate generation format, exact uint64 revision representation, safe timestamp conversion, finite positive tick duration, nonzero active tick count, bounded elapsed ticks, and optional coordinate shape. Reuse/extract the existing generation and uint64 comparison helpers instead of coercing revisions through JavaScript `Number`.

Binding-specific validity is expressed through generic def policies, such as needing a target for target-facing presentation. Missing required presentation context suppresses that binding and reports a configuration/context error without inventing a target or changing gameplay. Packet structural validation must not know the first binding's name, clips, or equipment.

Apply only to known entities in the current stream and matching character incarnation. Higher revisions replace state. Lower revisions and older same-revision sampling timestamps are ignored. Equal revisions can refresh the timing anchor only when animation identity and cycle duration remain consistent; contradictory equal-revision payloads are rejected. An unknown but structurally valid animation key is retained as authoritative state, logged without per-frame spam, and presented with ordinary base animation. Malformed state is rejected at the boundary and reported through existing dispatcher error handling.

Update the canonical plain entity held by `gameStore`. Forward accepted state through `GameFacade` and `Render`; do not put Three.js/Pixi objects in Pinia. Remove state with despawn and reset it with leave-world/reconnect bootstrap. A missing optional spawn state means idle for compatibility with old servers, without fabricating a revisioned network event.

### 7. Interpret definitions through generic actor and preview APIs

Keep the latest accepted plain snapshot in the render-side object/controller. Compute normalized phase using the render's server-time estimate and provide the actor with a binding/phase/facing input through a generic action-animation entry point. `ActorInstance` remains responsible for clip selection, pose weights, and loaded asset sampling; it does not import the network layer or know protobuf types.

Replace specialized action playback methods, fields, branches, and frame constants with the generic entry point. Do not retain an action-named compatibility adapter. The preview loads the same catalog, populates its binding/variant choices from defs, and supplies normalized phase through the same actor API. A generic local preview controller can loop or scrub, whereas the network controller holds the endpoint of the last confirmed cycle. Gameplay state and preview state cannot control the same actor simultaneously. Preview labels, default equipment, and example duration come from def metadata; frame counts come from clip metadata rather than hard-coded values.

If the actor or equipment is not ready, keep the latest state. Ready callbacks and equipment completion apply the current state/phase and check lifetime, rather than replaying the state captured when a request began. A generic selector evaluates the binding's ordered variants and predicates against current actor presentation and ready equipment. No match means ordinary base presentation while the authoritative cycle remains retained. Once the pose can be shown again, sample its current phase. Cancellation clears retained state before blending back to base, so a late asset result cannot restore it. A terminal KO pose remains a global actor safety priority; individual action eligibility combinations are still declared in defs.

Take blend duration and frame dimensions/origin from the resolved definition without delaying the cycle clock. Expose current generic output-frame metrics to `ObjectView`, hit testing, culling, and texture-pool accounting. Keep the ground anchor and native pixel density stable across the transition and context restoration. During blend-out, retain bounds covering the departing pose until its weight reaches zero. Phase, selected binding/variant, dimensions, and blend weight participate in pose invalidation; gameplay state changes refresh bounds before the first enlarged texture renders.

### 8. Prove extension through data rather than action-specific code

Use def fixtures for both the production binding and a second distinct execution-source/clip binding. Run the same lifecycle, selection, timing, and rendering harness with each catalog. The second fixture must work with identical compiled runtime code and protobuf schema. Exercise reversed variant order and changed blend/frame settings through fixture changes to detect implicit assumptions about hand, tool, clip, or dimensions.

Review the new/changed synchronization runtime, actor/frame implementation, and preview for concrete action/clip/tool literals and specialized names. Keep case-specific values in def fixtures, not executable test branches. Asset recipes and generated manifests can contain clip identifiers as data. Preserve the source assets; parameterize any authoring helper that must be changed instead of introducing another hard-coded runtime mapping.

## Risks / Trade-offs

- **Tick-counter and timestamp mismatch:** the first cycle can advance in its creation tick, and catch-up ticks can share a wall timestamp -> use actual `CycleElapsedTicks` at snapshot capture, not `Tick - StartedTick`; test creation, repeat, and snapshot phases explicitly.
- **Server stall or time-calibration error:** clients cannot infer ticks the server has not processed -> treat wall-time extrapolation as presentation between authoritative anchors, clamp at cycle end, and re-anchor on each cycle or fresh spawn snapshot. Tick scheduling and exact real-time guarantees during server pauses are outside this change.
- **Finite network/render precision:** phase may differ slightly across clients due to estimated clock offsets and sampling rates -> assert deterministic normalized phase for a shared clock in tests; accept visual quantization/frame latency in browser validation.
- **Missed cancellation path:** a retained public state could outlive the action -> route every identified direct cycle-removal path through the shared clear helper and test KO, death, transfer, link break, replacement, and natural completion.
- **Async visibility and equipment work:** older results can arrive after newer state -> keep the existing world-lock spawn ordering and guard updates/load completions with epoch, incarnation, revision, and actor lifetime.
- **Critical outbound queue overload:** dropping a stop would leave a client stale -> reuse enqueue-or-close behavior for the new state path; add a focused queue-overflow test. This can disconnect a slow client, consistent with existing critical chunk delivery.
- **Unknown future animation keys:** client/server catalogs can differ -> fall back to base presentation without affecting gameplay. Fail explicitly for malformed timing and for broken required assets in a known binding.
- **Defs and published assets diverge:** a valid server key might not exist in the client snapshot -> validate publication against the effective merged manifest set and deploy matching defs/server/client catalogs together; an unknown client binding safely uses base presentation.
- **Hidden action assumptions survive the migration:** an adapter or frame constant could reintroduce specialization -> remove the old client API and preview branches, audit identifiers, and prove a second binding plus alternate variant order through def-only changes.
- **Different source-clip durations and impact timing:** scaling changes apparent speed and does not align a visible impact with results/sounds -> this is intentional. No source editing, impact markers, or sound rescheduling is part of acceptance.

## Migration Plan

1. Add the def schema/loader and catalog publication. Declare the first binding entirely in data and validate it against preserved assets; publish without re-exporting source animations.
2. Implement the additive protobuf contract and regenerate Go plus client bindings; leave existing owner UI messages intact.
3. Centralize generic cycle lifecycle, critical delivery, and spawn snapshots. Supply execution descriptors at all existing timed-cycle creation points; resolve only loaded definitions.
4. Add guarded client state and def-driven phase/variant/frame handling. Migrate the standalone preview and remove specialized client APIs and presentation constants.
5. Run def-only extension checks, focused server/protocol/client/tooling suites, real-asset render regressions, and a two-client session covering late visibility and interruption.
6. Deploy matching server defs and published client catalog together. An older client ignores additive state, and a newer client with an older server continues ordinary presentation and a catalog-driven local preview.
7. Roll back synchronization code and regenerated bindings together if necessary; retain authored clips and gameplay timing/sound behavior. Do not restore specialized APIs as a new extension mechanism.

No material design questions remain open. Future animation bindings, untimed gestures, and optional drift-correction policies require their own scope decisions and do not block this first implementation.
