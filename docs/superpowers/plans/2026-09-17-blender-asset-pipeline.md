# Blender Asset Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild an editable Blender asset, selected animation clips, or the complete registered 3D catalog with one reproducible command, and migrate the current game without losing approved artwork or poses.

**Architecture:** A Node CLI discovers declarative asset recipes and runs isolated Blender exports, optimization, validation and transactional catalog publication. Canonical `.blend` files own artwork; content-addressed model, texture and clip files are consumed by the Three.js runtime through one catalog snapshot. Authoring and runtime have separate validation stages, with a semantic compatibility fingerprint guarding animation-only builds.

**Tech Stack:** Blender 5.2.1 LTS/Python, Node ESM, YAML, glTF Transform, meshoptimizer, Khronos KTX-Software and glTF Validator; existing Three.js/Pixi client.

**Spec:** `docs/superpowers/specs/2026-09-17-blender-asset-pipeline-design.md`

**Execution refinement (2026-09-17):** The user retained Node after discussion of
TypeScript. Maintained pipeline modules and tests below use `.ts` instead of `.mjs`,
with real TypeScript interfaces instead of JSDoc. Run them using the pinned Node's
type stripping and check with `tsc --noEmit`; add a strict `tsconfig.json`, TypeScript
and `@types/node` in Task 1. The small executable `tools/assets` and existing client
test-launcher `.mjs` files stay JavaScript. Task names, behavior and interfaces are unchanged.

## Global Constraints

- Blender закрепляется на 5.2.1; проверяется фактическая версия бинарника.
- Verified local Blender build: `9e2066aef7ef`, Darwin; local Node: `v25.8.0`.
- `source.blend` owns meshes, rig, actions, poses, sockets and grip transforms. Build must never overwrite it or regenerate these from legacy heuristics.
- Every export starts a fresh background Blender with factory startup settings and auto-execution disabled; no MCP or current interactive session is involved.
- `all` means every registered 3D asset, not unrelated existing PNG/audio resources.
- Runtime outputs remain standard glTF/GLB plus KTX2, compressed with meshopt.
- Animations-only builds require compatible published model inputs; edits to geometry, materials, rig/rest pose, sockets, model settings or toolchain require a full build.
- Two clean builds under the same locked toolchain/platform must produce identical runtime-file SHA-256 values. No cross-platform byte-identity promise.
- Inputs and installed tools are local. Normal builds must not download or install anything.
- Publication must preserve the previous working catalog on any failure. `build all` has one catalog switch, not one switch per asset.
- Legacy deletion follows migration, visual verification and a successful independent rebuild. Check tracked/ignored/untracked files before removing any directory.
- Preserve current movement, equipment policy, skinning, LOD behavior and upper-left lighting. Current client walk cycle distance is `1.677975879375`; older README/export reports are not authoritative for runtime behavior.
- Keep the source GLB's provenance; Meshy geometry does not acquire the old MakeHuman CC0 attribution. Preserve required provenance for retained donor motion separately.

---

## Execution boundaries and file map

This is one coupled delivery: publishing optimized artifacts without a compatible client would break rendering. Keep the old runtime paths usable until Task 8 changes the production loader. Task 10 is the deletion gate, after end-to-end acceptance.

Before code changes, use the workspace-isolation workflow and record HEAD/status. Preserve existing user changes. Install dependencies only in the chosen workspace; do not share a writable node_modules with another worktree. Baseline tests are the client character suite, not the unrelated entire Go backend.

```
tools/assets                              executable Node launcher
tools/asset_pipeline/
  package.json, package-lock.json          isolated locked build dependencies
  toolchain.lock.json                     binary versions, hashes and platform
  cli.mjs                                 argument parsing and exit/report handling
  catalog.mjs                             recipe parsing, paths, graph and validation
  toolchain.mjs                           offline version/hash preflight
  build.mjs                               build and partial-build orchestration
  optimize.mjs                            glTF/KTX transforms and glTF validation
  publish.mjs                             hash-named output and catalog transaction
  report.mjs                              deterministic reports and fingerprints
  blender/export_asset.py                 explicit source evaluation and raw export
  blender/source_contract.py              source checks and grip/rig extraction
  migration/migrate_sources.py            one-time source preparation
  migration/baseline.mjs                  old-runtime semantic snapshots
  tests/*.test.mjs                        Node unit/CLI tests
  tests/blender_fixtures.py               minimal Blender test scenes
  tests/integration.test.mjs              real Blender and artifact integration tests
  tests/migration.test.mjs                approved-content comparisons
art_source/character/male_commoner/
  source.blend, asset.yaml, references/
art_source/equipment/stone_axe/
  source.blend, asset.yaml, references/
web_new/src/game/actors/
  ActorAssetCatalog.ts                    runtime manifest parsing and validation
  ActorClipBinding.ts                     independent clip/model binding
  ActorAssetCache.ts                      shared artifact/bundle leases
  ActorSockets.ts                        required authored sockets
  ActorInstance.ts                       bundle data, clip timing and attachments
  ActorRenderer.ts                       compressed texture capability setup
  config.ts, equipment.ts                 presentation settings and equipment types
web_new/tests/asset-pipeline.test.ts       runtime catalog/binding/cache tests
web_new/scripts/test-asset-pipeline.mjs    existing esbuild + node:test pattern
docs/assets/README.md                     everyday author/build/review workflow
```

