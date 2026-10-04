- Follow KISS (Keep It Simple, Stupid): Prefer simple, readable solutions over clever complexity
- Follow DRY (Don't Repeat Yourself): Extract repeated logic into reusable functions/classes/modules
- Write self-documenting code: Use clear variable/function names that explain intent
- Fail fast: Validate inputs early and throw meaningful errors
- Single Responsibility: Each function/class should do one thing well

- Comment WHY, not WHAT (code shows what, comments explain reasoning)
- Handle errors explicitly; avoid silent failures
- Use meaningful variable names (no 'x', 'tmp', 'data' without context)
- Keep configuration separate from code
- Validate and sanitize all user inputs
- Avoid premature optimization
- Use appropriate data structures (dict for lookups, set for uniqueness)
- Consider time/space complexity for large datasets
- Cache expensive computations when appropriate
- Use lazy evaluation for large collections

## Client language

- All application-provided text displayed by the client must be in English: names, window titles, buttons, menus, tooltips, status/error messages, accessibility labels, and browser preview/debug controls.
- Server definitions and generated asset metadata consumed by the client must also use English display names. Change their sources and regenerate outputs through the existing tooling.
- This rule does not restrict user-provided character names, chat messages, or other user content.

## 2D tile atlas tooling (`tools/`)

TexturePacker atlas pack/extract for the web client. Full docs: `tools/README.md`.
Pure Python stdlib — there is no PIL/ImageMagick on this machine.

- `tools/pack_texturepacker_atlas.py <in_dir> <out_json> <out_png>` — pack a
  sprite directory into the game atlas. Deterministic; verifies every frame.
- `tools/extract_texturepacker_atlas.py [atlas.json] [sheet.png] [out_dir]` —
  split an atlas into PNGs.
- Frame keys equal paths under `art_source/tiles/` (no `tiles/` prefix).
  Rebuild command needs no key flags.
- Engine contract (PixiJS v8): `rotated` = region stored 90° CW with
  `frame.w/h` in original orientation; `trimmed` frames are positioned via
  `trim`. The tile mesh (`web_new/src/game/utils/VertexBuffer.ts`) is
  trim-aware — keep it that way. Never emit `aliases` (the parser ignores them).
- After touching the tools or atlas: `python3 tools/tests/roundtrip_test.py
  --regression` and `node tools/tests/pixi_semantics_test.mjs` must pass;
  `python3 tools/tests/render_sim.py` proves render equality offline.
