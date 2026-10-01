# Tasks

## 1. Protocol

- [x] 1.1 Add `S2C_ObjectMoveBatch { repeated S2C_ObjectMove moves = 1; }` and `S2C_ObjectSpawnBatch { repeated S2C_ObjectSpawn spawns = 1; }` on free `ServerMessage` oneof fields; verify existing singles/entry fields remain unchanged and field numbers are unused, with no outer spawn epoch or new movement epoch
- [x] 1.2 Regenerate Go and web protocol stacks through the existing Makefile/proto commands; verify payload-preserving protocol roundtrips, `go build ./...`, and `cd web_new && npm run type-check`

## 2. Event ownership

- [x] 2.1 Make transform movement publication own its slice instead of aliasing tick-reused scratch; preserve immutable pointed-to values and verify a regression test delays consumption until the next update has overwritten producer scratch (`go test ./internal/ecs/systems/ -run Transform`)
- [x] 2.2 Add `EntitySpawnBatchEvent` with observer ID, layer, and owned target handle/ID entries; verify source-slice mutation cannot change a published target list (`go test ./internal/ecs/`)

## 3. Suppress redundant appearance events

- [x] 3.1 Introduce a shared visibility-aware network appearance publication guard and use it in object behavior recomputation, object transformation, and direct tree/burner producers: after updating authoritative state, check current observers under the visibility read lock and skip `PublishAsync` when none exist; preserve recomputation, dirty-state handling, and dispatcher visibility rechecks; verify focused tests for zero observers, later visibility with final resource, and existing observers receiving an upsert

## 4. Aggregate spawn publication

- [x] 4.1 Publish one nonempty spawn-batch event per `VisionSystem` observer result, including the shared `ForceUpdateForObserver` path; leave despawns unchanged; verify multiple targets, different observer subsets, and empty results (`go test ./internal/ecs/systems/ -run Vision`)
- [x] 4.2 Replace reattach's per-target spawn publication with one owned event for the snapshot's live targets; verify multiple-target and empty-snapshot reattach coverage (`go test ./internal/game/`)

## 5. Aggregate periodic carry-follow only

- [x] 5.1 Separate shared relocation mutation/entry construction from immediate publication so periodic following can obtain an optional entry; preserve the existing immediate API's success/no-op result, synchronous spatial/chunk/transform/dirty updates, and pickup/drop/transfer callers; verify unchanged position emits no entry, forced reindex still emits, and immediate carry/teleport/sequence/timestamp fields remain unchanged (`go test ./internal/game/world/ ./internal/game/`)
- [x] 5.2 Have `SyncLiftCarryFollow` return an optional movement entry without publishing it and `LiftCarryFollowSystem` publish one owned nonempty event after its pass; preserve carry cleanup and cached-handle refresh, with no individual duplicate sends; verify several moving carries, stationary carries, and next-update slice ownership (`go test ./internal/ecs/systems/ ./internal/game/`)

## 6. Dispatch bulk messages while retaining singles

- [x] 6.1 Change `handleObjectMoveBatch` to send each observer's visible subset once: no message for zero entries, a single move for one, and `S2C_ObjectMoveBatch` for multiple entries; start with standard per-recipient `proto.Marshal`, preserving entry order/fields and independent link/lift publications; verify disjoint/overlapping observer subsets and mixed single/batch sends (`go test ./internal/game/events/`)
- [x] 6.2 Implement spawn-batch handling under one `WithWorldRead` scope through enqueue and one `ClientsMu` acquisition: validate each snapshot and visibility, require an in-world client, stamp each entry's epoch, and send zero/single/batch according to valid count; use `SendCritical` if any included entry carries action-animation state; verify dead/mismatched/invisible targets, snapshot failure isolation, empty results, epoch stamping, bootstrap ordering, and critical/noncritical delivery (`go test ./internal/game/events/`)
- [x] 6.3 Preserve occasional appearance-upsert and single movement send paths; update only assertions affected by bulk delivery and retain visual/name/animation/despawn invariants; verify appearance changes on existing visible objects still deliver (`go test ./internal/game/events/`)

## 7. Client batch dispatch

- [x] 7.1 Extract common single-entry spawn/move handlers, retaining spawn epoch checks and current movement-controller/store/carry behavior; verify existing single-message tests preserve stale movement, teleport, and carry-drop snap behavior without introducing new sequence or epoch semantics
- [x] 7.2 Register both batch types and apply entries in order with per-entry error containment; verify single/batch equivalence, that a bad animation-incarnation entry and a stale-epoch spawn do not stop later valid entries, and that single messages remain accepted; run `cd web_new && npm run type-check && npm run test:character-visual && npm run test:action-animations` plus focused batch-handler tests

## 8. Load validation and serialization cost

- [x] 8.1 Update load-client single/batch decoding, message/byte/entry counters, position tracking, and reporting; add an explicit full-decode validation mode bypassing the 100-message cap while keeping drain mode available with partial entry counters labeled; verify a test exceeding 100 messages and a short full-decode run count later entries and update position (`go test ./cmd/load_test/`)
- [x] 8.2 Compare standard per-recipient serialization with the current per-entry marshal/fanout baseline using identical entity and observer sets; record CPU time and allocations alongside message/byte/entry counts for sparse and dense fanout, and investigate material regressions before claiming success; verify the measured comparison is recorded, with no custom wire encoding or Prometheus expansion required absent measured need

## 9. End-to-end regression and acceptance

- [x] 9.1 Add a deterministic chunk activation -> Vision -> dispatcher -> outbound-message test with N restored mature trees/filled containers, N > 1: let delivery run after observers are added and assert one spawn message for N valid targets, final resources, each expected ID once, and no delayed restoration upserts; also cover a later-loaded chunk and the forced/reattach burst paths (`go test ./internal/game/... ./internal/ecs/systems/`)
- [x] 9.2 Verify combined periodic movement delivery through the dispatcher: one ordinary batch plus one carry-follow batch for multiple visible entries, stationary carries produce none, occasional transitions remain independent, and all expected entries arrive; record packet reductions under fixed inputs, including the 100-mover and 100-mover-plus-100-carry scenarios from design.md
- [x] 9.3 Run `go test ./internal/... ./cmd/load_test/...`, focused race checks for changed async ownership, web type checking and relevant network/visual tests, and the project's lint checks; perform a dev smoke for chunk population, ordinary/carry movement, pickup/drop, reattach, and appearance upserts; verify `openspec validate s2c-packet-batching --strict` and report actual message counts, payload completeness, CPU/allocations, and any unrelated baseline failures separately
