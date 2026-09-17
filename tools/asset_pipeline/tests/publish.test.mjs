import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { hostname, tmpdir } from 'node:os'
import { join } from 'node:path'
import { mkdir, mkdtemp, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import fs from 'node:fs/promises'
import test from 'node:test'

const digest = bytes => createHash('sha256').update(bytes).digest('hex')
const publisher = () => import('../publish.mjs')
async function project() {
  const publicRoot = await realpath(await mkdtemp(join(tmpdir(), 'asset-publish-test-')))
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
