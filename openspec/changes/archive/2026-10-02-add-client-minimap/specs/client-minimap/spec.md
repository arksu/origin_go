# Spec Delta

## Purpose

Help players orient themselves by showing currently available client terrain and their own position in a compact minimap, without accumulating exploration history or requiring server-side minimap features.

## ADDED Requirements

### Requirement: Minimap terrain follows existing chunk availability

The minimap SHALL display terrain within its viewport from active chunks and chunks still retained by the existing client cache. Active terrain SHALL take precedence at matching coordinates. A chunk absent from both sources SHALL produce an empty area. Terrain SHALL disappear on the next minimap frame after its source is removed, even while the player is stationary.

#### Scenario: Active chunk supplies terrain
- **WHEN** an accepted active chunk intersects the minimap viewport
- **THEN** its current tiles SHALL be displayed in their world positions

#### Scenario: Unloaded chunk remains in the existing cache
- **WHEN** a chunk leaves active state but remains in the existing client cache
- **THEN** its terrain SHALL remain available to the minimap until that cache removes it

#### Scenario: Cache eviction occurs without player movement
- **WHEN** the existing cache evicts a non-active chunk while the player remains stationary
- **THEN** the corresponding minimap area SHALL become empty on the next minimap frame

#### Scenario: Active data supersedes a cached version
- **WHEN** active tile data and older cached terrain exist at the same coordinates
- **THEN** the minimap SHALL display the active data

#### Scenario: Unavailable terrain is in the viewport
- **WHEN** a viewport area has no active or retained chunk
- **THEN** the area SHALL remain empty without requesting terrain or restoring previously displayed pixels

### Requirement: Minimap adds no terrain retention

The minimap SHALL NOT accumulate explored-area state, retain chunk data independently of its existing owner, or persist terrain. Reading terrain SHALL NOT extend cache lifetime, change eviction order, mark chunks active, or alter cache limits. Static texture resources and reusable buffers for the current viewport SHALL be allowed; separate chunk-image and zoom caches SHALL NOT be created.

#### Scenario: Repeated rendering does not keep a chunk alive
- **WHEN** the minimap repeatedly reads a retained chunk until its normal expiration or eviction
- **THEN** the chunk SHALL expire or be evicted as it would without minimap reads
- **AND** the minimap SHALL release access to that chunk rather than retain a copy or reference

#### Scenario: Traversal does not accumulate minimap history
- **WHEN** a player crosses many chunks and those chunks leave the existing cache
- **THEN** minimap terrain retention SHALL NOT grow with the player's route
- **AND** no terrain history SHALL be written to browser storage or an export file

### Requirement: Player remains centered independently of the camera

The minimap SHALL follow the player's valid world position and show a direction arrow at its center. Initial placement SHALL use the spawn position without waiting for movement. Subsequent following SHALL use the same smoothed player pose shown in the game. Main-camera pan and zoom SHALL NOT change the minimap center. Before a valid player pose exists, the minimap SHALL remain empty without a false marker at the origin.

#### Scenario: Player spawns away from the origin and stands still
- **WHEN** the player spawns at a nonzero world position and receives no movement update
- **THEN** the first minimap frame with a valid pose SHALL center on that position and show the player arrow there

#### Scenario: Player moves between server updates
- **WHEN** the game interpolates the player's movement
- **THEN** the minimap SHALL follow that smoothed pose and show the corresponding world-facing direction

#### Scenario: Main camera is moved away from the player
- **WHEN** the user pans or zooms the main camera
- **THEN** the minimap SHALL remain centered on the player with its own zoom unchanged

#### Scenario: World entry precedes player spawn
- **WHEN** the new world has not yet supplied a valid player pose
- **THEN** the minimap SHALL show neither previous-world terrain nor an origin-position player marker

### Requirement: Projection has a fixed top-down orientation

The minimap SHALL use a top-down projection with square terrain tiles, world positive X toward the right, and world positive Y toward the bottom. Its orientation SHALL remain fixed while the player turns. The player arrow SHALL use that same coordinate basis. Coordinate conversion SHALL use the world's supplied tile and chunk dimensions, including negative coordinates.

#### Scenario: Player turns
- **WHEN** the player's world-facing direction changes
- **THEN** the arrow SHALL rotate appropriately while terrain orientation remains unchanged

#### Scenario: Terrain crosses negative chunk coordinates
- **WHEN** the viewport spans the world origin or a negative chunk boundary
- **THEN** tiles SHALL appear at their correct positions without shifting, mirroring, or discontinuous coordinate conversion

### Requirement: Accepted terrain updates drive minimap changes

The minimap SHALL reflect terrain only after existing stream, event-order, and tile-version acceptance checks. Accepted replacements SHALL update the displayed terrain. Rejected or duplicate events SHALL NOT change minimap terrain or restore removed chunks. The minimap SHALL preserve the existing rules for active-world terrain lookup and retained render-cache contents.

#### Scenario: A visible tile is plowed
- **WHEN** the client accepts an updated chunk containing a plowed tile
- **THEN** the next minimap frame SHALL show the updated tile

#### Scenario: An obsolete load arrives after eviction
- **WHEN** an old or duplicate load is rejected after a chunk has become unavailable
- **THEN** that chunk SHALL remain absent from the minimap

