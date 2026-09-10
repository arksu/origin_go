# Baked player walk

The server's existing `player` resource now uses the corrected male commoner v3
pixel walk and standing idle in `src/game/objects/misc.json`. The shipped PNG is
`characters/male_commoner/walk_idle.png`, copied from
`art_source/characters/male_commoner_v3/pixel_style/idle/pixel/walk_idle_atlas.png`.
The original eight walk columns are preserved byte for byte; a ninth column holds
one separately posed standing frame per direction. Blender files remain art sources.

- Frame: 80 × 96 px at native scale, ground anchor (40, 84).
- Walk phase follows actual interpolated world displacement. A full cycle covers
  **0.765424409 tiles**, or **9.185092908 world units** with the server's current
  12 units/tile. This is **0.87097651 sprite frames per world unit**.
- The source clip's 8 × 120 ms = 960 ms remains reference metadata. It is not a
  fixed runtime timer. Equivalent playback is `fps = speed * 8 / cycleDistance`.
- Atlas rows: S, SE, E, NE, N, NW, W, SW. ResourceLoader maps these to
  MoveController's NE, E, SE, S, SW, W, NW, N order. No mirroring, so the baked
  light direction remains intact.
- Filtering: nearest, pixel rounding, original outlined palette and transparency.
- Movement/turns follow server positions with client interpolation. Turns preserve
  phase; the walk continues through the slowing visual travel after a server stop.
  Exact arrival displays column 8 (zero-based) in the last direction: both feet planted,
  arms relaxed, no walking sway. This pose is excluded from the walk cycle.
  Restarting begins at frame 0.
- Existing KO rotation remains supported; KO stops the walk. Other movement modes
  currently use the same ordinary walk; no run or crawl animation is authored.
- Movement speed and network protocol are unchanged. MoveController measures its
  displayed displacement after interpolation/smoothing and excludes teleport and
  large-error snaps. Render passes this distance to ObjectManager/ObjectView.
- Fractional travel accumulates through speed changes and turns. Zero displacement
  advances zero frames, regardless of elapsed time. `RenderPosition.isMoving`
  includes the remaining smoothing travel after the server flag becomes false.
  Its decreasing distance advances the walk more slowly; arrival/KO selects idle.
- Once the interpolation target is stationary, damping finishes exactly at the
  target within 1/256 tile (at most 0.18 native pixels). Moving targets before a
  future stop sample are not prematurely settled. Damping uses elapsed time with
  the existing 60 FPS response as reference, so stopping duration is consistent
  across display rates. Restart during settling preserves stride phase.
- Distances use world coordinates, normalized by the negotiated units/tile. Camera
  pan, zoom, carried visual offsets, and screen pixel rounding do not affect phase.
- At the default 32 units/s, the current short walk requires about 27.87 sprite
  frames/s. This synchronizes its stride distance; a natural fast gait would need
  a separately authored run with a longer stride. Eight discrete poses also leave
  quantized foot motion inside each held sprite frame; timing cannot remove it.

`spriteSheet` is an alternative to `img`, `frames` or `spine` on an ObjectView
layer. `directions` gives row order, `frameSize` cell dimensions, `frameCount`
the number of walk columns, `frameDurationMs` reference bake timing, and `idleFrame` the
zero-based standing column. The atlas has `max(frameCount, idleFrame + 1)` columns;
idle may reuse a walk frame or occupy the one additional column. Shared texture slices are owned by ResourceLoader; entity
destruction must not destroy the shared atlas.

`frameDurationMs` describes the original bake tempo for previews.
`cycleDistanceTiles` is the positive locomotion calibration used in the game.
See [measurement and research](player-walk-distance.md) for its derivation.

## Verification

Run `npm run validate-objects` and `npm run build` in `web_new`.
For browser integration checks, run `npm run dev` and open
`http://127.0.0.1:5173/tests/player-walk.html`.
The page tests the real ResourceLoader, ObjectView and ObjectManager, including
all 64 walk and 8 standing crops, distance playback at 30/60/144 render FPS,
variable speeds, load races, destruction, picking, KO culling and actual
MoveController interpolation/teleport handling. The stop regression exercises all
eight directions at 30/60/144 FPS and 12/32 world units per tile, deceleration,
exact arrival, repeated updates, interrupted stops, restart, KO and snaps.
It then displays all eight directions with simulated 10 Hz movement packets
through the real MoveController, world-speed, server stop/start and native/2× zoom
controls. A live readout shows actual client speed and animation FPS. Moving ground
markers show the projected travel under each following camera. It
requires no server/account and is excluded from the production entry point.

## Rebuilding idle

Run Blender in background with `tools/blender/render_male_commoner_idle.py`, then
run `tools/blender/bake_male_commoner_idle.py` with Python/Pillow/NumPy. The idle
uses the corrected walk rig and the same camera, palette and depth/material ink
pipeline. The bake verifies that packing leaves all existing walk pixels unchanged.
Copy `idle/pixel/walk_idle_atlas.png` to the client asset path above.

Run Blender with `tools/blender/calibrate_male_commoner_stride.py` after changing
the gait, bake camera or native sprite scale. It reads the saved pose bones and
camera, then writes `walk/stride_calibration.json`. Update `cycleDistanceTiles`
from that result. The calibration assumes the client's current 64×32 tile diamond.
