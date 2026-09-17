/// <reference types="node" />
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFile } from 'node:fs/promises'
import { AnimationClip, AnimationMixer, Bone, Group, VectorKeyframeTrack, BufferGeometry, BufferAttribute, Mesh, MeshStandardMaterial, CompressedTexture, RGBA_S3TC_DXT5_Format, type WebGLRenderer } from 'three'
import { GLTFLoader, type GLTF } from 'three/addons/loaders/GLTFLoader.js'
import { KTX2Loader } from 'three/addons/loaders/KTX2Loader.js'
import { MeshoptDecoder } from 'three/addons/libs/meshopt_decoder.module.js'
import { ActorAssetCache } from '../src/game/actors/ActorAssetCache'
import { parseActorManifest, loadActorCatalog, validateAssetURL, type ActorManifest } from '../src/game/actors/ActorAssetCatalog'
import { bindClips } from '../src/game/actors/ActorClipBinding'
import { ActorSockets } from '../src/game/actors/ActorSockets'

declare const ASSET_TEST_ROOT: string
const hash = 'a'.repeat(64)
const artifact = { url: `/assets/game/test/${hash}.glb`, sha256: hash, bytes: 100 }
function compatibleClipManifests(): ActorManifest {
  return { schema: 1, id: 'character/test', kind: 'character', rigHash: hash, model: { ...artifact },
    metadata: { ...artifact, url: artifact.url.replace('.glb', '.json') }, textures: [], bindings: {},
    equipmentSlots: [],
    sockets: { grip_l: 'grip_l', grip_r: 'grip_r', forearm_l: 'forearm_l', forearm_r: 'forearm_r' },
    clips: Object.fromEntries(['idle', 'walk', 'carry_idle', 'carry_walk'].map(name => [name, {
      artifact, rigHash: hash, channelMask: ['pelvis'], duration: 1, loop: true,
      playback: name.endsWith('walk') ? 'distance' : 'time', ...(name.endsWith('walk') ? { cycleDistanceTiles: 1.677975879375 } : {}),
    }])) } as ActorManifest
}
function makeRig(name: string) {
  const model = new Group(); model.name = name
  const pelvis = new Bone(); pelvis.name = 'pelvis'; model.add(pelvis)
  for (const [socket, joint] of Object.entries({ grip_l: 'handl', grip_r: 'handr', forearm_l: 'forearml', forearm_r: 'forearmr' })) {
    const bone = new Bone(); bone.name = joint; pelvis.add(bone)
    const child = new Group(); child.name = socket; bone.add(child)
  }
  return model
}
function makeWalkClip(donor: Group) {
  return [new AnimationClip('walk', 1, [new VectorKeyframeTrack(`${donor.getObjectByName('pelvis')!.name}.position`, [0, 1], [0, 0, 0, 0, .2, 0])])]
}
test('standalone clips bind to stable names and keep two mixer states independent', () => {
  const first = makeRig('first'); const second = makeRig('second'); const donor = makeRig('donor')
  const clips = makeWalkClip(donor)
  const firstMixer = new AnimationMixer(first); const secondMixer = new AnimationMixer(second)
  firstMixer.clipAction(bindClips(first, compatibleClipManifests(), clips)[0]!).play()
  secondMixer.clipAction(bindClips(second, compatibleClipManifests(), clips)[0]!).play()
  firstMixer.setTime(.5); secondMixer.setTime(.25)
  assert.ok(Math.abs(first.getObjectByName('pelvis')!.position.y - .1) < 1e-6)
  assert.ok(Math.abs(second.getObjectByName('pelvis')!.position.y - .05) < 1e-6)
  assert.equal(donor.getObjectByName('pelvis')!.position.y, 0)
})
test('binding rejects incompatible rigs, missing targets and ambiguous sanitized names', () => {
  const model = makeRig('model'); const clips = makeWalkClip(makeRig('donor'))
  const manifest = compatibleClipManifests(); manifest.clips.walk!.rigHash = 'b'.repeat(64)
  assert.throws(() => bindClips(model, manifest, clips), /rig/i)
  const missing = makeRig('missing'); missing.getObjectByName('pelvis')!.removeFromParent()
  assert.throws(() => bindClips(missing, compatibleClipManifests(), clips), /target/i)
  const duplicate = new Bone(); duplicate.name = 'pel.vis'; model.add(duplicate)
  assert.throws(() => bindClips(model, compatibleClipManifests(), clips), /duplicate/i)
})
test('sockets must be authored under their matching bones', () => {
  const model = makeRig('model'); assert.doesNotThrow(() => new ActorSockets(model))
  model.getObjectByName('grip_l')!.removeFromParent()
  assert.throws(() => new ActorSockets(model), /socket/i)
})
test('schema validates immutable paths and compatible distance metadata', () => {
  assert.equal(parseActorManifest(compatibleClipManifests()).clips.walk!.cycleDistanceTiles, 1.677975879375)
  for (const url of ['https://evil/assets/game/x.glb', '//evil/assets/game/x.glb', '/assets/game/../x.glb', '/assets/game/%2e%2e/x.glb', '/assets/game/x.glb?q=1', '/assets/game/x.glb#x', '/assets/game/x\\y.glb']) {
    assert.throws(() => validateAssetURL(url), /URL/)
  }
  for (const mutate of [
    (m: ActorManifest) => { m.schema = 2 },
    (m: ActorManifest) => { m.clips.walk!.cycleDistanceTiles = 0 },
    (m: ActorManifest) => { m.clips.carry_walk!.cycleDistanceTiles = 2 },
    (m: ActorManifest) => { m.model.url = '/assets/game/mutable.glb' },
    (m: ActorManifest) => { delete m.sockets.grip_l },
  ]) { const manifest = compatibleClipManifests(); mutate(manifest); assert.throws(() => parseActorManifest(manifest)) }
})
test('manifest kind and clip playback require primitive strings before conditional validation', () => {
  assert.throws(() => parseActorManifest({ ...compatibleClipManifests(), kind: ['character'], rigHash: null, sockets: {}, clips: {} }), /schema|kind/)
  for (const playback of [['time'], ['distance'], new String('time'), new String('distance')]) {
    const manifest = compatibleClipManifests()
    Object.assign(manifest.clips.idle!, { playback })
    assert.throws(() => parseActorManifest(manifest), /clip|playback/)
  }
})
test('published snapshot resolves immutable manifests and ordinary axe grip policy', async () => {
  const requests: { url: string; cache?: RequestCache }[] = []
  const catalog = await loadActorCatalog('/assets/game/asset-catalog.json', async (input, init) => {
    const url = String(input); requests.push({ url, cache: init?.cache })
    return new Response(await readFile(`${ASSET_TEST_ROOT}/public${url}`))
  })
  assert.equal(requests[0]!.cache, 'no-cache')
  assert.equal(catalog.manifests['character/male_commoner']!.clips.walk!.cycleDistanceTiles, 1.677975879375)
  const axe = catalog.equipment.stone_axe!
  assert.equal(axe.kind, 'rigid')
  if (axe.kind !== 'rigid') throw new Error('Expected rigid axe')
  assert.equal(axe.bindings.right_hand!.armMotion!.kind, 'ordinary')
  assert.ok(Math.abs(axe.bindings.right_hand!.transform!.position![0] - .019444145) < 1e-6)
  const shirt = catalog.equipment.nettle_shirt!
  assert.equal(shirt.kind, 'skinned')
  if (shirt.kind !== 'skinned') throw new Error('Expected skinned nettle shirt')
  assert.deepEqual(shirt.slots, ['chest'])
})

