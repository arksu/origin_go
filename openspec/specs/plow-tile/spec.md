# plow-tile Specification

## Purpose

Let players prepare eligible ground for future farming by repeatedly targeting individual tiles with the Actions menu's Plow tile action.

## Requirements

### Requirement: Plow tile changes one tile per completed attempt

The server SHALL provide a data-defined `plow_tile` action labeled Plow tile, with a tile target, the `dig` cursor, a 20-tick execution, a fixed cost of 250 stamina units, and repeat mode enabled. It SHALL have no skill or equipment requirement. Each successful completed attempt SHALL change exactly its targeted tile to Plowed Field. One click SHALL start at most one attempt; further tiles SHALL require further clicks after the current attempt ends.

#### Scenario: Target tile becomes plowed
- **WHEN** a Plow tile attempt completes for a valid target
- **THEN** that tile SHALL be plowed, no other tile SHALL change, and exactly 250 stamina units SHALL be charged

#### Scenario: Action appears in Actions
- **WHEN** a player enters the world and opens Actions
- **THEN** Plow tile SHALL be available for activation with the dig cursor and repeat mode

#### Scenario: One click does not plow neighboring tiles
- **WHEN** a player clicks one eligible tile and its attempt completes
- **THEN** no neighboring tile SHALL change without another click and completed attempt

### Requirement: Plowing filters terrain without an object-occupancy check

The server SHALL accept a target tile only when its current terrain is coniferous forest, broadleaf forest, thicket, or grass. Already-plowed and all other terrain SHALL be rejected with a terrain mini-alert. A tile whose chunk is not loaded SHALL be treated as unavailable rather than as ineligible terrain. Target validation SHALL use the tile coordinate derived from the click and SHALL NOT reject solely because an object is present. Physical colliders MAY prevent arrival at the tile center through ordinary movement; an object that does not block the center SHALL NOT block plowing by an occupancy rule. Terrain SHALL be rechecked immediately before the tile changes.

#### Scenario: Eligible terrain is accepted
- **WHEN** the player targets coniferous forest, broadleaf forest, thicket, or grass
- **THEN** the attempt SHALL proceed toward that tile's center

#### Scenario: Ineligible terrain is refused
- **WHEN** the player targets sand, swamp, mountain, clay, or paved ground
- **THEN** the server SHALL send a terrain mini-alert, SHALL NOT start movement for that click, and SHALL leave Plow tile armed

#### Scenario: Unloaded tile is unavailable
- **WHEN** the player targets a tile whose chunk is not loaded
- **THEN** the server SHALL send a target-unavailable alert, SHALL NOT start movement for that click, and SHALL leave Plow tile armed

#### Scenario: Already-plowed tile is refused
- **WHEN** the player targets a plowed tile
- **THEN** the server SHALL alert and SHALL leave Plow tile armed without changing the tile

#### Scenario: Object presence does not reject a tile
- **WHEN** an eligible tile has an object that does not prevent the player from reaching its center
- **THEN** the server SHALL allow the attempt and SHALL not return an occupied-tile rejection

#### Scenario: Another action changes the tile before completion
- **WHEN** the selected tile is no longer eligible at the completion check
- **THEN** the attempt SHALL fail without changing another tile or charging action stamina

### Requirement: Plowing begins after reaching the target tile center

A click SHALL identify exactly one tile, regardless of its position within that tile or any clicked object's ID. The player SHALL move to the selected tile's center even when already inside the tile boundaries but away from the center. The 20-tick cycle SHALL begin only after the player's actual position reaches the center within the movement arrival tolerance. Collision, inability to move, cancellation, and approach timeout SHALL not produce a tile effect or action stamina charge. Immediately before the effect, the player's actual position SHALL be within movement arrival tolerance of that center; being outside that tolerance SHALL fail the attempt without effect or charge. Leaving and returning within tolerance during the cycle SHALL be allowed, provided all other completion checks pass.

#### Scenario: Distant target
- **WHEN** the player clicks an eligible tile beyond the current position
- **THEN** the player SHALL approach the tile center before the plow cycle starts

#### Scenario: Same-tile target away from center
- **WHEN** the player clicks the eligible tile they stand in while away from its center
- **THEN** they SHALL approach the center before the cycle starts

