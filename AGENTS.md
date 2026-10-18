# AGENTS.md

Origin is a permadeath 2D MMO/survival game: Go server (ECS, sharded simulation,
event bus), Vue 3 + TypeScript + PixiJS client. Act as a senior game-server
engineer: favor simple, deterministic, testable code; protect server authority
and protocol compatibility. Target: 30,000 concurrent players per node. Evaluate
server changes for CPU cost, allocations, and memory.

## Read first

- Read nested `AGENTS.md` files for touched directories and relevant `openspec/specs/`,
  `openspec/changes/`, `docs/features/`, and `docs/adr/`. Check ADR status and
  verify architectural claims against code; proposed/superseded decisions may be unimplemented.
- Trace the affected runtime path before editing. Preserve unrelated working-tree changes.

## Absolute necessity, KISS, and DRY

- Make only changes required by the task and verification; omit anything removable
  without compromising either. Before editing, justify the simplest correct solution
  from code and requirements; explain why existing or less invasive mechanisms cannot suffice.
- Change server core only when existing mechanisms outside it cannot solve the task;
  explicitly justify each unavoidable core change.
- Reuse suitable existing logic and keep rules in one place; add abstractions, cleanup,
  refactoring, or optimizations only when required by the task.

## Architecture invariants

- **Ownership:** `ShardManager` creates one `Shard` per layer, with its own `World`
  and chunk manager. Shards share an event bus; the manager waits for all worker-pool
  updates before the next tick. Never depend on layer order (`internal/game/shard_manager.go`).
- **ECS access:** World/components/resources are not generally thread-safe.
  `Shard.Update` holds `s.mu` across chunk activation, sequential systems, and sound flush.
  Mutate through the owning shard's execution path or existing lifecycle methods holding its lock.
  Background reads: `WithWorldRead`; copy before unlocking, retaining no mutable references.
- **Ingress:** network goroutines enqueue `PlayerCommandInbox`; internal snapshots
  use `ServerJobInbox`. `NetworkCommandSystem` (priority `0`) drains player commands
  before server jobs under shard lock. Apply async chunk-load results in `Shard.Update`
  before systems. Never mutate ECS from network/I/O workers.
- **Ticks:** `Game.gameLoop` gives all shards the same `TimeState` per tick and fixed
  `Delta = TickPeriod.Seconds()`. Catch-up caps at `maxCatchUpTicks`, discarding excess time.
  Systems run by ascending `Priority()`, not registration; dependencies need distinct
  priorities. `UpdateEveryNTicks` uses World-local ticks, reset with each new World.
- **Time:** read `ecs.TimeState`: `Tick` for tick schedules; `Now`/`RuntimeSecondsTotal`
  for time paused offline; `UnixMs`/`WallNow` for network timestamps/explicit wall deadlines.
  Preserve feature time domains; runtime advances independently of catch-up
  (`internal/game/game.go`, `internal/ecs/resources_time_movement.go`).
- **Calls/events:** use direct calls for tick-ordered state changes. Preserve synchronous
  link/station lifecycle hooks. The shared bus is not a shard command queue;
  world-specific handlers must filter by layer.
- **Sync events:** `PublishSync` waits in descending event priority, but `safeCall`
  runs handlers in separate goroutines; timeout neither stops them nor guarantees mutation
  completion. Keep existing ECS-mutating sync handlers short: no background work or
  reacquiring the publisher's shard lock (`internal/eventbus/eventbus.go`).
- **Async events:** use for deferred notifications with owned, immutable payloads;
  copy slices/maps/pointer-backed data from reusable ECS buffers. Never mutate live
  ECS; lock live-world reads. Never depend on worker completion order.
- **Identity:** use `EntityID` at persistence/protocol boundaries. `Handle` is generational;
  recheck `World.Alive` before using delayed handles to protect replacement entities.
- **Authority/cost:** validate client input; revalidate state-dependent eligibility on
  execution. No new blocking DB/filesystem/socket I/O, unbounded queues, scans, or
  allocations on the tick path; use bounded work, reusable query buffers, worker snapshots.
  Preserve persistence ordering in durable-write-before-delete paths.

## Contracts and generated code

- Protocol: edit `api/proto/packets.proto`; run `make proto` at root and `npm run proto`
  in `web_new/`. Preserve field numbers; reserve removed fields/names, never reuse them.
- SQL: keep `migrations/`, `migrations/schema.sql`, and `internal/persistence/queries/`
  coherent; run `make sqlc` (`sqlc.yaml`).
- Never hand-edit generated Go/client protobuf or sqlc outputs.
- Atlas: follow `tools/README.md`; preserve `art_source/tiles/`-relative frame keys,
  PixiJS rotation/trim semantics, and no `aliases`.
