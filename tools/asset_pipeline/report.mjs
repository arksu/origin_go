import { createHash } from 'node:crypto'
import { readFile, stat } from 'node:fs/promises'
import { dirname, join, resolve, sep } from 'node:path'
import { createRequire } from 'node:module'
import { NodeIO } from '@gltf-transform/core'
import { ALL_EXTENSIONS } from '@gltf-transform/extensions'
import { MeshoptDecoder } from 'meshoptimizer'
import { decodeKTX2 } from './texture-review.mjs'

const validator = createRequire(import.meta.url)('gltf-validator')
export function canonicalJSON(value) {
  function normalize(entry) {
    if (typeof entry === 'number' && !Number.isFinite(entry)) throw new Error('Canonical JSON requires finite numbers')
    if (entry === undefined || typeof entry === 'function' || typeof entry === 'bigint') throw new Error('Unsupported canonical JSON value')
    if (Array.isArray(entry)) return entry.map(normalize)
    if (entry && typeof entry === 'object') return Object.fromEntries(Object.keys(entry).sort().map(key => [key, normalize(entry[key])]))
    return entry
  }
  return JSON.stringify(normalize(value))
}
export const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')
export const assetIO = () => new NodeIO().registerExtensions(ALL_EXTENSIONS).registerDependencies({ 'meshopt.decoder': MeshoptDecoder })

export async function compareBuilds(first, second) {
  const difference = label => { throw new Error(`Reproducibility failed: ${label} differs between clean builds`) }
  if (canonicalJSON(first.map(bundle => bundle.manifest.id)) !== canonicalJSON(second.map(bundle => bundle.manifest.id))) difference('asset set')
  for (const [index, bundle] of first.entries()) {
    const other = second[index]
    const artifacts = result => ({ model: result.model, metadata: result.metadata,
      ...Object.fromEntries(Object.entries(result.clips).map(([id, path]) => [`clip ${id}`, path])),
      ...Object.fromEntries(result.textures.map((path, index) => [`texture ${index}`, path])) })
    const left = artifacts(bundle.result), right = artifacts(other.result)
    if (canonicalJSON(Object.keys(left).sort()) !== canonicalJSON(Object.keys(right).sort())) difference(`${bundle.manifest.id} artifact set`)
    for (const label of Object.keys(left).sort()) {
      const [before, after] = await Promise.all([readFile(left[label]), readFile(right[label])])
      if (!before.equals(after) || sha256(before) !== sha256(after)) difference(`${bundle.manifest.id} ${label}`)
    }
    if (canonicalJSON(bundle.manifest) !== canonicalJSON(other.manifest)) difference(`${bundle.manifest.id} manifest`)
  }
}

export async function validateGLB(path) {
  const root = dirname(resolve(path))
  const report = await validator.validateBytes(await readFile(path), {
    uri: path.split(sep).at(-1),
    externalResourceFunction: async uri => {
      const target = resolve(root, decodeURIComponent(uri))
      if (!target.startsWith(root + sep)) throw new Error(`External resource escapes artifact directory: ${uri}`)
      return new Uint8Array(await readFile(target))
    },
  })
  if (report.issues.numErrors) throw new Error(`glTF validation failed for ${path}: ${JSON.stringify(report.issues.messages)}`)
  return report.issues
}

