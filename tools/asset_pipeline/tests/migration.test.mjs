import assert from 'node:assert/strict'
import { access, mkdir, readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import test from 'node:test'
import { NodeIO } from '@gltf-transform/core'
import { loadRecipes } from '../catalog.ts'
import { exportAsset } from '../blender.ts'
import { snapshotGlb } from '../migration/baseline.ts'
import { blenderPath, fileHash, readGlb, runBlender } from './blender_helpers.ts'

const root = resolve(import.meta.dirname, '../../..')
const characterReferences = join(root, 'art_source/character/male_commoner/references')
test('one-time migration refuses to overwrite an editable canonical source', async () => {
  const source = join(root, 'art_source/character/male_commoner/source.blend')
  const before = await fileHash(source)
  await assert.rejects(runBlender(join(root, 'tools/asset_pipeline/migration/migrate_sources.py'),
    ['--root', root, '--baseline', characterReferences]), /Refusing to overwrite an editable canonical source/)
  assert.equal(await fileHash(source), before)
})
export async function loadBaseline() {
  return JSON.parse(await readFile(join(characterReferences, 'approved-baseline.json'), 'utf8'))
}
export async function exportMigratedSources() {
  const recipes = await loadRecipes(root)
  const output = join(root, `build/migration-check-${Date.now()}`)
  await mkdir(output, { recursive: true })
  const results = {}
  for (const [id, recipe] of recipes) {
    const directory = join(output, id.replace('/', '-'))
    const before = await fileHash(recipe.source.absolutePath)
    const metadata = await exportAsset({ recipe, selectedClips: Object.keys(recipe.clips),
      resolvedDependencies: [...recipe.dependencies.export, ...recipe.dependencies.preview].map(assetId => ({
        assetId, absolutePath: recipes.get(assetId).source.absolutePath,
      })) }, directory, blenderPath)
    assert.equal(await fileHash(recipe.source.absolutePath), before, 'export must leave editable source unchanged')
    results[id] = { recipe, directory, metadata, model: await snapshotGlb(join(directory, 'raw/model.glb')) }
  }
  return { results, output }
}
const maxDifference = (a, b) => Math.max(0, ...a.map((value, index) => Math.abs(value - b[index])))
function interpolate(channel, time) {
  const times = channel.times.values, values = channel.values.values
  const size = values.length / times.length
  let index = 0
  while (index + 1 < times.length && times[index + 1] <= time) index++
  const first = values.slice(index * size, (index + 1) * size)
  if (index + 1 === times.length || channel.interpolation === 'STEP') return first
  const second = values.slice((index + 1) * size, (index + 2) * size)
  const factor = (time - times[index]) / (times[index + 1] - times[index])
  if (channel.targetPath !== 'rotation') return first.map((value, i) => value + factor * (second[i] - value))
  let cosine = first.reduce((sum, value, i) => sum + value * second[i], 0)
  if (cosine < 0) { cosine = -cosine; second.forEach((value, i) => { second[i] = -value }) }
  const angle = Math.acos(Math.min(1, cosine))
  const result = angle < 1e-5 ? first.map((value, i) => value + factor * (second[i] - value))
    : first.map((value, i) => (value * Math.sin((1 - factor) * angle) + second[i] * Math.sin(factor * angle)) / Math.sin(angle))
  const length = Math.hypot(...result)
  return result.map(value => value / length)
}
export function comparePoses(baseline, migrated) {
  let maxPositionError = 0, maxQuaternionError = 0, maxScaleError = 0
  for (const clip of baseline.animations) {
    const other = migrated[clip.name]
    assert.ok(other, `missing ${clip.name}`)
    const channels = other.animations[0].channels
    assert.equal(channels.length, clip.channels.length, `${clip.name} channel mask`)
    for (const channel of clip.channels) {
      const name = baseline.nodes[channel.targetNode].name
      const found = channels.find(entry => other.nodes[entry.targetNode].name === name && entry.targetPath === channel.targetPath)
      assert.ok(found, `${clip.name}/${name}/${channel.targetPath}`)
      for (let sample = 0; sample <= 192; sample++) {
        const time = sample / 200
        const first = interpolate(channel, time), second = interpolate(found, time)
        if (channel.targetPath === 'rotation') {
          const dot = Math.abs(first.reduce((sum, value, index) => sum + value * second[index], 0)) / (Math.hypot(...first) * Math.hypot(...second))
          maxQuaternionError = Math.max(maxQuaternionError, 2 * Math.acos(Math.min(1, dot)))
        } else if (channel.targetPath === 'translation') maxPositionError = Math.max(maxPositionError, maxDifference(first, second))
        else maxScaleError = Math.max(maxScaleError, maxDifference(first, second))
      }
    }
  }
  return { maxPositionError, maxQuaternionError, maxScaleError }
}
export function compareBindings(baseline, metadata) {
  let maxMatrixError = 0
  for (const [id, binding] of Object.entries(baseline.client.equipmentBindings)) {
    const [x, y, z, w] = binding.quaternion
    const expected = [1-2*(y*y+z*z),2*(x*y+z*w),2*(x*z-y*w),0,
      2*(x*y-z*w),1-2*(x*x+z*z),2*(y*z+x*w),0,
      2*(x*z+y*w),2*(y*z-x*w),1-2*(x*x+y*y),0,...binding.position,1]
    maxMatrixError = Math.max(maxMatrixError, maxDifference(expected, metadata.bindings[id].gripInverse))
  }
  return { maxMatrixError }
}

function multiply(a, b) {
  return Array.from({ length: 16 }, (_, index) => {
    const row = index % 4, column = Math.floor(index / 4)
    return [0,1,2,3].reduce((sum, k) => sum + a[k*4+row] * b[column*4+k], 0)
  })
}
function applyPose(document, snapshot, clips, name, time) {
  const nodes = new Map(document.getRoot().listNodes().map(node => [node.getName(), node]))
  for (const node of snapshot.nodes) {
    const actual = nodes.get(node.name)
    if (actual) actual.setTranslation(node.translation).setRotation(node.rotation).setScale(node.scale)
  }
  const apply = clip => {
    for (const channel of clip.animations[0].channels) {
      const node = nodes.get(clip.nodes[channel.targetNode].name)
      const value = interpolate(channel, time)
      if (channel.targetPath === 'translation') node.setTranslation(value)
      else if (channel.targetPath === 'rotation') node.setRotation(value)
      else node.setScale(value)
    }
  }
  if (name.startsWith('axe_')) apply(clips[name.includes('walk') ? 'walk' : 'idle'])
  apply(clips[name])
  return nodes
}
function skinPositions(meshNode) {
  const primitive = meshNode.getMesh().listPrimitives()[0]
  const positions = primitive.getAttribute('POSITION'), weights = primitive.getAttribute('WEIGHTS_0'), joints = primitive.getAttribute('JOINTS_0')
  const skin = meshNode.getSkin(), binds = skin.getInverseBindMatrices()
  const matrices = skin.listJoints().map((joint, index) => multiply(joint.getWorldMatrix(), binds.getElement(index, [])))
  const result = []
  for (let index = 0; index < positions.getCount(); index++) {
    const position = [...positions.getElement(index, []), 1], influence = weights.getElement(index, []), boneIndices = joints.getElement(index, [])
    for (let axis = 0; axis < 3; axis++) {
      let value = 0
      for (let slot = 0; slot < influence.length; slot++) {
        const matrix = matrices[boneIndices[slot]]
        for (let component = 0; component < 4; component++) value += influence[slot] * matrix[component*4+axis] * position[component]
      }
      result.push(value)
    }
  }
  return result
}

test('canonical editable sources export approved mesh, textures, masks, poses and bindings', { timeout: 300000 }, async () => {
  for (const path of ['art_source/character/male_commoner/source.blend', 'art_source/equipment/stone_axe/source.blend']) {
    await assert.doesNotReject(access(join(root, path)), `missing canonical source ${path}`)
  }
  const baseline = await loadBaseline()
  const inspection = await runBlender(join(root, 'tools/asset_pipeline/migration/migrate_sources.py'),
    ['--root', root, '--baseline', characterReferences, '--verify-only'])
  assert.match(inspection, /SOURCE_VERIFICATION=/)
  const { results, output } = await exportMigratedSources()
  const commoner = results['character/male_commoner'], axe = results['equipment/stone_axe']
  const modelDocument = readGlb(await readFile(join(commoner.directory, 'raw/model.glb')))
  assert.deepEqual(modelDocument.nodes.filter(node => node.mesh !== undefined).map(node => node.extras),
    [{ lod: 0, skinned: true, skinning: 'linear' }, { lod: 1, skinned: true, skinning: 'linear' }])
  assert.ok(modelDocument.materials.every(material => material.extras.region === 'textured'))
  assert.deepEqual(commoner.recipe.budgets.trianglesByLod, { '0': 16000, '1': 5500 })
  assert.equal(commoner.recipe.clips.walk.cycleDistanceTiles, 1.677975879375)
  const reference = baseline.assets[0]
  const overrides = JSON.parse(await readFile(join(root, 'art_source/equipment/stone_axe/references/idle-pose-overrides.json'), 'utf8'))
  for (const [name, bones] of Object.entries(overrides)) {
    const clip = reference.animations.find(clip => clip.name === name)
    for (const [bone, properties] of Object.entries(bones)) for (const [path, values] of Object.entries(properties)) {
      const channel = clip.channels.find(channel => reference.nodes[channel.targetNode].name === bone && channel.targetPath === path)
      assert.deepEqual(channel.values.values.slice(0, values.length), values, `${name}/${bone} must use confirmed idle override`)
    }
  }
  assert.deepEqual(Object.keys(commoner.recipe.clips).sort(), reference.animations.map(clip => clip.name).sort())
  const clips = Object.fromEntries(await Promise.all(Object.keys(commoner.recipe.clips).map(async name =>
    [name, await snapshotGlb(join(commoner.directory, `raw/animations/${name}.glb`))])))
  const pose = comparePoses(reference, clips)
  const binding = compareBindings(baseline, axe.metadata)
  const report = { pose, binding, meshes: {}, textures: {} }
  const identity = [1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1]
  for (const [name, socket] of Object.entries(commoner.metadata.sockets)) {
    assert.ok(maxDifference(socket.localMatrix, identity) <= 1e-5, `${name} must retain runtime bone-head identity frame`)
  }
  for (const [asset, source] of [[commoner, reference], [axe, baseline.assets[1]]]) {
    report.textures[asset.recipe.id] = asset.model.textures
    assert.deepEqual(asset.model.textures.map(texture => texture.pixelSha256).sort(), source.textures.map(texture => texture.pixelSha256).sort())
    const original = await new NodeIO().read(join(root, `art_source/${asset.recipe.id}/references/approved-runtime.glb`))
    const migrated = await new NodeIO().read(join(asset.directory, 'raw/model.glb'))
    const originals = original.getRoot().listMeshes(), actuals = migrated.getRoot().listMeshes()
    assert.equal(actuals.length, originals.length)
    for (let meshIndex = 0; meshIndex < originals.length; meshIndex++) {
      const expected = originals[meshIndex], actual = actuals.find(mesh => mesh.getName() === expected.getName()) ?? actuals[meshIndex]
      assert.equal(actual.listPrimitives().length, expected.listPrimitives().length)
      for (let index = 0; index < expected.listPrimitives().length; index++) {
        const first = expected.listPrimitives()[index], second = actual.listPrimitives()[index]
        // Vertex ordering may change through Blender; compare indexed triangle corners.
        const indicesA = first.getIndices().getArray(), indicesB = second.getIndices().getArray()
        assert.equal(indicesA.length, indicesB.length)
        for (const semantic of ['POSITION', 'NORMAL', 'TEXCOORD_0', 'WEIGHTS_0', 'JOINTS_0']) {
          const a = first.getAttribute(semantic), b = second.getAttribute(semantic)
          if (!a) { assert.equal(b, null); continue }
          assert.ok(b)
          let error = 0
          for (let corner = 0; corner < indicesA.length; corner++) error = Math.max(error,
            maxDifference(a.getElement(indicesA[corner], []), b.getElement(indicesB[corner], [])))
          report.meshes[`${asset.recipe.id}/${expected.getName()}/${semantic}`] = error
          assert.ok(error <= 1e-5, `${expected.getName()} ${semantic}: ${error}`)
        }
      }
    }
  }
  const originalSkin = reference.skins[0], migratedSkin = commoner.model.skins[0]
  let maxBindError = 0
  for (let i = 0; i < originalSkin.joints.length; i++) {
    const name = reference.nodes[originalSkin.joints[i]].name
    const j = migratedSkin.joints.findIndex(index => commoner.model.nodes[index].name === name)
    assert.ok(j >= 0, name)
    maxBindError = Math.max(maxBindError, maxDifference(originalSkin.inverseBindMatrices.values.slice(i*16,i*16+16), migratedSkin.inverseBindMatrices.values.slice(j*16,j*16+16)))
  }
  report.maxBindError = maxBindError
  const originalDocument = await new NodeIO().read(join(characterReferences, 'approved-runtime.glb'))
  const migratedDocument = await new NodeIO().read(join(commoner.directory, 'raw/model.glb'))
  const originalClips = Object.fromEntries(reference.animations.map(animation => [animation.name, { ...reference, animations: [animation] }]))
  let maxWorldMatrixError = 0, maxSkinnedPositionError = 0
  for (const name of Object.keys(clips)) {
    for (let sample = 0; sample <= 192; sample++) {
      const first = applyPose(originalDocument, reference, originalClips, name, sample / 200)
      const second = applyPose(migratedDocument, commoner.model, clips, name, sample / 200)
      for (const index of originalSkin.joints) {
        const bone = reference.nodes[index].name
        maxWorldMatrixError = Math.max(maxWorldMatrixError, maxDifference(first.get(bone).getWorldMatrix(), second.get(bone).getWorldMatrix()))
      }
      if (sample % 16 === 0 || sample === 13) {
        for (const name of ['meshy_commoner', 'meshy_commoner_low']) {
          maxSkinnedPositionError = Math.max(maxSkinnedPositionError, maxDifference(skinPositions(first.get(name)), skinPositions(second.get(name))))
        }
      }
    }
  }
  report.maxWorldMatrixError = maxWorldMatrixError
  report.maxSkinnedPositionError = maxSkinnedPositionError
  await writeFile(join(output, 'comparison.json'), JSON.stringify(report, null, 2))
  console.log(JSON.stringify({ output, ...report }))
  assert.ok(pose.maxPositionError <= 1e-5, JSON.stringify(pose))
  assert.ok(pose.maxQuaternionError <= 1e-5, JSON.stringify(pose))
  assert.ok(pose.maxScaleError <= 1e-5, JSON.stringify(pose))
  assert.ok(binding.maxMatrixError <= 1e-5, JSON.stringify(binding))
  assert.ok(maxBindError <= 1e-5, `inverse binds ${maxBindError}`)
  assert.ok(maxWorldMatrixError <= 1e-5, `world poses ${maxWorldMatrixError}`)
  assert.ok(maxSkinnedPositionError <= 1e-5, `skinned positions ${maxSkinnedPositionError}`)
})
