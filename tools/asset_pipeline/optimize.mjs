import { execFile } from 'node:child_process'
import { mkdir, mkdtemp, readFile, writeFile, rm } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { promisify } from 'node:util'
import { Format } from '@gltf-transform/core'
import { EXTMeshoptCompression, KHRTextureBasisu } from '@gltf-transform/extensions'
import { MeshoptEncoder } from 'meshoptimizer'
import sharp from 'sharp'
import { assetIO, canonicalJSON, sha256, skinContract, validateGLB, validateArtifacts } from './report.mjs'

const execute = promisify(execFile)
const lockPath = new URL('./toolchain.lock.json', import.meta.url)
export function optimizerProfile(recipe) {
  const texture = recipe.optimization.texture
  if (recipe.optimization.meshCompression !== 'meshopt' || texture.codec !== 'uastc' || !texture.mipmaps) {
    throw new Error('Reviewed optimizer profile requires lossless meshopt and mipmapped UASTC; profile changes require measured review')
  }
  return { schema: 1, geometry: 'meshopt-lossless-no-quantization', constantTracks: 'preserve-all-sampled-keys',
    texture: { ...texture, resize: 'fit-inside-no-enlargement-lanczos3', pixels: 'rgba8-srgb', wrapMode: 'clamp-pinned-4.4.2-default',
      flags: ['--t2', '--encode', 'uastc', '--uastc_quality', String(texture.quality), '--zcmp', '18',
        '--genmipmap', '--filter', 'lanczos4', '--fscale', '1', '--threads', '1',
        '--assign_oetf', 'srgb', '--assign_primaries', 'bt709', '--upper_left_maps_to_s0t0'] } }
}

