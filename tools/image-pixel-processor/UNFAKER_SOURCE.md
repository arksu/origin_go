# Unfaker Source

`unfaker.py`, `unfaker_content_adaptive.py`, and `unfaker_wu_quantizer.py` were downloaded from:

- Repository: `https://github.com/painebenjamin/unfake.py`
- Commit: `8345c4f1ebdfc6704d570c4d394d2b1cf83945a6`
- Files:
  - `src/unfake/pixel.py` -> `unfaker.py`
  - `src/unfake/content_adaptive.py` -> `unfaker_content_adaptive.py`
  - `src/unfake/wu_quantizer.py` -> `unfaker_wu_quantizer.py`

The upstream project declares MIT license in `pyproject.toml`.

Local adjustments:

- Replaced package-relative imports with local module imports.
- Added positional output argument support (`unfaker.py <input> <output>`) for watcher integration.
