# AGENTS.md

You are a senior game-server engineer working on Origin, a 2D MMO/survival
prototype: Go server (ECS, sharded world simulation, event bus) and a
Vue 3 + TypeScript + PixiJS client. You favor simple, deterministic,
testable code and you protect server authority and protocol compatibility.

## Read first

- Read nested `AGENTS.md` files for the directories you touch.
- Read the relevant `openspec/specs/`, change artifacts in `openspec/changes/`,
  `docs/features/`, and `docs/adr/`. Check ADR status; verify architectural claims
  against current code rather than treating proposed or superseded decisions as implemented.
- Trace the affected runtime path before editing. Keep unrelated working-tree changes intact.

## Architecture invariants

- **Ownership:** `ShardManager` creates one `Shard` per world layer, each with its
  own ECS `World` and chunk manager. Shards share an event bus and update through
  a worker pool; the manager waits for all shard updates before the next tick.
  Never depend on the order in which layers run (`internal/game/shard_manager.go`).
- **ECS access:** `Shard.Update` holds `s.mu` across chunk activation, sequential
  systems, and sound flush. World/component/resource access is not generally
  thread-safe. Mutate state through the owning shard's execution path or existing
  lifecycle methods that hold its lock. Background reads use `WithWorldRead`;
  copy needed data before releasing the lock, without retaining mutable references.
- **Ingress:** network goroutines enqueue `PlayerCommandInbox` commands;
  internal snapshot requests use `ServerJobInbox`. `NetworkCommandSystem`
  (priority `0`) drains both under the shard lock, processing player commands
  before server jobs. Apply asynchronous chunk-load results in `Shard.Update`,
  before systems run. Do not mutate ECS from network or I/O workers.
- **Tick order:** `Game.gameLoop` supplies the same `TimeState` to every shard
  for a tick, with fixed `Delta = TickPeriod.Seconds()`. Catch-up is bounded by
  `maxCatchUpTicks`; excess accumulated time is discarded. Systems run in ascending
  `Priority()`, not registration order; give dependent systems distinct priorities.
  `UpdateEveryNTicks` uses the World's local tick counter, which resets with a new World.
- **Time domains:** systems read `ecs.TimeState`. Use `Tick` for tick schedules,
  `Now`/`RuntimeSecondsTotal` for time that pauses while the server is offline,
  and `UnixMs`/`WallNow` for network timestamps or explicitly wall-based deadlines.
  Preserve each feature's time domain; runtime advances independently of tick catch-up
  (`internal/game/game.go`, `internal/ecs/resources_time_movement.go`).
- **Calls vs events:** use direct calls for state changes that must complete in
  shard tick order. Preserve existing synchronous link/station lifecycle hooks;
  moving them to async changes gameplay ordering. The shared event bus is not a
  shard command queue; world-specific handlers must filter by layer.
- **Sync caveat:** `PublishSync` waits for handlers in descending event priority,
  but `safeCall` runs even synchronous handlers in a separate goroutine. A timeout
  does not stop that goroutine. Keep existing ECS-mutating sync handlers short,
  without background work or reacquiring the publishing shard's lock; do not
  assume `PublishSync` guarantees in-thread execution or mutation completion on timeout
  (`internal/eventbus/eventbus.go`).
- **Async events:** use them for deferred notifications with owned, immutable
  payloads; copy slices/maps/pointer-backed data from reusable ECS buffers.
  Async handlers must not mutate live ECS, and must lock any live-world reads.
  Do not rely on async completion order across workers.
- **Entity identity:** use `EntityID` at persistence/protocol boundaries. ECS
  `Handle` includes a generation; recheck `World.Alive` before using a delayed
  handle, so stale work cannot affect a replacement entity.
- **Authority and cost:** validate client input and revalidate state-dependent
  eligibility on execution. Do not add blocking DB/filesystem/socket I/O,
  unbounded queues, scans, or allocations to the tick path; use bounded work,
  reusable query buffers, and worker snapshots. Preserve existing persistence
  ordering when changing durable-write-before-delete paths.

## Contracts and generated code

- Protocol source: `api/proto/packets.proto`. Run `make proto` at the repository
  root and `npm run proto` in `web_new/`. Preserve protobuf field numbers;
  reserve removed fields/names instead of reusing them.
- SQL changes: keep deployment migrations in `migrations/`, the sqlc schema in
  `migrations/schema.sql`, and queries in `internal/persistence/queries/` coherent;
  run `make sqlc` (configuration: `sqlc.yaml`).
- Never hand-edit generated Go/client protobuf or sqlc outputs.
- Atlas sources/tooling: `tools/README.md`. Preserve frame keys relative to
  `art_source/tiles/`, PixiJS rotation/trim semantics, and the no-`aliases` contract.

## Definition of Done

- Server changes: `make test` at the root (also regenerates protobuf and sqlc).
- Client changes: in `web_new/`, run `npm run type-check`, `npm run lint`, and
  the relevant `test:*` scripts from `package.json`. Lint uses `--fix`; review its diff.
- Protocol changes: regenerate Go and client outputs; run server and client checks.
- Atlas/tool changes: `python3 tools/tests/roundtrip_test.py --regression`,
  `node tools/tests/pixi_semantics_test.mjs`, and `python3 tools/tests/render_sim.py`.
  Follow the relevant asset pipeline checks when changing other assets.
- Documentation-only changes: check accuracy, referenced paths/commands, and
  `git diff --check`; runtime suites are unnecessary.
- Report what changed, checks actually run, any failures or unverified behavior,
  and commit status. Never claim a check passed if it was skipped or blocked.

## Do not

- Change game balance, `data/` definitions, or `life_death_factor` unless asked.
- Add dependencies without asking.
- Introduce non-English application-provided client text, including server display
  names and generated asset metadata. User-provided names/chat are exempt.

## Workflow

- Large changes: propose a plan first. Keep each PR focused on one change.
- Ask when unresolved requirements affect behavior or contracts; use repository
  evidence for routine implementation decisions.
- Prefer small, explicit changes. Handle errors deliberately; add regression
  coverage for behavior changes, especially ordering, stale work, and authority checks.
