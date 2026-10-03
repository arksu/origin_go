# Design

## Context

See `proposal.md` for motivation and scope. The relevant current contracts are:

- `SoundEventService.EmitForVisibleTarget` reads `VisibilityState`; `DefaultMaxHearDistance` is 200 while player vision radius is 600. `Shard.SendSound` marshals the same payload separately for each visible observer.
- `CyclicActionSystem` emits `CycleSoundKey` at cycle completion before the behavior effect. Tree behavior supplies hard-coded `chop` and `tree_fall` keys. A successful terminal effect can despawn its target before the completion sound is resolved.
- `cyclicaction.Start` already resolves an animation binding by generic action source. `tree_chop` has complete left/right clips and public cycle revisions. Go and TypeScript readers reject unknown fields; publication produces immutable hashed client projections.
- Client world audio currently computes distance/smoothstep itself. `SoundManager` constructs Howl objects lazily, rebuilds file lists, logs each playback and sets shared Howl volume.
- Interpolated movement already supplies displacement excluding teleport/correction snaps. Actor locomotion derives its phase from distance and a manifest-defined stride length, but there are no authored contact cues or footstep audio files.
- `TransformUpdateSystem` commits positions at priority 300 and filters movement publication by visibility afterward. Cyclic processing runs at 315. `Shard.Update` holds the world lock around `world.Update`.
- The project has no listener-only spatial index. Its chunk grids have 16-unit cells and query all static/dynamic objects. Client membership and character-save resources are not spatial hearing indexes.
- The user requires zero additional ECS systems. The current animation spec explicitly preserves old sound timing; this change replaces that guarantee while retaining gameplay timing.

## Goals / Non-Goals

**Goals:**

- Make source range, listener sensitivity, delivery mode and cue timing independently configurable and unambiguous.
- Make routing cost depend on nearby connected listeners, with bounded work, queues and playback.
- Retain source coordinates independently of source lifetime and client visibility.
- Keep quiet audio out of server sound scheduling, propagation and network traffic.
- Reuse existing tick/lifecycle/movement hooks and keep all index mutation on the shard's world-write path.

**Non-Goals:**

- Occlusion, wall/material acoustics, speed-of-sound delay, cross-layer propagation or physical decibel simulation.
- Moving gameplay effects/stamina to animation contact frames, splitting source clips or changing action duration.
- Individual hearing bonuses, hearing persistence, new ECS systems or a dedicated hearing component in this iteration.
- Replacing Howler or broadly rebuilding the asset pipeline.

## Decisions

### 1. Canonical sound profiles and independent cue ownership

Add strict canonical sound JSON under `data/sounds/`, loaded into a server registry and published as an immutable client sound catalog. A profile contains `key`, explicit `mode` (`world` or `local`), finite positive `loudness`, playback `volume` in `[0,1]`, nonempty asset `files`, priority and playback/attenuation settings. Publication checks that audio files exist and references resolve; server startup validates metadata and cue references without requiring client media files to be mounted.

Retain the existing sample paths and volumes. Proposed initial tuning, separate from fixed mechanics: `chop` world loudness 1000; `tree_fall` world loudness 1400; footsteps local loudness 120; discovery feedback local loudness 80. These are authored defaults to verify by listening, not hard-coded runtime branches. Both world profiles exceed the current 600-unit visibility radius.

Add optional `sound_cues` to action animation bindings: a cue has a stable ID, normalized `phase`, `sound_key` and `source` (`actor` or `target`). First-version action cue phases are `(0,1]`; locomotion contacts are `(0,1)` to avoid duplicate loop-boundary representations. Validate unique IDs, increasing phase order, finite phases, source availability and referenced sound profiles. Absence preserves a binding without audio cues. Prepare mode-specific cue arrays when loading definitions instead of filtering or parsing them each tick.

The `tree_chop` binding authors one world cue at `0.6` with `sound_key=chop` and `source=target`. The phase is shared by its left/right variants. Keep `tree_fall` as successful action-completion feedback rather than a repeated animation cue.

Footsteps belong in separate client locomotion-audio definitions under `data/locomotion_audio/`: bind actor identity and existing locomotion clip keys to authored contact cues, and take stride distance from the actor manifest. Do not pretend walking is an `ActiveCyclicAction` or add a new server action source. Publish these definitions through the existing immutable catalog mechanism, including reference validation against actor clips and local sound profiles.

