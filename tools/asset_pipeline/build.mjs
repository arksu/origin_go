import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { findRecipeFiles, loadRecipes, selectRecipes } from './catalog.ts'
import { exportAsset } from './blender.ts'
import { inspectToolchain } from './toolchain.ts'
import { optimizeExport } from './optimize.mjs'
import { canonicalJSON, sha256, validateArtifacts, measureArtifacts, compareBuilds } from './report.mjs'
import { artifactPath, publishCatalog, readArtifact, readCatalog, withPublishLock } from './publish.mjs'

const pipelineDirectory = fileURLToPath(new URL('./', import.meta.url))
export const defaultRoot = fileURLToPath(new URL('../../', import.meta.url))
const noLog = () => {}

function report(log, message) {
  log(`[assets] ${message}`)
}

function elapsedSince(startedAt) {
  return `${Date.now() - startedAt}ms`
}

async function hashes(paths) {
  return Object.fromEntries(await Promise.all([...new Set(paths)].sort().map(async path => [path, sha256(await readFile(path))])))
}

async function assertStable(inputs) {
  for (const [path, hash] of Object.entries(inputs)) {
    let current
    try { current = sha256(await readFile(path)) } catch (error) { throw new Error(`Build input changed or disappeared: ${path}`, { cause: error }) }
    if (current !== hash) throw new Error(`Build input changed: ${path}`)
  }
}

function dependenciesOf(recipe, recipes) {
  const dependencies = new Map()
  function visit(current) {
    for (const id of [...current.dependencies.export, ...current.dependencies.preview].sort()) {
      if (dependencies.has(id)) continue
      const dependency = recipes.get(id)
      dependencies.set(id, dependency)
      visit(dependency)
    }
  }
  visit(recipe)
  return dependencies
}

function recipeHash(recipe) {
  const { outputDirectory, source, ...settings } = recipe
  return sha256(canonicalJSON({ ...settings, source: source.relativePath }))
}

async function stage(staging, id, name, operation, log = noLog) {
  const startedAt = Date.now()
  report(log, `${id} ${name} start`)
  try {
    const result = await operation()
    report(log, `${id} ${name} complete (${elapsedSince(startedAt)})`)
    return result
  } catch (error) {
    const detailsPath = join(staging, `${id.replaceAll('/', '-')}-${name}.log`)
    const detail = [error.stack ?? String(error), error.stdout ?? '', error.stderr ?? ''].join('\n')
    await writeFile(detailsPath, detail)
    report(log, `${id} ${name} failed (${elapsedSince(startedAt)}); details: ${detailsPath}`)
    throw new Error(`${id} ${name} failed: ${error.message}; log: ${detailsPath}`, { cause: error })
  }
}

async function prepare(root, target, toolPaths, log = noLog) {
  const recipeFiles = await findRecipeFiles(join(root, 'art_source'))
  const recipeInputs = await hashes(recipeFiles)
  const recipes = await loadRecipes(root)
  await assertStable(recipeInputs)
  const selected = selectRecipes(recipes, target)
  if (!selected.length) throw new Error('No asset recipes found')
  report(log, `target ${target}`)
  for (const recipe of selected) {
    report(log, `source ${recipe.id} ${recipe.source.relativePath}`)
    for (const [id, dependency] of dependenciesOf(recipe, recipes)) {
      report(log, `dependency ${recipe.id} ${id} ${dependency.source.relativePath}`)
    }
  }
  const inputs = new Map(selected.map(recipe => [recipe.id, recipe]))
  for (const recipe of selected) for (const [id, dependency] of dependenciesOf(recipe, recipes)) inputs.set(id, dependency)
  const sourceInputs = await hashes([...inputs.values()].map(recipe => recipe.source.absolutePath))
  const parent = join(root, 'build/asset-pipeline')
  await mkdir(parent, { recursive: true })
  const staging = await realpath(await mkdtemp(join(parent, 'stage-')))
  const paths = { node: process.execPath, blender: '/Applications/Blender.app/Contents/MacOS/Blender',
    toktx: join(pipelineDirectory, '.tools/ktx-4.4.2/install/usr/local/bin/toktx'), ...toolPaths }
  const lock = JSON.parse(await readFile(join(pipelineDirectory, 'toolchain.lock.json'), 'utf8'))
  const identity = await stage(staging, target, 'preflight', () => inspectToolchain(paths, lock), log)
  return { root, recipes, selected, staging, sourceInputs, inputs: { ...recipeInputs, ...sourceInputs }, toolchain: { paths, identity }, recipeFiles }
}