// GLB permits external images. Core's GLB writer embeds them, so pack the GLTF
// representation ourselves while leaving meshopt's virtual fallback buffer empty.
async function writeExternalTextureGLB(io, document, output, source) {
  const { json, resources } = await io.writeJSON(document, { format: Format.GLTF, basename: 'model' })
  const sourceBytes = await readFile(source)
  const sourceJSON = JSON.parse(sourceBytes.subarray(20, 20 + sourceBytes.readUInt32LE(12)).toString())
  const sourceNodes = new Map(sourceJSON.nodes.map(node => [node.name, node]))
  // Core elides near-identity TRS values. Preserve authored frames bit-for-bit.
  for (const node of json.nodes ?? []) for (const field of ['matrix', 'translation', 'rotation', 'scale']) {
    delete node[field]
    const original = sourceNodes.get(node.name)
    if (original?.[field] !== undefined) node[field] = original[field]
  }
  const embedded = json.buffers?.[0]
  const binary = embedded ? resources[embedded.uri] : null
  if (embedded && !binary) throw new Error('Missing primary model buffer')
  if (embedded) delete embedded.uri
  for (const buffer of (json.buffers ?? []).slice(1)) {
    if (!buffer.extensions?.EXT_meshopt_compression?.fallback) throw new Error('Unexpected additional model buffer')
    delete buffer.uri
  }
  const text = Buffer.from(canonicalJSON(JSON.parse(JSON.stringify(json))))
  const jsonChunk = Buffer.alloc(8 + Math.ceil(text.length / 4) * 4, 0x20)
  jsonChunk.writeUInt32LE(jsonChunk.length - 8, 0)
  jsonChunk.writeUInt32LE(0x4e4f534a, 4)
  text.copy(jsonChunk, 8)
  const binChunk = binary ? Buffer.alloc(8 + Math.ceil(binary.length / 4) * 4) : Buffer.alloc(0)
  if (binary) {
    binChunk.writeUInt32LE(binChunk.length - 8, 0)
    binChunk.writeUInt32LE(0x004e4942, 4)
    Buffer.from(binary).copy(binChunk, 8)
  }
  const header = Buffer.alloc(12)
  header.writeUInt32LE(0x46546c67, 0)
  header.writeUInt32LE(2, 4)
  header.writeUInt32LE(header.length + jsonChunk.length + binChunk.length, 8)
  await writeFile(output, Buffer.concat([header, jsonChunk, binChunk]))
}
function compress(document) {
  // QUANTIZE is the extension's unfiltered writer mode, not a quantize transform.
  // Calling the functions meshopt() preset here would change vertex/rig values.
  document.createExtension(EXTMeshoptCompression).setRequired(true)
    .setEncoderOptions({ method: EXTMeshoptCompression.EncoderMethod.QUANTIZE })
}
export async function optimizeExport({ rawDirectory, recipe, toolchain, outputDirectory, log = () => {} }) {
  const lock = JSON.parse(await readFile(lockPath, 'utf8'))
  for (const field of ['platform', 'node', 'blender']) {
    if (canonicalJSON(toolchain.identity[field]) !== canonicalJSON(lock[field])) throw new Error(`Optimizer requires locked ${field} identity`)
  }
  const encoder = toolchain.paths.toktx
  if (sha256(await readFile(encoder)) !== lock.toktx.binarySha256) throw new Error('Optimizer requires locked toktx binary')
  const profile = optimizerProfile(recipe)
  const packageLockHash = sha256(await readFile(new URL('./package-lock.json', import.meta.url)))
  const profileHash = sha256(canonicalJSON(profile))
  const toolchainHash = sha256(canonicalJSON({ lock, packageLockHash, profileHash }))
  await MeshoptEncoder.ready
  const io = assetIO().registerDependencies({ 'meshopt.encoder': MeshoptEncoder })
  const rawModel = join(rawDirectory, 'model.glb')
  log(`optimize model ${rawModel}`)
  await validateGLB(rawModel)
  const model = await io.read(rawModel)
  const rawMetadata = JSON.parse(await readFile(join(rawDirectory, 'metadata.json'), 'utf8'))
  const rigHash = sha256(canonicalJSON({ rig: rawMetadata.rig, skins: skinContract(model) }))
  await mkdir(join(outputDirectory, 'textures'), { recursive: true })
  await mkdir(join(outputDirectory, 'animations'), { recursive: true })
  const scratch = await mkdtemp(join(outputDirectory, '.encode-'))
  const textures = []
  const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => key !== 'TOKTX_OPTIONS'))
  try {
    for (const [index, texture] of model.getRoot().listTextures().entries()) {
      for (const material of model.getRoot().listMaterials()) {
        if ([material.getNormalTexture(), material.getOcclusionTexture(), material.getMetallicRoughnessTexture()].includes(texture)) {
          throw new Error('The sRGB base-color profile cannot encode data textures')
        }
      }
      const png = await sharp(texture.getImage()).toColourspace('srgb').ensureAlpha()
        .resize({ width: profile.texture.width, height: profile.texture.height, fit: 'inside', withoutEnlargement: true, kernel: 'lanczos3' })
        .png({ compressionLevel: 9, adaptiveFiltering: false }).toBuffer()
      const input = join(scratch, `${index}.png`), output = join(scratch, `${index}.ktx2`)
      log(`optimize texture ${input}`)
      await writeFile(input, png)
      await execute(encoder, [...profile.texture.flags, output, input], { env: environment, maxBuffer: 8 * 1024 * 1024 })
      const bytes = await readFile(output), name = `${sha256(bytes)}.ktx2`
      const path = join(outputDirectory, 'textures', name)
      await writeFile(path, bytes)
      log(`optimized texture ${path} (${png.length} -> ${bytes.length} bytes)`)
      textures.push(path)
      texture.setImage(bytes).setMimeType('image/ktx2').setURI(`textures/${name}`)
    }
  } finally { await rm(scratch, { recursive: true, force: true }) }
  if (textures.length) model.createExtension(KHRTextureBasisu).setRequired(true)
  compress(model)
  const modelPath = join(outputDirectory, 'model.glb')
  await writeExternalTextureGLB(io, model, modelPath, rawModel)
  log(`optimized model ${modelPath}`)
  const clips = {}
  for (const id of Object.keys(rawMetadata.clips).sort()) {
    const raw = join(rawDirectory, 'animations', `${id}.glb`)
    log(`optimize animation ${id} ${raw}`)
    await validateGLB(raw)
    const document = await io.read(raw)
    compress(document)
    clips[id] = join(outputDirectory, 'animations', `${id}.glb`)
    await writeExternalTextureGLB(io, document, clips[id], raw)
    log(`optimized animation ${id} ${clips[id]}`)
  }
  const metadata = join(outputDirectory, 'metadata.json')
  await writeFile(metadata, canonicalJSON({ ...rawMetadata, textures: [...new Set(textures)].map(path => ({ uri: `textures/${path.split('/').at(-1)}` })),
    rigHash, profile, profileHash, packageLockHash, toolchainHash, modelSha256: sha256(await readFile(modelPath)) }) + '\n')
  const result = { model: modelPath, clips, textures: [...new Set(textures)], metadata, rawDirectory: resolve(rawDirectory) }
  const report = await validateArtifacts(result, recipe)
  return { ...result, metrics: report.metrics }
}
