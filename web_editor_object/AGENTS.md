# AGENTS.md

## Project Overview

Terrain Editor is a Vue 3 + Pinia + PixiJS v8 application for editing terrain configuration JSON files. It renders terrain layers using a spritesheet and allows interactive offset adjustments.

## Architecture

### Component Layout

```
┌─────────────────┬─────────────────────┬──────────────────┐
│   ObjectList    │     RenderView      │ LayerHierarchy   │
│   (Left 240px)  │    (Center flex)      │  (Right 280px)   │
│                 │                     │                  │
│ - File list     │ - PixiJS canvas     │ - Layer list     │
│ - Variant list  │ - Zoom/pan          │ - Visibility     │
│                 │ - Selection         │ - Save button    │
└─────────────────┴─────────────────────┴──────────────────┘
```

### Key Files

| File | Purpose |
|------|---------|
| `src/engine/TerrainRenderer.ts` | PixiJS rendering, zoom, crosshair, hit testing |
| `src/stores/terrainStore.ts` | State: selection, offsets (Record), visibility, save |
| `src/components/RenderView.vue` | Canvas mounting, keyboard handlers, watchers |
| `vite.config.ts` | Save API middleware, path resolution |
| `src/api/saveTerrainApi.ts` | POST wrapper for save endpoint |

## Critical Implementation Details

### Reactivity Pattern

Vue 3's `watch` with `deep: true` doesn't detect Record mutations properly. The store uses:

```typescript
const renderVersion = ref(0)
// Mutations increment this counter
// RenderView watches renderVersion to trigger redraw
```

### Layer Offset Storage

```typescript
// Key format: `${fileIndex}:${variantIndex}:${layerIndex}`
layerOffsetsMap.value['0:2:5'] = { dx: 3, dy: -1 }
```

### Zoom/Transform Math

```typescript
// Container is scaled around center using pivot
container.pivot.set(cx, cy)
container.position.set(cx, cy)
container.scale.set(scale)  // 1-16 range

// Hit testing uses toLocal() for coordinate transform
const local = container.toLocal({ x: canvasX, y: canvasY })
```

### Save Process

1. Build modified config by applying editor offsets to layer offsets
2. POST to `/__api/save-terrain` (Vite middleware)
3. Server resolves symlink and writes to actual file
4. Store clears editor offsets (they're now in the base config)

## Symlinks

```
src/terrain/wald.json  → web_new/src/game/terrain/configs/wald.json
src/terrain/heath.json → web_new/src/game/terrain/configs/heath.json
public/assets/game/    → web_new/public/assets/game/
```

## Common Tasks

### Adding a new terrain file

1. Create symlink in `src/terrain/`
2. Restart dev server (import.meta.glob is compile-time)

### Modifying save behavior

Edit `terrainSavePlugin()` in `vite.config.ts`. The middleware:
- Validates fileName (no `..` or `/`)
- Resolves symlink with `fs.realpathSync()`
- Writes formatted JSON

### Adding new render features

Modify `TerrainRenderer.ts`:
- Add graphics to `container` for world-space objects
- Add graphics to `app.stage` for screen-space UI
- Update `renderVariant()` to create/position sprites

## Type Definitions

```typescript
// src/types/terrain.ts
interface TerrainLayer {
  img: string        // spritesheet frame ID
  offset: number[]   // [x, y]
  p: number          // probability (editor ignores)
  z?: number         // z-index
}

interface TerrainVariant {
  chance: number
  offset: number[]
  layers: TerrainLayer[]
}

type TerrainConfig = TerrainVariant[]
```

## Debugging Tips

- Check browser console for `[TerrainEditor]` error logs
- Verify symlinks with `ls -la src/terrain/`
- Test save API with: `curl -X POST -d '{}' http://localhost:5174/__api/save-terrain`
- PixiJS devtools extension useful for scene inspection

## Dependencies

Lockfile pinned to match `web_new`:
- vue: ^3.5.27
- pinia: ^3.0.4
- pixi.js: ^8.16.0
- pixi-spine: ^4.2.101 (installed, unused)

## Notes for Future Agents

- **Do not** use `Map` for reactive state - use `Record` with string keys
- **Always** account for `container.scale` in hit testing
- **Crosshair** is drawn in screen space (not scaled)
- **Save button** is disabled when no unsaved changes (computed `hasUnsavedChanges`)
- **Zoom steps** are 0.25 increments for smooth feel
- Default zoom is 4x for better visibility of pixel art
