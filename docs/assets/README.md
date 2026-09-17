# Blender asset workflow

Canonical editable files are `art_source/character/male_commoner/source.blend`
and `art_source/equipment/stone_axe/source.blend`. Each adjacent `asset.yaml`
declares export objects, actions, grips, dependencies and budgets. Edit and save
these files in Blender; builds read saved bytes and never save the sources.
`EXPORT` is the deliverable collection. `PREVIEW` and references are authoring
helpers. The axe preview links the character with a relative library path; keep
the canonical directory structure intact. Packed material textures travel with
the blends. Generated runtime files are not authoring inputs.

## Setup

Run commands from the repository root. The checked-in toolchain lock currently
supports macOS arm64, Node 25.8.0, Blender 5.2.1 build `9e2066aef7ef`, and official
KTX-Software 4.4.2. Install the locked Node and Blender releases, then install npm
dependencies, decoder files and the pinned local KTX encoder:

```sh
npm ci --prefix tools/asset_pipeline
npm ci --prefix web_new
npm --prefix tools/asset_pipeline run install-decoders
tools/assets setup
```

`tools/assets setup` downloads the exact official KTX package URL in
`tools/asset_pipeline/toolchain.lock.json`, verifies its SHA-256, and extracts it
without `sudo` into
`tools/asset_pipeline/.tools/ktx-4.4.2/install/usr/local/bin/toktx`.
Builds do not download or install anything. To use a separately installed encoder:

```sh
tools/assets validate all --blender /Applications/Blender.app/Contents/MacOS/Blender --toktx /usr/local/bin/toktx
```

Use those overrides on each command when executable locations differ from the
defaults. A platform/version/hash mismatch requires the pinned installation or
a deliberately reviewed toolchain-lock update, not bypassing validation.

## Daily commands

Open either source in Blender with File → Open (or the following macOS commands),
make the edit, and save with Cmd-S before building:

```sh
open -a Blender art_source/character/male_commoner/source.blend
open -a Blender art_source/equipment/stone_axe/source.blend
tools/assets build character/male_commoner
tools/assets build equipment/stone_axe
tools/assets build character/male_commoner --animations --clip walk
tools/assets build character/male_commoner --animations
tools/assets build all
tools/assets validate all
tools/assets verify-reproducible all
```

Animation-only builds require a previous compatible full build. Editing only a
walk key publishes a new walk artifact and keeps the model, textures and other
clips unchanged. Changing a rest pose, socket, mesh, material, grip, optimization
setting or toolchain can require a full build. Locomotion distance comes from
`cycleDistanceTiles` in the recipe; keep walk and carry_walk compatible.

Successful builds atomically update
`web_new/public/assets/game/asset-catalog.json`. Models, separate animation GLBs,
KTX2 textures and metadata have immutable content-hash URLs. Reload the browser
after a build; running actor instances do not hot-reload their asset snapshot.
Validation checks sources and current published artifacts without publishing.
Reproducibility compares two clean exports without switching the runtime catalog.
Stage logs and reports are retained under ignored `build/asset-pipeline/`.

## Review

```sh
npm --prefix web_new run dev -- --host 127.0.0.1 --port 5181 --strictPort
```

Open `http://127.0.0.1:5181/tests/hybrid-character.html` for normal/carry motion,
eight directions and context restoration; `tests/axe-review.html` for both axe
hands; and `tests/hybrid-integration.html` for existing integration checks.
Inspect texture color, pixel silhouette and alpha over the dark green backdrop.
Static lighting comes from upper-left. After context restoration, actors should
reappear and `glError` should remain zero. The axe recipe currently uses ordinary
hand binding; axe-only arm actions are available in the source but are not its
active runtime binding policy.

For routine code verification, reuse `npm --prefix web_new run test:asset-pipeline`
and `npm --prefix tools/asset_pipeline test`. Saved-source partial-build coverage
already exists in `tools/asset_pipeline/tests/partial-build.test.mjs`; run the
focused case with:

```sh
node --test --test-name-pattern='editing walk preserves' tools/asset_pipeline/tests/partial-build.test.mjs
```

## Failure recovery

- Missing executable/dependency: run the setup commands, install the pinned tools,
  and supply absolute `--blender`/`--toktx` paths. Missing preview libraries must be
  restored at the recipe-declared relative source path. Do not copy preview
  geometry into `EXPORT` to work around a missing dependency.
- Incompatible animation-only build: run `tools/assets build character/male_commoner`
  after saving the intended source. Existing published assets remain usable when
  a build fails.
- Source changed during build: finish/save the edit, wait for the failed build to
  exit, and rerun. Do not edit sources or dependencies during a build.
- Invalid source, budget or export: read the asset/stage-specific log path printed
  by the CLI, correct that source/recipe, and rerun. Do not edit hashed output files.
- Locked publisher: inspect `web_new/public/assets/game/.publish.lock` and its
  recorded PID/host/owner. Wait for a live publisher. Dead locks owned by the same
  user on the same host recover automatically. Unknown ownership or a leftover
  `.publish.lock.guard` needs manual investigation; confirm that no publisher is
  alive before moving the exact stale lock/guard to a named recovery location.
  Never delete locks solely because a timeout occurred. Retry the original build.

Acceptance on 2026-09-17 reused the existing browser pages and cache tests, plus
one disposable Blender walk edit through the CLI. Evidence is in ignored
`build/task9-acceptance/`. Mobile/thermal performance and a populated live-server
scene remain separate from these local acceptance checks.

## Retired sources

The superseded v1–v3 trees, their Blender helper chain and five retired runtime
GLBs were removed after migration and runtime acceptance. The exact 270-file
inventory is [legacy-removal.json](legacy-removal.json); all removed bytes are
recoverable from Git commit `296ac1db46dd48e785d17b8f1f7ae6da9a1d7184`.
The retained motion-donor documents and licenses were verified byte-for-byte
against their originals before deletion. No untracked or ignored files existed
inside the deletion set.

V4 and axe working scenes remain historical references where unique edits have
not been proven redundant. The original baked-player documentation is marked
historical; existing fallback sprites are retained. `tools/blender/mcp_client.py`
remains a general authoring helper. `tools/asset_pipeline/migration/` records the
one-time migration with historical input paths; normal builds never execute it.