test('catalog exposes rigged garments only in their declared character slots', async () => {
  const garment = {
    schema: 1, id: 'equipment/nettle_shirt', kind: 'equipment', rigHash: hash,
    model: { ...artifact, url: artifact.url.replace('/test/', '/equipment/nettle_shirt/') },
    metadata: { ...artifact, url: artifact.url.replace('/test/', '/equipment/nettle_shirt/').replace('.glb', '.json') },
    textures: [], sockets: {}, clips: {}, bindings: {}, equipmentSlots: ['chest'],
  }
  const catalog = await loadActorCatalog('/assets/game/asset-catalog.json', async input => {
    const url = String(input)
    return Response.json(url.endsWith('asset-catalog.json')
      ? { schema: 1, assets: { 'equipment/nettle_shirt': { ...garment.metadata } } }
      : garment)
  })
  assert.deepEqual(catalog.equipment.nettle_shirt, {
    kind: 'skinned', assetId: 'equipment/nettle_shirt', slots: ['chest'],
  })
})

test('cache shares artifact leases across animation revisions, accounts decoded buffers once and releases final owners', async t => {
  const first = compatibleClipManifests(); const second = compatibleClipManifests(); second.id = 'character/revision'
  for (const [index, manifest] of [first, second].entries()) for (const [name, clip] of Object.entries(manifest.clips)) {
    const sha256 = String(index * 4 + ['idle', 'walk', 'carry_idle', 'carry_walk'].indexOf(name) + 1).repeat(64)
    clip.artifact = { ...artifact, sha256, url: `/assets/game/test/${sha256}.glb` }
  }
  const refs = ['b'.repeat(64), 'c'.repeat(64)].map(sha256 => ({ sha256, bytes: 100, url: `/assets/game/test/${sha256}.json` }))
  let catalogReads = 0
  t.mock.method(globalThis, 'fetch', async (input: string) => {
    if (input.endsWith('asset-catalog.json')) { catalogReads++; return Response.json({ schema: 1, assets: { [first.id]: refs[0], [second.id]: refs[1] } }) }
    return Response.json(input === refs[0]!.url ? first : second)
  })
  const scene = makeRig('model')
  const geometry = new BufferGeometry(); const buffer = new Float32Array(12)
  geometry.setAttribute('position', new BufferAttribute(buffer, 3)); geometry.setAttribute('normal', new BufferAttribute(buffer, 3))
  const texture = new CompressedTexture([{ data: new Uint8Array(16), width: 4, height: 4 }], 4, 4, RGBA_S3TC_DXT5_Format)
  const material = new MeshStandardMaterial({ map: texture }); scene.add(new Mesh(geometry, material))
  let geometryDisposals = 0; let textureDisposals = 0; let modelLoads = 0
  geometry.addEventListener('dispose', () => geometryDisposals++); texture.addEventListener('dispose', () => textureDisposals++)
  t.mock.method(GLTFLoader.prototype, 'loadAsync', async (url: string) => {
    if (url === artifact.url) { modelLoads++; return { scene, animations: [] } as unknown as GLTF }
    const [name] = Object.entries({ ...first.clips, ...Object.fromEntries(Object.entries(second.clips).map(([n,c]) => [`revision_${n}`,c])) }).find(([, clip]) => clip.artifact.url === url)!
    return { scene: new Group(), animations: [new AnimationClip(name.replace('revision_', ''), 1, [new VectorKeyframeTrack('pelvis.position', [0, 1], [0, 0, 0, 0, .2, 0])])] } as unknown as GLTF
  })
  const renderer = { extensions: { has: () => false }, capabilities: {} } as unknown as WebGLRenderer
  const cache = new ActorAssetCache(renderer)
  const [one, two] = await Promise.all([cache.acquire(first.id), cache.acquire(second.id)])
  assert.equal(catalogReads, 1); assert.equal(modelLoads, 1); assert.equal(one.asset.scene, two.asset.scene)
  // 48 shared geometry bytes + 16 compressed mip bytes + eight 32-byte tracks.
  assert.equal(cache.residentBytes, 320)
  one.release(); one.release(); assert.equal(geometryDisposals, 0)
  two.release(); assert.equal(geometryDisposals, 1); assert.equal(textureDisposals, 1)
  assert.equal(cache.residentBytes, 0); assert.equal(cache.loadedCount, 0)
  cache.destroy()
  await assert.rejects(cache.acquire(first.id), /disposed/)
})

