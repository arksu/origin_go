# Viewport Culling System

## Overview

Viewport culling system reduces GPU/CPU load by disabling rendering of objects outside the current viewport. The system uses AABB (Axis-Aligned Bounding Box) intersection tests with configurable tile-based margins.

## Architecture

```
ViewportCullingController (singleton)
    ├─ Subchunks culling (map-layer)
    ├─ Terrain culling (objects-layer)
    └─ Objects culling (objects-layer)
         └─ Interactivity toggle (eventMode)
```

## Components

### AABB.ts

Core bounding box utilities:
- `AABB` interface — minX, minY, maxX, maxY
- `intersects(a, b)` — Check if two AABBs overlap
- `expand(rect, dx, dy)` — Expand AABB by margins
- `fromMinMax`, `fromCenter`, `fromPoints` — AABB constructors

### ViewportUtils.ts

Viewport calculation utilities:
- `getViewportRectLocal(container, screenW, screenH)` — Get viewport AABB in container local coords
- `tilesToLocalMargin(tiles)` — Convert tile count to local coordinate margin
- `getCullRect(viewportRect, marginTiles)` — Compute cull rect with margin
- `getHysteresisRects(viewportRect, enterTiles, exitTiles)` — Compute enter/exit rects for hysteresis

### ViewportCullingController.ts

Main controller singleton (`cullingController`):

**Registration:**
- `registerSubchunk(key, container, bounds)` — Register subchunk for culling
- `registerTerrain(key, container, bounds)` — Register terrain sprite
- `registerObject(entityId, container, bounds)` — Register game object

**Update:**
- `update(app, mapContainer, objectsContainer)` — Called every frame after camera update

**Configuration:**
- `setMarginTiles(tiles)` — Set culling margin in tiles (default: 4)
- `setHysteresis(enabled, enterTiles?, exitTiles?)` — Configure hysteresis

**Metrics:**
- `getMetrics()` — Get culling statistics for debug overlay

## Culling Flow

```
1. updateMovement()
2. updateCamera()           ← Transforms are now valid
3. cullingController.update()
   ├─ Compute viewportRectLocal for mapContainer
   ├─ Compute viewportRectLocal for objectsContainer
   ├─ Expand by margin → cullRect (or enter/exit rects for hysteresis)
   ├─ For each subchunk: visible = intersects(bounds, cullRect)
   ├─ For each terrain: visible = intersects(bounds, cullRect)
   └─ For each object: visible = intersects(bounds, cullRect)
       └─ Toggle eventMode for interactivity
4. objectManager.update()   ← Z-sort only visible objects
5. updateDebugOverlay()
```

## Hysteresis

To prevent flickering at viewport edges, hysteresis uses two rects:
- **enterRect** — Larger rect; hidden objects become visible when entering
- **exitRect** — Smaller rect; visible objects hide only when fully outside

Default: `enterMarginTiles = 4`, `exitMarginTiles = 2`

## Bounds Calculation

### Subchunks
Bounds computed at build time from tile coordinates, converted to screen space via isometric projection. Stored in `SubchunkData`.

### Terrain
Bounds computed from sprite position and texture dimensions with padding.

### Objects
Bounds computed from container position with conservative size estimate based on game object size.

## Integration Points

| Component | Action |
|-----------|--------|
| `Chunk.ts` | Computes and stores `SubchunkData` with bounds |
| `ChunkManager.ts` | Registers/unregisters subchunks on load/unload |
| `TerrainSpriteRenderer.ts` | Registers/unregisters terrain sprites |
| `ObjectManager.ts` | Registers/unregisters objects, updates bounds on move |
| `Render.ts` | Calls `cullingController.update()` in game loop |
| `DebugOverlay.ts` | Displays culling metrics |

## Debug Metrics

Displayed in debug overlay (toggle with backtick key):
- `Subchunks: visible/total (culled: N)`
- `Terrain: visible/total (culled: N)`
- `Objects: visible/total (culled: N)`
- `Culling time: X.XXms`

## Performance Notes

- Culling check is O(n) per frame for each category
- AABB intersection is very fast (4 comparisons)
- Hysteresis adds minimal overhead
- Z-sorting optimized to only sort visible objects
- Culled objects have `eventMode = 'none'` to skip hit testing

## Files

| File | Purpose |
|------|---------|
| `AABB.ts` | Bounding box utilities |
| `ViewportUtils.ts` | Viewport and margin calculations |
| `ViewportCullingController.ts` | Main culling controller |
| `index.ts` | Module exports |
