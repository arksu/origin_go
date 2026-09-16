# Palette-aware 3D actor outline

The 3D actor pixel pass currently emits fixed near-black outer outlines and
fixed black or brown inner outlines. Replace both ordinary outline colors with
a darker variation of the adjacent actor colors so the silhouette reads as
painted pixel art rather than as a black post-process border.

For each outline pixel, inspect up to two nearest opaque neighboring pixels.
Cardinal neighbors take precedence, with diagonals only filling an unavailable
sample. Convert samples through the existing actor palette. When both samples
belong to the same encoded material region, average their palette colors;
when they span regions, use a deterministic dominant sample instead of
blending unrelated materials. Apply one fixed darkening factor to that chosen
color.

Use this rule for the transparent outer outline and for opaque pixels that the
existing depth/region rules classify as an inner outline. Preserve the yellow
hover outline, existing 128x128 native grid, alpha-coded region/height data,
and the one-pass GPU rendering design. Verify with the hybrid character review
page and the production TypeScript build.
