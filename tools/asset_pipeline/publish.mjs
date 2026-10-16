import { randomUUID } from 'node:crypto'
import fs from 'node:fs/promises'
import { hostname } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { canonicalJSON, sha256 } from './report.mjs'
import { parseActionAnimationProjection, validateActionAnimationAssets, validateAnimationUniqueness, validateActionSoundReferences } from '../../web_new/src/types/actionAnimationDefs.ts'
import { parseSoundFile, parseLocomotionAudioFile } from '../../web_new/src/types/soundDefs.ts'
import { projectLocomotionAudio, validateAudioMedia } from './audio-definitions.mjs'

const catalogFile = publicRoot => join(publicRoot, 'assets/game/asset-catalog.json')
const assetID = /^(character|equipment|world_object)\/[a-z0-9][a-z0-9_-]*$/

async function safeDirectory(root, directory) {
  if (await fs.realpath(root) !== root) throw new Error('Public root must be canonical')
  if (directory !== root && !directory.startsWith(`${root}/`)) throw new Error('Output escapes public root')
  let current = root
  for (const part of directory.slice(root.length).split('/').filter(Boolean)) {
    current = join(current, part)
    try { await fs.mkdir(current) } catch (error) { if (error.code !== 'EEXIST') throw error }
    if (!(await fs.lstat(current)).isDirectory()) throw new Error(`Output crosses non-directory or symlink: ${current}`)
  }
}

export function artifactPath(publicRoot, artifact) {
  if (!artifact || !/^\/assets\/game\/(?:[a-zA-Z0-9_-]+\/)+[a-f0-9]{64}\.(?:glb|ktx2|json)$/.test(artifact.url)
    || !/^[a-f0-9]{64}$/.test(artifact.sha256) || !artifact.url.endsWith(`/${artifact.sha256}.${artifact.url.split('.').at(-1)}`)
    || !Number.isSafeInteger(artifact.bytes) || artifact.bytes < 1) throw new Error('Invalid content-addressed artifact')
  return join(publicRoot, artifact.url)
}

export async function readArtifact(publicRoot, artifact) {
  const path = artifactPath(publicRoot, artifact)
  if (await fs.realpath(path) !== path || !(await fs.lstat(path)).isFile()) throw new Error(`Artifact crosses symlink: ${artifact.url}`)
  const bytes = await fs.readFile(path)
  if (bytes.length !== artifact.bytes || sha256(bytes) !== artifact.sha256) throw new Error(`Immutable content mismatch: ${artifact.url}`)
  return bytes
}

export async function readCatalog(publicRoot) {
  let bytes
  try { bytes = await fs.readFile(catalogFile(publicRoot), 'utf8') } catch (error) {
    if (error.code === 'ENOENT') return { schema: 1, assets: {} }
    throw error
  }
  const catalog = JSON.parse(bytes)
  validateCatalog(publicRoot, catalog)
  return catalog
}

function validateCatalog(publicRoot, catalog) {
  if (catalog?.schema !== 1 || !catalog.assets || Array.isArray(catalog.assets) || typeof catalog.assets !== 'object') throw new Error('Invalid published catalog')
  for (const [id, artifact] of Object.entries(catalog.assets)) {
    if (!assetID.test(id)) throw new Error(`Invalid catalog asset ID: ${id}`)
    artifactPath(publicRoot, artifact)
  }
  if (catalog.actionAnimations !== undefined) artifactPath(publicRoot, catalog.actionAnimations)
  if (catalog.sounds !== undefined) artifactPath(publicRoot, catalog.sounds)
  if (catalog.locomotionAudio !== undefined) artifactPath(publicRoot, catalog.locomotionAudio)
}

async function withOwnershipGuard(lockPath, deadline, callback) {
  // Every pathname ownership transition uses this guard, including release.
  // A crashed guard is left for manual inspection rather than unsafe recovery.
  const guard = `${lockPath}.guard`
  while (true) {
    try { await fs.mkdir(guard); break } catch (error) {
      if (error.code !== 'EEXIST') throw error
      if (Date.now() >= deadline) throw new Error(`Publish lock ownership guard cannot be acquired: ${guard}. Inspect before manual recovery.`)
      await delay(25)
    }
  }
  try { return await callback() } finally { await fs.rmdir(guard) }
}

