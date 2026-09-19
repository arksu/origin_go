# Verification — 2026-09-19

## Automated checks

- `go test ./...` — passed.
- `go build ./cmd/load_test` — passed.
- `npm --prefix web_new run type-check` — passed.
- `npm --prefix web_new run test:map-click` — 2 tests passed.
- `npm --prefix web_new run build` — passed, including browser protobuf generation. Non-fatal warnings: protobufjs eval, mixed static/dynamic network imports, large output chunks.

## Manual checks reported by the user

The user tested movement, item pickup, and administrator commands and reported correct behavior with no bugs found. In a follow-up, the user explicitly confirmed right-click interaction, `/info` and `/destroy` on empty ground or disappeared targets, and that administrator clicks do not also move or pick up items. These are user-run checks, not agent-run browser verification.

## Lifecycle verification

- Focused tests cover pending replacement, empty/missing targets, player destruction rejection, and cleanup isolation.
- `TestDeathClearsAllPendingAdminClicksForOnlyDeadPlayer` invokes the production death cleanup and verifies all four pending resources are cleared for the dead player and preserved for another player.
- Transfer and disconnect cleanup were verified by code inspection: both invoke the tested `ecs.ClearPendingAdminClicks` under the shard lock. Disconnect's stale-socket guard runs before cleanup, protecting a replacement session. No database-backed transfer/disconnect integration test was run.
- The destroy handler still invokes its existing persistence deletion and `lifecycle.DeleteObject` with `DeleteOwnedInventories: true`; existing deletion tests pass in the full suite.
- `go test ./...` passed again after adding the death regression test.

## Deployment

Deploy client and server together: `map_click = 5` replaces retired wire actions 1 and 2. Old clients cannot control movement on the updated server; reserved legacy actions are ignored. Regenerate browser protobuf bindings when building the client.
