# Validation evidence

Measured on 2026-09-30, darwin/arm64, Apple M3 Max. All byte counts below are protobuf payload bytes, excluding WebSocket framing, TLS, and TCP/IP headers. Message counts refer to application WebSocket messages, not TCP segments.

## Periodic movement through the actual outbound WebSocket

`TestPeriodicMovementBatchesDeliverEveryEntry` runs the real transform system and real `LiftService`/carry-follow system, dispatches their asynchronous events, and decodes the resulting messages from a loopback WebSocket connection. Each expected entity ID must appear once, with its final position, timestamp, carry relation, and existing sequence value. The baseline payload count is calculated by wrapping those same delivered entries in their previous single-message envelopes.

| Fixed input, one observer | Baseline messages/pass | New messages/pass | Baseline payload bytes/pass | New payload bytes/pass | Distinct delivered entries/pass |
| --- | ---: | ---: | ---: | ---: | ---: |
| 100 ordinary movers | 100 | 1 | 2,858 | 2,762 | 100 |
| 100 ordinary movers + 100 moving carried objects | 200 | 2 | 5,216 | 5,024 | 200 |

The second scenario produces one ordinary-movement batch and one carry-follow batch. A subsequent stationary carry pass produces zero messages. A forced drop at the same position produces one independent single movement message immediately, with `carried_by_entity_id = 0`.

At a fixed 10 TPS with the same inputs every tick, these counts imply 1,000 → 10 and 2,000 → 20 messages/s respectively. These rates are extrapolations from deterministic per-pass counts, not production traffic measurements.

Command:

```sh
go test ./internal/game/events/ -run 'TestPeriodicMovementBatchesDeliverEveryEntry|TestObjectMoveEncoder' -v
```

## Serialization and fanout cost

`BenchmarkMovementBatchSerialization` compares identical authored entries and ordered visibility subsets. The baseline builds and marshals each single entity once, then reuses its bytes for every observer. `per_recipient` builds shared protobuf entries and marshals each observer's whole subset independently. `batch` uses the production encoder: observers seeing the entire event share one standard `proto.Marshal` result; partial subsets retain per-recipient serialization. No custom protobuf wire construction is used.

Two 300 ms benchmark samples were collected for each row; the table reports their elapsed-time range. `ns/op` measures time for one serialization/fanout pass, not the whole server tick. The benchmark includes entry construction and fanout bookkeeping, but excludes visibility-map construction, locking, client queues, socket writes, and client decoding. Consequently it does not measure the CPU saved by reducing network sends or client dispatches, and it cannot establish a total-server CPU improvement.

| Visibility | Path | Time/pass (µs) | Allocated bytes/pass | Allocations/pass |
| --- | --- | ---: | ---: | ---: |
| 1 observer, all 100 entries | Baseline | 57.0–59.2 | 41,888 | 701 |
| 1 observer, all 100 entries | Per recipient | 33.6–34.1 | 33,304 | 407 |
| 1 observer, all 100 entries | Production batch | 33.2–33.4 | 32,408 | 406 |
| 10 observers, 10 entries each, disjoint | Baseline | 55.5–56.7 | 41,888 | 701 |
| 10 observers, 10 entries each, disjoint | Per recipient | 36.3–37.2 | 34,272 | 443 |
| 10 observers, 10 entries each, disjoint | Production batch | 36.0–36.7 | 33,624 | 447 |
| 100 observers, all 100 entries each | Baseline | 62.4–62.7 | 41,888 | 701 |
| 100 observers, all 100 entries each | Per recipient | 2,263.6–2,312.3 | 452,272 | 803 |
| 100 observers, all 100 entries each | Production batch | 33.9–33.9 | 32,408 | 406 |
| 100 observers, 99 entries each, unique subsets | Baseline | 62.8–63.1 | 41,888 | 701 |
| 100 observers, 99 entries each, unique subsets | Per recipient | 2,279.8–2,287.0 | 452,272–452,274 | 803 |
| 100 observers, 99 entries each, unique subsets | Production batch | 2,238.9–2,281.9 | 453,544 | 810 |