test('published meshopt model and standalone clips bind through the real GLTF parser', async t => {
  const previousSelf = globalThis.self
  Object.assign(globalThis, { self: globalThis })
  t.after(() => { Object.assign(globalThis, { self: previousSelf }) })
  const catalog = await loadActorCatalog('/assets/game/asset-catalog.json', async input => new Response(await readFile(`${ASSET_TEST_ROOT}/public${input}`)))
  const manifest = catalog.manifests['character/male_commoner']!
  const ktx = new KTX2Loader()
  // This test exercises real mesh/clip decoding. Pixel transcoding is measured separately.
  t.mock.method(ktx, 'load', (_url: string, onLoad: (texture: CompressedTexture) => void) => {
    onLoad(new CompressedTexture([{ data: new Uint8Array(16), width: 4, height: 4 }], 4, 4, RGBA_S3TC_DXT5_Format))
  })
  const loader = new GLTFLoader().setMeshoptDecoder(MeshoptDecoder).setKTX2Loader(ktx)
  async function parse(url: string) {
    const bytes = await readFile(`${ASSET_TEST_ROOT}/public${url}`)
    return loader.parseAsync(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength), url.slice(0, url.lastIndexOf('/') + 1))
  }
  const model = await parse(manifest.model.url)
  const donor = await parse(manifest.clips.walk!.artifact.url)
  assert.equal(model.animations.length, 0)
  assert.equal(donor.animations[0]!.name, 'walk')
  assert.doesNotThrow(() => new ActorSockets(model.scene, manifest.sockets))
  const pelvis = model.scene.getObjectByName('pelvis')!
  const initial = pelvis.position.clone()
  const mixer = new AnimationMixer(model.scene)
  mixer.clipAction(bindClips(model.scene, manifest, donor.animations)[0]!).play()
  mixer.setTime(.24)
  assert.ok(pelvis.position.distanceTo(initial) > .001)
  assert.ok(model.scene.getObjectByName('handl'))
  mixer.stopAllAction(); mixer.uncacheRoot(model.scene)
  ktx.dispose()
})

