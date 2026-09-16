# Player ground-shadow texture

The player resource already defines `obj/player_.png` as a dedicated shadow layer below the 3D actor. Add the missing 80×40 px transparent PNG at that path, without changing the loader or object configuration.

Use a soft, dark elliptical contact shadow with padding on every side. Keep the mass subtly biased toward the lower-right so it follows the game's fixed upper-left light direction. Verify the alpha edge over dark green terrain to ensure no white or checkerboard matte is visible.
