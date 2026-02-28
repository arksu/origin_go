# Unfaker Strict Parity Target

Target upstream:
- Repository: `https://github.com/jenissimo/unfake.js`
- Commit: `2b4e5d88bdc45626a1faedfc473445ef42a6e8bd`
- Source of truth files:
  - `browser-tool/lib/pixel.js`
  - `browser-tool/lib/utils.js`

Strict-parity functions and stages that must match algorithmically:
- Scale detection:
  - `runsBasedDetect`
  - `edgeAwareDetect`
  - `legacyEdgeAwareDetect`
  - `singleRegionEdgeDetect`
  - `detectScale`
- Grid alignment:
  - `findOptimalCrop`
- Color and quantization:
  - `countColors`
  - `detectOptimalColorCount`
  - `quantizeImage`
- Downscaling and finalization:
  - `downscaleByDominantColor`
  - `downscaleBlock`
  - `finalizePixels`
- Main pixel pipeline:
  - `processImage` stage order and conditions

Required strict-parity defaults:
- `maxColors = 32`
- `autoColorCount = false`
- `detectMethod = auto`
- `edgeDetectMethod = tiled`
- `downscaleMethod = dominant`
- `domMeanThreshold = 0.15`
- `alphaThreshold = 128`
- `snapGrid = true`

Notes:
- `pipeline.py` and watcher logic are orchestration layers and may keep operational safeguards.
- Strict parity applies to `unfaker.py` core algorithm flow.