Source and runtime schemas live in `catalog.mjs` and `ActorAssetCatalog.ts`; a shared JSON fixture tests agreement. Do not introduce a schema generator, remote asset service or generalized plugin framework.

## Shared artifact contracts

Node modules use ESM and JSDoc types; Blender consumes normalized JSON, never parses YAML.
All paths passed to modules are absolute, validated paths; persisted paths are repository-relative or same-origin runtime URLs. User-controlled traversal, symlink escape, overlapping outputs and duplicate YAML keys are errors.

```js
// catalog.mjs exports loadRecipes(root) -> Promise<Map<string, AssetRecipe>>,
// selectRecipes(recipes, target) -> AssetRecipe[] (stable topological order).
// Types below are carried in JSDoc and mirrored by runtime types where relevant.
// Artifact = {url: string, sha256: string, bytes: number}
// ClipEntry = {artifact: Artifact, rigHash: string, duration: number,
//              loop: boolean, playback: 'time'|'distance', cycleDistanceTiles?: number}
// AssetManifest = {schema: 1, id: string, kind: string,
//   model: Artifact, textures: Artifact[], clips: Record<string, ClipEntry>,
//   rigHash: string|null, modelInputHash: string,
//   sockets: Record<string, string>, bindings: Record<string, object>,
//   provenance: object, metrics: object}
// PublishedCatalog = {schema: 1, assets: Record<string, Artifact>}
// Each catalog asset points to an immutable hash-named manifest.
```

`modelInputHash` covers the canonical non-animation export, rig/rest data, sockets, bindings, textures, model settings, dependency content used in the model, and toolchain. Exclude action keyframes, editor state and animation-only metadata from this hash. Track those inputs separately in provenance/clip entries. Preview-only dependencies remain provenance inputs but must not invalidate model compatibility merely because their animation keys changed.

Use a stable JSON serializer (sorted object keys, arrays ordered by contract, reject non-finite numbers). Clip-specific changes must not churn unrelated clip files. A change to clip metadata may change its manifest entry without changing binary animation bytes.

### Task 1: Record a migration baseline and lock the toolchain

**Files:** Create `tools/asset_pipeline/{package.json,package-lock.json,toolchain.lock.json,toolchain.mjs,migration/baseline.mjs,tests/toolchain.test.mjs}`; create ignored `build/asset-pipeline-baseline/`; modify `.gitignore` only for tool-local dependencies. Read `web_new/package-lock.json` and the actual runtime assets.

**Interfaces:** `inspectToolchain({blender, node, toktx}, lock)` returns versions/hashes or throws; `snapshotLegacy({root, output})` writes read-only semantic snapshots and copied baseline artifacts outside the future legacy deletion set. Baseline is diagnostic, not a build dependency.

- [ ] Add the version rejection test before implementation:

```js
import test from 'node:test';
import assert from 'node:assert/strict';
import { validateBlenderIdentity } from '../toolchain.mjs';
test('rejects a Blender patch release mismatch', () => {
  assert.throws(() => validateBlenderIdentity(
    {version: '5.2.0', buildHash: '9e2066aef7ef'},
    {version: '5.2.1', buildHash: '9e2066aef7ef'}), /5\.2\.1/);
});
```