#### Scenario: Blocked center
- **WHEN** a collider prevents the player from reaching the center
- **THEN** no plow cycle SHALL start and the tile SHALL remain unchanged

#### Scenario: Player is away from the center at completion
- **WHEN** the player is outside movement arrival tolerance of the target center at the completion check
- **THEN** the attempt SHALL end without a tile change or action stamina charge

#### Scenario: Player leaves and returns before completion
- **WHEN** the player leaves the target center during the 20-tick cycle and returns within movement arrival tolerance before the completion check, with eligible terrain and sufficient stamina
- **THEN** the attempt SHALL plow the target tile and charge exactly 250 stamina units

### Requirement: Plowing charges a fixed stamina amount only on success

Each completed plow SHALL charge exactly 250 stamina units, independent of maximum stamina and character attributes. The server SHALL require at least 250 stamina units before beginning an attempt and immediately before changing its tile. An insufficient-stamina attempt SHALL alert, SHALL NOT move or change the tile, and SHALL leave the cursor armed if the action was already selected. A failed or canceled approach or cycle SHALL charge no action stamina. Ordinary movement stamina costs MAY still apply to walking toward the tile.

#### Scenario: Fixed cost
- **WHEN** players with different maximum stamina values each complete a plow
- **THEN** each SHALL lose exactly 250 stamina units for the action

#### Scenario: Insufficient stamina while armed
- **WHEN** a player with less than 250 stamina units clicks an eligible tile while Plow tile is armed
- **THEN** the server SHALL alert without moving for that click, charging action stamina, changing the tile, or disarming the cursor

#### Scenario: Stamina becomes insufficient before completion
- **WHEN** stamina falls below 250 during an approach or cycle
- **THEN** that attempt SHALL fail without changing the tile or charging action stamina, while Plow tile remains armed

### Requirement: Plowed tiles persist and propagate

A plowed tile SHALL enter the existing dirty-chunk save path and survive a clean server shutdown and restart after the chunk is saved. A save SHALL capture tile bytes, Version, and LastTick under the same read lock and SHALL store the captured Version with its tiles; the database SHALL reject a write whose Version is lower than the stored one. A rejected or failed save SHALL leave the chunk dirty, SHALL retain it in the chunk manager, and SHALL schedule a later retry without a tight retry loop. A successful save SHALL clear tilesDirty only when the saved Version still matches the current Version and the write was accepted. Loading a chunk SHALL restore its stored Tiles and Version without incrementing the version. Loss of AOI interest SHALL NOT evict a chunk with unsaved tile changes.

The server SHALL notify clients currently viewing the chunk of its updated tiles without requiring a relog or map reload. Every chunk-load producer, including initial loads, AOI loads, and plow updates, SHALL use the same snapshot accessor that copies tile bytes and reads Version under one read lock. Snapshot bytes SHALL remain unchanged after capture.

The client SHALL keep a per-coordinate version guard for the whole world stream; it SHALL survive chunk unload and server eviction because the persisted version no longer restarts. After stream and chunk-event order checks pass, a payload with a Version lower than the highest applied for that coordinate SHALL be rejected before updating either the store or the renderer. An equal Version after a reload describes unchanged tiles and SHALL reuse retained ready Chunk geometry when its built Version and neighbor inputs match; if no matching ready geometry remains, the client SHALL accept the payload and build from it. A greater Version SHALL rebuild the chunk from the payload. A world-stream reset SHALL clear all version guards and cached terrain.

#### Scenario: Other players see the new tile
- **WHEN** a plow completes while another player views the same chunk
- **THEN** that player's client SHALL receive and render the updated chunk tiles

#### Scenario: Consecutive plows retain the latest tiles
- **WHEN** two plows update the same viewed chunk in succession
- **THEN** viewers SHALL end with both tiles plowed

#### Scenario: Initial chunk load overlaps a plow
- **WHEN** an initial or AOI chunk-load snapshot is captured while a plow changes the chunk
- **THEN** its tile bytes and Version SHALL describe the same state, and later mutation SHALL not change those snapshot bytes

#### Scenario: Chunk changes during an in-flight save
- **WHEN** a plow changes a chunk after a save snapshot is captured, and all viewers leave before that save finishes
- **THEN** the changed chunk SHALL remain dirty and registered until a later save successfully writes its current tile version

