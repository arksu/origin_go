# Proposal

## Why

Players currently receive their own action progress, but nearby players do not receive the state needed to animate that action. A shared mechanism must fit complete clips to actual server-defined tick durations and allow concrete actions and animations to be configured entirely through def files; the preserved Mixamo animation supplies the first data binding.

## What Changes

- Introduce public, server-owned action-animation snapshots and updates for the performer and players who can see the performer. Include current state in object-spawn snapshots so observers entering visibility join the current phase.
- Describe each supported action cycle with a stable animation key, incarnation and revision, authoritative tick progress and duration, server-time anchor, and optional facing target. Transmit state, never skeletal frames or arbitrary client-selected animations.
- Compute the client playback phase from the shared timing anchor and scale the entire loaded clip to the cycle duration. A three-second clip for a two-second action plays at 1.5 times its original speed.
- Handle cancellation, completion, repeated cycles, visibility changes, world resets, stale messages, and asynchronous actor/equipment loading through the same lifecycle.
- Use the existing critical-message enqueue-or-close policy for animation transitions and their player-spawn snapshots so a dropped stop cannot leave a connected client with stale state.
- Load action-to-animation mappings, actor and clip references, ordered equipment variants, facing rules, pose eligibility, blending, and render-frame parameters from a versioned catalog in `data/action_animations/`. Publish its client projection through the existing asset pipeline; do not duplicate mappings in source code.
- Keep protocol messages, server cycle integration, client state, actor playback, and preview controls generic. No action-specific hooks, animation enums, clip/tool literals, or action-named APIs belong in the synchronization implementation. Another supported timed action must be enabled by defs alone.
- Configure the single first production binding only in defs, using the preserved hand-specific clip assets and ordered equipment variants. Replace the remaining specialized client/preview entry points with the common mechanism while retaining the source animation assets.
- Keep existing gameplay durations, validation, stamina costs, effects, sounds, and performer-only progress UI unchanged. Impact markers and synchronization of a visible impact with sounds or gameplay results are explicitly outside this change.

## Capabilities

### New Capabilities

- `character-action-animations`: Data-defined action presentation, public lifecycle and visibility delivery, tick-duration scaling, phase recovery, and the first binding declared in defs.

### Modified Capabilities

None. Existing `game-actions` requirements remain unchanged; the new capability adds presentation of executing cycles without changing targeting, activation, repeatability, completion, or cancellation rules.

## Impact

- Proposed protocol additions in `api/proto/packets.proto`, with regenerated Go and client bindings, plus server visibility snapshots and public animation delivery. No database schema, persistence, or new C2S command is needed.
- Server integration with `ActiveCyclicAction`, `TimeState`, a shared cycle-start/continuation/finish lifecycle, death cleanup, and existing visibility/dirty-queue patterns. Existing execution sources supply generic source identities; no binding is selected by a hard-coded action name. No per-tick world scan or observer progress broadcast is required.
- Client changes in network handlers, plain entity state, `GameFacade`, actor sampling, output-frame handling, and a def-driven preview. Reuse `TimeSync`, incarnation guards, equipment bindings, and preserved assets.
- A strict def loader, client catalog generation/publication in `tools/asset_pipeline`, and documentation for extension through data. No new external dependency is required.
- Focused protocol, lifecycle, timing, asset-catalog, and two-client validation, including a second binding supplied only by a test def, late visibility, and interruption. Additional production bindings and newly authored animation assets are deferred.
- This proposal creates planning artifacts only. Server and protocol implementation remains absent until a separate apply request.