For discovery feedback, define an optional local feedback trigger on its sound profile that matches the existing owner-only `S2C_Fx` key `exp_gain`. Trigger from that message, not every positive `S2C_ExpGained`: crafting and administrator grants also send LP deltas today and must not gain a new feedback trigger accidentally. Remove the redundant `SendSound` from the give-item adapter; preserve the existing FX and experience messages.

**Alternatives considered:** putting phase in tree behavior duplicates animation knowledge; inferring world/local from playback volume lets user sliders change gameplay routing; storing walking contacts in cyclic action defs invents a server execution source for client-only presentation.

### 2. Loudness is reference distance and hearing is a multiplier

`loudness = L` is a sound's audibility distance in world coordinates at hearing 1. `hearing = H` is listener sensitivity, currently the shared `game.audio.base_hearing = 1.0`. Validate finite `L > 0`, finite `H > 0` and finite effective radius before squaring. Compute `R = L * H` and accept only `distanceSquared < R * R`. Playback sliders and sample balancing never affect this test.

Use one effective-hearing accessor for players, returning the configured base in this iteration. Provide the current player's hearing once at world entry for local audio; refresh on reconnect/transfer. The same audio parameters expose configured world-event freshness so the client uses the server's chosen window rather than a separately authored constant. No database column or standalone ECS component is needed.

For audible world recipients, compute `t = distance / R` and `distance_gain = 1 - 3*t*t + 2*t*t*t` on the server. The square root is needed only after cheap squared-distance rejection. The client applies `distance_gain * profile.volume * masterVolume * sfxVolume` once.

Keep a configured upper bound on effective radius; reject invalid definition/config combinations instead of silently clamping the model. Currently the search radius is `L * base_hearing`. Preserve an index/service lookup for the maximum active listener hearing so future individual hearing cannot produce false negatives; no full-player scan per sound is allowed when that extension is added.

**Alternatives considered:** additive hearing distances lack a clear neutral value; physical dB/inverse-square simulation adds units and balancing complexity unnecessary for the requested game rule.

### 3. Listener-only coarse grid, owned by the existing sound service

Maintain a sparse uniform grid per shard/layer containing only attached, in-world listeners. Start with configurable 256-unit cells. Each listener records its generational handle, cell and slot; swap-remove permits constant-time cell membership changes. Empty cells are removed. Use floor-based cell coordinates, including negative positions.

Query cells intersecting the search square, then perform exact radius/hearing checks against current transforms. Reuse candidate buffers and skip dead handles, detached clients or a changed layer/stream. This is independent of visibility and chunk loading. Dead observer sessions retain environmental hearing while they remain attached and in-world, using their current world position.

Lifecycle hooks add/remove entries on attach, disconnect, transfer and relocation, including rollback. A narrow position-update interface injected into existing `TransformUpdateSystem` updates the index immediately after final collision-adjusted transform commit, before its visibility guard. Only listener entities incur the lookup, and cell membership changes only on cell crossings. Relocation paths update directly because they need not produce ordinary movement entries.

The index is a data structure owned by the shard sound service, not an ECS system. No background goroutine, extra world-wide player scan, chunk migration dependency or asynchronous index mutation is introduced.

**Alternatives considered:** scanning `Shard.Clients` is O(total players) per event; querying the existing fine object grid scales with trees/items and many cells; reusing visibility results excludes precisely the distant listeners this feature requires.

### 4. Cue execution stays in the existing cycle flow

Use a shared cue-crossing helper from existing validated cycle advancement, including context and menu-cycle branches. Resolve prepared bindings once for an installed cycle and retain a small cursor/state tied to its action incarnation/generation, cycle index and start tick. Advance a cue once when previous progress is below its threshold and new progress reaches or passes it; use the first tick at or beyond the phase (12 of 20 for chop, 8 of 13 for phase 0.6).

Validate the current cycle/target before cues, as before effects. Cancellation before the marker prevents emission; cancellation after it retains the already-authored event. Continue resets cue state; duplicate processing, replacement or stale completion cannot replay a marker. Unmapped actions preserve existing gameplay and do not invent animation cues.

Capture source coordinates into the event while the source is valid. For successful terminal feedback, capture the target point before a transform/despawn effect and submit `tree_fall` only after success. Emitting a point event no longer requires the target to remain alive afterward.

Only world cues enter the server sound queue. Remove old end-cycle `chop` emission; preserve all validation, stamina/effects and progress/finished packets. Client network animation playback never executes world cues. Optional explicit preview playback can inspect the same metadata without transmitting gameplay audio.