#### Scenario: Stale save does not overwrite newer tiles
- **WHEN** a save captured from an older tile version reaches the database after a newer version was already stored
- **THEN** the database SHALL reject it by Version, the server SHALL keep the chunk dirty, and a later retry SHALL store the current version

#### Scenario: Failed save does not discard the plow
- **WHEN** saving a dirty chunk fails and the chunk has no AOI viewers
- **THEN** the server SHALL retain the dirty chunk and schedule a delayed retry instead of evicting its unsaved tiles

#### Scenario: Reload after eviction reuses the cache when unchanged
- **WHEN** the client unloads a chunk, the server saves and evicts it, and the player returns within the same world stream to unchanged tiles and neighbor inputs while the ready client Chunk is retained
- **THEN** the client SHALL accept the payload with its equal Version and reuse the existing Chunk ground geometry without rebuilding it

#### Scenario: Reload after eviction rebuilds on a higher version
- **WHEN** the chunk content changed between unload and reload within the same world stream
- **THEN** the client SHALL rebuild the chunk from the payload with the greater Version

#### Scenario: Fresh world stream accepts its first chunk
- **WHEN** the client enters a new world stream after a server restart
- **THEN** its cleared guards SHALL allow the first chunk payload regardless of versions used in the previous stream

#### Scenario: Plowed tile survives a clean restart
- **WHEN** a changed chunk is saved and the server restarts cleanly
- **THEN** the tile SHALL still be plowed after loading the chunk

### Requirement: Client cache retains ready Chunk instances with bounded resource lifetime

The client SHALL retain fully built Chunk instances and their owned ground geometry after an accepted unload, hiding them and removing their active culling entries while preserving cache metadata. An incomplete build SHALL NOT count as reusable ready geometry. After a load passes stream, event-sequence, and tile-Version checks, a ready Chunk with matching built Version and current neighbor inputs SHALL be shown again without rebuilding its ground geometry. Missing or evicted ready geometry SHALL be built from the accepted payload even when its Version equals the retained guard. Active gameStore tiles SHALL reflect the accepted payload in both cases. Decorative terrain MAY retain its existing clear/regenerate lifecycle independently of ground geometry reuse.

The client SHALL track neighbor presence and tile Version for the inputs used to build borders/corners and SHALL refresh dependent geometry when those inputs change, even if the own tile Version is unchanged. A border refresh MAY rebuild the whole Chunk using the existing builder, but SHALL preserve its actual tile Version. Hidden chunks SHALL be marked for later refresh rather than rebuilt in the background.

Retained hidden ready chunks SHALL be bounded by the existing TTL/LRU settings. Active visible chunks SHALL NOT be removed by cache eviction. TTL/LRU eviction SHALL cancel pending work, destroy the owned Chunk graphics, remove the instance from the manager, and remove its metadata through one disposal path; it SHALL preserve the event-sequence and tile-version acceptance guards. A new world stream SHALL cancel pending work, destroy all active and retained Chunk instances, and clear metadata and both guards.

#### Scenario: Equal-version reload after client cache eviction
- **WHEN** TTL or LRU has removed the ready client Chunk and a newer valid load event arrives with its unchanged tile Version
- **THEN** the client SHALL accept the payload and build its geometry again

#### Scenario: Neighbor changes while own tiles are unchanged
- **WHEN** a retained Chunk is reloaded with the same own tile Version but a neighbor used for borders/corners has a different presence or tile Version
- **THEN** the client SHALL refresh dependent geometry and SHALL preserve the own tile Version

#### Scenario: TTL or LRU releases hidden graphics
- **WHEN** a retained hidden Chunk is evicted by TTL or LRU
- **THEN** its graphics and metadata SHALL be removed together exactly once, its acceptance guards SHALL remain, and active visible chunks SHALL remain available

#### Scenario: Unfinished build is not reused
- **WHEN** a Chunk unloads before its initial geometry build completes and is later loaded again
- **THEN** the obsolete build SHALL NOT restore it and the later accepted payload SHALL be built before its geometry is treated as ready

#### Scenario: Fresh stream releases retained graphics
- **WHEN** the client starts a new world stream
- **THEN** graphics and metadata from all active and retained chunks in the previous stream SHALL be cleared along with pending work and acceptance guards

### Requirement: Chunk visibility follows authoritative event order