async function checkInputs(context) {
  await assertStable(context.inputs)
  if (canonicalJSON(await findRecipeFiles(join(context.root, 'art_source'))) !== canonicalJSON(context.recipeFiles)) {
    throw new Error('Build input recipe set changed')
  }
}

async function makeBundle(recipe, result, context, exported) {
  const metadata = JSON.parse(await readFile(result.metadata, 'utf8'))
  const files = []
  async function artifact(source, subdirectory, extension) {
    const bytes = await readFile(source), hash = sha256(bytes)
    const entry = { url: `${recipe.runtimePath}/${subdirectory}${hash}.${extension}`, sha256: hash, bytes: bytes.length }
    files.push({ source, artifact: entry })
    return entry
  }
  const model = await artifact(result.model, '', 'glb')
  const textures = []
  for (const source of result.textures) textures.push(await artifact(source, 'textures/', 'ktx2'))
  const clips = {}
  for (const [id, source] of Object.entries(result.clips).sort()) {
    const clip = metadata.clips[id]
    clips[id] = { artifact: await artifact(source, 'animations/', 'glb'), rigHash: metadata.rigHash,
      duration: clip.durationSeconds, loop: clip.loop, playback: clip.playback, channelMask: clip.channelMask,
      ...(clip.cycleDistanceTiles === undefined ? {} : { cycleDistanceTiles: clip.cycleDistanceTiles }) }
  }
  const dependencies = Object.fromEntries([...dependenciesOf(recipe, context.recipes)].map(([id, dependency]) => [id, {
    path: dependency.source.relativePath, sha256: context.sourceInputs[dependency.source.absolutePath], recipeHash: recipeHash(dependency),
  }]))
  const exportModels = Object.fromEntries(recipe.dependencies.export.map(id => [id, exported.get(id).manifest.modelInputHash]))
  const modelInputHash = sha256(canonicalJSON({ model: metadata.modelFingerprint, rigHash: metadata.rigHash,
    sockets: metadata.sockets, bindings: metadata.bindings, toolchain: metadata.toolchainHash, dependencies: exportModels }))
  return { manifest: { schema: 1, id: recipe.id, kind: recipe.kind, model, textures, clips,
    rigHash: recipe.rig ? metadata.rigHash : null, modelInputHash,
    sockets: Object.fromEntries(Object.keys(metadata.sockets).sort().map(name => [name, name])),
    bindings: metadata.bindings, equipmentSlots: recipe.equipmentSlots,
    metadata: await artifact(result.metadata, '', 'json'),
    provenance: { source: { path: recipe.source.relativePath, sha256: context.sourceInputs[recipe.source.absolutePath] },
      dependencies, recipeHash: recipeHash(recipe), toolchain: context.toolchain.identity,
      toolchainHash: metadata.toolchainHash, profileHash: metadata.profileHash, packageLockHash: metadata.packageLockHash },
    metrics: result.metrics }, files, result }
}

