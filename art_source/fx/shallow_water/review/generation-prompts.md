# Generation prompts

Tool: built-in image_gen. The selected source is exported to the game's ripple texture by `../export.mjs`.

## Initial generation

Use case: stylized-concept
Asset type: a SINGLE transparent PNG game VFX sprite, shallow-water waist immersion ripple overlay for an isometric pixel-art survival game.
Primary request: ONLY a small subtle broken elliptical ring of water ripples around the waist of a wading character. No character in this asset. The front arc will be drawn over the character at the waterline to hide a cropped lower body.
Input images: Image 1 is a water tile, color reference only; image 2 is the game's character, style and static upper-left lighting reference only. Do not reproduce either reference.
Composition: centered isolated horizontal elliptical ripple, approximately 2:1 width-to-height, viewed at isometric 30 degree camera elevation. One tight inner water-contact rim and two slightly wider incomplete concentric ripples, irregular short arcs. Central opening fully transparent and about one third of the ripple's outer width, so a torso remains visible. Surroundings, gaps between arcs and all center pixels must be genuinely transparent alpha, not a painted checkerboard. Keep generous transparent padding.
Style: restrained crisp pixel art, like a hand-pixelled sprite with effective native footprint approximately 64x32 pixels enlarged with nearest-neighbor pixels. Small coherent clusters, hard pixel edges, no smooth gradients, no anti-aliased vector rings.
Palette: deep muted blue and dark teal matching the supplied blue water tile, sparse lighter desaturated blue highlights; upper-left arcs get stronger highlights, right and lower-right dimmer. Thin little wave crests, no pure white thick foam.
Constraints: ripple surface is flat, low amplitude water disturbance. Strongest inner foreground rim is continuous enough to conceal a straight crop seam at the character waist, with irregular 1-2 native pixel edges. Outer arcs are subtle and interrupted. No opaque filled water disk, no background patch, no legs, no body, no person, no shadow, no lettering, no labels, no border, no watermark. True RGBA transparency. This is a texture for overlay rendering, not a scene or sheet.

## Selected revision

Use case: precise-object-edit
Asset: transparent shallow-water ripple game sprite.
Edit the supplied ripple sprite. Keep its blue pixel art style and true transparent background. Tighten the INNER contact ring dramatically: central transparent opening must be only 25% of the full outer ripple width. Add one thin tight elliptical water-contact rim around that small opening, with a prominent but thin front lip (1-2 native pixels) that can cover a character waist crop. Retain two very sparse interrupted outer rings, each separated by transparent space. Make the overall full ripple approximately 2:1 width-to-height, flat isometric plane viewed at 30 degrees. Use small crisp pixel clusters, approximately 64x32 effective native pixels upscaled. Keep water turbulence subtle and close to the body. Reduce the bright right-side highlight substantially; the light comes only from SCREEN UPPER LEFT. No pure white foam. All gaps, the central hole and outside must remain fully transparent alpha. Remove distant isolated noise pixels. One sprite only, no character, no water disk, no shadow, no background, no checkerboard, no words.

## Dense knee-depth revision v2

Input image: `ripples-v1.png`, edit target. Output: `ripples-v2.png`.

Use case: precise-object-edit. Asset type: one transparent pixel-art water-ripple sprite for an isometric RPG, rendered over the character's legs at knee depth. Edit the attached ripple texture: preserve the blue palette, the horizontal concentric elliptical shape, transparent center and transparent background, and static light from the UPPER LEFT ONLY. Make the wave crests substantially thicker, denser, brighter and much more clearly visible on dark teal-blue water. Add detailed clustered pale cyan foam, short choppy wavelet ridges, small bubbles and several sparkling droplets; dense readable pixel clusters rather than thin sparse dashes. Four overlapping irregular elliptical wave crests, strong inner contact rim, extra short outward arcs and broken foam speckles within the compact ellipse. Deep blue lower/right sides, bright muted icy-blue/cyan upper-left crests; no pure white matte. Keep the central elliptical opening genuinely transparent so the legs remain visible above the waterline. The inner near-side crest should be immediately below the vertical center of the sprite, to meet the character's knee-level crop at that center. Composition: a SINGLE centered complete horizontal ripple, about 2:1 overall width-to-height, compact with modest transparent margins, intended to downsample to an 80x40 game sprite while keeping thick 2-3 pixel crests and clear detail. Use true RGBA transparency, including all background and the center opening. No opaque water surface, character, ground, cast shadow, text, labels, grid, checkerboard, frame or sprite sheet. Single image, single frame.
