import assert from 'node:assert/strict'
import test from 'node:test'
import { mkdtemp, mkdir, readFile, writeFile, realpath, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createHash } from 'node:crypto'
import { parseSoundFile, parseLocomotionAudioFile } from '../../../web_new/src/types/soundDefs.ts'
import { publishCatalog, readArtifact, withPublishLock } from '../publish.mjs'
import { publishActionAnimations } from '../action-animations.mjs'

const digest = bytes => createHash('sha256').update(bytes).digest('hex')
const profiles = JSON.parse(await readFile(new URL('../../../data/sounds/actions.json', import.meta.url))).sounds
const fixtureCases = JSON.parse(await readFile(new URL('../../../tests/fixtures/sounds/cases.json', import.meta.url)))
for (const fixture of fixtureCases) test(`shared sound validation: ${fixture.name}`, () => {
  const parse = () => parseSoundFile(fixture.input, 'fixture.json')
  if (fixture.valid) assert.doesNotThrow(parse)
  else assert.throws(parse, /fixture.json/)
})

test('locomotion contact schema rejects boundary, order and duplicate selectors', () => {
  const binding = { actor: 'character/test', clip: 'walk', contacts: [{ id: 'left', phase: 0.25, sound_key: 'footstep' }] }
  const parse = value => parseLocomotionAudioFile({ v: 1, bindings: value }, 'fixture')
  assert.equal(parse([binding]).length, 1)
  for (const phase of [0, 1, Infinity]) assert.throws(() => parse([{ ...binding, contacts: [{ ...binding.contacts[0], phase }] }]))
  assert.throws(() => parse([binding, binding]))
  assert.throws(() => parse([{ ...binding, contacts: [binding.contacts[0], { ...binding.contacts[0], id: 'right' }] }]))
})

async function project(t) {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'audio-publication-')))
  t.after(() => rm(root, { recursive: true, force: true }))
  const publicRoot = join(root, 'web_new/public'); await mkdir(publicRoot, { recursive: true })
  for (const profile of profiles) for (const file of profile.files) {
    const path = join(publicRoot, 'assets/game', file); await mkdir(join(path, '..'), { recursive: true })
    await writeFile(path, await readFile(new URL(`../../../web_new/public/assets/game/${file}`, import.meta.url)))
  }
  const bytes = Buffer.from('test model'), hash = digest(bytes), source = join(root, 'model.glb'); await writeFile(source, bytes)
  const model = { url: `/assets/game/character/test/${hash}.glb`, sha256: hash, bytes: bytes.length }, rigHash = 'a'.repeat(64)
  const clips = Object.fromEntries(['walk', 'carry_walk', 'chop'].map(key => [key, { artifact: model, rigHash, duration: 1, loop: true, playback: key === 'chop' ? 'time' : 'distance', ...(key === 'chop' ? {} : { cycleDistanceTiles: 1.75 }) }]))
  const manifest = { schema: 1, id: 'character/test', kind: 'character', rigHash, clips, model, textures: [], bindings: {} }
  const actions = [{ key: 'chop', source: { kind: 'context', namespace: 'tree', id: 'chop' }, actor: 'character/test', variants: [{ clip: 'chop', equipment: [] }], eligibility: [], facing: 'target', blend_ms: 0, frame: { width: 1, height: 1, origin_x: 0, origin_y: 0 }, sound_cues: [{ id: 'impact', phase: 0.6, sound_key: 'chop', source: 'target' }] }]
  const locomotion = ['walk', 'carry_walk'].map(clip => ({ actor: 'character/test', clip, contacts: [{ id: 'right', phase: 0.45, sound_key: 'footstep' }, { id: 'left', phase: 0.95, sound_key: 'footstep' }] }))
  const initial = { schema: 1, assets: {} }
  const options = { publicRoot, previousCatalog: initial, manifests: [{ manifest, files: [{ source, artifact: model }] }], actionAnimationDefinitions: actions, soundDefinitions: profiles, locomotionAudioDefinitions: locomotion }
  const publish = input => withPublishLock(publicRoot, () => publishCatalog(input))
  const catalog = await publish(options)
  return { root, options, catalog, actions, locomotion, publish, bytes: () => readFile(join(publicRoot, 'assets/game/asset-catalog.json')) }
}

test('audio/action projections publish reproducibly together with manifest-derived stride', async t => {
  const fixture = await project(t), before = await fixture.bytes()
  assert.ok(fixture.catalog.sounds && fixture.catalog.actionAnimations && fixture.catalog.locomotionAudio)
  const locomotion = JSON.parse(await readArtifact(fixture.options.publicRoot, fixture.catalog.locomotionAudio))
  assert.ok(locomotion.bindings.every(binding => binding.cycle_distance_tiles === 1.75))
  await fixture.publish({ ...fixture.options, previousCatalog: fixture.catalog, manifests: [] })
  assert.deepEqual(await fixture.bytes(), before)
})

test('missing media, cue sounds and locomotion clips leave all old catalogs active', async t => {
  const fixture = await project(t), before = await fixture.bytes()
  const mutations = [
    options => { options.soundDefinitions[0].files = ['sound/missing.mp3'] },
    options => { options.actionAnimationDefinitions[0].sound_cues[0].sound_key = 'missing' },
    options => { options.locomotionAudioDefinitions[0].clip = 'missing' },
    options => { options.locomotionAudioDefinitions[0].contacts[0].sound_key = 'chop' },
  ]
  for (const mutate of mutations) {
    const changed = structuredClone({ ...fixture.options, previousCatalog: fixture.catalog, manifests: [] }); mutate(changed)
    await assert.rejects(fixture.publish(changed))
    assert.deepEqual(await fixture.bytes(), before)
  }
})

test('menu target-source cue publication validates concrete action targets before switching catalogs', async t => {
  const fixture = await project(t), before = await fixture.bytes()
  await mkdir(join(fixture.root, 'data/action_animations'), { recursive: true })
  await mkdir(join(fixture.root, 'data/actions'), { recursive: true })
  const actions = structuredClone(fixture.actions)
  actions[0].source = { kind: 'menu', id: 'sample_menu' }
  await writeFile(join(fixture.root, 'data/action_animations/menu.json'), JSON.stringify({ v: 1, bindings: actions }))
  const actionFile = join(fixture.root, 'data/actions/menu.json')
  await writeFile(actionFile, JSON.stringify({ v: 1, actions: [{ id: 'sample_menu', target: { kind: 'none' } }] }))
  await assert.rejects(publishActionAnimations({ root: fixture.root }), /no available target sound source/)
  assert.deepEqual(await fixture.bytes(), before)
  for (const kind of ['object', 'tile']) {
    await writeFile(actionFile, JSON.stringify({ v: 1, actions: [{ id: 'sample_menu', target: { kind } }] }))
    const catalog = await publishActionAnimations({ root: fixture.root })
    assert.ok(catalog.actionAnimations && catalog.sounds && catalog.locomotionAudio)
  }
})
