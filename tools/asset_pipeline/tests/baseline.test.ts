import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'

import { Document, NodeIO } from '@gltf-transform/core'
import sharp from 'sharp'

import { LEGACY_GLB_PATHS, snapshotLegacy } from '../migration/baseline.ts'

async function writeFixtureGlb(path: string): Promise<void> {
  const document = new Document()
  const buffer = document.createBuffer('fixture')
  const scene = document.createScene('Scene')
  const root = document.createNode('root').setTranslation([1, 2, 3])
  const joint = document.createNode('hand.r')
  root.addChild(joint)
  scene.addChild(root)

  const positions = document.createAccessor('positions')
    .setType('VEC3')
    .setArray(new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0]))
    .setBuffer(buffer)
  const indices = document.createAccessor('indices')
    .setType('SCALAR')
    .setArray(new Uint16Array([0, 1, 2]))
    .setBuffer(buffer)
  const image = await sharp({
    create: { width: 1, height: 1, channels: 4, background: { r: 255, g: 0, b: 0, alpha: 1 } },
  }).png().toBuffer()
  const texture = document.createTexture('red').setMimeType('image/png').setImage(image)
  const material = document.createMaterial('material').setBaseColorTexture(texture)
  const primitive = document.createPrimitive().setAttribute('POSITION', positions).setIndices(indices).setMaterial(material)
  const mesh = document.createMesh('triangle').addPrimitive(primitive)
  root.setMesh(mesh)

  const inverseBindMatrices = document.createAccessor('inverse-binds')
    .setType('MAT4')
    .setArray(new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]))
    .setBuffer(buffer)
  root.setSkin(document.createSkin('skin').addJoint(joint).setInverseBindMatrices(inverseBindMatrices))

  const times = document.createAccessor('times')
    .setType('SCALAR')
    .setArray(new Float32Array([0, 1]))
    .setBuffer(buffer)
  const values = document.createAccessor('values')
    .setType('VEC3')
    .setArray(new Float32Array([0, 0, 0, 0, 1, 0]))
    .setBuffer(buffer)
  const sampler = document.createAnimationSampler().setInput(times).setOutput(values)
  const channel = document.createAnimationChannel().setSampler(sampler).setTargetNode(joint).setTargetPath('translation')
  document.createAnimation('walk').addSampler(sampler).addChannel(channel)

  await mkdir(dirname(path), { recursive: true })
  await writeFile(path, await new NodeIO().writeBinary(document))
}

test('snapshots decoded GLB semantics and preserves legacy inputs', async () => {
  const root = await mkdtemp(join(tmpdir(), 'asset-baseline-'))
  for (const relativePath of LEGACY_GLB_PATHS) await writeFixtureGlb(join(root, relativePath))
  await mkdir(join(root, 'web_new/src/game/actors'), { recursive: true })
  await writeFile(join(root, 'web_new/src/game/actors/config.ts'), `export const ACTOR_RENDER = { cycleDistanceTiles: 1.677975879375 } as const\n`)
  await writeFile(join(root, 'web_new/src/game/actors/equipment.ts'), `
export const EQUIPMENT = { stone_axe: { bindings: {
  left_hand: { socket: 'grip_l', transform: { position: [-.1, .2, 0], quaternion: [0, 0, 0, 1] } },
  right_hand: { socket: 'grip_r', transform: { position: [.1, .2, 0], quaternion: [0, 1, 0, 0] } },
} } } as const
`)

  const watchedPath = join(root, LEGACY_GLB_PATHS[0])
  const before = createHash('sha256').update(await readFile(watchedPath)).digest('hex')
  const output = join(root, 'build/asset-pipeline-baseline')
  const result = await snapshotLegacy({ root, output })
  const after = createHash('sha256').update(await readFile(watchedPath)).digest('hex')

  assert.equal(after, before)
  assert.equal(result.client.cycleDistanceTiles, 1.677975879375)
  assert.deepEqual(result.client.equipmentBindings.left_hand.position, [-0.1, 0.2, 0])
  assert.equal(result.assets[0]?.nodes[0]?.name, 'root')
  assert.equal(result.assets[0]?.nodes[1]?.parent, 0)
  assert.equal(result.assets[0]?.skins[0]?.joints[0], 1)
  assert.equal(result.assets[0]?.meshes[0]?.primitives[0]?.attributes.POSITION?.count, 3)
  assert.equal(result.assets[0]?.meshes[0]?.primitives[0]?.indices?.count, 3)
  assert.deepEqual(result.assets[0]?.animations[0]?.channels[0]?.times.values, [0, 1])
  assert.deepEqual(result.assets[0]?.animations[0]?.channels[0]?.values.values, [0, 0, 0, 0, 1, 0])
  assert.deepEqual(result.assets[0]?.textures[0], {
    name: 'red', mimeType: 'image/png', width: 1, height: 1, channels: 4,
    pixelSha256: '34aaa746c25a0f105c4316bbb1f009aa359f49582656ee97d73c58132d563423',
  })
  assert.equal(result.comparisons[0]?.identicalSemantics, true)
  await assert.doesNotReject(readFile(join(output, 'baseline.json')))
  await assert.doesNotReject(readFile(join(output, 'artifacts', LEGACY_GLB_PATHS[0])))

  const changedPath = join(root, LEGACY_GLB_PATHS[1])
  const changed = await new NodeIO().read(changedPath)
  const animationValues = changed.getRoot().listAnimations()[0]!.listSamplers()[0]!.getOutput()!
  animationValues.setArray(new Float32Array([0, 0, 0, 0, 2, 0]))
  await writeFile(changedPath, await new NodeIO().writeBinary(changed))
  const comparison = await snapshotLegacy({ root, output: join(root, 'build/changed-baseline') })
  assert.equal(comparison.comparisons[0]?.identicalSemantics, false)
  assert.deepEqual(comparison.comparisons[0]?.differingSections, ['animations'])
})
