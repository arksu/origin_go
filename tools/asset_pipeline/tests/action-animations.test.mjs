import assert from 'node:assert/strict'
import test from 'node:test'
import { mkdtemp, mkdir, readFile, writeFile, realpath, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createHash } from 'node:crypto'
import { parseActionAnimationFile, validateAnimationUniqueness } from '../../../web_new/src/types/actionAnimationDefs.ts'
import { publishCatalog, readArtifact, readCatalog, withPublishLock } from '../publish.mjs'
import { runCli } from '../cli.ts'

const fixtureURL = new URL('../../../tests/fixtures/action_animations/', import.meta.url)
const digest = bytes => createHash('sha256').update(bytes).digest('hex')
const definitions = parseActionAnimationFile(JSON.parse(await readFile(new URL('bindings.json', fixtureURL))), 'fixture')

for (const fixture of JSON.parse(await readFile(new URL('cases.json', fixtureURL)))) {
  test(`shared definition validation: ${fixture.name}`, () => {
    const parse = () => validateAnimationUniqueness(parseActionAnimationFile(fixture.input, 'fixture.json'), 'fixture.json')
    if (fixture.valid) assert.doesNotThrow(parse)
    else assert.throws(parse, /fixture.json/)
  })
}

async function project(t) {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'action-animation-publication-')))
  t.after(() => rm(root, { recursive: true, force: true }))
  const publicRoot = join(root, 'web_new/public')
  await mkdir(publicRoot, { recursive: true })
  await mkdir(join(root, 'data/action_animations'), { recursive: true })
  await writeFile(join(root, 'data/action_animations/bindings.json'), JSON.stringify({ v: 1, bindings: definitions }))
  const manifests = new Map()
  for (const definition of definitions) {
    if (!manifests.has(definition.actor)) manifests.set(definition.actor, { kind: 'character', rigHash: 'a'.repeat(64), clips: {}, bindings: {}, textures: [] })
    const actor = manifests.get(definition.actor)
    for (const variant of definition.variants) actor.clips[variant.clip] = { rigHash: actor.rigHash, duration: 3 }
    for (const equipment of definition.variants.flatMap(variant => variant.equipment)) {
      const id = `equipment/${equipment.visual_key}`
      if (!manifests.has(id)) manifests.set(id, { kind: 'equipment', rigHash: null, clips: {}, bindings: {}, textures: [] })
      manifests.get(id).bindings[equipment.slot] = { slot: equipment.slot }
    }
  }
  const bundles = []
  for (const [id, manifest] of manifests) {
    const bytes = Buffer.from(id), hash = digest(bytes), source = join(root, `${hash}.glb`)
    await writeFile(source, bytes)
    const model = { url: `/assets/game/${id}/${hash}.glb`, sha256: hash, bytes: bytes.length }
    Object.assign(manifest, { schema: 1, id, model })
    for (const clip of Object.values(manifest.clips)) clip.artifact = model
    bundles.push({ manifest, files: [{ source, artifact: model }] })
  }
  const catalog = await withPublishLock(publicRoot, () => publishCatalog({ publicRoot, previousCatalog: { schema: 1, assets: {} }, manifests: bundles, actionAnimationDefinitions: definitions }))
  return { root, publicRoot, catalog, bundles, bytes: () => readFile(join(publicRoot, 'assets/game/asset-catalog.json')) }
}

test('def-only command publishes the two bindings without exporting or changing assets', async t => {
  const fixture = await project(t)
  const result = await runCli(['publish-action-animations'], { root: fixture.root, stderr: message => assert.fail(message) })
  assert.equal(result, 0)
  const catalog = await readCatalog(fixture.publicRoot)
  assert.deepEqual(catalog.assets, fixture.catalog.assets)
  const projection = JSON.parse(await readArtifact(fixture.publicRoot, catalog.actionAnimations))
  assert.deepEqual(projection.bindings.map(binding => binding.key), definitions.map(binding => binding.key))
  assert.ok(projection.bindings.every(binding => !('source' in binding)))
  assert.equal(await runCli(['publish-action-animations', 'all'], { stderr: () => {} }), 1)
})

test('missing clips, incompatible equipment and invalid frame bounds preserve previous public catalog', async t => {
  const fixture = await project(t), before = await fixture.bytes()
  const mutations = [
    binding => { binding.variants[0].clip = 'missing_reference' },
    binding => { binding.variants[0].equipment[0].visual_key = 'missing_reference' },
    binding => { binding.frame.width = 1025 },
  ]
  for (const mutate of mutations) {
    const changed = structuredClone(definitions); mutate(changed[0])
    await assert.rejects(withPublishLock(fixture.publicRoot, () => publishCatalog({ publicRoot: fixture.publicRoot, previousCatalog: fixture.catalog, manifests: [], actionAnimationDefinitions: changed })))
    assert.deepEqual(await fixture.bytes(), before)
  }
})

test('partial asset publication validates existing projection against merged manifests', async t => {
  const fixture = await project(t), before = await fixture.bytes()
  const changed = structuredClone(fixture.bundles.find(bundle => bundle.manifest.kind === 'character'))
  delete changed.manifest.clips[definitions[0].variants[0].clip]
  await assert.rejects(withPublishLock(fixture.publicRoot, () => publishCatalog({ publicRoot: fixture.publicRoot, previousCatalog: fixture.catalog, manifests: [changed] })), /clip/)
  assert.deepEqual(await fixture.bytes(), before)
})
