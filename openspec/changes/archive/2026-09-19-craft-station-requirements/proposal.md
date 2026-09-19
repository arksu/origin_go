# Proposal

## Why

Crafting currently validates a linked object by its object key, but cannot express the runtime conditions of a production station. This prevents mechanics such as cooking on an independently burning campfire, heating a furnace before smelting, or consuming station-local resources at the end of a successful craft cycle.

## What Changes

- Add an autonomous station runtime model for state, capabilities, scalar values, and station-local resources.
- Persist station state, values, and local resources through chunk unload/reload and server restart.
- Add an extensible requirement evaluator that can validate station-local conditions in v1 and later add operator, terrain, and nearby-object providers without changing crafting orchestration.
- Extend craft definitions with station requirements and per-cycle station consumptions.
- Evaluate requirements twice for every cycle: before the cycle starts and again when it completes.
- Defer all `Consume` mutations until the successful cycle-completion commit, then atomically consume station resources and craft inputs and create the craft output.
- Cancel the cycle without mutations when the final requirement check fails.
- Keep autonomous station resource consumption, such as fuel burning over time, outside the crafting service.
- Exclude operator, terrain/tile, and nearby-object checks from v1 while preserving interfaces for them.

## Capabilities

### New Capabilities

- `station-runtime`: Autonomous world-object station state, capabilities, local resources, and state updates.
- `craft-station-requirements`: Declarative station requirements, extensible evaluation providers, start and finalization validation, and atomic cycle completion.

### Modified Capabilities

None. The repository has no existing OpenSpec capability specifications; this change introduces the first contracts for these behaviors.

## Impact

- Content definitions under `data/objects` and `data/crafts`.
- Craft definition loading and validation under `internal/craftdefs`.
- Object definition/runtime construction under `internal/objectdefs` and `internal/game/world`.
- ECS components/resources and station update systems under `internal/ecs`.
- Craft orchestration under `internal/game/crafting_service.go` and inventory transaction paths.
- Craft list and station-state protocol payloads under `api/proto/packets.proto`.
- Unit and integration tests for definition loading, evaluator providers, station updates, and cycle completion.
