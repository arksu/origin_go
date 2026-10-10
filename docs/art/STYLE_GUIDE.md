# Origin visual style

The user approved this style on **2026-10-10** as the default for future Origin
game-art generation and edits. Apply it unless the user explicitly requests a
different style. It covers Origin scenes, characters, animals, objects, resources
and terrain, rather than unrelated image requests.

## Approved reference

The canonical reference is [Origin forest camp v1](style/origin-forest-camp-v1.png).
Visually inspect the image before generating or editing art. It shows an
isometric forest clearing with four adult players, a linen tent, a campfire,
wooden storage and a workbench, a boar, loose resources and a rocky stream.

![Approved Origin forest camp](style/origin-forest-camp-v1.png)

This is an art-direction reference. Its image dimensions, composition and
individual object sizes are not universal production sprite specifications.
Existing asset definitions and the requested deliverable determine projection,
pixel density, frame dimensions, scale, direction order and ground anchors.

## Visual language

- **Isometric, stylized naturalism.** Use believable anatomy and materials with
  clear silhouettes. Human characters have adult proportions; animals retain
  recognizable species anatomy rather than mascot proportions.
- **Textured pixel art.** Build volume and material detail from deliberate pixel
  clusters. The approved forest has substantial foliage, bark, grass and stone
  texture; do not reinterpret it as flat, minimalist art. Keep edges crisp and
  avoid random isolated noise that weakens the shape at gameplay scale.
- **Muted natural palette.** Favor moss and olive greens, deep forest shadows,
  earth browns, ochre, warm stone gray and linen beige. Use darker desaturated
  browns for outlines. Reserve stronger warm accents for sources such as fire
  and small material details; choose subject colors that belong in this world.
- **Matte materials.** Distinguish fur, rough wood, worn cloth, leather and stone
  through their shapes and clustered shading. Highlights remain restrained.
- **Upper-left illumination.** Use coherent soft daylight from the upper-left,
  with readable light-facing planes and grounded shadows. Local light such as a
  campfire may add a limited warm accent when the requested scene includes it.
- **Restrained outlines.** Use selective dark edges and contact shadows where
  needed for separation, rather than a thick black outline around every detail.
- **Gameplay readability.** Characters, animals, tools and loose resources must
  remain recognizable at the intended camera scale. Terrain texture supports
  them; important shapes must not disappear into similarly colored foliage.

Avoid glossy plastic or polished 3D rendering, chibi proportions, oversized cute
eyes, neon colors, heavy bloom, blur, smooth painted gradients, and photographic
detail with a pixelation filter. Preserve the reference's natural, grounded
survival atmosphere.

## Reference and edit procedure

1. Read this guide and inspect the canonical approved image.
2. Inspect the requested asset and its current source or candidate. Establish
   which reference defines the **edit target** (subject identity and asset
   contract) and which defines the **style** (the approved forest camp).
3. Use the built-in imagegen tool for raster generation and image edits. Include
   both references when editing, and explicitly identify their roles in the
   prompt. Do not ask the generator to replace the target subject or copy the
   camp composition when only a sprite redraw is requested.
4. For sprite edits, preserve the task's subject identity, direction ordering,
   relative scale, frame layout, ground anchor and transparency unless the user
   explicitly requests a change. Ask for a transparent background when the
   deliverable requires alpha; a drawn checkerboard is not transparency.
5. Retain original sources and the tool's original output. Save revisions as
   separate, clearly versioned candidates with their prompt and reference paths
   rather than overwriting earlier candidates or the approved style reference.
6. Inspect the result against the approved style and the asset contract. Review
   sprites at their intended gameplay size as well as enlarged. Generation alone
   does not establish frame, anchor, alpha or animation correctness.

Creating or redrawing art does not authorize publishing it into runtime
catalogs, changing game definitions or replacing the existing asset pipeline.
Production integration follows the relevant asset workflow in
[docs/assets/README.md](../assets/README.md) and [tools/README.md](../../tools/README.md).

## Reusable prompt fragment

Replace the bracketed fields with the task's actual requirements. Attach the
approved image as a style reference, and the current sprite as an edit target
when applicable.

> Render [subject] in Origin's approved isometric, stylized naturalistic pixel-art
> style, matching the attached forest-camp style reference. Use believable
> anatomy, adult human proportions where applicable, matte materials, muted
> moss/olive/earth/stone/linen colors, upper-left illumination, restrained dark
> outlines and deliberate textured pixel clusters. Keep crisp pixels and a clear
> silhouette at gameplay scale. Preserve the target asset's [projection, scale,
> frame dimensions, direction order, ground anchor and transparency]. The target
> reference defines the subject and layout; the forest-camp reference defines
> the visual treatment. Avoid chibi proportions, glossy 3D surfaces, neon colors,
> bloom, blur, smooth gradients and photographic pixelation filters. Deliver
> [requested frames or image] with [required background/alpha].