| Visibility | Delivered entries/pass | Baseline → batch messages/pass | Baseline → batch payload bytes/pass |
| --- | ---: | ---: | ---: |
| 1 observer, all 100 entries | 100 | 100 → 1 | 3,900 → 3,804 |
| 10 observers, 10 entries each, disjoint | 100 | 100 → 10 | 3,900 → 3,840 |
| 100 observers, all 100 entries each | 10,000 | 10,000 → 100 | 390,000 → 380,400 |
| 100 observers, 99 entries each, unique subsets | 9,900 | 9,900 → 100 | 386,100 → 376,600 |

The initial standard per-recipient implementation had a material dense-fanout regression because the nested entries were serialized once per observer, rather than once per entity. Sharing the encoded complete subset removes that repeated work in the all-visible case, while preserving the same bytes and per-observer routing.

**Remaining limit:** many distinct, heavily overlapping partial subsets still require repeated protobuf serialization. The 99-of-100 scenario remains about 36 times slower than the serialization-only baseline and allocates about 11 times as many bytes, despite reducing messages from 9,900 to 100. At 10 passes/s this isolated benchmark uses about 22–23 ms/s of serialization time (roughly 2.3% of one core for this fixed fixture). That is not a bound for larger populations or an estimate of complete server CPU usage. This result has been investigated and is reported explicitly; this change does not claim that all observer distributions improve server CPU. The implementation retains standard protobuf serialization and does not expand into custom wire encoding.

Command:

```sh
go test ./internal/game/events/ -run '^$' -bench BenchmarkMovementBatchSerialization -benchmem -benchtime=300ms -count=2
```

## Carry relocation and ownership regressions

The focused regression tests cover successful unchanged-position relocation returning no entry; forced same-position reindex still returning/emitting an entry; synchronous transform, spatial, chunk, and dirty-state updates; inactive-target rejection; unchanged carry/teleport/default-sequence/timestamp fields; moving versus stationary carries; cached-handle refresh and invalid-carry cleanup; same-position pickup, forced drop, and put-down retaining individual immediate events; and a queued follow batch surviving the next pass's buffer reuse.

These checks passed:

```sh
go test ./internal/game/ ./internal/game/world/ ./internal/ecs/systems/ -run 'RelocateWorldObject|LiftCarryFollow|LiftServiceFollow|LiftServiceSamePosition' -count=5
go test -race ./internal/game/world/ ./internal/game/ ./internal/ecs/systems/ -run 'RelocateWorldObject|LiftCarryFollow|LiftServiceFollow|LiftServiceSamePosition' -count=1
go test -race ./internal/game/events/ -run 'TestPeriodicMovementBatchesDeliverEveryEntry|TestObjectMoveEncoder' -count=1
git diff --check
```

Async assertions use completion barriers before examining captured messages. Event-bus shutdown by itself can cancel a handler context before its goroutine finishes, so it is not used as proof that all captured events have been observed.

## Isolated development server smoke

The final smoke ran on 2026-09-30 against a freshly built game-server binary and a separate PostgreSQL database, `origin_s2c_smoke_20260930_a1`, on the existing local PostgreSQL instance at `127.0.0.1:5430`. The temporary database was initialized from `migrations/schema.sql` and populated with a 4 × 4 grass-chunk world, five fixture accounts/characters, 32 mature birch trees, and 32 boxes containing an item. Fixture entities were clustered in one chunk so the initial visibility result included all 64 objects.

The server listened only on `127.0.0.1:18080` and ran from `/private/tmp/origin-s2c-smoke-t2jbz9ps/`, with its own `config.yaml`, logs, and binaries. Its `data` symlink pointed to the repository's definition files. The user's existing server, PID 19706 on port 8080, and its database/world were not restarted or modified.

The gameplay smoke was a **headless protocol client using the generated web protobuf stack over a real WebSocket**, not a visual browser inspection. It authenticated through the REST endpoints, entered the world, decoded every incoming message, and asserted:

- Initial delivery contained all 32 trees with `trees/birch/6` and all 32 filled boxes with `box/normal`. Each object ID appeared exactly once: one spawn batch carried those 64 restored objects plus the player, for 65 spawn entries. A further receive interval produced no restoration-triggered appearance duplicates.
- Ordinary movement reached explicit waypoints. A secondary click opened a box and delivered `box/open`; closing it delivered `box/normal`, preserving visible-object appearance upserts.
- The client activated lift, picked up that box, moved while carrying it, observed movement with the expected carrier ID, and put it down through the lift-down action.
- Disconnect/reconnect within the detach timeout reattached the existing player. The reattachment snapshot arrived as one spawn batch containing 65 entities.

| Capture | All received messages | Payload bytes | Spawn entries | Move entries | Spawn batch messages | Movement batch messages |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Initial bootstrap | 28 | 298,640 | 65 | 0 | 1 | 0 |
| Initial connection through appearance, pickup, carry, and drop | 110 | 301,839 | 67 | 70 | 1 | 0 |
| Separate reattach connection | 19 | 150,980 | 65 | 0 | 1 | 0 |

The 67 spawn entries comprise the initial 65 plus two appearance upserts. This single-player gameplay smoke has only one ordinary mover and one carried object, so the separate movement streams correctly retain single-message delivery; the multi-entity movement reductions are established by the deterministic scenarios above and the load run below. These totals include bootstrap/chunk and other gameplay messages, not only spawn/move messages.

### Full-decode load run

The final load run used the rebuilt load client with its receive-loop completion fix:

```sh
go build -o /private/tmp/origin-s2c-smoke-t2jbz9ps/gameserver ./cmd/gameserver
go build -o /private/tmp/origin-s2c-smoke-t2jbz9ps/load_test ./cmd/load_test
```

From the isolated temporary directory:

```sh
./gameserver > server-console-2.log 2>&1
node smoke.mjs > gameplay-smoke-2.log 2>&1
./load_test --server_host=127.0.0.1 --server_port=18080 \
  --db_host=127.0.0.1 --db_port=5430 --db_name=origin_s2c_smoke_20260930_a1 \
  --clients=3 --ramp-up=3 --duration=12s --period=500ms \
  --move-radius=80 --seed=12345 --full-decode > load-full-2.log 2>&1
```

The three clients all logged in and entered the world successfully. Final accounting reports `decoded_entry_scope=full_run`, **389 received messages and 389 decoded messages**, 505,619 received payload bytes, 204 spawn entries in four spawn batches plus one single spawn, and 895 movement entries in 301 movement batches plus 29 single moves. The clients sent 66 movement commands; 318 decoded movement entries concerned the receiving clients' own characters. Server error and decode-error counters were zero. The final isolated server log contains no WARN or ERROR entries. This run validates live delivery and full decoding beyond the old 100-message limit; it is not a controlled before/after performance comparison.

During smoke preparation, inspection exposed a load-client lifecycle race: receive message/byte counters are buffered inside `readLoop`, but cleanup previously returned without waiting for that loop's final flush. Cleanup now closes the connection and joins the receive goroutine before the runner prints its summary. This also waits for the last decoded entity updates. `TestVirtualClientCleanupWaitsForReceiveMetrics` covers the final message, byte, and entry counts; both `go test ./cmd/load_test -race` and `go test ./cmd/load_test -race -run CleanupWaits -count=30` passed. Interval message/byte counters still use the existing buffered accounting, so short-interval diagnostics may lag decoded-entry counters; the final joined summary is complete.

### Evidence and cleanup

Final smoke artifacts remain under `/private/tmp/origin-s2c-smoke-t2jbz9ps/` for inspection:

- `smoke.ts` and its bundled `smoke.mjs`: protocol-client assertions and gameplay sequence.
- `config.yaml` and `seed.sql`: isolated server configuration and fixture setup. The final fixture uses object-state version 1 for boxes.
- `gameplay-smoke-2.log`: successful initial/gameplay/reattach captures.
- `load-full-2.log`: final 12-second full-decode load report.
- `server-console-2.log`: successful server operation, reattach, and graceful shutdown.