#### Scenario: A lower tile version is rejected
- **WHEN** an incoming chunk is rejected because its tile version is older than the accepted version
- **THEN** the minimap SHALL retain the currently accepted terrain

### Requirement: Every world transition clears the minimap

Every world entry and reset SHALL clear the displayed terrain, player marker, and access to previous-world chunks before displaying the new world. This SHALL include reconnects, teleports, and transitions between any world layers, including future mines. Deferred rendering or asset completion from before the transition SHALL NOT restore the old world. Matching chunk coordinates across layers SHALL NOT permit terrain reuse.

#### Scenario: Teleport starts a fresh world entry
- **WHEN** a teleport starts a new world entry
- **THEN** the old map and marker SHALL be cleared before destination terrain and the destination player pose are displayed

#### Scenario: Player enters or leaves a mine
- **WHEN** the player moves from the surface into a mine, returns to the surface, or moves between underground layers through the world-transition lifecycle
- **THEN** the minimap SHALL clear for each transition
- **AND** matching coordinates SHALL NOT preserve terrain from the previous layer

#### Scenario: Re-entry has no preceding leave event
- **WHEN** a fresh world-entry snapshot arrives without a preceding leave-world event
- **THEN** the minimap SHALL still clear its previous terrain and marker

#### Scenario: Old asynchronous work completes after transition
- **WHEN** an asset load or scheduled draw started before a world transition completes afterward
- **THEN** it SHALL NOT display terrain or a player pose captured from the previous world

### Requirement: Static base textures cover all renderable terrain

The client distribution SHALL include `minimap_base.png` and `minimap_base.json` with a valid tile-ID-to-rectangle mapping for every renderable terrain type, currently 17. Each base texture SHALL be an opaque 16-by-16 square depicting the terrain material from above, without isometric corners or labels. Each mapped rectangle SHALL lie within the image. The void tile SHALL have no opaque terrain texture.

#### Scenario: Distributed atlas is inspected
- **WHEN** the minimap atlas and its mapping are validated
- **THEN** every renderable tile ID SHALL have one correctly mapped base texture, including plowed terrain and all swamp variants
- **AND** the regions SHALL contain the intended distinct terrain materials

### Requirement: Terrain sampling is anchored in world tile coordinates

At base resolution, one map pixel SHALL represent one world tile. The displayed color SHALL sample that tile type's base texture using nonnegative modulo of its world tile coordinates. Sampling SHALL NOT restart at chunk or viewport boundaries. Unavailable terrain and the void tile SHALL remain transparent within the map image.

#### Scenario: Matching terrain spans a chunk seam
- **WHEN** the same tile type appears across adjacent chunk boundaries
- **THEN** the texture phase SHALL continue according to world tile coordinates without restarting at the seam

#### Scenario: Viewport moves or reaches negative coordinates
- **WHEN** the player moves the viewport over the same world tile or across negative coordinates
- **THEN** that tile SHALL retain the same sampled color at base resolution

#### Scenario: A void tile is present in a known chunk
- **WHEN** a known chunk contains the void tile
- **THEN** that tile SHALL contribute transparent pixels rather than a terrain color

### Requirement: Asset failure uses a complete terrain fallback

If the atlas cannot load or a texture mapping is missing or invalid, the minimap SHALL use a defined fallback color for each affected renderable tile type and SHALL report an actionable diagnostic without blocking the game. The fallback palette SHALL cover all 17 types. Void terrain SHALL remain transparent. Successfully loading valid textures SHALL replace fallback colors using only currently available terrain.

#### Scenario: Atlas loading fails
- **WHEN** the base texture image or mapping fails to load
- **THEN** available terrain SHALL remain visible through fallback colors and the player marker SHALL continue working
- **AND** the failure SHALL be reported without repeated per-frame diagnostics

#### Scenario: One terrain mapping is invalid
- **WHEN** one tile type has an absent or out-of-bounds texture region
- **THEN** that type SHALL use its fallback color without affecting valid regions or the rest of the game

### Requirement: Corner controls affect only the minimap

The minimap SHALL provide discrete zoom controls with a default scale of one pixel per tile. Zoom SHALL keep the player centered, preserve fixed orientation, and show only available terrain. Magnification SHALL preserve crisp tile pixels. Interacting with the minimap SHALL NOT send world clicks, start gameplay actions, move the main camera, or change its zoom. This release SHALL provide no free pan or expanded world-map view.

#### Scenario: Player changes minimap zoom
- **WHEN** the user selects a different supported zoom level
- **THEN** the minimap scale SHALL change while the player remains centered and terrain availability remains unchanged

#### Scenario: User clicks or scrolls over the minimap
- **WHEN** the user interacts with the map surface or its controls
- **THEN** the input SHALL be consumed by the minimap without producing a world command or main-camera operation

### Requirement: Minimap requires no server feature

The minimap SHALL operate using existing world-entry, chunk, and player state supplied to the client. Displaying, zooming, loading texture assets, or clearing the minimap SHALL NOT require new game-server requests, protocol fields, endpoints, or server-side exploration state. The world-entry/reset lifecycle SHALL be the integration boundary for future mine-layer transitions.

#### Scenario: Existing server serves a minimap-enabled client
- **WHEN** the updated client connects to a server that implements the current protocol
- **THEN** the minimap SHALL work from existing game messages and static client assets without a server upgrade
