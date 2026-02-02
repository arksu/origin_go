# Object Bounds Visualization

## Overview

Added object bounds visualization feature that displays orange borders around game objects when debug info is enabled. The bounds are based on the bounding box data from `S2C_ObjectSpawn` packet and are rendered in **isometric projection** to match the game world.

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
- Added `updateBoundsGraphics()` method that draws **isometric rectangle** based on object size
- **Isometric projection**: Bounds are calculated by converting 4 corners from game coordinates to screen coordinates
- Bounds update automatically when object position changes

### Isometric Projection Algorithm

1. Calculate 4 corners of bounding box in **game coordinates**:
   ```
   (x - width/2, y - height/2)  ← top-left
   (x + width/2, y - height/2)  ← top-right  
   (x + width/2, y + height/2)  ← bottom-right
   (x - width/2, y + height/2)  ← bottom-left
   ```

2. Transform each corner to **screen coordinates** using `coordGame2Screen()`

3. Convert to **local coordinates** relative to object container

4. Draw connected lines between corners to form isometric rectangle

### ObjectManager Changes

- Added `setBoundsVisible(visible: boolean)` method to toggle bounds for all objects
- Added `areBoundsVisible(): boolean` method to check if any bounds are visible

### Render Integration

- Bounds visibility is synchronized with debug overlay toggle
- When debug overlay is enabled (`` ` `` key), object bounds are automatically shown
- When debug overlay is disabled, object bounds are hidden
- Initial state respects `config.DEBUG` setting

## Data Flow

1. Server sends `S2C_ObjectSpawn` with bounding box size in **game coordinates**
2. Handler extracts size and passes to `ObjectView` constructor
3. `ObjectView` stores size and uses it for bounds visualization
4. When debug mode is enabled, bounds are displayed as **isometric orange rectangles**
5. Bounds update automatically when objects move
6. **Isometric transformation** ensures bounds match visual object size on map

## Usage

- Press `` ` `` (backtick) key to toggle debug overlay and object bounds
- Bounds are only shown when debug overlay is visible
- Bounds appear as **isometric orange rectangles** with 2px width
- Bounds are drawn around the actual object size from server data
- **Isometric shape** shows real object dimensions on the game map

## Technical Details

- Bounds use PIXI.Graphics with stroke style
- **Game coordinates** → **Screen coordinates** → **Local coordinates** transformation
- Rectangle spans from game coordinate bounds, transformed to isometric view
- Bounds are automatically cleaned up when objects are destroyed
- Performance optimized - bounds graphics only created when needed
- **Zero-size protection**: Objects with 0 width/height skip bounds rendering
