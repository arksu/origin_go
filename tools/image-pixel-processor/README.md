# Image Pixel Processor

Watches an `input` directory and processes image files through this pipeline:

1. Validate transparency ratio.
2. Remove background with `rembg` when transparency is not meaningful.
3. If `rembg` mask is too soft, fallback to deterministic edge-connected background removal.
4. Detect frame rows/columns as a rectangular grid using a bright-spot method (adaptive downscale + blurred mass peaks, with fallback strategies).
5. Run Unfaker for pixel-art style conversion.
6. Save resulting PNG files into `output` and delete source from `input` only after full success.

Splitter supports both transparent spritesheets and fully opaque backgrounds (for example solid black) by estimating edge-connected background regions before grid detection.
Before Unfaker, frames are normalized to hard alpha (`alpha > 0 -> 255`) with RGB de-premultiplication to prevent object loss on soft-alpha masks from `rembg`.
Runner also enforces `--scale 1` for `unfaker.py` by default (unless `--scale`/`-s` is explicitly set in your `--unfaker-command`) to avoid unexpected frame size reduction.

## Requirements

- Python 3.11+ (tested with 3.14 in this repo environment)
- Dependencies from `requirements.txt`
- Included `unfaker.py` script (downloaded from `unfake.py` project and adapted for local CLI use)

## Setup (virtual environment)

Do not install dependencies globally on macOS/Homebrew Python.
Use a local virtual environment:

```bash
python3 -m venv tools/image-pixel-processor/.venv
source tools/image-pixel-processor/.venv/bin/activate
python3 -m pip install -r tools/image-pixel-processor/requirements.txt
```

`requirements.txt` installs `rembg[cpu]`, which includes the ONNX Runtime backend needed for background removal.

## Usage

Recommended launcher (auto-creates `.venv` and installs dependencies):

```bash
tools/image-pixel-processor/run_watcher.sh \
  --input ./input \
  --output ./output \
  --failed-dir ./input_failed \
  --unfaker-command "/Users/park/projects/origin_go/tools/image-pixel-processor/.venv/bin/python ./tools/image-pixel-processor/unfaker.py -c 16"
```

Manual launch from activated venv:

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 tools/image-pixel-processor/watcher.py \
  --input ./input \
  --output ./output \
  --failed-dir ./input_failed \
  --unfaker-command "/Users/park/projects/origin_go/tools/image-pixel-processor/.venv/bin/python ./tools/image-pixel-processor/unfaker.py -c 16"
```

Example with a command template that explicitly maps input/output paths:

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 tools/image-pixel-processor/watcher.py \
  --input ./input \
  --output ./output \
  --failed-dir ./input_failed \
  --unfaker-command "/Users/park/projects/origin_go/tools/image-pixel-processor/.venv/bin/python ./tools/image-pixel-processor/unfaker.py -c 16 {input} {output}"
```

Example when Unfaker accepts positional `<input> <output>`:

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 tools/image-pixel-processor/watcher.py \
  --input ./input \
  --output ./output \
  --unfaker-command "/Users/park/projects/origin_go/tools/image-pixel-processor/.venv/bin/python ./tools/image-pixel-processor/unfaker.py -c 16"
```

Direct one-off `unfaker.py` call:

```bash
source tools/image-pixel-processor/.venv/bin/activate
python3 tools/image-pixel-processor/unfaker.py ./input.png ./output.png
```

## Defaults

- Poll interval: `500ms`
- Stability window before processing: `500ms`
- Transparency threshold: at least `2%` pixels with `alpha < 250`
- Min frame area (`--frame-min-area`): `16` non-transparent pixels
- Extensions: `png,jpg,jpeg,webp`

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
  --unfaker-command "/Users/park/projects/origin_go/tools/image-pixel-processor/.venv/bin/python /Users/park/projects/origin_go/tools/image-pixel-processor/unfaker.py" \
  --log-level DEBUG
```

Look for:
- `bg_fallback method=edge_connected ...` (when `rembg` returns low-confidence alpha)
- `grid_detect ... rows(empty=.. peak=.. comp=.. final=..) cols(empty=.. peak=.. comp=.. final=..)`
- `grid_segments rows=[...] cols=[...]`
- `split_done ... frames=.. rows=.. cols=..`
- `unfaker_degraded_output ... action=fallback_to_source_frame`
