# Verification — 2026-09-26

## Automated gates

All passed:

- `GOCACHE=/tmp/origin-go-build go test ./...`
- `GOCACHE=/tmp/origin-go-build go test -race ./internal/core ./internal/game/world ./internal/game ./internal/ecs/systems ./internal/network`
- `GOCACHE=/tmp/origin-go-build go build -o /tmp/origin-plow-mapgen ./cmd/mapgen`
- In `web_new`: `npm run test:actions`, `npm run test:map-click`, `npm run test:chunks`, `npm run type-check`, `npm run build`.
- `openspec validate add-plow-field-action --strict`
- `git diff --check`

The frontend build reports dependency/bundle warnings (protobufjs eval, mixed static/dynamic imports, bundle size); it completes successfully.

## Live gameplay

Built the game server and ran it against a separate PostgreSQL 14 cluster created from `migrations/schema.sql`, with an 8 × 8 grass map. Used a Vite browser client and two protobuf WebSocket test clients. Existing local services and databases were not used.

| Scenario | Observed result |
| --- | --- |
| Select Plow tile through Actions | Menu shows the dig icon; activation arms the action. |
| Click within the player's tile, away from its center | Player moves from `(121,126)` to `(126,126)`, then plows. A separate protocol test moves from `(1033,138)` to `(1038,138)` before execution. |
| Repeat without reselecting | Second browser map click plows another tile. Both changes are received by the second client: chunk versions `1` and `2`, with both plowed bytes retained. |
| Hold an item and retarget during approach | A branch remains in hand during both browser clicks; the new target is plowed and no world drop occurs. |
| Distant target and retargeting | Approach lasts **21.6 seconds**, reaches `(990,138)`, and succeeds. A valid retarget replaces the destination; a disallowed retarget alerts `BAD_TERRAIN` and preserves the approach. |
| Timed effect and stamina | Progress reaches tick `20`; success costs exactly `250`. The action returns to Selecting with cursor `dig`. |
| Remain away at completion | Moving away during execution produces a canceled result, leaves grass unchanged, and leaves stamina at `1000`. |
| Leave and return before completion | Moving three world units away and returning to the center before completion succeeds. |
| Spend the last affordable cost | A cycle starting at exactly `250` finishes at `0` and retains Selecting/`dig`. Another click alerts `LOW_STAMINA` and does not move the player. |
| Obstructed center | A pine stump stops the player at `(1243,138)` before the target center `(1254,138)`. Attempt ends with `ACTION_INVALID_TARGET`; the tile remains grass. |
| Clean server restart | Database contains chunk `(0,0)` at version `6`, with six plowed tiles. A fresh server/client connection restores all six and version `6`; the browser also renders the restored plowed tiles without console errors. |
| AOI exit, server eviction, and re-entry | With active/preload radii `1/2`, LRU capacity `2000`, and test TTL `2s`, the player walks from chunk `3` beyond its preload ring into chunk `6`, then returns. Chunk `(3,0)` is saved at version `2`, evicted, and loaded from DB again. Its unload/reload events have sequences `12` and `26` in the same epoch `2`; both tile edits survive. |

Initial AOI trials used an undersized test configuration (active radius `0`, LRU capacity `4`), which prevented normal migration/bootstrap. The final successful trial above uses the normal active/preload radii and capacity. Production configuration was not changed.

## Persistence and failure cases

`TestChunkPersistencePostgres` passed against the temporary database via `ORIGIN_TEST_DATABASE_URL`. It uses the regenerated sqlc API inside a transaction that rolls back:

- Initial version is `0`; restore at `17` followed by plow saves exactly `18` and the captured LastTick.
- A write at `17` after `18` affects zero rows and cannot replace tiles.
- An equal-version retry at `18` affects one row.

Core/world tests also cover an edit overlapping a save, unchanged snapshot bytes, dirty retention after failure/supersession, delayed retry, and final eviction guards. Handler tests cover all four allowed terrains, missing/disallowed/already-plowed tiles, terrain changes at arrival/completion, failed mutation, and lost affordability. Different CON/max-stamina values still cost exactly `250`.

## Client stream and graphics lifecycle

The seven `test:chunks` tests exercise the actual handlers, Pinia store, ChunkManager, and Pixi Chunk/mesh/geometry/buffer objects. Texture loading and the WebGL capability probe are replaced for Node execution; resource lifecycle is real. Coverage includes:

- Reversed load/unload/reload delivery, duplicate/zero sequences, independent coordinates, old epochs, and uint64 values above `2^53` through the protobuf codec.
- One acceptance decision before store, renderer, and bootstrap; version history survives unload and cache eviction, and resets with the stream.
- Same Chunk and geometry references on an unchanged equal-version reload; higher versions and equal-version cache misses build geometry.
- Neighbor presence/version changes refresh borders while preserving the own version; hidden chunks defer rebuilding and do not extend retention.
- TTL (with an advanced sweep clock) and LRU dispose hidden geometry/buffers/shaders exactly once; active chunks remain retained.
- Unload/reset cancel buffered and queued builds; incomplete builds cannot be reused; reset clears all owned resources.

Movement tests exercise movement → collision → transform → arrival event, including collision-shortened snaps. Client map-click tests cover all three action phases with an empty visual cursor as well as ordinary idle dropping. Existing lift/lift_down regressions pass.

## Deployment

Deploy the server and client bindings together: chunk visibility now requires stream epoch and event sequence. The schema change follows the planned fresh-database contract (`version INT NOT NULL DEFAULT 0`).