**Alternatives considered:** renderer contact callbacks cannot reach listeners who do not know the performer; per-cue timers/goroutines create unnecessary state and cancellation races.

### 5. Bounded propagation and same-tick per-listener batches

The service collects immutable point events during a tick. After `world.Update`, `Shard.Update` propagates selected events using current listener positions and flushes per-listener batches before leaving the same shard update. The event is authored on its crossing tick; there is no extra periodic timer. Source point/time are captured at creation; listener eligibility is evaluated during that tick's propagation.

Proposed operational defaults, all validated/configurable: maximum effective radius 4096; 1024 world events per shard tick; 65,536 queried cells and 65,536 candidate checks per shard tick; 16,384 admitted recipient entries per shard tick; 64 entries and 16 KiB encoded size per listener batch; 500 ms event freshness. These are initial bounded settings to measure and tune, not a throughput guarantee.

Select events by stable priority with creation order as tie-breaker, and visit cells/listener slots in a deterministic order. Higher-priority fall feedback can displace lower-priority chop noise within bounded buffers. Debit cell, candidate and admitted-entry work before performing it; stop propagation when a global budget is exhausted, even if recipients remain or their individual batches are already full. Do not materialize an unbounded candidate list before enforcing the budget. Count the exhausted budget, truncated queries and remaining discarded events; skipped recipient counts are reported only when already known, never discovered by scanning after cutoff. Do not keep a service event backlog across ticks or delay gameplay to wait for audio.

Add `optional distance_gain` to `S2C_Sound` without reusing existing field tags, plus a new sound-batch envelope with ordered entries, stream epoch and the tick's server-time anchor. Provide current hearing and freshness window in world-entry audio parameters. New server world output always uses the gain-bearing, epoch-tagged batch, including singleton batches. The client validates each entry independently; zero gain is valid and is never treated as a missing field.

For legacy single world messages without gain, the new client retains the old coordinate/radius attenuation fallback. Gain-present messages never receive another distance multiplier. Ignore server sound packets for profiles marked local so an old server cannot duplicate migrated local feedback. Old clients do not understand the batch type; there is no claimed mixed-version negotiation.

Outgoing buffers handed to the network own their contents. Encode once per recipient batch because gains differ per recipient. The existing socket writer already coalesces flushes; protobuf batching additionally reduces message/enqueue overhead.

Do not feed audio into the current shared `sendCh`: a sound burst could fill it and make `SendCritical` close the connection. Add a separate nonblocking bounded audio queue with an initial capacity of one batch per connection and an observable admission result. The existing writer drains pending gameplay messages before taking at most one audio batch, rechecks gameplay when an audio wakeup wins, and preserves shutdown handling. If the audio slot is occupied or the connection is detached/closed, discard and count the new batch. Audio cannot consume gameplay queue capacity; existing critical failure policy remains intact for actual gameplay congestion. The shared socket can still be slow, so bounded batch bytes and freshness limit audio's transport cost without claiming reliability under a stalled connection.

Use the existing server-time estimate to check freshness on receipt and again immediately before playback after any asynchronous sample loading. Avoid relying on Howler's implicit delayed-play queue: retain the event deadline and stream identity, and discard expired or superseded work before starting a voice. Clear queued playback/index state on transfer/reset; metric instrumentation distinguishes work-budget, entry/byte-budget, stale, disconnected and audio-admission/transport drops. This network queue is not an ECS system or a persistent sound backlog.

**Alternatives considered:** one shared encoded sound cannot carry individual gains; unbounded queues convert a burst into delayed irrelevant sounds; stronger audio reliability would compete with gameplay state during overload.

### 6. Client local audio uses movement state and a log curve fading to silence

A local audio controller consumes existing interpolated displacement/locomotion timing, independently of model render FPS and screen culling. It considers the local character and already-known characters inside local audibility range; it does not create unknown performers or request additional movement updates. Source identity distinguishes own from other sounds.

For a local source at distance `d`, use the player's entry-provided hearing and profile loudness for `R`. Reject `d >= R`. Inside the radius let `t=d/R`, `q=ln(1+4*t)/ln(5)`. Own gain is 1; other footstep gain is `0.50 * (1-q)`. Store near/far gain and log shape as validated local attenuation settings: finite `0 <= far_gain <= near_gain <= 1` and finite `shape > 0`, using `q=ln(1+shape*t)/ln(1+shape)` generically. Owner-only feedback always uses own gain. The footstep profile authors near gain `0.50`, far gain `0`, and shape `4`, so its gain approaches silence continuously before range exclusion. This replaces the initially approved `0.90`→`0.80` settings after user playtesting found nearly indistinguishable own/other steps and an abrupt range cutoff.

