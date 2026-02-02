# Object Bounds Visualization

## Overview

Added object bounds visualization feature that displays orange borders around game objects when debug info is enabled. The bounds are based on the bounding box data from `S2C_ObjectSpawn` packet.

## Implementation

### Constants

Added in `/src/constants/render.ts`:
```typescript
export const OBJECT_BOUNDS_COLOR = 0xff8800  // Orange color for object bounds
export const OBJECT_BOUNDS_WIDTH = 2         // Line width for object bounds in pixels
```

### ObjectView Changes

- Added `boundsGraphics: Graphics | null` field for storing bounds visualization
- Added `setBoundsVisible(visible: boolean)` method to toggle bounds display
- Added `isBoundsVisible(): boolean` method to check current state
- Added `createBoundsGraphics()` and `removeBoundsGraphics()` private methods
- Added `updateBoundsGraphics()` method that draws rectangle based on object size
- Bounds are drawn in local coordinates relative to object container
- Bounds update automatically when object position changes

### ObjectManager Changes

- Added `setBoundsVisible(visible: boolean)` method to toggle bounds for all objects
- Added `areBoundsVisible(): boolean` method to check if any bounds are visible

### Render Integration

- Bounds visibility is synchronized with debug overlay toggle
- When debug overlay is enabled (`` ` `` key), object bounds are automatically shown
- When debug overlay is disabled, object bounds are hidden
- Initial state respects `config.DEBUG` setting

## Data Flow

1. Server sends `S2C_ObjectSpawn` with bounding box size in `position.size` field
2. Handler extracts size and passes to `ObjectView` constructor
3. `ObjectView` stores size and uses it for bounds visualization
4. When debug mode is enabled, bounds are displayed as orange rectangles
5. Bounds update automatically when objects move

## Usage

- Press `` ` `` (backtick) key to toggle debug overlay and object bounds
- Bounds are only shown when debug overlay is visible
- Bounds appear as orange rectangles with 2px width
- Bounds are drawn around the actual object size from server data

## Technical Details

- Bounds use PIXI.Graphics with stroke style
- Coordinates are in local space relative to object container
- Rectangle spans from `(-size.x/2, -size.y/2)` to `(size.x/2, size.y/2)`
- Bounds are automatically cleaned up when objects are destroyed
- Performance optimized - bounds graphics only created when needed