// Caller holds the ownership guard across inspection, liveness and unlink.
async function recoverDeadLock(lockPath) {
  let owner, entry
  try {
    entry = await fs.lstat(lockPath)
    if (!entry.isFile()) return
    owner = JSON.parse(await fs.readFile(lockPath, 'utf8'))
  } catch (error) {
    if (error.code === 'ENOENT' || error instanceof SyntaxError) return
    throw error
  }
  if (owner.hostname !== hostname() || owner.uid !== process.getuid?.() || entry.uid !== owner.uid
    || !Number.isSafeInteger(owner.pid) || owner.pid < 1 || typeof owner.token !== 'string') return
  try { process.kill(owner.pid, 0); return } catch (error) {
    // EPERM and unknown errors cannot establish that the owner is dead.
    if (error.code !== 'ESRCH') return
  }
  const current = await fs.lstat(lockPath)
  const currentOwner = JSON.parse(await fs.readFile(lockPath, 'utf8'))
  if (current.dev !== entry.dev || current.ino !== entry.ino || currentOwner.token !== owner.token) {
    throw new Error(`Publish lock ownership changed during recovery: ${lockPath}`)
  }
  await fs.unlink(lockPath)
}

export async function withPublishLock(publicRoot, callback, { timeoutMs = 120_000 } = {}) {
  publicRoot = resolve(publicRoot)
  await safeDirectory(publicRoot, dirname(catalogFile(publicRoot)))
  const lockPath = join(dirname(catalogFile(publicRoot)), '.publish.lock')
  const token = randomUUID()
  const candidate = `${lockPath}.${token}`
  const owner = { hostname: hostname(), uid: process.getuid?.() ?? null, pid: process.pid, token }
  // Linking a completely written file avoids an empty-owner recovery race.
  await fs.writeFile(candidate, canonicalJSON(owner), { flag: 'wx', mode: 0o600 })
  let acquired = false
  const deadline = Date.now() + timeoutMs
  try {
    while (!acquired) {
      await withOwnershipGuard(lockPath, deadline, async () => {
        try { await fs.link(candidate, lockPath); acquired = true } catch (error) {
          if (error.code !== 'EEXIST') throw error
          await recoverDeadLock(lockPath)
        }
      })
      if (!acquired) {
        if (Date.now() >= deadline) throw new Error(`Publish lock is active or ownership cannot be verified: ${lockPath}. Inspect its owner before manual recovery.`)
        await delay(25)
      }
    }
    return await callback()
  } finally {
    if (acquired) {
      await withOwnershipGuard(lockPath, Date.now() + timeoutMs, async () => {
        const current = JSON.parse(await fs.readFile(lockPath, 'utf8'))
        if (current.token !== token) throw new Error(`Publish lock ownership changed: ${lockPath}`)
        await fs.unlink(lockPath)
      })
    }
    await fs.unlink(candidate)
  }
}

async function installImmutable(publicRoot, artifact, bytes) {
  const path = artifactPath(publicRoot, artifact)
  if (sha256(bytes) !== artifact.sha256 || bytes.length !== artifact.bytes) throw new Error(`Staged content mismatch: ${artifact.url}`)
  await safeDirectory(publicRoot, dirname(path))
  const candidate = join(dirname(path), `.install-${randomUUID()}`)
  await fs.writeFile(candidate, bytes, { flag: 'wx' })
  try {
    try { await fs.link(candidate, path) } catch (error) { if (error.code !== 'EEXIST') throw error }
    await readArtifact(publicRoot, artifact)
  } finally { await fs.unlink(candidate) }
}

function manifestArtifacts(manifest) {
  return [manifest.model, ...(manifest.textures ?? []), ...Object.values(manifest.clips ?? {}).map(clip => clip.artifact),
    ...(manifest.metadata ? [manifest.metadata] : [])]
}