- [ ] Run `node --test tools/asset_pipeline/tests/toolchain.test.mjs`; establish the missing version guard, then implement parsing/validation in `toolchain.mjs` and rerun.
- [ ] Provision the official KTX encoder locally, recording release URL and binary/archive SHA-256. Resolve exact dependency versions once with `npm install --prefix tools/asset_pipeline --save-exact yaml @gltf-transform/core @gltf-transform/extensions @gltf-transform/functions meshoptimizer gltf-validator`; commit the resulting package lock. Resolve compatibility here before writing optimizer code. Do not use `npx` downloads during build.
- [ ] Record `process.version`, Blender version/build hash, embedded Python version, bundled `io_scene_gltf2` tree hash and encoder identity in the lock. Use configurable executable paths (`--blender`, `--toktx`) without embedding machine-specific paths in the lock. Match the observed platform; unknown platform/tool identity fails with setup instructions.
- [ ] Implement semantic GLB snapshots: node hierarchy/rest transforms, skin bind matrices, primitive attributes/indices, decoded image pixels, clip sample times/values and current left/right bindings. Record the actual `cycleDistanceTiles` from client config. Snapshot both runtime and source-side GLB; report any mismatch rather than preferring an older README.
- [ ] Run `npm --prefix web_new run test:character-visual` and `npm --prefix web_new run type-check`; save evidence in the task report. Run the read-only baseline extractor before any source migration.
- [ ] Commit only toolchain/baseline tooling and lock files: `chore: pin 3d asset toolchain and baseline checks`.

### Task 2: Discover recipes and expose the CLI

**Files:** Create `tools/assets`, `tools/asset_pipeline/{cli.mjs,catalog.mjs,tests/catalog.test.mjs,tests/cli.test.mjs,tests/fixtures/recipe.yaml}`; update `package.json` with `test` using `node --test tests/*.test.mjs` (integration tests opt in explicitly).

**Interfaces:** `loadRecipes(root)`, `selectRecipes(recipes,target)` from Shared artifact contracts; `parseArguments(argv)` returns `{command,target,animations,clip,blender,toktx}`. Commands: `build`, `validate`, `verify-reproducible`; `--clip` requires `--animations` and a character target.

- [ ] Write tests for duplicate IDs/YAML keys, unknown fields, parent traversal, escaping symlinks, output collisions, cyclic/missing dependencies, unsupported kind and flags. Use temporary fixture directories, never real production source paths.

```js
import test from 'node:test';
import assert from 'node:assert/strict';
import { parseArguments } from '../cli.mjs';
test('a clip cannot silently turn a full build into a partial build', () => {
  assert.throws(() => parseArguments(['build', 'character/male_commoner', '--clip', 'walk']),
    /--animations/);
});
test('one explicit clip is preserved', () => {
  assert.equal(parseArguments(['build', 'character/male_commoner', '--animations', '--clip', 'walk']).clip,
    'walk');
});
```

- [ ] Run the two test files, implement the recipe schema and CLI validation, then rerun. Keep command dispatch injectable for CLI tests; importing cli.mjs must not launch Blender.
- [ ] Define normalized recipes with `schema`, `id`, `kind`, `source`, `runtimePath`, `dependencies`, `rig`, `clips`, `bindings`, `budgets`, `optimization`. Character rig declares export object, required sockets/bones. Clips declare action, start/end/fps, loop, playback and optional cycle distance. Bindings declare slot/socket/grip and ordinary/layered clip policy, never numerical grip transforms.
- [ ] Require explicit positive budgets for triangles by LOD, texture dimensions, total published bytes and bone influences. Reject NaN/infinity and nonsensical frame ranges. Define UI-only `PREVIEW` dependencies separately from export dependencies.
- [ ] Make `tools/assets` a small executable Node launcher resolving paths from its own location. `--help` lists six accepted examples from the spec and setup errors. Stub unimplemented command handlers with an explicit nonzero error; never report a build success without artifacts.
- [ ] Commit: `feat: add validated 3d asset catalog and command interface`.

### Task 3: Export a source file without changing it

**Files:** Create `tools/asset_pipeline/blender/{source_contract.py,export_asset.py}`, `tools/asset_pipeline/tests/{blender_fixtures.py,integration.test.mjs}`; add `test:integration` npm script executing that test with real tools.

**Interfaces:** Blender worker invocation is `--request <absolute-json> --output <absolute-directory>`. Request contains normalized recipe and selected clip names. Output is `raw/model.glb`, `raw/animations/<clip>.glb`, `raw/textures/`, `raw/metadata.json`. Metadata contains canonical rig/socket/binding data, loop/clip checks and non-animation export fingerprint inputs.

- [ ] Add fixture generation for a static triangle, a two-bone skinned mesh with two actions, a socket and two grip empties; save source files in a temporary directory. Integration helpers `createFixture(kind, overrides)` and `runExport(recipe, output)` belong in the test harness and invoke the real worker.
- [ ] Add a regression that will fail until export is independent of the saved active action/frame:

