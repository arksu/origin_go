# Image Pixel Processor

Source: `tools/image-pixel-processor/`

Use a virtual environment for dependencies:

```bash
python3 -m venv tools/image-pixel-processor/.venv
source tools/image-pixel-processor/.venv/bin/activate
python3 -m pip install -r tools/image-pixel-processor/requirements.txt
```

The requirements include `rembg[cpu]` to ensure ONNX Runtime is present for background removal.

This utility watches a flat `input` directory and processes supported image files (`.png`, `.jpg`, `.jpeg`, `.webp`).

Bundled Unfaker command:

```bash
python3 ./tools/image-pixel-processor/unfaker.py
```

Pipeline:

1. Detect stable file in `input`.
2. Check existing alpha coverage with threshold defaults (`alpha < 250` on >=2% pixels).
3. Skip remove-bg when alpha coverage is meaningful, otherwise remove background via `rembg`.
4. Detect a rectangular frame grid from alpha occupancy and split into frame cells.
5. Run Unfaker command on each frame.
6. Save frame(s) to `output` as `<base>_f0001.png`, `<base>_f0002.png`, ...
7. Delete source from `input` only after full success.

Failures:

- Input file is moved to `failed-dir`.
- Sidecar `<filename>.error.json` contains failure reason.
