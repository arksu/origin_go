# Testing Object Bounds Visualization

## How to Test

1. **Start the application**
   - Open http://localhost:5175 (or the network URL shown in terminal)
   - Login and enter the game world

2. **Enable debug mode**
   - Press the backtick key ( `` ` `` ) to toggle debug overlay
   - You should see debug information in the top-left corner

3. **Verify object bounds**
   - When debug overlay is visible, orange rectangles should appear around all objects
   - Rectangles should be 2px wide and orange colored
   - **Important**: Bounds should appear as **isometric shapes** (diamond-like), not axis-aligned rectangles
   - Bounds should match the actual object size from server data in game coordinates

4. **Test movement**
   - Move around the game world
   - Object bounds should follow objects as they move
   - Bounds should maintain proper isometric shape during movement
   - Bounds should stay properly positioned relative to objects

5. **Test different object sizes**
   - Observe objects with different sizes from server
   - Larger objects should have proportionally larger bounds
   - Bounds should accurately reflect the game coordinate dimensions

6. **Toggle debug mode**
   - Press `` ` `` again to disable debug overlay
   - Object bounds should disappear along with debug info
   - Press `` ` `` again to re-enable

## Expected Behavior

- ✅ **Isometric orange rectangles** appear around objects when debug mode is on
- ✅ Rectangles are 2px wide with orange color (0xff8800)
- ✅ Bounds are **diamond-shaped** (isometric projection), not axis-aligned
- ✅ Bounds are based on server-provided size data from S2C_ObjectSpawn
- ✅ Bounds correctly represent game coordinate dimensions in isometric view
- ✅ Bounds update automatically when objects move
- ✅ Bounds are hidden when debug mode is off
- ✅ No performance impact when bounds are disabled

## Visual Verification

The bounds should look like:
```
    /\
   /  \  ← Isometric diamond shape
   \  /
    \/
```

NOT like:
```
+----+  ← Axis-aligned rectangle (incorrect)
|    |
+----+
```

## Troubleshooting

If bounds don't appear:
1. Ensure debug overlay is visible (green text in top-left)
2. Check that objects have size data from server (non-zero values)
3. Verify browser console for any errors
4. Check that OBJECT_BOUNDS constants are properly imported

If bounds appear incorrectly shaped:
1. Verify isometric transformation in updateBoundsGraphics()
2. Check that coordGame2Screen() is working correctly
3. Ensure 4 corners are calculated in game coordinates first
4. Verify local coordinate transformation

If bounds appear wrong size:
1. Check server data for object size values
2. Verify game coordinate calculations are correct
3. Ensure bounds use game coordinates, not pixel coordinates

If bounds don't follow movement:
1. Check that updateBoundsGraphics() is called in updatePosition()
2. Verify position updates are working correctly
3. Ensure container position is updated before bounds