test('published nettle shirt selects the same linear skinning mode as the character body', async t => {
  const previousSelf = globalThis.self
  Object.assign(globalThis, { self: globalThis })
  t.after(() => { Object.assign(globalThis, { self: previousSelf }) })
  const catalog = await loadActorCatalog('/assets/game/asset-catalog.json', async input => new Response(await readFile(`${ASSET_TEST_ROOT}/public${input}`)))
  const manifest = catalog.manifests['equipment/nettle_shirt']!
  const ktx = new KTX2Loader()
  t.mock.method(ktx, 'load', (_url: string, onLoad: (texture: CompressedTexture) => void) => {
    onLoad(new CompressedTexture([{ data: new Uint8Array(16), width: 4, height: 4 }], 4, 4, RGBA_S3TC_DXT5_Format))
  })
  const loader = new GLTFLoader().setMeshoptDecoder(MeshoptDecoder).setKTX2Loader(ktx)
  const bytes = await readFile(`${ASSET_TEST_ROOT}/public${manifest.model.url}`)
  const model = await loader.parseAsync(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength), manifest.model.url.slice(0, manifest.model.url.lastIndexOf('/') + 1))
  assert.equal(model.scene.getObjectByName('nettle_shirt_tunic')!.userData.skinning, 'linear')
  ktx.dispose()
})