async function stageAssets(context, { animations = false, clip, log = noLog } = {}) {
  const exported = new Map()
  for (const recipe of context.selected) {
    const directory = join(context.staging, recipe.id.replaceAll('/', '-'))
    const resolvedDependencies = [...new Set([...recipe.dependencies.export, ...recipe.dependencies.preview])].sort()
      .map(id => ({ assetId: id, absolutePath: context.recipes.get(id).source.absolutePath }))
    await stage(context.staging, recipe.id, 'export', async () => {
      await checkInputs(context)
      try {
        const selectedClips = animations && clip !== undefined
          ? (recipe.id === context.target ? [clip] : []) : Object.keys(recipe.clips).sort()
        report(log, `${recipe.id} export model ${join(directory, 'raw/model.glb')}`)
        for (const name of selectedClips) report(log, `${recipe.id} export animation ${name} ${join(directory, 'raw/animations', `${name}.glb`)}`)
        const metadata = await exportAsset({ recipe, selectedClips, resolvedDependencies }, directory, context.toolchain.paths.blender)
        for (const texture of metadata.textures) report(log, `${recipe.id} export texture ${join(directory, 'raw', texture.path)}`)
      } finally { await checkInputs(context) }
    }, log)
    const result = await stage(context.staging, recipe.id, 'optimization', () => optimizeExport({
      rawDirectory: join(directory, 'raw'), recipe, toolchain: context.toolchain, outputDirectory: join(directory, 'optimized'),
      log: message => report(log, `${recipe.id} ${message}`),
    }), log)
    // Keep raw stages until all validation (including published comparison) has finished.
    await stage(context.staging, recipe.id, 'validation', () => validateArtifacts(result, recipe), log)
    exported.set(recipe.id, await makeBundle(recipe, result, context, exported))
  }
  await checkInputs(context)
  return [...exported.values()]
}

async function publishedForPartial(context, publicRoot, catalog, clip) {
  if (clip !== undefined && (!context.target.startsWith('character/') || !context.recipes.get(context.target).clips[clip])) {
    throw new Error(`Unknown or invalid selected clip: ${clip}`)
  }
  const published = new Map()
  for (const recipe of context.selected) {
    const entry = catalog.assets[recipe.id]
    if (!entry) throw new Error(`${recipe.id} requires a prior published manifest; run a full build first`)
    const manifest = JSON.parse(await readArtifact(publicRoot, entry))
    if (manifest.id !== recipe.id || manifest.schema !== 1
      || canonicalJSON(manifest.provenance.toolchain) !== canonicalJSON(context.toolchain.identity)) {
      throw new Error(`${recipe.id} toolchain/manifest compatibility changed; run a full build`)
    }
    for (const artifact of [manifest.model, manifest.metadata, ...manifest.textures, ...Object.values(manifest.clips).map(value => value.artifact)]) {
      await readArtifact(publicRoot, artifact)
    }
    published.set(recipe.id, manifest)
  }
  return published
}

async function retainPublishedModel(bundle, previous, publicRoot, recipe) {
  const { manifest, result } = bundle
  // Compare final output too: the joint order and inverse binds are part of
  // the skin contract, even when the authored bone hierarchy is unchanged.
  if (manifest.modelInputHash !== previous.modelInputHash || manifest.rigHash !== previous.rigHash
    || canonicalJSON(manifest.model) !== canonicalJSON(previous.model)
    || canonicalJSON(manifest.textures) !== canonicalJSON(previous.textures)) {
    throw new Error(`${recipe.id} model compatibility changed; run a full build`)
  }
  const previousMetadata = JSON.parse(await readArtifact(publicRoot, previous.metadata))
  const metadata = JSON.parse(await readFile(result.metadata, 'utf8'))
  metadata.clips = { ...previousMetadata.clips, ...metadata.clips }
  await writeFile(result.metadata, canonicalJSON(metadata) + '\n')
  const bytes = await readFile(result.metadata), hash = sha256(bytes)
  const selected = new Set(Object.values(manifest.clips).map(value => value.artifact.url))
  manifest.model = previous.model
  manifest.textures = previous.textures
  manifest.clips = { ...previous.clips, ...manifest.clips }
  manifest.metadata = { url: `${recipe.runtimePath}/${hash}.json`, sha256: hash, bytes: bytes.length }
  bundle.files = bundle.files.filter(file => selected.has(file.artifact.url))
  bundle.files.push({ source: result.metadata, artifact: manifest.metadata })
  const mergedResult = { ...result, model: artifactPath(publicRoot, previous.model),
    textures: previous.textures.map(entry => artifactPath(publicRoot, entry)),
    clips: Object.fromEntries(Object.entries(manifest.clips).map(([id, value]) =>
      [id, result.clips[id] ?? artifactPath(publicRoot, value.artifact)])) }
  manifest.metrics = await measureArtifacts(mergedResult)
  if (manifest.metrics.totalBytes > recipe.budgets.totalPublishedBytes) throw new Error(`${recipe.id} total bytes budget exceeded`)
  return bundle
}

