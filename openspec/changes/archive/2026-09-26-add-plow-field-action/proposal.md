# Proposal

## Why

Players need a way to prepare ground for future farming. Add a repeatable, tile-targeted **Plow tile** action to the existing Actions menu so one click starts one attempt to turn one eligible tile into Plowed Field.

## What Changes

- Add the `plow_tile` action with the existing `dig` cursor asset registered in the client cursor catalog, a 20-tick cycle, and a fixed cost of 250 stamina units per successful tile. It has no skill or equipment requirement.
- Accept only coniferous forest (20), broadleaf forest (25), thicket (30), and grass (35). Reject other terrain and tiles already plowed. Do not inspect objects when validating a tile; ordinary movement collisions determine whether the player can reach its center.
- Convert the clicked world position to a tile coordinate. Walk to that tile's center and start the cycle only after arrival at the center is confirmed from the player's actual position. Revalidate the tile and player position immediately before changing one tile; leaving and returning to the center during the cycle is allowed. A failed approach or cycle changes no tile and costs no action stamina.
- Keep the action armed after each attempt, including when stamina is temporarily insufficient. Each new tile requires another click and a completed cycle; cancellation or switching actions ends the armed mode.
- Give an active target action priority over dropping a hand-held inventory item on the client. Primary map clicks remain MapClick commands while the action is selecting, approaching, or executing, so an item in hand does not prevent target selection or approach retargeting.
- Add an opt-in tile-center approach mode to action definitions. Existing tile actions, including `lift_down`, retain their current behavior.
- Persist the edited tile through the existing chunk save path together with its tile-content version: the database stores the saved Version and rejects older-version writes (`chunk.version` becomes `NOT NULL DEFAULT 0`), and chunk load restores the stored Version without resetting it. Retain and retry dirty chunks after failed or superseded saves. Use one atomic Tiles + Version snapshot for initial chunk loads, AOI loads, and updates sent to current viewers.
- Add `stream_epoch` and `event_seq` to both chunk-load and chunk-unload messages. Assign the sequence at event creation in authoritative state order, before asynchronous dispatch; the client rejects old-stream, duplicate, and older events per coordinate, retaining this guard across unloads.
- Reject stale chunk updates per coordinate for the whole world stream: the persisted version survives unload and eviction. Retain ready client Chunk instances after unload and reuse their ground geometry when the received Version and neighbor inputs match. Rebuild from the payload when the version is higher or the ready chunk was evicted; refresh dependent geometry when neighbors changed. Bound hidden chunks with the existing TTL/LRU settings, release their graphics and metadata together on eviction, and reset all guards and retained chunks on a new world stream.

## Capabilities

### New Capabilities

- `plow-tile`: activation, tile eligibility, center approach, fixed stamina cost, one-tile effect, persistence, visibility, and repeatable targeting for Plow tile.

### Modified Capabilities

- `game-actions`: opt-in center approach for tile-targeted actions, repeatable armed-state handling while stamina is insufficient, and client priority for active target actions over hand-item dropping.

## Impact

- **Server:** action definition target validation, ActionService tile approach and repeat state, movement arrival reporting after collision resolution, plow handler, chunk tile write, persistent chunk versions with compare-and-set writes (`chunk.version` becomes `NOT NULL DEFAULT 0`, sqlc regen, and mapgen supplies initial Version 0 and checks the affected-row result), a shared atomic tile snapshot for all chunk-load producers, safe save retries before eviction, and sequenced AOI updates.
- **Client:** the action uses the existing Actions menu; register `dig` in the cursor catalog using its existing asset; active target actions take priority over dropToWorld; guard chunk-event order and tile versions per world stream before store/render updates, cancel obsolete buffered loads, and reuse ready Chunk geometry when versions and neighbor inputs match. Couple TTL/LRU eviction of hidden chunks to graphics disposal and metadata removal; accept equal-version reloads after cache eviction by building from the payload.
- **Protocol:** extend S2C_ChunkLoad and S2C_ChunkUnload with `stream_epoch` and `event_seq`, and regenerate the Go and client bindings. The action catalog and map-click protocol retain their existing format.
- **Data:** `data/actions/plow_tile.json`.