for (const mode of ['partial failure', 'destroy during load'] as const) test(`cache releases resources after ${mode}`, async t => {
  const manifest = compatibleClipManifests()
  for (const [index, clip] of Object.values(manifest.clips).entries()) {
    const sha256 = String(index + 1).repeat(64)
    clip.artifact = { ...artifact, sha256, url: `/assets/game/test/${sha256}.glb` }
  }
  t.mock.method(globalThis, 'fetch', async (input: string) => Response.json(input.endsWith('asset-catalog.json')
    ? { schema: 1, assets: { [manifest.id]: { ...artifact, url: artifact.url.replace('.glb', '.json') } } } : manifest))
  const scene = makeRig('model'); const geometry = new BufferGeometry(); scene.add(new Mesh(geometry, new MeshStandardMaterial()))
  let disposals = 0; geometry.addEventListener('dispose', () => disposals++)
  let finish!: () => void; let started!: () => void
  const pending = new Promise<void>(resolve => { finish = resolve })
  const loading = new Promise<void>(resolve => { started = resolve })
  t.mock.method(GLTFLoader.prototype, 'loadAsync', async (url: string) => {
    if (url === artifact.url) { started(); await pending; return { scene, animations: [] } as unknown as GLTF }
    if (mode === 'partial failure') throw new Error('Failed clip request')
    const name = Object.entries(manifest.clips).find(([, clip]) => clip.artifact.url === url)![0]
    return { scene: new Group(), animations: [new AnimationClip(name, 1, [new VectorKeyframeTrack('pelvis.position', [0, 1], [0, 0, 0, 0, .2, 0])])] } as unknown as GLTF
  })
  const cache = new ActorAssetCache({ extensions: { has: () => false }, capabilities: {} } as unknown as WebGLRenderer)
  const result = cache.acquire(manifest.id)
  await loading
  if (mode === 'destroy during load') cache.destroy()
  finish()
  await assert.rejects(result, /Unable to load/)
  assert.equal(disposals, 1); assert.equal(cache.residentBytes, 0); assert.equal(cache.loadedCount, 0)
  cache.destroy()
})

test('different models lease one decoded texture and dispose it only after the final model', async t => {
  const textureHash = 'f'.repeat(64)
  const textureArtifact = { sha256: textureHash, bytes: 16, url: `/assets/game/test/${textureHash}.ktx2` }
  const manifests = ['first', 'second'].map((name, index) => {
    const sha256 = String(index + 1).repeat(64)
    return { ...compatibleClipManifests(), id: `equipment/${name}`, kind: 'equipment', rigHash: null,
      sockets: {}, clips: {}, textures: [textureArtifact], model: { sha256, bytes: 100, url: `/assets/game/test/${sha256}.glb` } }
  })
  t.mock.method(globalThis, 'fetch', async (input: string) => Response.json(input.endsWith('asset-catalog.json')
    ? { schema: 1, assets: Object.fromEntries(manifests.map(manifest => [manifest.id, { ...manifest.model, url: manifest.model.url.replace('.glb', '.json') }])) }
    : manifests.find(manifest => manifest.model.url.replace('.glb', '.json') === input)))
  const texture = new CompressedTexture([{ data: new Uint8Array(16), width: 4, height: 4 }], 4, 4, RGBA_S3TC_DXT5_Format)
  let textureLoads = 0; let disposals = 0; let workerDisposals = 0
  texture.addEventListener('dispose', () => disposals++)
  t.mock.method(KTX2Loader.prototype, 'load', (_url: string, onLoad: (texture: CompressedTexture) => void) => { textureLoads++; onLoad(texture) })
  t.mock.method(KTX2Loader.prototype, 'dispose', () => { workerDisposals++ })
  t.mock.method(GLTFLoader.prototype, 'loadAsync', async () => {
    const scene = new Group(); scene.add(new Mesh(new BufferGeometry(), new MeshStandardMaterial({ map: texture })))
    return { scene, animations: [] } as unknown as GLTF
  })
  const cache = new ActorAssetCache({ extensions: { has: () => false }, capabilities: {} } as unknown as WebGLRenderer)
  const [first, second] = await Promise.all(manifests.map(manifest => cache.acquire(manifest.id)))
  assert.equal(textureLoads, 1); assert.equal(cache.residentBytes, 16)
  first!.release(); assert.equal(disposals, 0); assert.equal(cache.residentBytes, 16)
  second!.release(); assert.equal(disposals, 1); assert.equal(cache.residentBytes, 0)
  cache.destroy(); await Promise.resolve()
  assert.equal(workerDisposals, 1)
})