async function pruneArtifacts(publicRoot, previousCatalog, catalog, runtimePaths) {
  const live = new Set()
  const roots = new Set([...runtimePaths, ...Object.values(previousCatalog.assets).map(artifact => dirname(artifact.url))])
  // Validate the complete current closure before deleting anything, including
  // unchanged assets and artifacts retained by animation-only builds.
  for (const [id, reference] of Object.entries(catalog.assets)) {
    const manifest = JSON.parse(await readArtifact(publicRoot, reference))
    if (manifest.schema !== 1 || manifest.id !== id) throw new Error(`Manifest ID/schema mismatch: ${id}`)
    live.add(reference.url)
    roots.add(dirname(reference.url))
    for (const artifact of manifestArtifacts(manifest)) {
      await readArtifact(publicRoot, artifact)
      live.add(artifact.url)
    }
  }
  for (const key of ['actionAnimations', 'sounds', 'locomotionAudio']) {
    if (catalog[key] === undefined) continue
    await readArtifact(publicRoot, catalog[key])
    live.add(catalog[key].url)
  }
  for (const root of roots) {
    if (!/^\/assets\/game\/[a-z0-9][a-z0-9_-]*\/[a-z0-9][a-z0-9_-]*$/.test(root)) throw new Error(`Invalid cleanup output directory: ${root}`)
  }
  async function pruneDirectory(url, pattern) {
    const directory = join(publicRoot, url)
    let entries
    try {
      // realpath also rejects symlinks in any ancestor. Do not create missing
      // directories or follow links while searching for obsolete artifacts.
      if (await fs.realpath(directory) !== directory || !(await fs.lstat(directory)).isDirectory()) return
      entries = await fs.readdir(directory, { withFileTypes: true })
    } catch (error) { if (error.code === 'ENOENT') return; throw error }
    for (const entry of entries) {
      if (entry.isFile() && pattern.test(entry.name) && !live.has(`${url}/${entry.name}`)) {
        await fs.unlink(join(directory, entry.name))
      }
    }
  }
  const artifactName = /^[a-f0-9]{64}\.(?:glb|ktx2|json)$/
  for (const root of roots) {
    for (const suffix of ['', '/textures', '/animations']) await pruneDirectory(`${root}${suffix}`, artifactName)
  }
  for (const directory of ['action_animations', 'sounds', 'locomotion_audio']) {
    await pruneDirectory(`/assets/game/${directory}`, /^[a-f0-9]{64}\.json$/)
  }
}