export async function buildAssets({ root = defaultRoot, target, animations = false, clip, toolPaths = {}, log = noLog }) {
  if (clip !== undefined && !animations) throw new Error('--clip requires --animations')
  root = await realpath(root)
  const publicRoot = await realpath(join(root, 'web_new/public'))
  return withPublishLock(publicRoot, async () => {
    const previousCatalog = await readCatalog(publicRoot)
    const context = await prepare(root, target, toolPaths, log)
    context.target = target
    const published = animations ? await publishedForPartial(context, publicRoot, previousCatalog, clip) : null
    const manifests = await stageAssets(context, { animations, clip, log })
    if (animations) for (const bundle of manifests) {
      await stage(context.staging, bundle.manifest.id, 'compatibility', () =>
        retainPublishedModel(bundle, published.get(bundle.manifest.id), publicRoot, context.recipes.get(bundle.manifest.id)), log)
    }
    await checkInputs(context)
    const catalog = await stage(context.staging, target, 'publication', () => publishCatalog({ publicRoot, previousCatalog, manifests }), log)
    report(log, `publication ${join(publicRoot, 'assets/game/asset-catalog.json')} complete (${manifests.length} assets)`)
    // Publication is committed. A scratch cleanup problem must not report a failed build.
    await rm(context.staging, { recursive: true }).catch(error => { process.stderr.write(`Build committed; staging cleanup failed: ${error.message}\n`) })
    return catalog
  })
}

export async function verifyReproducible({ root = defaultRoot, target, toolPaths = {}, log = noLog }) {
  root = await realpath(root)
  const first = await prepare(root, target, toolPaths, log)
  const firstBuild = await stageAssets(first, { log })
  const second = await prepare(root, target, toolPaths, log)
  const secondBuild = await stageAssets(second, { log })
  await checkInputs(first)
  await checkInputs(second)
  await compareBuilds(firstBuild, secondBuild)
  await rm(first.staging, { recursive: true })
  await rm(second.staging, { recursive: true })
  return { assets: firstBuild.map(bundle => bundle.manifest.id) }
}

export async function validateAssets({ root = defaultRoot, target, toolPaths = {}, log = noLog }) {
  root = await realpath(root)
  const publicRoot = await realpath(join(root, 'web_new/public'))
  const catalog = await readCatalog(publicRoot)
  const context = await prepare(root, target, toolPaths, log)
  const published = new Map()
  for (const recipe of context.selected) {
    const artifact = catalog.assets[recipe.id]
    if (!artifact) throw new Error(`Missing published asset: ${recipe.id}; run build first`)
    const manifest = JSON.parse(await readArtifact(publicRoot, artifact))
    if (manifest.provenance?.source?.sha256 !== context.sourceInputs[recipe.source.absolutePath]) throw new Error(`${recipe.id} source input differs from published manifest`)
    for (const entry of [manifest.model, manifest.metadata, ...manifest.textures, ...Object.values(manifest.clips).map(clip => clip.artifact)]) {
      await readArtifact(publicRoot, entry)
    }
    published.set(recipe.id, manifest)
  }
  const rebuilt = await stageAssets(context, { log })
  for (const { manifest, result } of rebuilt) {
    const previous = published.get(manifest.id)
    if (canonicalJSON(previous) !== canonicalJSON(manifest)) throw new Error(`${manifest.id} published source, toolchain or artifact compatibility mismatch; run build`)
    await validateArtifacts({ ...result, model: artifactPath(publicRoot, previous.model),
      metadata: artifactPath(publicRoot, previous.metadata), textures: previous.textures.map(entry => artifactPath(publicRoot, entry)),
      clips: Object.fromEntries(Object.entries(previous.clips).map(([id, clip]) => [id, artifactPath(publicRoot, clip.artifact)])),
    }, context.recipes.get(manifest.id))
  }
  await checkInputs(context)
  await rm(context.staging, { recursive: true })
  return catalog
}