test('cache rejects ambiguous exported names before GLTFLoader silently renames them', async t => {
  const manifest = { ...compatibleClipManifests(), id: 'equipment/test', kind: 'equipment', rigHash: null, clips: {}, sockets: {} }
  t.mock.method(globalThis, 'fetch', async (input: string) => Response.json(input.endsWith('asset-catalog.json')
    ? { schema: 1, assets: { [manifest.id]: { ...artifact, url: artifact.url.replace('.glb', '.json') } } } : manifest))
  t.mock.method(GLTFLoader.prototype, 'loadAsync', function (this: GLTFLoader) {
    return this.parseAsync(JSON.stringify({ asset: { version: '2.0' }, scene: 0, scenes: [{ nodes: [0, 1] }], nodes: [{ name: 'pel.vis' }, { name: 'pelvis' }] }), '/assets/game/test/')
  })
  const cache = new ActorAssetCache({ extensions: { has: () => false }, capabilities: {} } as unknown as WebGLRenderer)
  await assert.rejects(cache.acquire(manifest.id), (error: unknown) => error instanceof Error && error.cause instanceof Error && /Duplicate/.test(error.cause.message))
  cache.destroy()
})

function texturedGLB(textureURL: string, textureSource = 0): ArrayBuffer {
  const geometry = new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 1])
  const json = Buffer.from(JSON.stringify({
    asset: { version: '2.0' }, scene: 0, scenes: [{ nodes: [0] }], nodes: [{ name: 'triangle', mesh: 0 }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0, TEXCOORD_0: 1 }, material: 0 }] }],
    buffers: [{ byteLength: geometry.byteLength }],
    bufferViews: [{ buffer: 0, byteOffset: 0, byteLength: 36 }, { buffer: 0, byteOffset: 36, byteLength: 24 }],
    accessors: [{ bufferView: 0, componentType: 5126, count: 3, type: 'VEC3', min: [0, 0, 0], max: [1, 1, 0] },
      { bufferView: 1, componentType: 5126, count: 3, type: 'VEC2' }],
    images: [{ uri: textureURL, mimeType: 'image/ktx2' }],
    textures: [{ extensions: { KHR_texture_basisu: { source: textureSource } } }],
    materials: [{ pbrMetallicRoughness: { baseColorTexture: { index: 0 } } }],
    extensionsUsed: ['KHR_texture_basisu'], extensionsRequired: ['KHR_texture_basisu'],
  }))
  const paddedLength = Math.ceil(json.length / 4) * 4
  const glb = Buffer.alloc(12 + 8 + paddedLength + 8 + geometry.byteLength)
  glb.writeUInt32LE(0x46546c67, 0); glb.writeUInt32LE(2, 4); glb.writeUInt32LE(glb.length, 8)
  glb.writeUInt32LE(paddedLength, 12); glb.writeUInt32LE(0x4e4f534a, 16)
  glb.fill(0x20, 20, 20 + paddedLength); json.copy(glb, 20)
  glb.writeUInt32LE(geometry.byteLength, 20 + paddedLength); glb.writeUInt32LE(0x004e4942, 24 + paddedLength)
  Buffer.from(geometry.buffer).copy(glb, 28 + paddedLength)
  return glb.buffer.slice(glb.byteOffset, glb.byteOffset + glb.byteLength)
}

