# Earthen tower kiln on a 2×2 footprint

Replace the earlier mound kiln with a small, primitive earthen tower based on the three supplied photographs. Its rough hand-built clay walls lean inward toward a dark open top; a small dark firing aperture sits low on the visible front face. The sprite has neither flames nor smoke, which may be supplied by separate FX. Match the game's fine pixel-art texture density and keep the silhouette readable at native size.

The game projects each tile as a 64×32 px diamond, so the tower's ground base spans 128×64 px for a 2×2 footprint. Generate the art in Higgsfield with the photographs as references, prepare a transparent 128 px wide sprite, and align its base over that diamond. Resize the contact shadow to the same footprint. Change the kiln collider from 36×36 to 24×24 world units because a tile is 12 world units per axis. Other objects and gameplay behavior remain outside this change.

Verify sprite alpha, visual anchor, and shadow dimensions; validate the object definitions and build the client. Preserve unrelated working-tree changes.
