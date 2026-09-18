import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import test from 'node:test'
import { accessorValues, blenderPath, createFixture, fileHash, runExport } from './blender_helpers.ts'
import { exportAsset } from '../blender.ts'

test('model output ignores current pose and never changes the blend', async () => {
  const fixture = await createFixture('character', { savedFrame: 6, savedAction: 'walk', savedPose: true, savedNla: true, hidden: true })
  const before = await fixture.sourceHash()
  const first = await runExport(fixture.recipe, fixture.directory('first'))
  assert.equal(before, await fixture.sourceHash())
  await fixture.saveEditorState({ frame: 1, action: 'idle' })
  const secondSource = await fixture.sourceHash()
  const second = await runExport(fixture.recipe, fixture.directory('second'))
  assert.equal(first.modelHash, second.modelHash)
  assert.equal(secondSource, await fixture.sourceHash())
  assert.notEqual(before, secondSource)
  assert.equal(first.model.animations, undefined)
  assert.equal(first.model.nodes.some((node: any) => node.name === 'NeverExportPreview'), false)
  assert.deepEqual(first.metadata.rig, second.metadata.rig)
  for (const name of ['idle', 'walk']) {
    const clip = await first.clip(name)
    assert.equal(clip.animations.length, 1)
    assert.equal(clip.animations[0].name, name)
    assert.equal(clip.meshes, undefined)
    assert.equal(clip.images, undefined)
    assert.equal(clip.nodes.some((node: any) => node.mesh !== undefined), false)
    assert.equal(first.metadata.clips[name].durationSeconds, 11 / 24)
  }
})

test('static model and transformed grips export full inverse frames in glTF basis', async () => {
  const fixture = await createFixture('equipment')
  const result = await runExport(fixture.recipe, fixture.directory('export'))
  // Blender right grip T(1,2,3) Rz(90): inverse translation (-2,1,-3), then Y-up basis.
  const expected = [0, 0, 1, 0, 0, 1, 0, 0, -1, 0, 0, 0, -2, -3, -1, 1]
  result.metadata.bindings.right.gripInverse.forEach((value: number, index: number) => assert.ok(Math.abs(value - expected[index]!) < 1e-6))
  assert.equal(result.metadata.rig, null)
  assert.equal(result.model.meshes.length, 1)
  // Left inverse: Rz(+90) * -(-2,1,.5) = (1,2,-.5), converted to (1,-.5,-2).
  const left = result.metadata.bindings.left.gripInverse
  assert.ok(Math.abs(left[12] - 1) < 1e-6 && Math.abs(left[13] + 0.5) < 1e-6 && Math.abs(left[14] + 2) < 1e-6)
})

for (const [flag, message] of Object.entries({ missingSocket: 'socket', duplicateNames: 'name collision',
  invalidWeights: 'weights', nonFinite: 'finite', externalImage: 'undeclared', externalLibrary: 'undeclared',
  badLoop: 'loop closure', driver: 'driver', badGrip: 'scale' })) {
  test(`rejects ${flag} without modifying input`, async () => {
    const fixture = await createFixture(flag === 'badGrip' ? 'equipment' : 'character', { [flag]: true })
    const before = await fixture.sourceHash()
    const dependency = flag === 'externalImage' ? 'undeclared.png' : flag === 'externalLibrary' ? 'undeclared.blend' : null
    const dependencyHash = dependency ? await fileHash(join(fixture.root, dependency)) : null
    await assert.rejects(runExport(fixture.recipe, fixture.directory('export')), new RegExp(message, 'i'))
    assert.equal(before, await fixture.sourceHash())
    if (dependency) assert.equal(dependencyHash, await fileHash(join(fixture.root, dependency)))
  })
}

test('native constraints are sampled and authored masks exclude unrelated bones', async () => {
  const fixture = await createFixture('character', { constraint: true, mask: ['hand.r'] })
  const result = await runExport(fixture.recipe, fixture.directory('export'), ['walk'])
  const clip = await result.clip('walk')
  assert.ok(clip.animations[0].channels.length > 0)
  assert.ok(clip.animations[0].channels.every((channel: any) => clip.nodes[channel.target.node].name === 'hand.r'))
  assert.ok(result.metadata.clips.walk.sampleCount > 12)
  assert.equal(result.metadata.clips.idle, undefined)
  assert.ok((await readFile(join(fixture.root, 'export/raw/animations/walk.glb'))).length > 0)
  const bytes = await readFile(join(fixture.root, 'export/raw/animations/walk.glb'))
  const animation = clip.animations[0]
  const rotation = animation.channels.find((channel: any) => channel.target.path === 'rotation')
  const sampler = animation.samplers[rotation.sampler]
  const times = accessorValues(bytes, sampler.input)
  const values = accessorValues(bytes, sampler.output)
  const frame6 = times.findIndex(time => Math.abs(time[0]! - 5 / 24) < 1e-6)
  // COPY_ROTATION at .5 influence halves the authored .5 radian local angle.
  assert.ok(Math.abs(2 * Math.acos(Math.abs(values[frame6]![3]!)) - 0.25) < 1e-5)
})