for (const scenario of ['undeclared', 'undeclared already loaded', 'undeclared cached model', 'declared shared', 'declared cached model', 'invalid image index', 'decode failure', 'parser texture failure'] as const) {
  test(`real GLTF parser enforces manifest texture ownership: ${scenario}`, async t => {
    const previousSelf = globalThis.self
    Object.assign(globalThis, { self: globalThis })
    t.after(() => { Object.assign(globalThis, { self: previousSelf }) })
    const textureHash = 'f'.repeat(64)
    const textureArtifact = { sha256: textureHash, bytes: 16, url: `/assets/game/test/${textureHash}.ktx2` }
    const needsOwner = ['undeclared already loaded', 'undeclared cached model', 'declared shared', 'declared cached model'].includes(scenario)
    const manifests = ['owner', 'candidate'].map((name, index) => {
      const sha256 = String(scenario.endsWith('cached model') ? 1 : index + 1).repeat(64)
      return { ...compatibleClipManifests(), id: `equipment/${name}`, kind: 'equipment', rigHash: null, sockets: {}, clips: {},
        textures: name === 'candidate' && scenario.startsWith('undeclared') ? [] : [textureArtifact],
        model: { sha256, bytes: 100, url: `/assets/game/test/${sha256}.glb` } }
    })
    const references = manifests.map((manifest, index) => {
      const sha256 = String(index + 3).repeat(64)
      return { id: manifest.id, sha256, bytes: 100, url: `/assets/game/test/${sha256}.json` }
    })
    t.mock.method(globalThis, 'fetch', async (input: string) => Response.json(input.endsWith('asset-catalog.json')
      ? { schema: 1, assets: Object.fromEntries(references.map(reference => [reference.id, reference])) }
      : manifests[references.findIndex(reference => reference.url === input)]))
    const texture = new CompressedTexture([{ data: new Uint8Array(16), width: 4, height: 4 }], 4, 4, RGBA_S3TC_DXT5_Format)
    let textureDisposals = 0
    texture.addEventListener('dispose', () => textureDisposals++)
    t.mock.method(KTX2Loader.prototype, 'load', (_url: string, onLoad: (texture: CompressedTexture) => void, _progress: unknown, onError: (error: Error) => void) => {
      if (scenario === 'decode failure') onError(new Error('Required KTX2 decode failed'))
      else onLoad(texture)
    })
    const payload = texturedGLB(`${textureHash}.ktx2`, scenario === 'invalid image index' ? 7 : 0)
    t.mock.method(GLTFLoader.prototype, 'loadAsync', function (this: GLTFLoader) {
      if (scenario === 'parser texture failure') {
        // Fail the actual parser's image-loader callback after a successful preload.
        // GLTFLoader normally logs this error and resolves a null texture dependency.
        t.mock.method(console, 'error', () => {})
        t.mock.method(this.ktx2Loader!, 'load', (_url: string, _load: unknown, _progress: unknown, onError: (error: Error) => void) => onError(new Error('Image callback failed')))
      }
      return this.parseAsync(payload, '/assets/game/test/')
    })
    const cache = new ActorAssetCache({ extensions: { has: () => false }, capabilities: {} } as unknown as WebGLRenderer)
    t.after(() => cache.destroy())
    const owner = needsOwner ? await cache.acquire(manifests[0]!.id) : undefined
    if (scenario === 'declared shared' || scenario === 'declared cached model') {
      const candidate = await cache.acquire(manifests[1]!.id)
      const material = (candidate.asset.scene.getObjectByName('triangle') as Mesh).material as MeshStandardMaterial
      assert.equal(material.map, texture)
      owner!.release()
      assert.equal(textureDisposals, 0, 'another bundle must retain its own texture lease')
      assert.equal(material.map, texture, 'a ready bundle retains its required material map')
      candidate.release()
      assert.equal(textureDisposals, 1)
    } else {
      await assert.rejects(cache.acquire(manifests[1]!.id), /Unable to load actor asset/)
      if (owner) assert.equal(textureDisposals, 0, 'rejected candidate must not dispose another bundle texture')
      owner?.release()
      assert.equal(cache.loadedCount, 0)
      assert.equal(cache.residentBytes, 0)
    }
  })
}
