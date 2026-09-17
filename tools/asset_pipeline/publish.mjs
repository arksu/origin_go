import { randomUUID } from 'node:crypto'
import fs from 'node:fs/promises'
import { hostname } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { canonicalJSON, sha256 } from './report.mjs'

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
}

async function recoverDeadLock(lockPath) {
  // Serialize reclaimers so an old observation can never unlink a new owner.
  const guard = `${lockPath}.recovery`
  try { await fs.mkdir(guard) } catch (error) { if (error.code === 'EEXIST') return; throw error }
  try {
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
    try { process.kill(owner.pid, 0) } catch (error) {
      if (error.code === 'ESRCH') await fs.unlink(lockPath)
      // EPERM and unknown errors cannot establish that the owner is dead.
    }
  } finally { await fs.rmdir(guard) }
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
      try { await fs.link(candidate, lockPath); acquired = true } catch (error) {
        if (error.code !== 'EEXIST') throw error
        await recoverDeadLock(lockPath)
        if (Date.now() >= deadline) throw new Error(`Publish lock is active or ownership cannot be verified: ${lockPath}. Inspect its owner before manual recovery.`)
        await delay(25)
      }
    }
    return await callback()
  } finally {
    if (acquired) {
      const current = JSON.parse(await fs.readFile(lockPath, 'utf8'))
      if (current.token !== token) throw new Error(`Publish lock ownership changed: ${lockPath}`)
      await fs.unlink(lockPath)
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

// Caller holds withPublishLock across reading previousCatalog and this switch.
// A bundle contains deterministic manifest JSON plus staged {source, artifact} files.
export async function publishCatalog({ publicRoot, previousCatalog, manifests }) {
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
    const references = [manifest.model, ...manifest.textures, ...Object.values(manifest.clips).map(clip => clip.artifact),
      ...(manifest.metadata ? [manifest.metadata] : [])]
    for (const artifact of references) {
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
  const path = catalogFile(publicRoot)
  await safeDirectory(publicRoot, dirname(path))
  const candidate = join(dirname(path), `.catalog-${randomUUID()}.json`)
  await fs.writeFile(candidate, `${canonicalJSON(catalog)}\n`, { flag: 'wx' })
  try { await fs.rename(candidate, path) } catch (error) {
    await fs.rm(candidate, { force: true }).catch(cleanup => { process.stderr.write(`Catalog candidate cleanup failed: ${cleanup.message}\n`) })
    throw error
  }
  return catalog
}