```js
test('model output ignores current pose and never changes the blend', async () => {
  const fixture = await createFixture('character', {savedFrame: 12, savedAction: 'walk'});
  const before = await fixture.sourceHash();
  const first = await runExport(fixture.recipe, fixture.directory('first'));
  await fixture.saveEditorState({frame: 1, action: 'idle'});
  const secondSource = await fixture.sourceHash();
  const second = await runExport(fixture.recipe, fixture.directory('second'));
  assert.equal(first.modelHash, second.modelHash);
  assert.equal(secondSource, await fixture.sourceHash());
  assert.notEqual(before, secondSource); // intentional fixture edit, not exporter mutation
});
```

- [ ] Implement the worker process flags as an argument array, with order explicit:

```js
const args = ['--background', '--factory-startup', '--disable-autoexec',
  '--python-exit-code', '1', '--python', workerPath, '--',
  '--request', requestPath, '--output', outputDirectory];
```

The worker validates the request, then calls `bpy.ops.wm.open_mainfile` itself. Disable audio/simulation caches or require explicit baked inputs; fail on undeclared external dependencies and scripted drivers requiring auto-exec. Native supported constraints are evaluated and baked in memory.
- [ ] Validate `EXPORT`, rig, supported materials/packed images, unique stable names (including collisions after Three.js sanitization), finite transforms/vertices/weights, max influences, required sockets and grip scale/shear. Reset rig to rest and evaluate modifiers for model export; mute every NLA track before explicitly selecting a clip. The current UI selection never selects export objects.
- [ ] Export model with rest-position skinning, LOD/skinning extras and sockets; export each animation independently from the same rig with explicit sampling/fps/range. Use a temporary deformable fixture only if the installed exporter cannot emit standalone joints, and remove it from the emitted clip document before validation. Resulting clip artifacts have no mesh/image dependency.
- [ ] Extract attachment by aligning the item grip frame with the character socket, using explicit basis conversions. Test a translated and rotated left/right grip against full matrix composition; bone parenting tail offsets must be evaluated, not guessed.
- [ ] Add failing fixtures for missing socket, duplicate exported names, invalid weights, non-finite geometry, undeclared image/library and bad loop closure. Sample poses at frame boundaries and between keys; keyframe-only checks miss interpolation errors.
- [ ] Run integration tests with Blender 5.2.1; verify no writes to source or source dependency files. Commit: `feat: export canonical blend assets in isolated Blender processes`.

### Task 4: Migrate commoner and axe to editable canonical sources

**Files:** Create `tools/asset_pipeline/migration/migrate_sources.py`, `tools/asset_pipeline/tests/migration.test.mjs`, `art_source/character/male_commoner/{source.blend,asset.yaml,references/}` and `art_source/equipment/stone_axe/{source.blend,asset.yaml,references/}`. Existing source scenes remain available until Task 10.

**Interfaces:** Migration runs once against the Task 1 baseline and writes only the two new canonical source files and their reference/provenance files. Normal build never imports or calls migration tooling.

- [ ] Inspect all candidate source actions and the current shipped GLB before picking artistic inputs. Source commoner is `art_source/characters/male_commoner_v4/commoner_rigged.blend`; axe candidates include `stone_axe_user_idle_pose.blend`, `stone_axe_walk_pose_edit.blend`, both confirmed grip scenes and overrides. Newer mtime alone does not establish approval; compare numerical poses to the shipped result.
- [ ] Write migration checks before changing binaries: preservation of skinned mesh positions, normals, UVs, bind matrices, texture pixels, all eight runtime clips, current left/right attachment transforms and current stride metadata. Define baseline pose tolerance of `1e-5` metres before lossy optimization, quaternion angular error `1e-5` radians; explain any export float tolerance increase with measured error.

```js
test('migration keeps approved runtime poses', async () => {
  const baseline = await loadBaseline();
  const migrated = await exportMigratedSources();
  assert.equal(migrated.cycleDistanceTiles, 1.677975879375);
  assert.deepEqual(migrated.clipNames.sort(), baseline.clipNames.sort());
  assert.ok(comparePoses(baseline, migrated).maxPositionError <= 1e-5);
  assert.ok(compareBindings(baseline, migrated).maxMatrixError <= 1e-5);
});
```

