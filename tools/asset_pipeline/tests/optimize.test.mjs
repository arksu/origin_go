import assert from 'node:assert/strict'
import { mkdtemp, readFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, relative } from 'node:path'
import test from 'node:test'
import { createFixture, runExport, pipelineDirectory, readGlb } from './blender_helpers.ts'
import { optimizeExport } from '../optimize.mjs'
import { canonicalJSON, sha256, validateArtifacts, validateGLB, catalogBytes } from '../report.mjs'
import { installDecoders } from '../install-decoders.mjs'
import { decodeKTX2, comparePixels } from '../texture-review.mjs'

const identity = JSON.parse(await readFile(new URL('../toolchain.lock.json', import.meta.url)))
const toolchain = { identity, paths: { toktx: join(pipelineDirectory, '.tools/ktx-4.4.2/install/usr/local/bin/toktx') } }

test('canonical hashing ignores object order but retains array order and rejects nonfinite values', () => {
  assert.equal(canonicalJSON({ z: [2, 1], a: { b: true } }), '{"a":{"b":true},"z":[2,1]}')
  assert.equal(sha256('abc'), 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  assert.throws(() => canonicalJSON({ invalid: NaN }), /finite/)
})

test('optimization preserves authored rig, clips, external textures and rejects every exceeded or omitted budget', async () => {
  const fixture = await createFixture('character', { packedTexture: true, runtimeExtras: true })
  fixture.recipe.budgets.bones = 2
  const raw = await runExport(fixture.recipe, fixture.directory('export'))
  const input = { rawDirectory: fixture.directory('export/raw'), recipe: fixture.recipe, toolchain,
    outputDirectory: fixture.directory('optimized') }
  const result = await optimizeExport(input)
  const report = await validateArtifacts(result, fixture.recipe)
  assert.equal((await validateGLB(relative(process.cwd(), result.model))).numErrors, 0)
  assert.equal(report.errors, 0)
  assert.ok(report.nodeNames.includes('socket_hand_right'))
  assert.deepEqual(report.clipTargets.walk, ['hand.r/rotation', 'hand.r/scale', 'hand.r/translation', 'root/rotation', 'root/scale', 'root/translation'])
  assert.equal(report.maxGeometryError, 0)
  assert.equal(report.maxAnimationError, 0)
  assert.equal(report.maxSkinError, 0)
  const model = readGlb(await readFile(result.model))
  assert.ok(model.extensionsRequired.includes('EXT_meshopt_compression'))
  assert.ok(model.images[0].uri.match(/^textures\/[a-f0-9]{64}\.ktx2$/))
  assert.equal(model.images[0].bufferView, undefined)
  assert.deepEqual(model.nodes.find(node => node.name === 'Body').extras, { lod: 0, skinned: true, skinning: 'linear' })
  assert.deepEqual(model.materials[0].extras, { region: 'textured' })
  for (const clip of Object.values(result.clips)) {
    const document = readGlb(await readFile(clip))
    for (const property of ['meshes', 'materials', 'images']) assert.equal(document[property], undefined)
  }
  const originalOptions = process.env.TOKTX_OPTIONS
  process.env.TOKTX_OPTIONS = '--encode etc1s --lower_left_maps_to_s0t0 --mipmap'
  let second
  try { second = await optimizeExport({ ...input, outputDirectory: fixture.directory('second') }) }
  finally {
    if (originalOptions === undefined) delete process.env.TOKTX_OPTIONS
    else process.env.TOKTX_OPTIONS = originalOptions
  }
  assert.equal(sha256(await readFile(result.model)), sha256(await readFile(second.model)))
  assert.equal(sha256(await readFile(result.metadata)), sha256(await readFile(second.metadata)))
  assert.equal(catalogBytes([result.metrics, second.metrics]),
    2 * (result.metrics.geometryBytes + result.metrics.animationBytes + result.metrics.metadataBytes) + result.metrics.textureBytes)
  for (const override of [{ trianglesByLod: {} }, { trianglesByLod: { '0': 0 } }, { bones: 1 }, { boneInfluences: 0 },
    { textureDimensions: { width: 1, height: 1 } }, { totalPublishedBytes: 1 }, { totalPublishedBytes: undefined },
    { trianglesByLod: undefined }, { bones: undefined }, { boneInfluences: undefined }, { textureDimensions: undefined }]) {
    await assert.rejects(validateArtifacts(result, { ...fixture.recipe, budgets: { ...fixture.recipe.budgets, ...override } }), /budget/i)
  }
  assert.deepEqual(raw.metadata.rig.bones.map(bone => bone.name), ['hand.r', 'root'])
  await assert.rejects(validateArtifacts({ ...result, clips: {} }, fixture.recipe), /selected clip set/)
  await assert.rejects(validateArtifacts({ ...result, textures: [] }, fixture.recipe), /external texture set/)
  await assert.rejects(optimizeExport({ ...input, toolchain: { ...toolchain, identity: { ...identity, blender: { ...identity.blender, version: '5.2.0' } } } }), /locked blender/)
  await assert.rejects(optimizeExport({ ...input, recipe: { ...fixture.recipe, optimization: { ...fixture.recipe.optimization,
    texture: { ...fixture.recipe.optimization.texture, codec: 'etc1s' } } } }), /profile/)
  const pixels = await decodeKTX2(result.textures[0])
  assert.equal(pixels.width, 2)
  assert.equal(pixels.height, 2)
  assert.equal(pixels.bytes.length, 16)
  assert.deepEqual(comparePixels(pixels.bytes, pixels.bytes), { meanAbsoluteError: 0, maxAbsoluteError: 0, rootMeanSquareError: 0 })
})

test('local decoders are copied from locked Three with verifiable source hashes', async () => {
  const destination = await mkdtemp(join(tmpdir(), 'asset-decoders-'))
  const manifest = await installDecoders(destination)
  assert.equal(manifest.package, 'three')
  for (const entry of manifest.files) {
    assert.equal(sha256(await readFile(join(destination, entry.file))), entry.sha256)
    assert.equal(entry.sha256, sha256(await readFile(new URL(`../../../web_new/node_modules/three/${entry.source}`, import.meta.url))))
  }
})

test('texture color error measures every channel against an independent reference', () => {
  assert.deepEqual(comparePixels(Buffer.from([0, 10, 20, 255]), Buffer.from([2, 8, 22, 253])),
    { meanAbsoluteError: 2, maxAbsoluteError: 2, rootMeanSquareError: 2 })
  assert.throws(() => comparePixels(Buffer.alloc(4), Buffer.alloc(8)), /dimensions/)
})
