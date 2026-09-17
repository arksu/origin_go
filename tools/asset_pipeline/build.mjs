import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { findRecipeFiles, loadRecipes, selectRecipes } from './catalog.ts'
import { exportAsset } from './blender.ts'
import { inspectToolchain } from './toolchain.ts'
import { optimizeExport } from './optimize.mjs'
import { canonicalJSON, sha256, validateArtifacts } from './report.mjs'
import { artifactPath, publishCatalog, readArtifact, readCatalog, withPublishLock } from './publish.mjs'

const pipelineDirectory = fileURLToPath(new URL('./', import.meta.url))
export const defaultRoot = fileURLToPath(new URL('../../', import.meta.url))

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

async function stage(staging, id, name, operation) {
  try { return await operation() } catch (error) {
    const log = join(staging, `${id.replaceAll('/', '-')}-${name}.log`)
    const detail = [error.stack ?? String(error), error.stdout ?? '', error.stderr ?? ''].join('\n')
    await writeFile(log, detail)
    throw new Error(`${id} ${name} failed: ${error.message}; log: ${log}`, { cause: error })
  }
}

async function prepare(root, target, toolPaths) {
  const recipeFiles = await findRecipeFiles(join(root, 'art_source'))
  const recipeInputs = await hashes(recipeFiles)
  const recipes = await loadRecipes(root)
  await assertStable(recipeInputs)
  const selected = selectRecipes(recipes, target)
  if (!selected.length) throw new Error('No asset recipes found')
  const inputs = new Map(selected.map(recipe => [recipe.id, recipe]))
  for (const recipe of selected) for (const [id, dependency] of dependenciesOf(recipe, recipes)) inputs.set(id, dependency)
  const sourceInputs = await hashes([...inputs.values()].map(recipe => recipe.source.absolutePath))
  const parent = join(root, 'build/asset-pipeline')
  await mkdir(parent, { recursive: true })
  const staging = await realpath(await mkdtemp(join(parent, 'stage-')))
  const paths = { node: process.execPath, blender: '/Applications/Blender.app/Contents/MacOS/Blender',
    toktx: join(pipelineDirectory, '.tools/ktx-4.4.2/install/usr/local/bin/toktx'), ...toolPaths }
  const lock = JSON.parse(await readFile(join(pipelineDirectory, 'toolchain.lock.json'), 'utf8'))
  const identity = await stage(staging, target, 'preflight', () => inspectToolchain(paths, lock))
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
  const modelInputHash = sha256(canonicalJSON({ model: metadata.modelFingerprint, toolchain: metadata.toolchainHash, dependencies: exportModels }))
  return { manifest: { schema: 1, id: recipe.id, kind: recipe.kind, model, textures, clips,
    rigHash: recipe.rig ? metadata.rigHash : null, modelInputHash,
    sockets: Object.fromEntries(Object.keys(metadata.sockets).sort().map(name => [name, name])), bindings: metadata.bindings,
    metadata: await artifact(result.metadata, '', 'json'),
    provenance: { source: { path: recipe.source.relativePath, sha256: context.sourceInputs[recipe.source.absolutePath] },
      dependencies, recipeHash: recipeHash(recipe), toolchain: context.toolchain.identity,
      toolchainHash: metadata.toolchainHash, profileHash: metadata.profileHash, packageLockHash: metadata.packageLockHash },
    metrics: result.metrics }, files, result }
}

async function stageAssets(context) {
  const exported = new Map()
  for (const recipe of context.selected) {
    const directory = join(context.staging, recipe.id.replaceAll('/', '-'))
    const resolvedDependencies = [...new Set([...recipe.dependencies.export, ...recipe.dependencies.preview])].sort()
      .map(id => ({ assetId: id, absolutePath: context.recipes.get(id).source.absolutePath }))
    await stage(context.staging, recipe.id, 'export', async () => {
      await checkInputs(context)
      try {
        await exportAsset({ recipe, selectedClips: Object.keys(recipe.clips).sort(), resolvedDependencies }, directory, context.toolchain.paths.blender)
      } finally { await checkInputs(context) }
    })
    const result = await stage(context.staging, recipe.id, 'optimization', () => optimizeExport({
      rawDirectory: join(directory, 'raw'), recipe, toolchain: context.toolchain, outputDirectory: join(directory, 'optimized'),
    }))
    // Keep raw stages until all validation (including published comparison) has finished.
    await stage(context.staging, recipe.id, 'validation', () => validateArtifacts(result, recipe))
    exported.set(recipe.id, await makeBundle(recipe, result, context, exported))
  }
  await checkInputs(context)
  return [...exported.values()]
}

export async function buildAssets({ root = defaultRoot, target, animations = false, toolPaths = {} }) {
  if (animations) throw new Error('Animation-only builds are not implemented yet; no artifacts were published')
  root = await realpath(root)
  const publicRoot = await realpath(join(root, 'web_new/public'))
  return withPublishLock(publicRoot, async () => {
    const previousCatalog = await readCatalog(publicRoot)
    const context = await prepare(root, target, toolPaths)
    const manifests = await stageAssets(context)
    const catalog = await stage(context.staging, target, 'publication', () => publishCatalog({ publicRoot, previousCatalog, manifests }))
    // Publication is committed. A scratch cleanup problem must not report a failed build.
    await rm(context.staging, { recursive: true }).catch(error => { process.stderr.write(`Build committed; staging cleanup failed: ${error.message}\n`) })
    return catalog
  })
}

export async function validateAssets({ root = defaultRoot, target, toolPaths = {} }) {
  root = await realpath(root)
  const publicRoot = await realpath(join(root, 'web_new/public'))
  const catalog = await readCatalog(publicRoot)
  const context = await prepare(root, target, toolPaths)
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
  const rebuilt = await stageAssets(context)
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