Author contact phases from the actual walk/carry-walk clips, using manifest stride metadata. Maintain unwrapped cycle/contact state rather than relying only on modulo phase or a render callback. Stops/restarts reset/rebase the tracker consistently with locomotion; teleports, correction snaps, loading after a contact, lost visibility, local-radius entry, world reset and long presentation pauses do not produce catch-up footsteps. Switching locomotion clips rebases audio contacts to the current gait phase without replaying the previous clip's contact or producing two contacts at the transition. Local action cues use accepted action incarnation/cycle identity, skip already-passed markers on late visibility entry, and retain consumed markers across backward progress corrections within that same cycle. Only a new accepted cycle can re-arm its markers.

Provision real footstep audio in the implementation with its source/license or project-authored provenance documented. The repository currently has none; publication fails explicitly for a missing referenced sample rather than using chop or a fake alias. Asset selection and exact gait contact phases are reviewable implementation work, not an unresolved architecture choice.

Cache normalized sample lists/profile metadata and Howl objects. Set gain per playback ID, not on the shared Howl, so concurrent nearby/distant instances cannot change each other's volume. Prepare/preload action samples before their marker when practical. Gate verbose logs behind diagnostics; enforce configurable voice/per-source limits and reuse tracking structures.

**Alternatives considered:** keeping a high nonzero far gain produces an audible range cutoff; a uniform multiplier alone does not convey distance. The authored logarithmic curve retains simple tuning, gives co-located other footsteps gain `0.50`, and reaches zero at the radius.

### 7. Performance acceptance is structural and measured

Require unchanged ECS system registration count and no dedicated hearing component. In warmed deterministic benchmarks, adding remote listeners outside queried cells or adding world objects does not increase candidate/recipient counts for a fixed event. Report query cells, candidates, recipients, events, dropped events/entries, encoded messages/bytes, allocations and propagation/encode time separately.

Cover sparse listeners, a dense crowd, moving/cross-cell listeners and many quiet footsteps. Dense fan-out requires O(real recipients) until the explicit global work cutoff; verify cell, candidate and admitted-entry work stay within configured limits even when many listener batches are full. Verify that footsteps generate zero server sound events and sound packets, discarded audio never changes gameplay results, and an audio burst cannot fill the gameplay queue or cause a critical-send closure. Absolute throughput numbers are reported with hardware/workload details, not invented as planning guarantees.

## Risks / Trade-offs

- Missing relocation/disconnect hooks -> cover same-cell moves, cell crossings, negative coordinates, teleports, rollback, detach/reattach and layer changes with lifecycle tests.
- Sound and published animation catalogs diverge -> validate cross-references and publish sound/animation/locomotion projections atomically under the existing catalog publication lock; deploy matching server defs.
- A dense crowd exceeds audio budgets -> stable priority selection, explicit drop counters and bounded client voices; validate both normal delivery and deliberate overload loss.
- Individual gain increases encoding work -> per-listener batching and reusable buffers; measure encoding separately from spatial search.
- Mild local attenuation has a range boundary -> use one-shot clips in this iteration and review by listening; do not silently introduce continuous loops.
- No current footstep assets -> add a concrete asset/provenance task and reject dangling publication references.
- New batch output is unreadable by old clients -> coordinated rollout; retain legacy-single input fallback on the new client, not an unsupported bidirectional compatibility promise.

## Migration Plan

1. Implement registry/config, strict parsers and shared validation fixtures, then protocol generation and client handlers before enabling new definitions/output.
2. Implement listener/cycle/lifecycle hooks, bounded propagation and local playback; add footstep files and authored gait contacts.
3. Publish matching immutable sound/action/locomotion catalogs. Replace authored client `sounds/actions.json` with a generated projection while preserving existing sample choices and volumes.
4. Deploy matching server and client together. Remove end-cycle chop emission and redundant discovery sound sending when the replacement routes are active; do not run both paths for the same sound.
5. Run correctness, browser listening and load checks. Record chosen tuning/budget values and supersede the visibility-based sound ADR/PRD and the old animation sound-timing guarantee.
6. Roll back as one unit: previous server binary/defs, previous client bundle and previous catalog manifest. Per-tick sound queues and indexes are transient; no persisted migration is involved.
