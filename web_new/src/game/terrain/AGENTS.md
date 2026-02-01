# Terrain System

## Overview

Terrain system generates client-side decorative objects (trees, bushes, stones) on top of tiles. Terrain is **not stored on server** — it's generated deterministically on the client.

**Key optimizations (chunk_terrain_opt.md):**
- Subchunk-based incremental building with frame budget
- Sprite pooling to avoid GC pressure
- Visibility radius with hysteresis
- Optimized zIndex calculation using `anchorScreenY`
- No individual culling registration (bulk cleanup only)

## Architecture

```
TerrainManager (singleton)
    ├─ TerrainBuildQueue (priority queue with frame budget)
    │   └─ TerrainBuildTask[] (sorted by distance to camera)
    ├─ TerrainSpriteRenderer
    │   └─ TerrainSpritePool (sprite reuse)
    └─ TerrainMetricsCollector (debug stats)

TerrainGenerator (per tile type)
  ↓ generates
TerrainDrawCmd[]
  ↓ rendered by
TerrainSpriteRenderer (with pooling)
  ↓ adds to
objectsContainer (shared with game objects for correct Z-sorting)
```

## Configuration Format

### TerrainLayer

```typescript
interface TerrainLayer {
  img: string       // Texture frame ID from spritesheet
  offset: number[]  // [x, y] offset from anchor
  p: number         // Probability / generation mode
  z?: number        // Optional Z-index offset
}
```

### Probability Parameter `p`

| Value | Behavior |
|-------|----------|
| `p: 0` | Always generated (typically shadows). Shadow layer with `p:0` forces retry loop until at least 2 layers generated. |
| `p: N` | Generated if `seed % N === 0`. Higher N = lower probability (1/N chance). |

Example:
```json
{
  "img": "terrain/heath/1/image_5.png",
  "offset": [-25, 12],
  "p": 0    // Shadow - always generated
},
{
  "img": "terrain/heath/1/image_0.png",
  "offset": [0, 0],
  "p": 5    // 20% chance (1 in 5 tiles)
}
```

### TerrainVariant

```typescript
interface TerrainVariant {
  chance: number      // 1/chance probability for this variant
  offset: number[]    // Anchor point adjustment
  layers: TerrainLayer[]
}
```

Each tile type can have multiple variants. First variant with matching `chance` is selected.

## Deterministic Generation

Generation uses `getRandomByCoord(x, y, z?, seed?)` — LCG-based seeded random:

```typescript
// Same coordinates → same result every time
const seed = getRandomByCoord(tileX, tileY)
if (seed % variant.chance === 0) {
  // Generate this variant
}
```

This ensures:
- **Rebuild gives same result** — unloading and reloading chunk produces identical terrain
- **No state synchronization needed** — server doesn't track terrain

## Z-Order Integration

Terrain sprites live in `objectsContainer` (same as players/objects) for correct depth sorting:

```typescript
// @TerrainSpriteRenderer.ts
sprite.zIndex = BASE_Z_INDEX + screenY + TILE_HEIGHT_HALF + layer.zOffset
```

Objects with higher Y (lower on screen) render on top.

## Border/Corner Exclusion

Terrain **not generated** on tiles with borders or corners (transition textures). This prevents visual clutter on tile type boundaries.

```typescript
// @Chunk.ts
if (hadBordersOrCorners) {
  hasBordersOrCorners[x][y] = true  // Mark for terrain exclusion
}
```

## Lifecycle

1. **Chunk load** → `TerrainManager.generateTerrainForChunk()` enqueues subchunk build tasks
2. **Frame update** → `TerrainManager.update()` processes tasks within budget
3. **Visibility update** → Subchunks outside radius are hidden (sprites returned to pool)
4. **Chunk unload** → `TerrainManager.clearChunk()` returns sprites to pool
5. **World reset** → `TerrainManager.resetWorld()` clears all and shrinks pool

### Subchunk States

| State | Description |
|-------|-------------|
| `NotBuilt` | Initial state, waiting in queue |
| `Building` | Currently being built |
| `BuiltVisible` | Built and within show radius |
| `BuiltHidden` | Built but outside hide radius (sprites in pool) |

### Visibility Radius (Hysteresis)

- `TERRAIN_VISIBLE_WIDTH_SUBCHUNKS = 7` — Show radius width (3.5 subchunks on each side)
- `TERRAIN_VISIBLE_HEIGHT_SUBCHUNKS = 7` — Show radius height (3.5 subchunks on each side)
- `TERRAIN_HIDE_WIDTH_SUBCHUNKS = 7.5` — Hide radius width (3.75 subchunks on each side)
- `TERRAIN_HIDE_HEIGHT_SUBCHUNKS = 7.5` — Hide radius height (3.75 subchunks on each side)
- Hysteresis prevents flickering at boundary: show when entering visible rect, hide when leaving hide rect