test('socket local matrix includes Blender bone-parent tail offset', async () => {
  const fixture = await createFixture('character')
  const result = await runExport(fixture.recipe, fixture.directory('export'), [])
  const socket = result.metadata.sockets.socket_hand_right
  assert.equal(socket.parent, 'hand.r')
  // Bone parent uses its tail: local Blender translation (.25, .5 + length1, -.75).
  assert.ok(Math.abs(socket.localMatrix[12] - 0.25) < 1e-6)
  assert.ok(Math.abs(socket.localMatrix[13] - 1.5) < 1e-6)
  assert.ok(Math.abs(socket.localMatrix[14] + 0.75) < 1e-6)
})

test('model evaluates static modifiers', async () => {
  const fixture = await createFixture('world_object', { modifier: true })
  const result = await runExport(fixture.recipe, fixture.directory('export'))
  assert.ok(result.model.accessors[result.model.meshes[0].primitives[0].indices].count > 3)
})

test('exporter normalizes excessive skin influences in memory without changing source', async () => {
  const skinned = await createFixture('character', { tooManyInfluences: true })
  skinned.recipe.budgets.boneInfluences = 1
  const sourceHash = await skinned.sourceHash()
  const result = await runExport(skinned.recipe, skinned.directory('export'))
  assert.equal(await skinned.sourceHash(), sourceHash)
  const modelBytes = await readFile(join(skinned.root, 'export/raw/model.glb'))
  const primitive = result.model.meshes[0].primitives[0]
  const weights = accessorValues(modelBytes, primitive.attributes.WEIGHTS_0)
  for (const vertex of weights) {
    assert.ok(Math.abs(vertex.reduce((sum, weight) => sum + weight, 0) - 1) < 1e-6)
    assert.equal(vertex.filter(weight => weight > 1e-6).length, 1)
  }
})

for (const [options, message] of [[{ mask: ['root'] }, /mask/], [{ mask: ['unknown'] }, /mask/],
  [{ unsafeConstraint: true }, /constraint/], [{ unbakedModifier: true }, /modifier/]] as const) {
  test(`rejects unsupported source contract ${JSON.stringify(options)}`, async () => {
    const fixture = await createFixture('character', options)
    await assert.rejects(runExport(fixture.recipe, fixture.directory('export')), message)
  })
}

test('required rig bones can be a subset without shrinking default clip channel masks', async () => {
  const fixture = await createFixture('character')
  fixture.recipe.rig!.bones = ['root']
  const result = await runExport(fixture.recipe, fixture.directory('export'), ['walk'])
  assert.deepEqual(result.metadata.clips.walk.channelMask, ['hand.r', 'root'])
})

test('fractional frame ranges fail explicitly instead of truncating', async () => {
  const fixture = await createFixture('character')
  fixture.recipe.clips.walk!.range.start = 1.5
  await assert.rejects(runExport(fixture.recipe, fixture.directory('export')), /range/)
})

test('packed texture bytes are extracted without changing source or acquiring clip dependencies', async () => {
  const fixture = await createFixture('character', { packedTexture: true })
  const before = await fixture.sourceHash()
  const result = await runExport(fixture.recipe, fixture.directory('export'), ['idle'])
  assert.equal(await fixture.sourceHash(), before)
  assert.equal(result.metadata.textures.length, 1)
  const texture = result.metadata.textures[0]
  assert.equal(await fileHash(join(fixture.root, 'export/raw', texture.path)), texture.sha256)
  assert.equal(result.model.images.length, 1)
  const clip = await result.clip('idle')
  assert.equal(clip.images, undefined)
  assert.equal(clip.materials, undefined)
})

test('declared local preview library is accepted and remains unmodified', async () => {
  const fixture = await createFixture('world_object', { externalLibrary: true })
  const dependency = join(fixture.root, 'undeclared.blend')
  const before = await fileHash(dependency)
  fixture.recipe.dependencies.preview = ['world_object/preview']
  const metadata = await exportAsset({ recipe: fixture.recipe, selectedClips: [],
    resolvedDependencies: [{ assetId: 'world_object/preview', absolutePath: dependency }] }, fixture.directory('export'), blenderPath)
  assert.equal(before, await fileHash(dependency))
  assert.equal(metadata.assetId, fixture.recipe.id)
})

test('samples between keys reject a singular interpolated pose despite valid loop endpoints', async () => {
  const fixture = await createFixture('character', { singularBetweenKeys: true })
  await assert.rejects(runExport(fixture.recipe, fixture.directory('export'), ['walk']), /singular sampled pose .* at 6.5/)
})

