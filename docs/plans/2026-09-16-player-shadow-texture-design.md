# Player ground-shadow texture

The player resource defines `obj/player_.png` as a dedicated shadow layer below the 3D actor. Add the missing 80×40 px transparent PNG at that path. The 3D actor branch must also build ordinary resource layers before creating the actor output; otherwise it returns before loading the configured shadow.

Use a soft, dark elliptical contact shadow with padding on every side. Keep the mass subtly biased toward the lower-right so it follows the game's fixed upper-left light direction. The normal layer z-order keeps its `shadow: true` layer below the actor. Verify the alpha edge over dark green terrain to ensure no white or checkerboard matte is visible, and type-check the client.