function array(accessor) { return accessor ? Array.from(accessor.getArray()) : null }
function triangleIndices(primitive) {
  const indices = array(primitive.getIndices())
  if (!indices || primitive.getMode() !== 4) return indices
  // Meshopt TRIANGLES may rotate each triangle's first corner, preserving its
  // vertices, winding and order. Canonicalize only that legal cyclic rotation.
  for (let offset = 0; offset < indices.length; offset += 3) {
    const triangle = indices.slice(offset, offset + 3)
    const first = triangle.indexOf(Math.min(...triangle))
    for (let corner = 0; corner < 3; corner++) indices[offset + corner] = triangle[(first + corner) % 3]
  }
  return indices
}
function nodeContract(document) {
  return document.getRoot().listNodes().map(node => ({ name: node.getName(), extras: node.getExtras(),
    children: node.listChildren().map(child => child.getName()), matrix: node.getMatrix(),
    mesh: node.getMesh()?.getName() ?? null, skin: node.getSkin()?.getName() ?? null }))
}
function geometryContract(document) {
  return document.getRoot().listMeshes().map(mesh => ({ name: mesh.getName(), extras: mesh.getExtras(),
    primitives: mesh.listPrimitives().map(primitive => ({ mode: primitive.getMode(), material: primitive.getMaterial()?.getName() ?? null,
      attributes: Object.fromEntries(primitive.listSemantics().sort().map(semantic => [semantic, array(primitive.getAttribute(semantic))])),
      indices: triangleIndices(primitive),
      targets: primitive.listTargets().map(target => Object.fromEntries(target.listSemantics().sort().map(semantic => [semantic, array(target.getAttribute(semantic))]))),
    })) }))
}
export function skinContract(document) {
  return document.getRoot().listSkins().map(skin => ({ name: skin.getName(), joints: skin.listJoints().map(node => node.getName()),
    skeleton: skin.getSkeleton()?.getName() ?? null, inverseBindMatrices: array(skin.getInverseBindMatrices()) }))
}
function animationContract(document) {
  return document.getRoot().listAnimations().map(animation => ({ name: animation.getName(),
    channels: animation.listChannels().map(channel => ({ node: channel.getTargetNode()?.getName(), path: channel.getTargetPath(),
      interpolation: channel.getSampler().getInterpolation(), times: array(channel.getSampler().getInput()),
      values: array(channel.getSampler().getOutput()) })).sort((a, b) => `${a.node}/${a.path}`.localeCompare(`${b.node}/${b.path}`)) }))
}
function sameContract(before, after, label) {
  if (canonicalJSON(before) !== canonicalJSON(after)) throw new Error(`Lossless ${label} contract changed during optimization`)
}
export function catalogBytes(metrics) {
  const textures = new Map()
  let bytes = 0
  for (const item of metrics) {
    bytes += item.geometryBytes + item.animationBytes + item.metadataBytes
    for (const texture of item.textures) {
      if (textures.has(texture.sha256) && textures.get(texture.sha256) !== texture.bytes) throw new Error('Texture hash byte count mismatch')
      textures.set(texture.sha256, texture.bytes)
    }
  }
  return bytes + [...textures.values()].reduce((sum, size) => sum + size, 0)
}
export async function measureArtifacts(result) {
  const textures = []
  for (const path of [...new Set(result.textures)]) {
    const bytes = await readFile(path)
    textures.push({ sha256: sha256(bytes), bytes: bytes.length, width: bytes.readUInt32LE(20), height: bytes.readUInt32LE(24) })
  }
  const metrics = { geometryBytes: (await stat(result.model)).size,
    animationBytes: (await Promise.all(Object.values(result.clips).map(path => stat(path)))).reduce((sum, entry) => sum + entry.size, 0),
    metadataBytes: (await stat(result.metadata)).size,
    textureBytes: [...new Map(textures.map(texture => [texture.sha256, texture.bytes])).values()].reduce((sum, size) => sum + size, 0), textures }
  return { ...metrics, totalBytes: catalogBytes([metrics]) }
}
function checkBudget(value, limit, label, allowZero = false) {
  if (!Number.isSafeInteger(limit) || limit < (allowZero ? 0 : 1)) throw new Error(`Missing or invalid ${label} budget`)
  if (value > limit) throw new Error(`${label} budget exceeded: ${value} > ${limit}`)
}
export async function validateArtifacts(result, recipe) {
  const io = assetIO()
  const rawMetadata = JSON.parse(await readFile(join(result.rawDirectory, 'metadata.json'), 'utf8'))
  sameContract(Object.keys(rawMetadata.clips).sort(), Object.keys(result.clips).sort(), 'selected clip set')
  const modelBytes = await readFile(result.model)
  const modelJSON = JSON.parse(modelBytes.subarray(20, 20 + modelBytes.readUInt32LE(12)).toString())
  const modelDirectory = dirname(resolve(result.model))
  const texturePaths = (modelJSON.images ?? []).map(image => {
    if (image.bufferView !== undefined || !/^textures\/[a-f0-9]{64}\.ktx2$/.test(image.uri ?? '')) {
      throw new Error('Model must reference content-named external KTX2 textures')
    }
    return join(modelDirectory, image.uri)
  })
  sameContract([...new Set(texturePaths)].sort(), [...new Set(result.textures.map(path => resolve(path)))].sort(), 'external texture set')
  for (const path of new Set(texturePaths)) {
    if (!path.endsWith(`${sha256(await readFile(path))}.ktx2`)) throw new Error('External texture content hash mismatch')
    await decodeKTX2(path)
  }
  const issues = []
  for (const path of [result.model, ...Object.values(result.clips)]) issues.push(await validateGLB(path))
  const model = await io.read(result.model)
  const raw = await io.read(join(result.rawDirectory, 'model.glb'))
  sameContract(nodeContract(raw), nodeContract(model), 'node hierarchy')
  sameContract(geometryContract(raw), geometryContract(model), 'geometry')
  sameContract(skinContract(raw), skinContract(model), 'skin')
  sameContract(raw.getRoot().listMaterials().map(material => material.getExtras()), model.getRoot().listMaterials().map(material => material.getExtras()), 'material extras')
  const trianglesByLod = {}
  let maxInfluences = 0
  for (const node of model.getRoot().listNodes()) {
    if (!node.getMesh()) continue
    const lod = node.getExtras().lod
    if (!Number.isSafeInteger(lod) || lod < 0) throw new Error(`Mesh ${node.getName()} is missing a numeric LOD budget tag`)
    for (const primitive of node.getMesh().listPrimitives()) {
      if (primitive.getMode() !== 4) throw new Error('Only triangle primitives are budgeted')
      trianglesByLod[lod] = (trianglesByLod[lod] ?? 0) + (primitive.getIndices()?.getCount() ?? primitive.getAttribute('POSITION').getCount()) / 3
      const weights = primitive.listSemantics().filter(name => /^WEIGHTS_\d+$/.test(name)).map(name => primitive.getAttribute(name))
      for (let vertex = 0; vertex < (weights[0]?.getCount() ?? 0); vertex++) {
        let influences = 0
        for (const accessor of weights) for (const weight of accessor.getElement(vertex, [])) if (weight > 0) influences++
        maxInfluences = Math.max(maxInfluences, influences)
      }
    }
  }
  const bones = Math.max(0, ...model.getRoot().listSkins().map(skin => skin.listJoints().length))
  for (const [lod, count] of Object.entries(trianglesByLod)) checkBudget(count, recipe.budgets?.trianglesByLod?.[lod], `LOD ${lod} triangles`)
  checkBudget(bones, recipe.budgets?.bones, 'bones')
  checkBudget(maxInfluences, recipe.budgets?.boneInfluences, 'bone influences')
  const metrics = await measureArtifacts(result)
  for (const texture of metrics.textures) {
    checkBudget(texture.width, recipe.budgets?.textureDimensions?.width, 'texture width')
    checkBudget(texture.height, recipe.budgets?.textureDimensions?.height, 'texture height')
  }
  // Empty texture sets must not turn a missing budget into a pass.
  checkBudget(0, recipe.budgets?.textureDimensions?.width, 'texture width')
  checkBudget(0, recipe.budgets?.textureDimensions?.height, 'texture height')
  checkBudget(metrics.totalBytes, recipe.budgets?.totalPublishedBytes, 'total bytes')
  const clipTargets = {}
  for (const [id, path] of Object.entries(result.clips).sort()) {
    const clip = await io.read(path)
    const root = clip.getRoot()
    if (root.listAnimations().length !== 1 || root.listAnimations()[0].getName() !== id) throw new Error(`Clip ${id} must contain exactly its named animation`)
    if (root.listTextures().length || root.listMaterials().length || root.listMeshes().length) throw new Error(`Clip ${id} contains model resources`)
    const original = await io.read(join(result.rawDirectory, 'animations', `${id}.glb`))
    sameContract(nodeContract(original), nodeContract(clip), `clip ${id} nodes`)
    sameContract(animationContract(original), animationContract(clip), `clip ${id} animation`)
    clipTargets[id] = animationContract(clip).flatMap(animation => animation.channels.map(channel => `${channel.node}/${channel.path}`)).sort()
  }
  return { errors: 0, warnings: issues.reduce((sum, issue) => sum + issue.numWarnings, 0),
    nodeNames: model.getRoot().listNodes().map(node => node.getName()), clipTargets, trianglesByLod, bones, maxInfluences,
    maxGeometryError: 0, maxAnimationError: 0, maxSkinError: 0, metrics }
}
