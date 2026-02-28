# Image Pixel Processor

Watches an `input` directory and processes image files through this pipeline:

1. Validate transparency ratio.
2. Remove background with `rembg` when transparency is not meaningful (`--bg-model auto` by default).
3. If `rembg` mask is too soft, fallback to deterministic edge-connected background removal.
4. Detect frame rows/columns as a rectangular grid using a bright-spot method (adaptive downscale + blurred mass peaks, with fallback strategies).
5. Run Unfaker for pixel-art style conversion.
6. Save resulting PNG files into `output` and delete source from `input` only after full success.

Splitter supports both transparent spritesheets and fully opaque backgrounds (for example solid black) by estimating edge-connected background regions before grid detection.

## Strict Parity Mode (jenissimo)

`unfaker.py` is now aligned to strict core-flow parity with `jenissimo/unfake.js` (target commit `2b4e5d8`) for:

- stage order in `processImage`
- scale detection flow (`runs` / `edge` / `auto`)
- edge detect method selection (`tiled`/`legacy`)
- dominant threshold default (`0.15`)
- color limit default (`32`, without auto-detect by default)

See [PARITY_TARGET.md](/Users/park/projects/origin_go/tools/image-pixel-processor/PARITY_TARGET.md).

## Operational Wrapper Mode

Watcher and pipeline (`watcher.py`, `pipeline.py`) may keep operational safety behavior (file stability checks, failure handling, output degradation fallback). This is outside strict `unfaker.py` core parity.
`watcher.py` now builds Unfaker command internally (`<tool>/.venv/bin/python <tool>/unfaker.py`, fallback to current Python interpreter if venv python is absent).

## Requirements

- Python 3.11+ (tested with 3.14 in this repo environment)
- Dependencies from `requirements.txt`
- Included `unfaker.py` script, aligned to `jenissimo/unfake.js` strict parity target

## Setup (virtual environment)

Do not install dependencies globally on macOS/Homebrew Python.
Use a local virtual environment:

```bash
python3 -m venv tools/image-pixel-processor/.venv
source tools/image-pixel-processor/.venv/bin/activate
python3 -m pip install -r tools/image-pixel-processor/requirements.txt
```

`requirements.txt` installs `rembg[cpu]`, which includes the ONNX Runtime backend needed for background removal.
`run_watcher.sh` installs dependencies only when `requirements.txt` hash changes (or when `FORCE_PIP_INSTALL=1`).

## Usage

Recommended launcher (auto-creates `.venv` and installs dependencies):

```bash
tools/image-pixel-processor/run_watcher.sh \
  --input ./input \
  --output ./output \
  --failed-dir ./input_failed \
  --bg-model auto \
  --stages remove_background,frame_split,unfaker
```

Manual launch from activated venv:

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 tools/image-pixel-processor/watcher.py \
  --input ./input \
  --output ./output \
  --failed-dir ./input_failed \
  --bg-model auto \
  --stages remove_background,frame_split,unfaker
```

Direct one-off `unfaker.py` call (strict parity defaults):

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 tools/image-pixel-processor/unfaker.py ./input.png ./output.png
```

## Defaults

- Poll interval: `500ms`
- Stability window before processing: `500ms`
- Background model: `auto` with priority `bria-rmbg,birefnet-general,isnet-general-use,u2net`
- Unfaker input guard: max area `16,777,216` pixels (`4096x4096` equivalent)
- Transparency threshold: at least `2%` pixels with `alpha < 250`
- Min frame area (`--frame-min-area`): `16` non-transparent pixels
- Extensions: `png,jpg,jpeg,webp`
- Stages (`--stages`): `remove_background,frame_split,unfaker`

## Background Model Selection

- `--bg-model auto` picks the best available model in this order:
  - `bria-rmbg` (state-of-the-art in rembg model list)
  - `birefnet-general`
  - `isnet-general-use`
  - `u2net`
- You can pin a model explicitly, for example:

```bash
tools/image-pixel-processor/run_watcher.sh \
  --input ./input \
  --output ./output \
  --bg-model birefnet-general
```

## Stage Reordering

You can reorder or disable stages, for example:

```bash
# Run Unfaker before split
tools/image-pixel-processor/run_watcher.sh \
  --input ./input \
  --output ./output \
  --stages unfaker,frame_split
```

```bash
# Skip Unfaker completely
tools/image-pixel-processor/run_watcher.sh \
  --input ./input \
  --output ./output \
  --stages remove_background,frame_split
```

## Output naming

For `hero.png`, output frames are:

- `hero_f0001.png`
- `hero_f0002.png`
- ...

If the same name already exists in `output`, version suffix is appended:

- `hero_f0001_v2.png`

## Failure behavior

- If any pipeline step fails, input file is moved to `failed-dir`.
- A sidecar JSON is created:
  - `<failed_filename>.error.json`
- Source file is deleted from `input` only when all frames were processed and written successfully.

## Troubleshooting

If you see:

`No onnxruntime backend found`

run this inside the tool venv:

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 -m pip install --upgrade pip
python3 -m pip install 'rembg[cpu]'
python3 -m pip install -r tools/image-pixel-processor/requirements.txt
```

For split diagnostics, run with debug logs:

```bash
tools/image-pixel-processor/run_watcher.sh \
  --input ./input \
  --output ./output \
  --failed-dir ./input_failed \
  --log-level DEBUG
```

Look for:
- `bg_fallback method=edge_connected ...` (when `rembg` returns low-confidence alpha)
- `grid_detect ... rows(empty=.. peak=.. comp=.. final=..) cols(empty=.. peak=.. comp=.. final=..)`
- `grid_segments rows=[...] cols=[...]`
- `split_done ... frames=.. rows=.. cols=..`
- `unfaker_degraded_output ... action=fallback_to_source_frame`