test('non-animation export fingerprint includes model settings and excludes selected clip list', async () => {
  const fixture = await createFixture('character')
  const first = await runExport(fixture.recipe, fixture.directory('first'), [])
  const clips = await runExport(fixture.recipe, fixture.directory('clips'), ['idle'])
  assert.equal(first.metadata.modelFingerprint, clips.metadata.modelFingerprint)
  fixture.recipe.optimization.texture.quality += 1
  const settings = await runExport(fixture.recipe, fixture.directory('settings'), [])
  assert.equal(first.modelHash, settings.modelHash)
  assert.notEqual(first.metadata.modelFingerprint, settings.metadata.modelFingerprint)
})

test('clip sampling obeys the entire recipe range including held endpoints outside authored keys', async () => {
  const fixture = await createFixture('character')
  fixture.recipe.clips.walk!.range = { start: 0, end: 13 }
  const result = await runExport(fixture.recipe, fixture.directory('export'), ['walk'])
  const clip = await result.clip('walk')
  const bytes = await readFile(join(fixture.root, 'export/raw/animations/walk.glb'))
  const times = accessorValues(bytes, clip.animations[0].samplers[0].input).flat()
  assert.equal(times.length, 14)
  assert.ok(Math.abs(times.at(-1)! - 13 / 24) < 1e-6)
})

test('worker rejects frame and fps values outside native Blender representation', async () => {
  const fixture = await createFixture('character')
  fixture.recipe.clips.walk!.range.start = -1
  await assert.rejects(runExport(fixture.recipe, fixture.directory('negative')), /range/)
  fixture.recipe.clips.walk!.range.start = 1
  fixture.recipe.clips.walk!.fps = 1e-10
  await assert.rejects(runExport(fixture.recipe, fixture.directory('fps')), /fps/)
})

test('optional equipment rig declarations still require the declared rig object', async () => {
  const fixture = await createFixture('equipment')
  fixture.recipe.rig = { object: 'MissingRig', bones: ['root'], sockets: [] }
  await assert.rejects(runExport(fixture.recipe, fixture.directory('export')), /rig/)
})

test('exports only intentional LOD/skinning extras without author paths or editor properties', async () => {
  const fixture = await createFixture('character', { customExtras: true })
  const result = await runExport(fixture.recipe, fixture.directory('export'), ['walk'])
  const body = result.model.nodes.find((node: any) => node.name === 'Body')
  assert.deepEqual(body.extras, { lod: 0, skinned: true })
  assert.equal(JSON.stringify(result.model).includes(fixture.root), false)
  assert.equal(JSON.stringify(await result.clip('walk')).includes('private-editor-state'), false)
})

test('raw model and independent masked animation satisfy Khronos glTF validation', async () => {
  const validator = createRequire(import.meta.url)('gltf-validator') as {
    validateBytes(bytes: Uint8Array): Promise<{ issues: { numErrors: number; messages: unknown[] } }>
  }
  const fixture = await createFixture('character', { packedTexture: true, mask: ['hand.r'] })
  await runExport(fixture.recipe, fixture.directory('export'), ['walk'])
  for (const filename of ['model.glb', 'animations/walk.glb']) {
    const result = await validator.validateBytes(await readFile(join(fixture.root, 'export/raw', filename)))
    assert.equal(result.issues.numErrors, 0, JSON.stringify(result.issues.messages))
  }
})

test('native IK secondary targets must be declared inside EXPORT', async () => {
  const fixture = await createFixture('character', { undeclaredPoleTarget: true })
  await assert.rejects(runExport(fixture.recipe, fixture.directory('export')), /undeclared.*target/)
})

test('runtime mesh skinning and material region extras survive author-property filtering', async () => {
  const fixture = await createFixture('character', { packedTexture: true, runtimeExtras: true, customExtras: true })
  const result = await runExport(fixture.recipe, fixture.directory('export'), [])
  assert.deepEqual(result.model.nodes.find((node: any) => node.name === 'Body').extras,
    { lod: 0, skinned: true, skinning: 'linear' })
  assert.deepEqual(result.model.materials.find((material: any) => material.name === 'Surface').extras, { region: 'textured' })
  assert.equal(JSON.stringify(result.model).includes(fixture.root), false)
  assert.equal(JSON.stringify(result.model).includes('private-editor-state'), false)
})

for (const overrides of [{ skinning: 'unknown' }, { region: 'unknown' }]) {
  test(`rejects unsupported runtime extras ${JSON.stringify(overrides)}`, async () => {
    const fixture = await createFixture('character', { packedTexture: true, runtimeExtras: true, ...overrides })
    await assert.rejects(runExport(fixture.recipe, fixture.directory('export'), []), /skinning|region/)
  })
}

test('saved singular pose cannot reject or change the canonical rest model', async () => {
  const fixture = await createFixture('character')
  const first = await runExport(fixture.recipe, fixture.directory('rest'), [])
  await fixture.saveEditorState({ frame: 6, action: 'walk', singularPose: true })
  const sourceHash = await fixture.sourceHash()
  const singular = await runExport(fixture.recipe, fixture.directory('singular'), [])
  assert.equal(singular.modelHash, first.modelHash)
  assert.equal(await fixture.sourceHash(), sourceHash)
})