Implement `loadBaseline`, `exportMigratedSources`, `comparePoses`, `compareBindings` in migration test helpers using Task 1 snapshots and Task 3 export.
- [ ] Retain the editable rig/mesh from the authored commoner; resolve source/runtime differences by importing approved clip channels, not regenerating the donor gait. Rename actions to their stable runtime IDs; preserve layer channel masks. Import confirmed axe idle overrides as editable source keyframes, and bake approved equipped walk clips into actions.
- [ ] Build `EXPORT`, `PREVIEW`, `ASSET_INFO`. Pack material textures. Add actual bone-parented grip/forearm sockets positioned to preserve the existing runtime attachment frames. Mark actions with fake-user retention; select an immediately playable action on opening, with other NLA tracks muted.
- [ ] In the axe source, preserve the current game mesh and create editable `GRIP_L`/`GRIP_R` frames from the approved bindings. Link a preview collection from the new commoner source using a relative path. Give left/right preview assemblies visible labels and use native constraints whose evaluated transforms demonstrate grip alignment. Blender file opens with a framed preview and usable Action Editor, no scripts required.
- [ ] Store original Meshy sources and motion provenance in references. Start declared budgets at existing measured limits (commoner high 16,000 triangles, low 5,500; axe 1,200; textures 1024 and 512); measure total output budgets in Task 5 before final acceptance.
- [ ] Run migration comparison and open both sources for visual/editor inspection. Save screenshots/reports under ignored build output. Commit: `feat: migrate editable commoner and axe source assets`.

### Task 5: Optimize and validate standard runtime artifacts

**Files:** Create `tools/asset_pipeline/{optimize.mjs,report.mjs,tests/optimize.test.mjs}`; extend integration tests and recipes; install local decoder assets for the client from its locked Three.js package with provenance.

**Interfaces:** `optimizeExport({rawDirectory,recipe,toolchain,outputDirectory})` returns `{model, clips, textures, metadata, metrics}` with local paths. `canonicalJSON(value)` and `sha256(bytes)` are exported from `report.mjs`. `validateArtifacts(result, recipe)` returns a structured report or throws.

- [ ] Write an integration assertion that compressed output preserves sockets and animation target names and passes Khronos glTF Validator:

```js
test('optimization retains authored socket and clip targets', async () => {
  const raw = await exportedCharacterFixture();
  const result = await optimizeExport(raw);
  const report = await validateArtifacts(result, raw.recipe);
  assert.equal(report.errors, 0);
  assert.ok(report.nodeNames.includes('grip_l'));
  assert.deepEqual(report.clipTargets, raw.metadata.clipTargets);
});
```

`exportedCharacterFixture` is a test adapter over Task 3 `createFixture`/`runExport`; it returns the full optimizeExport input object. Tests use real optimizer/encoder outputs.
- [ ] Implement an explicit transform sequence, avoiding broad automatic optimize presets that flatten the rig or discard extras. Register meshopt encode/decode, preserve node names, hierarchy, LOD/skinning tags and socket empties. Use lossless meshopt first; quantization is opt-in with measured deformation bounds, not required to achieve compression.
- [ ] Extract images to canonical pixels and encode base color to KTX2 UASTC with Zstd level 18, mipmaps, sRGB transfer and one encoder thread. Fix CLI flags for the installed pinned KTX version and include them in toolchain/profile hashes. Compare file size and rendered colors; if quality/size tradeoff requires ETC1S, change the declared profile with a measured before/after report rather than a silent fallback.
- [ ] Write external texture URI references relative to the final model location before hashing model bytes. Texture names derive from content. Clip outputs contain no images/materials/meshes, and animations are sorted by ID with explicit constant-track handling.
- [ ] Validate before and after compression; glTF Validator cannot decode every compressed payload, so also decode through glTF Transform and compare geometry/clip values against tolerances. Report texture, geometry, animation, metadata and total byte counts separately. Count unique texture bytes once in total catalog accounting.
- [ ] Add budget exceedance failures for triangles, bones/weights, texture dimensions and total bytes; no omitted budget passes. Set production byte budgets from measured optimized results plus explicit 20% headroom, rounded up to KiB. Record measured values in recipe review notes.
- [ ] Run optimizer unit/integration tests and compare full-resolution reference sampling as well as the game's palette-rendered result. Commit: `feat: compress and validate game models textures and clips`.

### Task 6: Publish complete builds transactionally

**Files:** Create `tools/asset_pipeline/{build.mjs,publish.mjs,tests/publish.test.mjs}`; connect `cli.mjs`; create generated files under the recipe runtime paths and `web_new/public/assets/game/asset-catalog.json`.

**Interfaces:** `buildAssets({root,target,animations:false,toolPaths})`; `publishCatalog({publicRoot,previousCatalog,manifests})` writes content-addressed files and atomically replaces the catalog. `withPublishLock(publicRoot,callback)` serializes mutations and releases on exceptions.

- [ ] Write publication failure tests using actual temporary files:

```js
test('a failed asset leaves the entire catalog unchanged', async () => {
  const project = await createBuildProject();
  await project.build('all');
  const before = await project.catalogBytes();
  await project.breakSource('equipment/test_axe', 'missing EXPORT');
  await assert.rejects(project.build('all'), /EXPORT/);
  assert.deepEqual(await project.catalogBytes(), before);
  await project.assertPublishedArtifactsReadable();
});
```

`createBuildProject` writes a temporary recipe/source fixture project and invokes the actual `buildAssets` interface; it uses Task 3 fixture generation. Do not simulate success by mocking the publisher.
- [ ] Implement one lock covering read-modify-publish so concurrent single-asset builds cannot overwrite each other's entries. Create a staging directory with `mkdtemp`; subprocess failures include asset ID, stage and captured log path. Existing catalog is read once after acquiring the lock.
- [ ] Complete preflight/source/export/optimization validation for the whole selected set before switching anything. Install immutable artifacts and manifests with collision verification; then write a temporary catalog in the same directory and `rename` over the active file. A failed rename leaves the old catalog; unused immutable files are harmless and are not garbage-collected by build.
- [ ] Prevent mutation of input sources during a build: hash inputs/dependencies before and after reading/export; reject if they changed. Build a stable dependency snapshot or fail on changes rather than mixing versions.
- [ ] Add tests for a second concurrent publisher, asset path collision, immutable-file content mismatch, subprocess exit, input mutation and failure after artifacts are written but before catalog rename. Lock recovery must check ownership/process liveness; never silently remove an active lock.
- [ ] Connect full `build <id>`/`build all` and offline `validate all` to CLI. Validate checks source and published hashes/compatibility without rewriting runtime files. Run full builds while production still uses old URLs. Commit: `feat: publish validated asset catalogs atomically`.

### Task 7: Implement partial animation builds and reproducibility checks

**Files:** Extend `build.mjs`, `report.mjs`, `cli.mjs`; create `tests/{partial-build,reproducibility}.test.mjs`.

**Interfaces:** `buildAssets({root,target,animations:true,clip,toolPaths})`; `verifyReproducible({root,target,toolPaths})` builds into two separate staging roots, compares every runtime hash, reports first difference and never publishes.

- [ ] Add the key behavioral test before wiring partial build:

```js
test('editing walk preserves the model texture and idle bytes', async () => {
  const project = await createBuildProject();
  await project.build('character/test_actor');
  const before = await project.manifest('character/test_actor');
  await project.editAction('walk', {bone: 'root', frame: 12, rotationZ: 0.1});
  await project.build('character/test_actor', {animations: true, clip: 'walk'});
  const after = await project.manifest('character/test_actor');
  assert.deepEqual(after.model, before.model);
  assert.deepEqual(after.textures, before.textures);
  assert.deepEqual(after.clips.idle, before.clips.idle);
  assert.notEqual(after.clips.walk.artifact.sha256, before.clips.walk.artifact.sha256);
});
```

Extend the integration fixture helper with explicit saved action mutations using Blender. The test alters source, not runtime bytes.
- [ ] Compute modelInputHash from deterministic, rest-state raw export plus extracted sockets/bindings/model settings/toolchain. Avoid hashing the entire blend or normalized recipe: either would reject legitimate action-only edits. RigHash includes node hierarchy, bind/rest transforms and skinning contract; animation-node layout must match.
- [ ] Require a prior manifest; validate its referenced model/textures and toolchain compatibility before replacing clips. Use already-published immutable model/texture entries verbatim. Rebuild selected clips only; retain untouched clip entries and hashes. A `--clip` selection must exist in both recipe/source.
- [ ] Test rig/rest change, socket move, texture edit, mesh modifier edit and tool upgrade all reject `--animations` and preserve the catalog. Test that saved viewport/active frame changes and unrelated action edits do not invalidate model compatibility.
- [ ] Implement verify-reproducible with two fresh Blender processes and empty staging directories; compare model, every clip, textures and deterministic manifest content. Diagnostic timestamps/log paths are excluded from persisted runtime output. Include a fixture intentionally embedding nondeterministic output and assert verification fails.
- [ ] Run both tests against fixture assets, then `tools/assets verify-reproducible character/male_commoner` and `tools/assets verify-reproducible equipment/stone_axe`. Commit: `feat: rebuild individual clips and verify reproducible assets`.

### Task 8: Load catalog bundles in the client

