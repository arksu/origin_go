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
