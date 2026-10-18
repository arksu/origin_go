# Atlas tools (stdlib-only, no PIL/ImageMagick on this machine)

Pair of scripts for TexturePacker JSON-hash atlases compatible with the game
engine (PixiJS v8 `Spritesheet`, see `web_new/src/game/ChunkManager.ts`). The
project-level overview lives in the [main README](../README.md#2d-tile-atlas-texturepacker).

## Extract an atlas into individual PNGs

```
python3 tools/extract_texturepacker_atlas.py [atlas.json] [sheet.png] [out_dir] [--strip-prefix P]...
# defaults: web_new tiles atlas -> art_source/tiles (frame keys are the
# relative paths under art_source/tiles; use --strip-prefix tiles/ for
# pre-migration atlases whose keys still carry the "tiles/" group prefix)
```

Supports plain, trimmed and rotated frames; every written file is verified
against the atlas pixels.

## Pack a directory into an atlas

```
python3 tools/pack_texturepacker_atlas.py <in_dir> <out_json> <out_png> [flags]
```

Subdirectories become frame-key groups (`sub/sprite.png`). Flags:

| flag | default | meaning |
|---|---|---|
| `--algorithm` | `maxrects` | layout: `maxrects` (BSSF) or `skyline` (bottom-left) |
| `--max-size` | 2048 | sheet is an auto-sized square power of two, never larger |
| `--padding` | 0 | spacing between sprites (kept out of frame rects) |
| `--border-padding` | 0 | empty margin around the sheet edge |
| `--no-rotate` | off | disable 90° rotation |
| `--no-trim` | off | disable transparent-border trimming |
| `--alpha-threshold` | 1 | alpha >= N counts as opaque for trimming |
| `--key-prefix` | — | prefix for frame keys (e.g. `tiles/`) |
| `--key-exclude` | — | comma-separated prefixes exempt from `--key-prefix` |
| `--animations-from-sequences` | off | group `name.NNN.png` into `animations` |
| `--scale` | 1 | written to `meta.scale` (no resampling) |

To rebuild the game tiles atlas from `art_source/tiles` (frame keys are the
relative paths, subdirectory = group):

```
python3 tools/pack_texturepacker_atlas.py art_source/tiles <out.json> <out.png>
```

## Engine conventions (verified against Pixi v8.21 sources)

- `rotated: true` -> texture `rotate: 2`; the on-sheet region is the sprite
  rotated 90° **clockwise**; JSON `frame.w/h` are the sprite's ORIGINAL
  orientation dims — the parser swaps them (`Spritesheet.mjs`);
- `spriteSourceSize` is always in original sprite coords; sprite quads take
  UVs directly from `texture.uvs` (frame corners) and use trim only for
  positions (`DefaultBatcher.packQuadAttributes`, `updateQuadBounds.mjs`);
- full frame field set is always emitted (`trimmed: false` included);
- `aliases` are never emitted (the Pixi parser does not read them);
  duplicate images are packed as separate frames.

## Tests

```
python3 tools/tests/roundtrip_test.py --regression   # pack->extract pixel-exact, determinism, tiles regression
node tools/tests/pixi_semantics_test.mjs             # fixtures through the REAL Pixi parser + sprite UV math
```

`png_codec.py` is the shared stdlib PNG codec (decode/encode/rotate).

## Tile transition review

With the Vite server running in `web_new/`, open `/tests/tile-seams.html`.
The default **All transition pieces** view shows every registered border/corner
mask and every selectable texture variant for the chosen pair of tile types.
The smaller tile ID supplies the overlays; the larger ID supplies the ground.
Each card includes the isolated transparent piece, the piece over that ground,
and an optional 3×3 neighborhood rendered by the game's `Chunk` builder with the
card's exact variant forced at the outlined center tile. The neighbor diagram
uses world X/Y axes, with X across columns and Y down rows.

Use the source path on the card to edit the PNG under `art_source/tiles/`.
Rebuild the published atlas with the pack command above, using
`web_new/public/assets/game/tiles.json` and `web_new/public/assets/game/tiles.png`
as outputs, then click **Reload atlas**. Select **Seam field** to check continuous
boundaries, islands, and three-way junctions. **Pieces** filters borders/corners;
**Piece zoom** enlarges pixels without smoothing.

**Export review sheet** creates one PNG containing every displayed variant from
the real renderer. Use the download link to save the review. To compare a local
candidate before switching the game atlas, pass a same-origin atlas URL, e.g.
`/tests/tile-seams.html?atlas=/art-preview/water-shore-v1/tiles.json`.

Run `npm run test:tile-seams` in `web_new/` to verify complete water-variant
coverage and that all 30 neighborhood fixtures select their labeled masks in
the real chunk renderer. Art/atlas edits still require the pipeline checks above.

The first water-shore revision keeps the original authored interior contours,
textures and all three variants. Only the tile-join band is normalized using a
shared alpha profile averaged from the original borders and corners; a smooth
blend rejoins the untouched interior. Originals, the imagegen edit target and
raw output, prompt, mask recipe and rendered sand review are retained in
`art_source/terrain_revisions/water_shore_v1/`, outside the atlas input directory.
Regenerate the archived candidates with
`python3 tools/export_water_shore_sprites.py --revision water_shore_v1`.
Run `python3 tools/tests/water_shore_test.py` to check shared-edge
continuity, preserved authored interiors and variant differences.

The v2 revision refines the four single-side borders (`b1`, `b2`, `b4`,
`b8`) and gives each six variations. Small asymmetric interior bends soften
long straight cuts; subpixel alpha coverage softens the shore without blurring
RGB texture. The v1 joining bands remain exact, and the other 78 overlays are
unchanged. Its sources, prompt, raw imagegen output and recipe are in
`art_source/terrain_revisions/water_shore_v2/`. Regenerate its 24 source frames
with `python3 tools/export_water_shore_sprites.py --revision water_shore_v2`; `--check` verifies candidate
and published PNGs without writing. After copying candidates into
`art_source/tiles/water/`, repack with the command above. `water.json` registers
all six variants for these four masks, giving 102 water overlays in the gallery.
Run `python3 tools/tests/straight_water_shore_test.py` for join-band, contour
variation, transparency and unchanged-overlay coverage.

The current v3 revision rounds the interior shelves in adjacent-side borders
(`b3`, `b6`, `b9`, `b12`) and single corners (`c1`, `c2`, `c4`, `c8`). It preserves
the exact v1 join bands and the original RGB texture. Imagegen edits a grayscale
coverage sheet; area sampling converts its gray levels to native alpha without
blurring the material. Three existing variants remain distinct for each mask.
The v2 straight borders and the other 54 v1 overlays remain unchanged.
Sources, raw outputs, prompts, mask recipe and rendered reviews are retained in
`art_source/terrain_revisions/water_shore_v3/`. Regenerate its 24 frames with
`python3 tools/export_water_shore_sprites.py`; `--check` verifies the candidates
and published PNGs. Run `python3 tools/tests/curved_water_shore_test.py` for
exact joins, alpha/texture contracts, variant differences and limits on long
horizontal/vertical interior contours at six opacity levels. Add `--report
--source candidate` to print native-pixel contour measurements.

## Player skeleton sprites

`python3 tools/export_skeleton_sprites.py` exports the selected variant 3 source
pair from `art_source/objects/skeleton/` to the client's `obj/skeleton/` assets.
`--check` verifies the source SHA-256 checksums and byte-exact output without
writing files. The export preserves the shared canvas and uses nearest-neighbor
resampling to 85×85, the original 128×128 size divided by 1.5 and rounded to whole pixels.
The resources share abdomen anchor `(43, 37)`; removing the skull never recenters
the remaining body. Separate compact contact shadows offset down/right can be
hidden while carrying. No model, texture or animation assets are rebuilt.

Run the Vite dev server and open `/tests/skeleton.html` to compare the sprites
with standing and lying actors over dark green ground at the same scale.

## Skull inventory icon

`python3 tools/export_skull_icon.py` publishes
`art_source/items/skull/skull-generated.png` as the transparent 32×32
`web_new/public/assets/game/items/skull.png` using nearest-neighbor resampling.
`--check` verifies the pinned source SHA-256 and byte-exact output. The source's
generation prompt and tool provenance are saved alongside it in `generation.json`.
The icon matches the skeleton's warm bone palette, right-facing view and fixed
upper-left lighting. Its alpha edges are checked over dark green terrain.

## Rabbit idle sprites

`python3 tools/export_rabbit_idle_sprites.py` exports the approved rabbit v2
source sheet to `web_new/public/assets/game/animals/rabbit/idle/0.png` through
`7.png`, ordered NE, E, SE, S, SW, W, NW, N. Explicit source regions and
nearest-neighbor sampling produce transparent 64×64 canvases with binary alpha.
All views use one scale: the tallest silhouette is 30 pixels, with other views
retaining their relative heights. Reviewed body-support anchors share the
client ground origin `(32, 48)` without centering the ears or tail. The original
source is retained. No Blender build or atlas publication is required.

`python3 tools/export_rabbit_idle_sprites.py --check` verifies the pinned source
SHA-256, dimensions, silhouette margins and byte-exact outputs without writing.

## Rabbit walk sprites

`python3 tools/export_rabbit_walk_sprites.py` copies the 64 approved walk frames
from `art_source/animals/rabbit/walk_candidate_v1/` into
`web_new/public/assets/game/animals/rabbit/walk/{0..7}/{00..07}.png` without
resampling. Directions are NE, E, SE, S, SW, W, NW, N. Each frame keeps its 64×64
canvas, binary alpha and common `(32, 48)` origin, including the approved
push-off/flight gap. The catalog registers one looping 8 FPS frame layer per
direction and leaves idle resources unchanged.

`python3 tools/export_rabbit_walk_sprites.py --check` verifies the pinned approval
manifest, every candidate SHA-256, frame contracts and byte-exact runtime copies.
The original sheets, prompts and preparation recipe remain with the source.
Open `/tests/rabbit-walk.html` on the Vite server to review all eight published
resources with playback controls and an idle comparison. No Blender or atlas
rebuild is needed for these ordinary frame-layer PNGs.

## Wild Windsown Weed sprites

`python3 tools/export_wild_windsown_sprites.py` publishes the four approved WWW
stages to `web_new/public/assets/game/obj/wild_windsown_weed/{0..3}.png`:
cut stems, young shoots, closed panicles, and the approved taller mature C.
The original imagegen sources and prompts remain in
`art_source/nature/wild_windsown_weed/growth_c_v1/` and `mature_variants_v1/`.

All four 32×40 frames share ground anchor `(16, 36)`. One source sampling grid
retains their relative heights and basal alignment without recentering individual
stages. The client catalog registers `wild_windsown_weed/0` through `/3` as ordinary
interactive PNG resources. No atlas build is needed.

`python3 tools/export_wild_windsown_sprites.py --check` verifies the approved source
hashes, dimensions, alpha, shared ground contact and byte-exact runtime PNGs.
The same exporter publishes `items/wild_windsown_weed.png` as a transparent 30x30
inventory icon, fitting the approved mature frame proportionally with nearest
neighbor sampling. It verifies that icon with the same `--check` command.

## Mulberry tree sprites

`python3 tools/export_mulberry_sprites.py` exports the generated mature mulberry
and matching stump from `art_source/objects/trees/mulberry/candidate_v1/` to
`web_new/public/assets/game/obj/trees/mberry/`. The pinned recipe retains source
hashes, crop regions and reviewed ground anchors. Nearest-neighbor sampling
preserves the selected source RGBA; separate subdued silhouette shadows use the
existing tree shadow-layer contract.

The body canvases are 136×136 with anchor `(67, 114)` and 80×48 with anchor
`(40, 24)`. Their 104×40 and 80×32 shadows use layer offsets `(15, 103)` and
`(0, 12)`. `python3 tools/export_mulberry_sprites.py --check` verifies the original
sources, anchors, transparent margins and byte-exact outputs without writing.
Prompts, raw imagegen outputs and native/enlarged terrain-background reviews
remain beside the recipe. No Blender build or atlas publication is required.
