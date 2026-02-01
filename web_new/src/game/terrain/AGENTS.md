# Terrain System

## Overview

Terrain system generates client-side decorative objects (trees, bushes, stones) on top of tiles. Terrain is **not stored on server** — it's generated deterministically on the client.

## Architecture

```
TerrainGenerator (per tile type)
  ↓ generates
TerrainDrawCmd[]
  ↓ rendered by
ITerrainRenderer → TerrainSpriteRenderer (now) / TerrainMeshRenderer (future)
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

1. **Chunk load** → `TerrainManager.generateTerrainForChunk()` creates sprites
2. **Chunk unload** → `TerrainManager.clearChunk()` destroys sprites
3. **Chunk rebuild** → old terrain cleared, new deterministic generation

## Future: VBO Baking

Current: `TerrainSpriteRenderer` creates individual PIXI.Sprite instances

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

## Files

- `types.ts` — TypeScript interfaces
- `TerrainGenerator.ts` — Deterministic generation logic
- `TerrainRegistry.ts` — Type → Generator mapping
- `TerrainManager.ts` — Chunk-level lifecycle management
- `ITerrainRenderer.ts` — Renderer interface
- `TerrainSpriteRenderer.ts` — Sprite-based implementation
- `configs/` — JSON configurations