Every chunk-load and chunk-unload packet SHALL carry the captured world-stream epoch and a nonzero uint64 event sequence. The server SHALL allocate sequences from one monotonically increasing counter per client's world stream at event creation, ordered with authoritative AOI transitions and load snapshot capture before asynchronous dispatch. Initial loads, AOI loads/unloads, and plow updates SHALL use that same creation path. The counter SHALL NOT reset on chunk unload or server eviction, and asynchronous workers SHALL NOT allocate or replace the captured sequence. The dispatcher SHALL recheck the captured epoch immediately before client enqueue.

The client SHALL compare sequences losslessly and SHALL retain the last accepted event sequence per coordinate for the whole world stream. It SHALL reject packets from another epoch, with zero sequence, or with an older/equal sequence before any store, renderer, or bootstrap side effect. Loads SHALL additionally pass the tile-Version guard. An accepted unload SHALL advance the sequence guard, unload active state, and cancel obsolete buffered loads and pending builds; it SHALL preserve both event and tile-version guards and any retained render cache. Deferred work SHALL NOT restore a chunk after a newer unload or world reset. A new world stream SHALL clear the guards and pending work. If a chunk visibility packet cannot enter the client's send queue, the server SHALL end that connection so reconnection starts a fresh world stream rather than silently losing the transition.

#### Scenario: Delayed load cannot undo an unload
- **WHEN** a chunk load is created before a later unload but arrives after that unload within the same world stream
- **THEN** the client SHALL reject the older load and SHALL keep that chunk unloaded, even when its tile Version equals the retained version

#### Scenario: Delayed unload cannot hide a reloaded chunk
- **WHEN** a chunk unload is created before a later reload but arrives after that reload within the same world stream
- **THEN** the client SHALL reject the older unload and SHALL keep the reloaded chunk visible, even when the reload's tile Version is unchanged

#### Scenario: Duplicate event has no side effects
- **WHEN** a chunk event is delivered again with its already accepted sequence
- **THEN** the client SHALL NOT repeat its store, render, or bootstrap effects

#### Scenario: Chunk coordinates have independent acceptance guards
- **WHEN** a valid event for chunk A arrives after an event for chunk B with a greater sequence
- **THEN** the client SHALL judge A against A's last accepted sequence and SHALL NOT reject it solely because B's sequence is greater

#### Scenario: Old stream cannot affect the current world
- **WHEN** an event from a previous world stream arrives after a new stream starts
- **THEN** the client SHALL reject it regardless of its event sequence or tile Version

#### Scenario: Buffered load cannot restore an unloaded chunk
- **WHEN** a chunk load is buffered while rendering is unready and a newer unload is accepted before that load can apply
- **THEN** later renderer initialization SHALL NOT build or show the unloaded chunk from that obsolete load

#### Scenario: Send-queue overflow forces fresh synchronization
- **WHEN** the client's send queue cannot accept a chunk visibility packet
- **THEN** the server SHALL end that connection and a subsequent connection SHALL initialize a fresh world stream

### Requirement: Plow tile remains armed for repeated clicks

After each completed or failed plow attempt, Plow tile SHALL return to target selection with the dig cursor, including when current stamina is insufficient for another attempt. Its selecting, approaching, and executing states SHALL take priority over dropToWorld for primary map clicks when an inventory item is in hand. It SHALL perform no automatic additional cycle and SHALL not queue clicks made during an active cycle. Explicit cancellation or activating another action SHALL end the armed mode.

#### Scenario: Consecutive plows without re-selection
- **WHEN** a player finishes plowing tile A, then clicks eligible tile B
- **THEN** tile B SHALL start its own approach and cycle without re-selecting Plow tile from Actions

#### Scenario: Held item does not prevent approach retargeting
- **WHEN** a player holding an inventory item clicks eligible tile B while approaching tile A for Plow tile
- **THEN** the client SHALL send MapClick without dropToWorld, the approach SHALL retarget to tile B, and the item SHALL remain in hand

#### Scenario: Stamina loss does not disarm
- **WHEN** a completed plow leaves less than 250 stamina units
- **THEN** the dig cursor SHALL remain armed and a later eligible click SHALL alert until enough stamina is available

#### Scenario: Explicit cancellation ends plowing
- **WHEN** the player cancels Plow tile
- **THEN** the action SHALL become idle, its cursor SHALL reset, and no unfinished tile SHALL change