// Caller holds withPublishLock across reading previousCatalog and this switch.
// A bundle contains deterministic manifest JSON plus staged {source, artifact} files.
export async function publishCatalog({ publicRoot, previousCatalog, manifests, actionAnimationDefinitions, soundDefinitions, locomotionAudioDefinitions, runtimePaths = [] }) {
  validateCatalog(publicRoot, previousCatalog)
  const assets = { ...previousCatalog.assets }
  const owners = new Map(Object.entries(assets).map(([id, artifact]) => [dirname(artifact.url), id]))
  const selected = new Set()
  for (const { manifest } of manifests) {
    if (!assetID.test(manifest.id) || manifest.schema !== 1) throw new Error('Invalid asset manifest')
    const directory = dirname(manifest.model.url)
    if (!/^\/assets\/game\/[a-z0-9][a-z0-9_-]*\/[a-z0-9][a-z0-9_-]*$/.test(directory)) throw new Error('Invalid asset output directory')
    if (selected.has(manifest.id) || (owners.has(directory) && owners.get(directory) !== manifest.id)) throw new Error(`Asset path collision: ${manifest.id}`)
    owners.set(directory, manifest.id)
    selected.add(manifest.id)
  }
  // No mutable output is touched until the entire selected set has validated.
  for (const { manifest, files } of manifests) {
    for (const { source, artifact } of files) await installImmutable(publicRoot, artifact, await fs.readFile(source))
    for (const artifact of manifestArtifacts(manifest)) {
      if (!artifact.url.startsWith(`${dirname(manifest.model.url)}/`)) throw new Error(`Artifact escapes asset output: ${artifact.url}`)
      await readArtifact(publicRoot, artifact)
    }
    const bytes = Buffer.from(`${canonicalJSON(manifest)}\n`)
    const hash = sha256(bytes)
    const artifact = { url: `${dirname(manifest.model.url)}/${hash}.json`, sha256: hash, bytes: bytes.length }
    await installImmutable(publicRoot, artifact, bytes)
    assets[manifest.id] = artifact
  }
  const catalog = { schema: 1, assets }
  const hasAudio = soundDefinitions !== undefined || previousCatalog.sounds !== undefined || locomotionAudioDefinitions !== undefined || previousCatalog.locomotionAudio !== undefined
  const hasActions = actionAnimationDefinitions !== undefined || previousCatalog.actionAnimations !== undefined
  const effectiveManifests = hasAudio || hasActions ? Object.fromEntries(await Promise.all(Object.entries(assets).map(async ([id, reference]) => {
    const manifest = JSON.parse(await readArtifact(publicRoot, reference))
    if (manifest.id !== id) throw new Error(`Manifest ID mismatch: ${id}`)
    for (const artifact of manifestArtifacts(manifest)) await readArtifact(publicRoot, artifact)
    return [id, manifest]
  }))) : {}
  let profiles = []
  if (hasAudio) {
    if (soundDefinitions === undefined && previousCatalog.sounds === undefined) throw new Error('Audio publication requires sound definitions')
    if (locomotionAudioDefinitions === undefined && previousCatalog.locomotionAudio === undefined) throw new Error('Audio publication requires locomotion definitions')
    profiles = parseSoundFile(soundDefinitions === undefined ? JSON.parse(await readArtifact(publicRoot, previousCatalog.sounds)) : { v: 1, sounds: soundDefinitions }, 'sound publication')
    await validateAudioMedia(publicRoot, profiles)
    const locomotion = parseLocomotionAudioFile(locomotionAudioDefinitions === undefined ? JSON.parse(await readArtifact(publicRoot, previousCatalog.locomotionAudio)) : { v: 1, bindings: locomotionAudioDefinitions }, 'locomotion audio publication')
    const bindings = projectLocomotionAudio(locomotion, effectiveManifests, profiles)
    parseLocomotionAudioFile({ v: 1, bindings }, 'locomotion audio projection')
    for (const [key, directory, projection] of [['sounds', 'sounds', { v: 1, sounds: profiles }], ['locomotionAudio', 'locomotion_audio', { v: 1, bindings }]]) {
      const bytes = Buffer.from(`${canonicalJSON(projection)}\n`), hash = sha256(bytes)
      catalog[key] = { url: `/assets/game/${directory}/${hash}.json`, sha256: hash, bytes: bytes.length }
      await installImmutable(publicRoot, catalog[key], bytes)
    }
  }
  if (actionAnimationDefinitions !== undefined || previousCatalog.actionAnimations !== undefined) {
    const definitions = actionAnimationDefinitions ?? parseActionAnimationProjection(
      JSON.parse(await readArtifact(publicRoot, previousCatalog.actionAnimations)), 'published action animations')
    validateAnimationUniqueness(definitions, 'action animation publication')
    validateActionAnimationAssets(definitions, effectiveManifests, 'action animation publication')
    validateActionSoundReferences(definitions, new Map(profiles.map(profile => [profile.key, profile])), 'action sound publication')
    const bindings = definitions.map(({ source, ...presentation }) => presentation)
    // Also validate the exact projection that clients will load.
    parseActionAnimationProjection({ v: 1, bindings }, 'action animation projection')
    const bytes = Buffer.from(`${canonicalJSON({ v: 1, bindings })}\n`), hash = sha256(bytes)
    catalog.actionAnimations = { url: `/assets/game/action_animations/${hash}.json`, sha256: hash, bytes: bytes.length }
    await installImmutable(publicRoot, catalog.actionAnimations, bytes)
  }
  const path = catalogFile(publicRoot)
  await safeDirectory(publicRoot, dirname(path))
  const candidate = join(dirname(path), `.catalog-${randomUUID()}.json`)
  await fs.writeFile(candidate, `${canonicalJSON(catalog)}\n`, { flag: 'wx' })
  try { await fs.rename(candidate, path) } catch (error) {
    await fs.rm(candidate, { force: true }).catch(cleanup => { process.stderr.write(`Catalog candidate cleanup failed: ${cleanup.message}\n`) })
    throw error
  }
  // The catalog is committed. Cleanup failure must not turn publication into a
  // reported failure or roll back its valid references; retry on the next publish.
  await pruneArtifacts(publicRoot, previousCatalog, catalog, runtimePaths).catch(error => {
    process.stderr.write(`Publication committed; artifact cleanup failed: ${error.message}\n`)
  })
  return catalog
}
