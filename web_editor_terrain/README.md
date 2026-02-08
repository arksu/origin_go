# Terrain Editor

A standalone Vue 3 SPA for editing terrain configurations with real-time PixiJS rendering.

## Features

- **Visual Terrain Editing**: View terrain layers rendered with PixiJS
- **Layer Management**: Toggle visibility, select layers, view hierarchy
- **Interactive Offset Adjustment**: Move layers with arrow keys or mouse drag
- **Zoom & Pan**: Mouse wheel zoom (1x-16x) with smooth scaling
- **Center Crosshair**: Visual guide for the (0,0) anchor point
- **Save to JSON**: Persist offset changes back to terrain config files
- **Live Reload**: HMR support for development

## Tech Stack

- **Vue 3** (v3.5.27) - Composition API, `<script setup>`
- **Pinia** (v3.0.4) - State management with reactive maps
- **PixiJS** (v8.16.0) - WebGL rendering, spritesheet loading
- **TypeScript** - Strict type checking
- **Vite** - Build tool, dev server, HMR

## Project Structure

```
web_editor_terrain/
├── src/
│   ├── components/
│   │   ├── ObjectList.vue      # Left panel: terrain files/variants
│   │   ├── LayerHierarchy.vue  # Right panel: layers, visibility, save button
│   │   └── RenderView.vue      # Center: PixiJS canvas
│   ├── engine/
│   │   └── TerrainRenderer.ts  # PixiJS rendering logic
│   ├── stores/
│   │   └── terrainStore.ts     # Pinia store (offsets, visibility, selection)
│   ├── loaders/
│   │   └── terrainLoader.ts    # JSON loading via import.meta.glob
│   ├── api/
│   │   └── saveTerrainApi.ts   # Save API client
│   ├── types/
│   │   └── terrain.ts          # TypeScript interfaces
│   └── terrain/               # Symlinks to web_new terrain configs
├── public/assets/game/        # Symlinks to spritesheet assets
└── vite.config.ts             # Vite plugin for save API
```

## Symlinks

Terrain configs and assets are symlinked from `web_new`:

```bash
src/terrain/*.json → ../../../web_new/src/game/terrain/configs/*.json
public/assets/game/tiles.* → ../../../../web_new/public/assets/game/tiles.*
```

## Getting Started

```bash
cd web_editor_terrain
npm install
npm run dev
```

Dev server runs at http://localhost:5174

## Usage

1. **Select a terrain file** from the left panel (e.g., "wald", "heath")
2. **Select a variant** to view its layers
3. **Click a layer** to select it (green border appears)
4. **Move layers**: 
   - Arrow keys: 1px nudge
   - Mouse drag: free movement
5. **Toggle visibility**: Click the eye button (👁/—)
6. **Zoom**: Mouse wheel (1x-16x range)
7. **Save**: Click "Save" button to persist offset changes to JSON

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| Arrow Keys | Move selected layer 1px |

## Save API

The editor includes a Vite dev server middleware (`/__api/save-terrain`) that writes modified terrain configs back to the source JSON files via their symlinks.

## Data Flow

```
Terrain JSON (web_new) ←── symlink ──→ src/terrain/*.json
                                     ↓
                         import.meta.glob (Vite)
                                     ↓
                         terrainStore (Pinia)
                                     ↓
                         + editor offsets
                                     ↓
                         TerrainRenderer (PixiJS)
                                     ↓
                         Canvas rendering
```

On save:
1. Editor offsets are added to layer offsets
2. Modified config is POSTed to `/__api/save-terrain`
3. Server writes to real file (resolving symlink)
4. Editor offsets are cleared from state

## Development Notes

- **Reactivity**: Store uses `renderVersion` counter to trigger canvas redraws (Vue 3 deep watchers don't track Record mutations well)
- **Hit Testing**: Uses `container.toLocal()` to handle zoom/pan transforms correctly
- **Scale**: Default 4x zoom, mouse wheel changes by 0.25 steps
- **Crosshair**: Drawn in screen space (zIndex 999999), not affected by zoom

## License

Part of origin_go project.
