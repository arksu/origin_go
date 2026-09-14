# Primitive kiln sprite on a 3×3 footprint

The early-game kiln should retain the reference's hand-shaped clay mound, small firebox, and simple top vent. Avoid brickwork, masonry, metal, and a manufactured chimney.

The game projects a tile as a 64×32 px diamond. A 3×3 tile footprint therefore spans 192×96 px on screen. Generate the sprite with Higgsfield using the approved primitive kiln and an in-game screenshot as references, then prepare a transparent, crisp pixel-art PNG whose base is centered over that footprint. Match the surrounding art's fine pixel density at native display size, rather than using broad color blocks. Do not include smoke in the sprite; smoke is a separate FX. Update the kiln resource offset and shadow to match the new image. Keep the existing 36×36 world-unit collider: at 12 units per tile it already occupies 3×3 tiles.

Verify the PNG alpha bounds and resource positioning against a 192×96 px isometric footprint. Run the client asset/schema check relevant to the resource change. Leave unrelated worktree files untouched.
