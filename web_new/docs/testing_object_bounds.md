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
   - Bounds should be 2px wide and orange colored
   - Bounds should match the actual object size from server data

4. **Test movement**
   - Move around the game world
   - Object bounds should follow objects as they move
   - Bounds should stay properly positioned relative to objects

5. **Toggle debug mode**
   - Press `` ` `` again to disable debug overlay
   - Object bounds should disappear along with debug info
   - Press `` ` `` again to re-enable

## Expected Behavior

- ✅ Orange rectangles appear around objects when debug mode is on
- ✅ Rectangles are 2px wide with orange color (0xff8800)
- ✅ Bounds are based on server-provided size data from S2C_ObjectSpawn
- ✅ Bounds update automatically when objects move
- ✅ Bounds are hidden when debug mode is off
- ✅ No performance impact when bounds are disabled

## Troubleshooting

If bounds don't appear:
1. Ensure debug overlay is visible (green text in top-left)
2. Check that objects have size data from server (non-zero values)
3. Verify browser console for any errors
4. Check that OBJECT_BOUNDS constants are properly imported

If bounds appear incorrectly:
1. Verify object size data is correct in network packets
2. Check coordinate calculations in updateBoundsGraphics()
3. Ensure bounds are drawn in local coordinates relative to object container