**Files:** Create `ActorAssetCatalog.ts`, `ActorClipBinding.ts`, `web_new/tests/asset-pipeline.test.ts`, `web_new/scripts/test-asset-pipeline.mjs`; modify `ActorAssetCache.ts`, `ActorInstance.ts`, `ActorRenderer.ts`, `ActorSockets.ts`, `config.ts`, `equipment.ts`, `web_new/package.json`, `web_new/tests/character-visual.test.ts` and review-page imports.

**Interfaces:** `loadActorCatalog(url): Promise<ActorCatalog>` loads one catalog snapshot and its immutable manifests. `ActorAssetCache.acquire(id): Promise<{asset: ActorBundle; release():void}>`; `ActorBundle` contains `scene`, `animations`, `manifest`. Expose equipment definitions from the loaded catalog, not a mutable module-level static constant. `ActorClipBinding.bindClips(model, manifests, clips)` returns name-bound validated clips.

- [ ] Add tests for manifest/schema/path validation, mismatched rigHash, missing node/socket, and binding to separate model instances using real Three.js groups/bones and AnimationMixer:

```ts
test('a standalone walk clip animates the model rig by stable node name', () => {
  const model = makeRig('model-instance');
  const donor = makeRig('different-uuid');
  const clips = makeWalkClip(donor);
  const bound = bindClips(model, compatibleClipManifests(), clips);
  const mixer = new AnimationMixer(model);
  mixer.clipAction(bound[0]!).play();
  mixer.setTime(0.5);
  assert.ok(Math.abs(model.getObjectByName('pelvis')!.position.y - 0.1) < 1e-6);
});
```

Define `makeRig`, `makeWalkClip`, `compatibleClipManifests` in test helpers with explicit two-key tracks and exported-name contract. Include a second actor to verify mixer state is not shared.
- [ ] Implement manifest parsing with same-origin `/assets/game/` URL constraints and schema checks. Fetch catalog once per cache lifecycle; immutable manifest and artifact URLs prevent mixed generations. Runtime catalog fetch uses cache policy allowing new page loads to observe a new catalog. Validate clip compatibility before actor readiness.
- [ ] Configure GLTFLoader with local MeshoptDecoder and KTX2Loader. Detect support on the real Three renderer after its construction; rework the current cache field initialization accordingly and retain safe cleanup when renderer construction fails. No CDN decoder requests.
- [ ] Keep low-level artifact promises/leases shared across bundles so a different animation revision reuses unchanged models/textures. On partial load failure or actor destruction, release every acquired lease. Count shared arrays and compressed texture mip data once; do not count compressed bytes as decompressed geometry memory. Dispose KTX workers, clips, textures and geometries on final ownership release.
- [ ] Bind tracks by unique sanitized exported names; reject missing targets or duplicate sanitized names. Authored sockets are required rather than silently synthesized. Map generated grip bindings to existing ordinary/layered arm behavior; do not activate axe-specific arm poses for ordinary carrying.
- [ ] Move `cycleDistanceTiles` from `ACTOR_RENDER` into the loaded walk metadata; preserve current value during migration and validate carry_walk compatibility. Update existing actor fixtures to supply metadata. Keep rendering FPS, facing and movement interpolation settings in their current modules.
- [ ] Run `npm --prefix web_new run test:asset-pipeline`, `npm --prefix web_new run test:character-visual`, `npm --prefix web_new run type-check`, `npm --prefix web_new run build`. Inspect shader color-space behavior with KTX2: the existing custom shader applies a texture gamma conversion, so verify decoded colors rather than assuming standard material behavior.
- [ ] Switch production actor construction to the catalog after tests pass. Commit: `feat: load compressed model and animation bundles from asset catalog`.

### Task 9: Exercise authoring, gameplay rendering and failure recovery

**Files:** Extend `web_new/tests/{hybrid-character,hybrid-integration,axe-review}.ts`; extend integration/migration tests; create `docs/assets/README.md`; update `web_new/src/game/actors/README.md`. Reports/screenshots remain in ignored `build/`.

**Interfaces:** Existing review HTML pages remain the manual/browser entrypoints. Fixture-based edits run in temporary copies of the project, not in canonical approved artwork.

