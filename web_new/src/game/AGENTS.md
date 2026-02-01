# Game Rendering System

## Architecture Overview

Game rendering is built on PixiJS v8 with a strict separation between Vue UI and Pixi render. All game logic lives in `game/` directory.

```
Vue Component (GameView.vue)
    ↓ calls
GameFacade — Single entry point for Vue
    ↓ owns
Render — Main Pixi Application
    ├─ mapContainer — Ground tiles, borders, corners
    ├─ objectsContainer — Players, objects, terrain (Z-sorted together)
    └─ uiContainer — In-game debug overlay
```

## Core Components

### GameFacade.ts

**Role**: Isolation layer between Vue and Pixi. Only file Vue imports from `game/`.

**Key Methods**:
- `init(canvas)` — Initialize Pixi application
- `destroy()` — Cleanup all resources
- `loadChunk(x, y, tiles)` — Load tile data for chunk
- `spawnObject(options)` — Create game object
- `updateObjectPosition(entityId, x, y)` — Update object visual position
- `screenToWorld(x, y)` / `worldToScreen(x, y)` — Coordinate conversion

### Render.ts

**Role**: Owns Pixi Application and game loop.

**Containers**:
| Container | Content | Sorting |
|-----------|---------|---------|
| `mapContainer` | Ground tiles (VBO meshes) | `sortableChildren: true` |
| `objectsContainer` | Players, objects, terrain sprites | `sortableChildren: true` |
| `uiContainer` | Debug overlay | Manual |

**Update Loop** (per frame):
1. `MoveController.update()` — Get interpolated positions
2. `ObjectManager.update()` — Update sprites, Z-sort if needed
3. Camera update — Apply pan/follow/zoom
4. Debug overlay refresh

## Subsystems

### Chunk Rendering

```
ChunkManager
    ├─ Chunk[] (per chunk)
    │   └─ Subchunks (4x4 grid per chunk)
    │       └─ PIXI.Mesh (VBO for ground tiles)
    └─ TerrainManager
        └─ Terrain sprites in objectsContainer
```

**Chunk.ts**:
- Builds 16 subchunks (DIVIDER=4) per chunk
- Each subchunk = one VBO mesh with ground + borders + corners
- Returns `ChunkBuildResult` with `hasBordersOrCorners[][]` for terrain exclusion

**ChunkManager.ts**:
- Manages chunk lifecycle (load/unload/rebuild)
- Rebuilds neighbor chunks for smooth borders
- Triggers terrain generation after tile build

### Object Rendering

```
ObjectManager
    ├─ Map<entityId, ObjectView>
    └─ PIXI.Container (in objectsContainer)

ObjectView
    └─ PIXI.Container with sprite(s)
```

**Z-Sorting**: Objects sorted by Y coordinate every frame when positions change.

### Movement Interpolation

```
MoveController (singleton)
    ├─ Map<entityId, MoveState>
    │   ├─ keyframes: MoveKeyframe[] (server positions + timestamps)
    │   ├─ renderPosition: interpolated visual position
    │   └─ streamEpoch, moveSeq: sync validation
    └─ timeSync: estimate server time from Ping/Pong
```

**Interpolation**:
- `renderTime = serverNow - interpolationDelay`
- Interpolate between keyframes A and B where `A.t <= renderTime <= B.t`
- Extrapolate up to `maxExtrapolationMs` if buffer underrun

See `MoveController.ts` AGENTS.md for details.

### Input & Camera

**InputController.ts**:
- Pointer events (click, drag, wheel)
- Drag threshold for pan vs click distinction
- Modifier keys (Shift/Ctrl/Alt) tracking

**CameraController.ts** (singleton):
- `follow` mode — Smooth follow player with damping
- `pan` mode — Middle-mouse drag
- Zoom with wheel

**PlayerCommandController.ts** (singleton):
- Converts input to `C2S_PlayerAction` messages
- `MoveTo(x, y)` for ground clicks
- `MoveToEntity(entityId)` for object clicks

### Terrain System

See `terrain/AGENTS.md` for full documentation.

**Key Points**:
- Client-only decorative objects (trees, bushes)
- Deterministic generation via `getRandomByCoord()`
- Lives in `objectsContainer` for Z-sorting with players
- Not generated on tiles with borders/corners

### Resource Management

**TileSet.ts**:
- Tile type → ground/border/corner texture mappings
- Weighted random selection

**tileSetLoader.ts**:
- Registers all tile types on init
- Links terrain configs to tile types

## Coordinate Systems

```
Game Coords (server)     Screen Coords (Pixi)
    x,y ──────────────→ screenX,screenY
    (0,0 is origin)      (isometric projection)
         │
         │  camera.x,y
         ▼
    Render-time world position
```

**Conversion Functions** (`utils/coordConvert.ts`):
- `coordGame2Screen(x, y)` — Game → Screen (isometric)
- `coordScreen2Game(screenX, screenY)` — Screen → Game

## Utils

- `VertexBuffer.ts` — Float32Array builder for VBO geometry
- `random.ts` — Deterministic LCG (`getRandomByCoord()`)
- `Coord.ts` — Coord type alias

## Data Flow Example

1. Server sends `S2C_ChunkLoad { x, y, tiles }`
2. `gameStore` receives → `gameFacade.loadChunk(x, y, tiles)`
3. `Render.loadChunk` → `ChunkManager.loadChunk`
4. `ChunkManager`:
   - Creates/updates `Chunk`
   - `chunk.buildTiles()` builds VBO meshes
   - `TerrainManager.generateTerrainForChunk()` adds sprites
5. `Render.update()` displays chunk on next frame

## Files

| File | Purpose |
|------|---------|
| `GameFacade.ts` | Vue ↔ Pixi interface |
| `Render.ts` | Main render loop, containers |
| `Chunk.ts` / `ChunkManager.ts` | Tile mesh generation |
| `ObjectView.ts` / `ObjectManager.ts` | Game object rendering |
| `MoveController.ts` | Movement interpolation |
| `InputController.ts` | Mouse/keyboard input |
| `CameraController.ts` | Camera follow/pan/zoom |
| `PlayerCommandController.ts` | Input → network commands |
| `DebugOverlay.ts` | FPS, metrics display |
| `TileSet.ts` / `tileSetLoader.ts` | Tile type definitions |
| `ResourceLoader.ts` | Asset loading |
| `terrain/` | Terrain generation system |
| `utils/` | Helper utilities |
| `tiles/` | JSON tile configurations |
| `types.ts` / `index.ts` | Types and exports |