The owned server PID 39834 was stopped with `kill -TERM 39834`, and its process exited successfully after saving its temporary world. After its connections closed, `DROP DATABASE origin_s2c_smoke_20260930_a1` completed on the local PostgreSQL instance. A final listener check showed no server on port 18080 and the user's original PID 19706 still listening on port 8080. Only temporary evidence files remain; no smoke database or server process was left running.

## Deterministic chunk and delivery regressions

`TestChunkActivationSpawnBatchHasFinalAppearanceWithoutUpserts` uses the real chunk manager, object factory, behavior restoration/recomputation, Vision, asynchronous dispatcher, and network server. Each chunk contains 150 restored mature trees and 150 filled containers. Delivery is held until Vision installs observers, which would expose a dispatch-time-only appearance suppression bug. Both the initial chunk and a later loaded neighboring chunk produce exactly one WebSocket spawn message containing all 300 expected IDs once, with final resources and current entry epochs. The later scenario moves the observer and runs both normal and forced Vision paths. Restoration publishes zero appearance events, and restored persistence dirty flags stay clear. An unchanged subsequent forced update produces no message.

`TestSpawnBatchOutboundValidationEpochAndAppearance` checks identity mismatches, dead/invisible targets, missing required components, failed animation snapshots, bootstrap-before-spawn queue order, epoch changes, singleton fallback, empty results, and visible appearance upserts. `TestSpawnBatchCriticalDeliveryOnFullQueue` holds the real client before its writer starts, fills its send queue, and verifies that an animated entry makes the batch close the overflowing connection while an ordinary batch retains the existing drop policy.

The later-chunk fixture intentionally exercises an observer moving into view. During fixture development, a separate pre-existing visibility-cache limitation was observed: a stationary observer can skip recomputation when a newly loaded neighboring chunk was absent from its previous `LastChunkGens` list; `ForceUpdateForObserver` bypasses the timer but still honors that cache. The current batching change does not alter cache invalidation or promise delivery earlier than the existing Vision pass discovers objects. The tests and message-reduction claims above apply once visibility is recomputed. This is distinct from batching and appearance duplication.

## Final verification

Passed on the final implementation:

```sh
go build ./...
go test ./internal/... ./cmd/load_test/...
go test -race ./internal/ecs/... ./internal/game/... ./cmd/load_test/...
go vet ./internal/... ./cmd/load_test/...
npm --prefix web_new run type-check
npm --prefix web_new run test:character-visual
npm --prefix web_new run test:action-animations
(cd web_new && node scripts/test-character-visual.mjs tests/object-batches.test.ts)
(cd web_new && ./node_modules/.bin/eslint src/network/handlers.ts src/network/MessageDispatcher.ts tests/object-batches.test.ts)
openspec validate s2c-packet-batching --strict
git diff --check
```

Web tests passed: 24 character-visual, 9 action-animation, and 7 batch-handler tests. The focused batch tests preserve existing stale-sequence, teleport, carry-drop, epoch, and per-entry failure semantics. Both protocol generators were run using `make proto` and `npm --prefix web_new run proto`; the repository ignores the generated web protobuf files, which were regenerated locally.

The first broad race run exposed an existing unsynchronized assertion in `TestPointStopBlockedByDeepWaterBroadcastsStop`: async workers wrote `lastMove` while the test read it, and shutdown was incorrectly treated as a completion barrier. This was reproduced on an isolated clean HEAD under `/private/tmp/origin-s2c-baseline` before changing the test. The regression now receives each tick's immutable entry through a channel before checking it. Its repeated focused race check and the final broad race run pass; no production point-arrival behavior was changed.

The full nonmutating ESLint command (`eslint . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx,.cts,.mts`, without the package script's `--fix`) reports 443 errors and one warning across other handwritten files and generated protobuf declarations/module. The three changed handwritten client files pass focused lint. No unrelated lint cleanup was performed. Full lint output is saved at `/tmp/s2c-batching-eslint-full.log`; final Go test/race logs are `/tmp/s2c-batching-go-tests-final.log` and `/tmp/s2c-batching-race-final.log`.
