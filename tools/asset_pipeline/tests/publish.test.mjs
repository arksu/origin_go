import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { hostname, tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, symlink, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import fs from 'node:fs/promises'
import test from 'node:test'
import { once } from 'node:events'

const digest = bytes => createHash('sha256').update(bytes).digest('hex')
const publisher = () => import('../publish.mjs')
async function project(context) {
  const publicRoot = await realpath(await mkdtemp(join(tmpdir(), 'asset-publish-test-')))
  context?.after(() => rm(publicRoot, { recursive: true, force: true }))
  const catalogPath = join(publicRoot, 'assets/game/asset-catalog.json')
  const previousCatalog = { schema: 1, assets: {} }
  await mkdir(join(publicRoot, 'assets/game'), { recursive: true })
  await writeFile(catalogPath, JSON.stringify(previousCatalog))
  async function bundle(id, bytes = Buffer.from('model')) {
    const source = join(publicRoot, `${id.split('/').at(-1)}.glb`)
    await writeFile(source, bytes)
    const model = { url: `/assets/game/${id}/${digest(bytes)}.glb`, sha256: digest(bytes), bytes: bytes.length }
    return { manifest: { schema: 1, id, kind: id.split('/')[0], model, textures: [], clips: {},
      rigHash: null, modelInputHash: digest(bytes), sockets: {}, bindings: {}, provenance: {}, metrics: {} },
    files: [{ source, artifact: model }] }
  }
  return { publicRoot, catalogPath, previousCatalog, bundle }
}

async function stagedArtifact(fixture, directory, extension, value) {
  const bytes = Buffer.from(value), hash = digest(bytes)
  const source = join(fixture.publicRoot, `source-${hash}.${extension}`)
  await writeFile(source, bytes)
  return { source, artifact: { url: `${directory}/${hash}.${extension}`, sha256: hash, bytes: bytes.length } }
}

async function installedArtifact(fixture, directory, extension, value) {
  const { source, artifact } = await stagedArtifact(fixture, directory, extension, value)
  const path = join(fixture.publicRoot, artifact.url)
  await mkdir(dirname(path), { recursive: true })
  await writeFile(path, await readFile(source))
  return artifact
}

async function commit(fixture, options) {
  const { publishCatalog, withPublishLock } = await publisher()
  return withPublishLock(fixture.publicRoot, () => publishCatalog({ ...fixture, ...options }))
}

async function assertReadableCatalog(fixture, catalog) {
  const { readArtifact } = await publisher()
  for (const reference of Object.values(catalog.assets)) {
    const manifest = JSON.parse(await readArtifact(fixture.publicRoot, reference))
    for (const artifact of [manifest.model, ...manifest.textures, ...Object.values(manifest.clips).map(clip => clip.artifact),
      ...(manifest.metadata ? [manifest.metadata] : [])]) await readArtifact(fixture.publicRoot, artifact)
  }
  for (const key of ['actionAnimations', 'sounds', 'locomotionAudio']) {
    if (catalog[key]) await readArtifact(fixture.publicRoot, catalog[key])
  }
}

const assertRemoved = (fixture, artifact) => assert.rejects(readFile(join(fixture.publicRoot, artifact.url)), { code: 'ENOENT' })

test('installs immutable files and switches one catalog preserving existing entries', async () => {
  const { publishCatalog } = await publisher()
  const fixture = await project()
  const first = await fixture.bundle('equipment/first')
  const firstCatalog = await publishCatalog({ ...fixture, manifests: [first] })
  const second = await fixture.bundle('equipment/second')
  const catalog = await publishCatalog({ ...fixture, previousCatalog: firstCatalog, manifests: [second] })
  assert.deepEqual(Object.keys(catalog.assets).sort(), ['equipment/first', 'equipment/second'])
  assert.deepEqual(catalog.assets['equipment/first'], firstCatalog.assets['equipment/first'])
  for (const artifact of Object.values(catalog.assets)) {
    assert.equal(digest(await readFile(join(fixture.publicRoot, artifact.url))), artifact.sha256)
  }
})

test('immutable mismatch and asset output collision leave the old catalog unchanged', async () => {
  const { publishCatalog } = await publisher()
  const fixture = await project()
  const first = await fixture.bundle('equipment/first')
  const before = await readFile(fixture.catalogPath)
  await mkdir(join(fixture.publicRoot, 'assets/game/equipment/first'), { recursive: true })
  await writeFile(join(fixture.publicRoot, first.manifest.model.url), 'corrupt existing bytes')
  await assert.rejects(publishCatalog({ ...fixture, manifests: [first] }), /immutable.*mismatch/i)
  assert.deepEqual(await readFile(fixture.catalogPath), before)
  const second = await fixture.bundle('equipment/second')
  second.manifest.model = first.manifest.model
  second.files = first.files
  await assert.rejects(publishCatalog({ ...fixture, manifests: [first, second] }), /collision/i)
  assert.deepEqual(await readFile(fixture.catalogPath), before)
})

test('failure after artifact installation leaves previous catalog and unused immutable files', async context => {
  const { publishCatalog } = await publisher()
  const fixture = await project()
  const bundle = await fixture.bundle('equipment/first')
  const before = await readFile(fixture.catalogPath)
  // Fault only the filesystem switch; all staging and immutable writes are real.
  context.mock.method(fs, 'rename', async () => { throw new Error('rename failed: EIO') })
  await assert.rejects(publishCatalog({ ...fixture, manifests: [bundle] }), /rename failed: EIO/)
  assert.deepEqual(await readFile(fixture.catalogPath), before)
  assert.deepEqual(await readFile(join(fixture.publicRoot, bundle.manifest.model.url)), Buffer.from('model'))
})

test('animation-only publication removes old manifest and clip while preserving the entire current catalog', async context => {
  const fixture = await project(context)
  const actor = await fixture.bundle('character/test')
  const directory = dirname(actor.manifest.model.url)
  const texture = await stagedArtifact(fixture, `${directory}/textures`, 'ktx2', 'current texture')
  const metadata = await stagedArtifact(fixture, directory, 'json', '{"current":"metadata"}')
  const walk = await stagedArtifact(fixture, `${directory}/animations`, 'glb', 'old walk')
  const idle = await stagedArtifact(fixture, `${directory}/animations`, 'glb', 'current idle')
  actor.manifest.textures = [texture.artifact]
  actor.manifest.metadata = metadata.artifact
  actor.manifest.clips = { walk: { artifact: walk.artifact }, idle: { artifact: idle.artifact } }
  actor.files.push(texture, metadata, walk, idle)
  const equipment = await fixture.bundle('equipment/untouched', Buffer.from('unrelated model'))
  const previousCatalog = await commit(fixture, { manifests: [actor, equipment] })
  const nextWalk = await stagedArtifact(fixture, `${directory}/animations`, 'glb', 'new walk')
  const changed = structuredClone(actor)
  changed.manifest.clips.walk.artifact = nextWalk.artifact
  changed.files = [nextWalk]
  const oldTexture = await installedArtifact(fixture, `${directory}/textures`, 'ktx2', 'unused texture')
  const catalog = await commit(fixture, { previousCatalog, manifests: [changed] })

  await assertRemoved(fixture, previousCatalog.assets['character/test'])
  await assertRemoved(fixture, walk.artifact)
  await assertRemoved(fixture, oldTexture)
  assert.deepEqual(catalog.assets['equipment/untouched'], previousCatalog.assets['equipment/untouched'])
  await assertReadableCatalog(fixture, catalog)
  for (const artifact of [actor.manifest.model, texture.artifact, metadata.artifact, idle.artifact, nextWalk.artifact, equipment.manifest.model]) {
    assert.equal(digest(await readFile(join(fixture.publicRoot, artifact.url))), artifact.sha256)
  }
})

test('replacement cleans previous and recipe-only outputs without touching manual files or unrelated trees', async context => {
  const fixture = await project(context)
  const original = await fixture.bundle('equipment/test')
  const previousCatalog = await commit(fixture, { manifests: [original] })
  const replacement = await fixture.bundle('equipment/test', Buffer.from('replacement model'))
  replacement.manifest.model.url = replacement.manifest.model.url.replace('/equipment/test/', '/equipment/moved/')
  const recipeRoot = '/assets/game/equipment/not-yet-published'
  const orphan = await installedArtifact(fixture, recipeRoot, 'glb', 'failed earlier build')
  const nestedOrphan = await installedArtifact(fixture, `${recipeRoot}/animations`, 'glb', 'failed earlier clip')
  const unrelated = await installedArtifact(fixture, '/assets/game/equipment/unmanaged', 'glb', 'unmanaged content')
  const deeper = await installedArtifact(fixture, `${recipeRoot}/other`, 'json', '{"manual":"subtree"}')
  const manual = join(fixture.publicRoot, recipeRoot, 'manual.glb')
  await writeFile(manual, 'keep manual file')
  const catalog = await commit(fixture, { previousCatalog, manifests: [replacement], runtimePaths: [recipeRoot] })

  for (const artifact of [original.manifest.model, previousCatalog.assets['equipment/test'], orphan, nestedOrphan]) await assertRemoved(fixture, artifact)
  for (const artifact of [unrelated, deeper]) assert.equal(digest(await readFile(join(fixture.publicRoot, artifact.url))), artifact.sha256)
  assert.equal(await readFile(manual, 'utf8'), 'keep manual file')
  await assertReadableCatalog(fixture, catalog)
})

test('generated projection cleanup removes only obsolete hash-named JSON', async context => {
  const fixture = await project(context)
  const previousCatalog = await commit(fixture, { manifests: [], actionAnimationDefinitions: [], soundDefinitions: [], locomotionAudioDefinitions: [] })
  const obsolete = [], preserved = []
  for (const directory of ['action_animations', 'sounds', 'locomotion_audio']) {
    const root = `/assets/game/${directory}`
    obsolete.push(await installedArtifact(fixture, root, 'json', `{"obsolete":"${directory}"}`))
    preserved.push(await installedArtifact(fixture, root, 'glb', `manual ${directory}`))
    await writeFile(join(fixture.publicRoot, root, 'manual.json'), 'manual projection')
  }
  const catalog = await commit(fixture, { previousCatalog, manifests: [] })

  for (const artifact of obsolete) await assertRemoved(fixture, artifact)
  for (const artifact of preserved) assert.equal(digest(await readFile(join(fixture.publicRoot, artifact.url))), artifact.sha256)
  for (const directory of ['action_animations', 'sounds', 'locomotion_audio']) {
    assert.equal(await readFile(join(fixture.publicRoot, 'assets/game', directory, 'manual.json'), 'utf8'), 'manual projection')
  }
  assert.deepEqual(catalog, previousCatalog)
  await assertReadableCatalog(fixture, catalog)
})

test('failed catalog switch retains the working version and a later success removes abandoned artifacts', async context => {
  const fixture = await project(context)
  const original = await fixture.bundle('equipment/test', Buffer.from('working model'))
  const previousCatalog = await commit(fixture, { manifests: [original] })
  const before = await readFile(fixture.catalogPath)
  const abandoned = await fixture.bundle('equipment/test', Buffer.from('abandoned model'))
  const rename = context.mock.method(fs, 'rename', async () => { throw new Error('catalog switch failed: EIO') })
  await assert.rejects(commit(fixture, { previousCatalog, manifests: [abandoned] }), /catalog switch failed: EIO/)
  rename.mock.restore()
  assert.deepEqual(await readFile(fixture.catalogPath), before)
  await assertReadableCatalog(fixture, previousCatalog)
  assert.deepEqual(await readFile(join(fixture.publicRoot, abandoned.manifest.model.url)), Buffer.from('abandoned model'))
  const replacement = await fixture.bundle('equipment/test', Buffer.from('final model'))
  const catalog = await commit(fixture, { previousCatalog, manifests: [replacement] })

  await assertRemoved(fixture, abandoned.manifest.model)
  await assertRemoved(fixture, original.manifest.model)
  assert.equal((await readdir(dirname(join(fixture.publicRoot, replacement.manifest.model.url)))).filter(name => /^[a-f0-9]{64}\.json$/.test(name)).length, 1)
  await assertReadableCatalog(fixture, catalog)
})

test('artifact cleanup skips symlink files, managed directories and symlink ancestors', async context => {
  const fixture = await project(context)
  const target = '/assets/game/equipment/target'
  const artifact = await installedArtifact(fixture, target, 'glb', 'linked data must survive')
  const linkRoot = '/assets/game/equipment/linked'
  await symlink(join(fixture.publicRoot, target), join(fixture.publicRoot, linkRoot), 'dir')
  const live = await fixture.bundle('equipment/test')
  const previousCatalog = await commit(fixture, { manifests: [live] })
  const fileLink = join(fixture.publicRoot, 'assets/game/equipment/test', `${'a'.repeat(64)}.glb`)
  await symlink(join(fixture.publicRoot, artifact.url), fileLink)
  const texturesLink = join(fixture.publicRoot, 'assets/game/equipment/test/textures')
  await symlink(join(fixture.publicRoot, target), texturesLink, 'dir')
  const ancestorTarget = join(fixture.publicRoot, 'elsewhere')
  await mkdir(join(ancestorTarget, 'nested'), { recursive: true })
  const ancestorFile = join(ancestorTarget, 'nested', `${'b'.repeat(64)}.glb`)
  await writeFile(ancestorFile, 'symlink ancestor data')
  await symlink(ancestorTarget, join(fixture.publicRoot, 'assets/game/linked_category'), 'dir')
  const catalog = await commit(fixture, { previousCatalog, manifests: [], runtimePaths: [linkRoot, '/assets/game/linked_category/nested'] })

  assert.equal(digest(await readFile(join(fixture.publicRoot, artifact.url))), artifact.sha256)
  assert.equal(await readFile(ancestorFile, 'utf8'), 'symlink ancestor data')
  for (const path of [fileLink, texturesLink, join(fixture.publicRoot, linkRoot)]) assert.ok((await lstat(path)).isSymbolicLink())
  await assertReadableCatalog(fixture, catalog)
})

test('complete retained catalog closure is verified before any obsolete artifact is removed', async context => {
  const fixture = await project(context)
  const first = await fixture.bundle('equipment/first'), retained = await fixture.bundle('equipment/retained')
  const previousCatalog = await commit(fixture, { manifests: [first, retained] })
  const orphan = await installedArtifact(fixture, dirname(first.manifest.model.url), 'glb', 'must survive failed closure validation')
  await writeFile(join(fixture.publicRoot, retained.manifest.model.url), 'corrupted retained model')
  const warnings = []
  context.mock.method(process.stderr, 'write', message => { warnings.push(String(message)); return true })
  const replacement = await fixture.bundle('equipment/first', Buffer.from('new selected model'))
  const catalog = await commit(fixture, { previousCatalog, manifests: [replacement] })

  assert.deepEqual(JSON.parse(await readFile(fixture.catalogPath)), catalog)
  assert.match(warnings.join(''), /Publication committed; artifact cleanup failed:.*Immutable content mismatch/)
  for (const artifact of [orphan, first.manifest.model, previousCatalog.assets['equipment/first']]) {
    assert.equal(digest(await readFile(join(fixture.publicRoot, artifact.url))), artifact.sha256)
  }
})

test('cleanup filesystem errors report committed publication and leave its current catalog readable', async context => {
  const fixture = await project(context)
  const original = await fixture.bundle('equipment/test', Buffer.from('old model'))
  const previousCatalog = await commit(fixture, { manifests: [original] })
  const obsoletePath = join(fixture.publicRoot, original.manifest.model.url)
  const originalUnlink = fs.unlink.bind(fs), warnings = []
  context.mock.method(fs, 'unlink', async path => {
    if (path === obsoletePath) throw new Error('obsolete artifact unlink failed: EACCES')
    return originalUnlink(path)
  })
  context.mock.method(process.stderr, 'write', message => { warnings.push(String(message)); return true })
  const replacement = await fixture.bundle('equipment/test', Buffer.from('new model'))
  const catalog = await commit(fixture, { previousCatalog, manifests: [replacement] })

  assert.deepEqual(JSON.parse(await readFile(fixture.catalogPath)), catalog)
  assert.match(warnings.join(''), /Publication committed; artifact cleanup failed: obsolete artifact unlink failed: EACCES/)
  await assertReadableCatalog(fixture, catalog)
  assert.equal(await readFile(obsoletePath, 'utf8'), 'old model')
})

test('a second publisher waits for the entire read-modify-publish transaction', async () => {
  const { withPublishLock, publishCatalog, readCatalog } = await publisher()
  const fixture = await project()
  const first = await fixture.bundle('equipment/first'), second = await fixture.bundle('equipment/second')
  let entered
  const inside = new Promise(resolve => { entered = resolve })
  let release
  const barrier = new Promise(resolve => { release = resolve })
  const firstBuild = withPublishLock(fixture.publicRoot, async () => {
    const previousCatalog = await readCatalog(fixture.publicRoot)
    entered()
    await barrier
    return publishCatalog({ ...fixture, previousCatalog, manifests: [first] })
  })
  await inside
  let secondEntered = false
  const secondBuild = withPublishLock(fixture.publicRoot, async () => {
    secondEntered = true
    const previousCatalog = await readCatalog(fixture.publicRoot)
    return publishCatalog({ ...fixture, previousCatalog, manifests: [second] })
  })
  await new Promise(resolve => setTimeout(resolve, 60))
  assert.equal(secondEntered, false)
  release()
  await Promise.all([firstBuild, secondBuild])
  assert.deepEqual(Object.keys((await readCatalog(fixture.publicRoot)).assets).sort(), ['equipment/first', 'equipment/second'])
})

test('locks release on exceptions, recover dead local owners, and preserve live/unknown owners', async () => {
  const { withPublishLock } = await publisher()
  const fixture = await project()
  await assert.rejects(withPublishLock(fixture.publicRoot, async () => { throw new Error('deliberate') }), /deliberate/)
  await withPublishLock(fixture.publicRoot, async () => {})
  const child = spawn(process.execPath, ['-e', 'process.exit(0)'])
  await new Promise(resolve => child.once('exit', resolve))
  const lockPath = join(fixture.publicRoot, 'assets/game/.publish.lock')
  const owner = { pid: child.pid, hostname: hostname(), uid: process.getuid(), token: 'dead-owner' }
  await writeFile(lockPath, JSON.stringify(owner))
  await withPublishLock(fixture.publicRoot, async () => {})
  for (const info of [{ ...owner, pid: process.pid }, { ...owner, hostname: 'another-machine' }]) {
    await writeFile(lockPath, JSON.stringify(info))
    const before = await readFile(lockPath)
    await assert.rejects(withPublishLock(fixture.publicRoot, async () => assert.fail('must not enter'), { timeoutMs: 80 }), /lock/i)
    assert.deepEqual(await readFile(lockPath), before)
    await rm(lockPath)
  }
  assert.equal((await readdir(join(fixture.publicRoot, 'assets/game'))).filter(name => name.startsWith('.publish')).length, 0)
})

test('recovery cannot remove a new owner after the inspected owner releases and exits', { timeout: 10_000 }, async context => {
  const { withPublishLock } = await publisher()
  const fixture = await project()
  const lockPath = join(fixture.publicRoot, 'assets/game/.publish.lock')
  const script = `
    import { withPublishLock } from ${JSON.stringify(new URL('../publish.mjs', import.meta.url).href)};
    import { once } from 'node:events';
    import fs from 'node:fs/promises';
    await withPublishLock(process.argv[1], async () => {
      process.send('owns-lock');
      await once(process, 'message');
      // Report the real release syscall outcome, without changing its behavior.
      const mkdir = fs.mkdir.bind(fs), unlink = fs.unlink.bind(fs);
      let reported = false;
      const report = state => { if (!reported) { reported = true; process.send(state); } };
      fs.mkdir = async (...args) => {
        try { return await mkdir(...args); } catch (error) {
          if (error.code === 'EEXIST') report('release-waits');
          throw error;
        }
      };
      fs.unlink = async (...args) => {
        const result = await unlink(...args);
        if (args[0] === ${JSON.stringify(lockPath)}) report('released');
        return result;
      };
    });
    process.disconnect();
  `
  const ownerA = spawn(process.execPath, ['--input-type=module', '-e', script, fixture.publicRoot],
    { stdio: ['ignore', 'ignore', 'pipe', 'ipc'] })
  context.after(() => { if (ownerA.exitCode === null) ownerA.kill() })
  assert.deepEqual(await once(ownerA, 'message'), ['owns-lock', undefined])
  const ownerAExited = once(ownerA, 'exit')
  const inspected = Promise.withResolvers(), resumeInspection = Promise.withResolvers()
  const originalRead = fs.readFile.bind(fs)
  let paused = false
  context.mock.method(fs, 'readFile', async (...args) => {
    const bytes = await originalRead(...args)
    if (args[0] === lockPath && !paused && JSON.parse(bytes).pid === ownerA.pid) {
      paused = true
      inspected.resolve()
      await resumeInspection.promise
    }
    return bytes
  })
  let active = 0, maximumActive = 0
  const recoveringEntered = Promise.withResolvers()
  const recovering = withPublishLock(fixture.publicRoot, async () => {
    maximumActive = Math.max(maximumActive, ++active)
    recoveringEntered.resolve()
    --active
  })
  await inspected.promise
  const releaseRequested = once(ownerA, 'message')
  ownerA.send('release')
  const [releaseState] = await releaseRequested
  assert.ok(['released', 'release-waits'].includes(releaseState))
  const earlyExit = releaseState === 'released'
  if (earlyExit) await ownerAExited
  const newOwnerEntered = Promise.withResolvers(), releaseNewOwner = Promise.withResolvers()
  const newOwner = withPublishLock(fixture.publicRoot, async () => {
    maximumActive = Math.max(maximumActive, ++active)
    newOwnerEntered.resolve()
    await releaseNewOwner.promise
    --active
  })
  // Force the old race completely: A exits, B owns the pathname, then resume
  // inspection holding A's bytes. Fixed release reports waiting on the guard.
  if (earlyExit) await newOwnerEntered.promise
  resumeInspection.resolve()
  await newOwnerEntered.promise
  if (earlyExit) await recoveringEntered.promise
  const newOwnerBytes = await originalRead(lockPath, 'utf8').catch(() => null)
  releaseNewOwner.resolve()
  const outcomes = await Promise.allSettled([recovering, newOwner, ownerAExited])
  assert.equal(maximumActive, 1, 'recoverer must not delete a live replacement lock and enter concurrently')
  assert.equal(earlyExit, false, 'release must use the same guard as recovery inspection')
  assert.equal(JSON.parse(newOwnerBytes).pid, process.pid)
  for (const outcome of outcomes) assert.equal(outcome.status, 'fulfilled', String(outcome.reason))
})
