# DONE_SERVER

Compact backend restore context for coding agents.

## Core Mechanics (server-authoritative)
- Authentication: REST issues short-lived character auth token; WebSocket `Auth` binds client to `CharacterID`.
- Spawn/reattach: on login server either reattaches detached entity or spawns new ECS player entity after AOI preload + collision-safe spawn checks.
- Movement/combat-adjacent runtime: intent -> movement -> collision -> transform -> chunk migration in fixed system order.
- Interaction model: RMB/context actions resolved from object behaviors; execution requires link-to-target and can start cyclic actions.
- Cyclic actions: per-tick progress/finish controlled on server (`ActiveCyclicAction`), validated each cycle.
- Inventory/container: server owns inventory ops, nested container rules, open/close state, snapshots, and object-container sync.
- Craft/build/lift: command handling is ECS-thread only; build/craft use runtime state + cyclic progress.
- Local chat: validated at network edge, processed in shard context, delivered to nearby/eligible players.
- Chunked world: AOI drives chunk states (`Unloaded -> Preloaded -> Active -> Inactive`), only active chunks keep live ECS objects.
- Disconnect behavior: detached mode keeps character in world for `DisconnectDelay`; reconnect reattaches same entity if still alive.
- Multi-layer world: one shard per layer; transfer service can move player between layers with rollback path.

## Architecture Patterns
- ECS + shard model: each shard has its own `ecs.World`, systems pipeline, chunk manager, and command inboxes.
- Network-to-ECS decoupling: WebSocket goroutines only parse/validate/enqueue (`PlayerCommandInbox`, `ServerJobInbox`).
- Double-buffer queues: lock-short write path, drain at tick start, fairness/rate-limit/dedup built into inbox.
- Event bus fanout: gameplay systems publish events; async handlers push visibility-driven network messages.
- Data-driven gameplay: items/objects/crafts/build defs loaded from `data/*` with startup validation (fail-fast).
- Unified behavior runtime: single behavior registry validates contracts, drives runtime recompute, context actions, cyclic hooks, scheduled ticks.
- Dirty/budget processing: object behavior recompute and scheduled behavior ticks are budgeted to avoid full-world per-tick scans.
- Async chunk IO + LRU: background load/save workers, activation/deactivation serialize/rehydrate object runtime state.
- Time-domain split: tick/runtime/wall separated (`ecs.TimeState`, persisted runtime+tick, wall time for network timestamps).
- SQLC repository boundary: persistence through generated queries; transaction wrappers used for critical auth/state updates.

## Non-Negotiable Principles / Invariants
- Mutate ECS world only inside shard update under shard lock.
- Do not mutate game state from network threads; enqueue commands/jobs instead.
- Use `ecs.TimeState` in systems; avoid ad-hoc `time.Now()` for game logic.
- Preserve deterministic system order in `internal/game/shard.go` when adding mechanics.
- Keep `EntityID <-> Handle`, `InventoryRefIndex`, `CharacterEntities`, `DetachedEntities`, `LinkState`, `VisibilityState` consistent on spawn/despawn/transfer.
- Mark behavior dirty when runtime state affecting flags/appearance changes.
- Scheduled behavior ticks must be canceled on despawn/transform where behavior no longer applies.
- Chunk activation must run restore init + immediate behavior recompute before gameplay continues.
- For client stream safety, respect `StreamEpoch` and chunk load/unload sequencing.
- Prefer fail-fast startup/config validation over runtime silent fallback.

## Key Backend Entry Points
- `cmd/gameserver/main.go`: process bootstrap and wiring.
- `internal/game/game.go`: game loop, packet routing, lifecycle.
- `internal/game/game_auth.go`: auth, spawn, reattach bootstrap snapshots.
- `internal/game/shard.go`: per-shard wiring and system order.
- `internal/network/server.go`: WS server and client loops.
- `internal/network/command_queue.go`: inbox mechanics (fairness/rate-limit/dedup).
- `internal/ecs/*`: ECS world/resources/systems.
- `internal/game/world/chunk_manager.go`: AOI, chunk lifecycle, async persistence.
- `internal/game/behaviors/*`: unified behavior contracts/registry/implementations.
- `internal/eventbus/*`: async event distribution.
- `internal/persistence/*`: DB connection + sqlc queries.
