# Terrain editor nearest-neighbor filtering design

## Goal

Render terrain sprites with nearest-neighbor texture sampling so pixel-art
edges remain crisp at every editor zoom level.

## Design

`TerrainRenderer.loadSpritesheet` will load `tiles.json` with Pixi's
spritesheet `textureOptions.scaleMode` set to `nearest`. Pixi forwards those
options to the atlas image loader, so every frame sharing that atlas uses
nearest-neighbor filtering. This is scoped to the terrain editor and does not
change global Pixi defaults or other web clients.

The editor will retain its current fractional zoom increments. The viewport
can still change continuously; individual texture samples will not blend.

## Verification

Run the terrain editor TypeScript production build. The load call's typed asset
options also guard the expected Pixi v8 API shape.