## Sprite Pooling

`TerrainSpritePool` manages sprite lifecycle:

```typescript
// Acquire from pool or create new
const sprite = terrainSpritePool.acquire(textureFrameId, x, y, zIndex)

// Return to pool (not destroyed)
terrainSpritePool.release(sprite)
```

**Configuration** (`constants.ts`):
| Constant | Default | Description |
|----------|---------|-------------|
| `MAX_TERRAIN_SPRITES_IN_POOL` | 20000 | Max pooled sprites |
| `TERRAIN_POOL_SHRINK_THRESHOLD` | 30000 | Shrink trigger |
| `TERRAIN_POOL_SHRINK_TARGET` | 20000 | Shrink target |

## Incremental Building

`TerrainBuildQueue` processes subchunks within frame budget:

**Configuration**:
| Constant | Default | Description |
|----------|---------|-------------|
| `TERRAIN_BUILD_BUDGET_MS` | 2 | Max ms per frame |
| `MAX_TERRAIN_SUBCHUNKS_PER_FRAME` | 4 | Max subchunks per frame |

**Priority**: Tasks sorted by distance to camera (nearest first).

**Cancellation**: Epoch tokens ensure stale builds are ignored after chunk unload.

## zIndex Optimization

Uses `anchorScreenY` directly instead of calling `coordGame2Screen()`:

```typescript
// Before (slow)
const screenPos = coordGame2Screen(context.tileX, context.tileY)
sprite.zIndex = BASE_Z_INDEX + screenPos.y + TILE_HEIGHT_HALF + cmd.zOffset

// After (fast)
sprite.zIndex = TERRAIN_BASE_Z_INDEX + context.anchorScreenY + TILE_HEIGHT_HALF + cmd.zOffset
```

## Viewport Culling

Terrain uses **visibility-based culling** (not individual sprite registration):

**How it works:**
1. `TerrainManager.setCameraPosition()` updates camera position every frame
2. `TerrainManager.updateSubchunkVisibility()` checks each subchunk against visibility rects:
   - **Show**: If subchunk enters visible rect → rebuild sprites immediately
   - **Hide**: If subchunk leaves hide rect → remove sprites from container and return to pool
3. `TerrainSpriteRenderer.hideSubchunk()` removes sprites from `objectsContainer` before pooling

**Benefits:**
- Avoids overhead of registering/unregistering thousands of individual sprites with culling controller
- Bulk cleanup is more efficient than per-sprite operations
- Hysteresis prevents flickering at visibility boundaries

## Future: VBO Baking

Current: `TerrainSpriteRenderer` creates individual PIXI.Sprite instances with pooling

Future: `TerrainMeshRenderer` will batch terrain into VBO meshes (like ground tiles):
- Collect `TerrainDrawCmd[]` into `VertexBuffer`
- Single `PIXI.Mesh` per subchunk
- One draw call for all static terrain

Interface `ITerrainRenderer` abstracts both approaches.

## Registered Types

| Tile Type | Terrain Config | Description |
|-----------|---------------|-------------|
| 13 (wald) | `configs/wald.json` | Forest vegetation |
| 15 (leaf) | `configs/wald.json` | Leafy forest |
| 17 (grass) | `configs/heath.json` | Grassland features |
| 18 (heath) | `configs/heath.json` | Heathland vegetation |

## Metrics

Available via `terrainManager.getMetrics()` and debug overlay:

- `spritesActive` / `spritesPooled` / `spritesCreatedTotal`
- `subchunksQueued` / `subchunksDone` / `subchunksCanceled`
- `buildMsAvg` / `buildMsP95`
- `clearMsAvg` / `clearMsP95`

## Files

| File | Purpose |
|------|--------|
| `constants.ts` | Configuration constants |
| `types.ts` | TypeScript interfaces |
| `TerrainSubchunkTypes.ts` | Subchunk state and task types |
| `TerrainGenerator.ts` | Deterministic generation logic |
| `TerrainRegistry.ts` | Type → Generator mapping |
| `TerrainManager.ts` | Subchunk lifecycle, visibility, queue processing |
| `TerrainSpriteRenderer.ts` | Sprite-based implementation with pooling |
| `TerrainSpritePool.ts` | Sprite object pool |
| `TerrainBuildQueue.ts` | Priority queue with frame budget |
| `TerrainMetricsCollector.ts` | Metrics aggregation |
| `ITerrainRenderer.ts` | Renderer interface |
| `configs/` | JSON configurations |