- [ ] Add full workflow integration tests that copy source/recipe into a temporary project, move a grip or edit one walk key, save through Blender, invoke the CLI and inspect the resulting runtime change. Assert the source hash after build matches the intentionally edited input hash.
- [ ] Run browser checks on eight directions, all current locomotion/carry clips, both axe hands, LOD transitions and 1/8/30 actors. Verify fixed upper-left lighting, pixel silhouette, texture color and alpha over dark green terrain. Include a single supported world_object fixture in the offline build tests without expanding production world rendering.
- [ ] Exercise WebGL context loss/restoration with loaded KTX2 textures; compare restored rendering and check GPU errors. Verify canceled loads, eviction and destroyed actors don't leak bundle leases or throw after disposal.
- [ ] Confirm through network requests that clips have separate URLs and repeat actors share requests; a page load after a walk-only build fetches the new clip and references the unchanged model/texture URLs. Do not promise live hot reload of already running actor instances.
- [ ] Inspect both source files in Blender: visible grips, rig, available actions, preview character, packed textures and relative link resolution. Validate the checked-in opening workspace/UI state, not just headless exports.
- [ ] Write the daily workflow with exact commands: open/save source, build single asset, build a selected clip, build all, validate all and verify reproducibility. Document missing-tool setup, model-incompatible partial build, source changes during build, missing dependencies and locked publisher errors with recovery instructions.
- [ ] Commit: `test: verify asset authoring and compressed runtime integration`.

### Task 10: Remove legacy assets after migration verification

**Files:** Delete the verified v1–v3 source trees, obsolete blender generators/bake/review scripts, legacy `commoner.glb`/linen exports and superseded working duplicates whose content is preserved. Update `.gitignore`, runtime `SOURCES.txt`, active docs and historical superseded labels. Keep the canonical sources/references and general MCP helper if it remains useful outside builds.

**Interfaces:** New pipeline must build with no reference to removed legacy paths. Migration tooling may retain documented historical input paths but must never run from normal build or be mistaken for maintained generation tooling.

- [ ] Generate an exact deletion inventory from `git ls-files`, `git status --short --ignored art_source tools/blender` and dependency searches. For ignored/untracked files, preserve unique content in canonical references or an explicitly named backup outside the deletion set before removal. Never recursively target the repository root.
- [ ] Run a pre-deletion gate: migration comparison, runtime/browser acceptance, full build and reproducibility all pass. Record the exact commit containing migrated sources and references so recovery is concrete.
- [ ] Check each obsolete helper's imports; remove the connected obsolete chain, not a filename glob that could remove the new pipeline. V4 source duplicates can be removed only after proving their approved contents are represented in the canonical files.
- [ ] Delete tracked legacy files with exact reviewed paths. Remove ignored generated debris only from validated legacy asset directories. Preserve required legal/provenance notes for retained animations even if the old geometry is gone. Report the deletion scope and recovery location to the user.
- [ ] Verify the new pipeline independently of previous cache output:

```sh
tools/assets build all
tools/assets validate all
tools/assets verify-reproducible all
npm --prefix web_new run test:asset-pipeline
npm --prefix web_new run test:character-visual
npm --prefix web_new run type-check
npm --prefix web_new run build
git diff --check
```

Reproducibility runs use fresh temporary directories; don't delete production artifacts as a way to simulate a clean build. Search active code/config/docs for `male_commoner_v1`, `male_commoner_v2`, `male_commoner_v3`, the old Downloads path and retired runtime URLs. Historical design/migration records may mention them explicitly as history.
- [ ] Commit: `chore: remove superseded Blender asset pipelines and sources`.

## Final review and handoff

- [ ] Review the whole change against the approved spec, including binary-source editability and publication failure behavior. Fix findings before declaring completion.
- [ ] Record actual output sizes before/after, reproducibility hashes, tested Blender/toolchain identity, current model/clip counts and browser verification results. Do not infer size reduction or visual equivalence from a successful compiler run.
- [ ] Report committed versus uncommitted changes, source paths to open, four everyday build commands and any concrete unverified behavior. No push/merge is implied.

## Spec coverage/self-review

| Requirement | Tasks |
|---|---|
| Canonical editable sources, sockets, preview on opening | 3, 4, 9 |
| Explicit config/CLI and all three source kinds | 2, 3, 9 |
| Fixed toolchain, offline build, deterministic exports | 1, 3, 5, 7 |
| Independent clips, compatibility and stride metadata | 3, 7, 8 |
| Meshopt/KTX2, budgets, standard validation | 5, 8, 9 |
| Transactional catalog, failure/concurrency handling | 6 |
| Runtime cache/resource/context lifecycle | 8, 9 |
| Baseline-preserving migration and legacy deletion | 1, 4, 9, 10 |
| Practical documentation and verification evidence | 9, 10, final review |

Planning verification on 2026-09-17: read current exporter, loader, socket, equipment,
animation and renderer paths; confirmed Blender `5.2.1 LTS` build `9e2066aef7ef`.
KTX encoder was not found on PATH; provisioning in Task 1 is required. None of the
implementation tests or acceptance steps above are marked complete by this plan.
