# Proposal

## Why

Chopping audio currently arrives at cycle completion and only reaches players who see the tree, so it misses the animation's impact and cannot travel beyond visibility. A high-load server needs explicit server-managed world sounds and client-only frequent noises, with audibility defined by sound loudness and player hearing from the outset.

## What Changes

- Add canonical sound definitions with explicit `world` / `local` delivery, reference-distance `loudness`, separate playback `volume`, assets, attenuation and bounded processing settings; generate server and client views from the same authored definitions.
- Introduce configurable `base_hearing = 1.0` for every player and compute effective audibility radius as `loudness * hearing`.
- Author the `chop` cue at normalized phase `0.6` in the `tree_chop` animation binding; emit once per server cycle and preserve gameplay effects at cycle completion.
- Route world sounds by distance through a coarse listener-only spatial index, regardless of performer/source visibility; compute individual attenuation on the server.
- Batch world sounds per listener at the end of the current tick with explicit propagation-work budgets, bounded audio queues isolated from gameplay messages, stale-event handling and load metrics. Integrate with existing movement/cycle/shard hooks without registering additional ECS systems or adding a dedicated hearing component.
- Play footsteps entirely on the client using authored locomotion contact cues, existing interpolated movement and logarithmic attenuation for other characters that reaches zero at the footstep radius. Add the currently missing footstep assets as an explicit implementation task.
- Move quiet discovery feedback to client playback from the existing owner-only feedback event, preserving its current trigger and removing the redundant server sound packet.
- **BREAKING**: add world-sound batches and authoritative per-listener gain, update strict definition readers/catalogs, and replace the old guarantee that animation work preserves end-cycle sound timing. Deploy matching server, protocol and client artifacts together.

## Capabilities

### New Capabilities

- `sound-delivery`: authored sound modes, loudness/hearing audibility, visibility-independent world propagation, bounded spatial routing and batching, and client-only local audio including footsteps.

### Modified Capabilities

- `character-action-animations`: sound cues become authored animation metadata with strict validation and once-per-cycle execution; public animation timing continues to preserve gameplay while the chop sound moves to phase `0.6`.

## Impact

- Server: `internal/game/sound_event_service.go`, cyclic/context action flow, tree behavior, player attach/disconnect/relocation hooks, `TransformUpdateSystem`, `Shard.Update`, configuration and a sound-definition loader/listener index.
- Client: sound catalog/settings/manager, network handlers, local audio controller and existing movement/locomotion presentation.
- Data/tooling: canonical sound and locomotion-audio definitions, `data/action_animations/tree.json`, strict Go/TypeScript parsers, publication and shared fixtures; existing chop/tree-fall/feedback files remain usable.
- Protocol: `api/proto/packets.proto` and generated Go/JavaScript/TypeScript bindings; listener audio parameters at world entry and gain-bearing sound batches.
- Validation/docs: hearing and cue tests, routing lifecycle tests, client playback tests, deterministic scalability benchmarks and browser listening checks; reconcile the existing sound ADR/PRD and action-animation documentation.
- No database migration, additional ECS system, physics-based acoustic simulation or new runtime audio dependency is required.
