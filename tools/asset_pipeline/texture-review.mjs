import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { runInNewContext } from 'node:vm'
import { fileURLToPath } from 'node:url'
import sharp from 'sharp'

let basisPromise
async function basisModule() {
  if (!basisPromise) basisPromise = (async () => {
    const directory = new URL('../../web_new/node_modules/three/examples/jsm/libs/basis/', import.meta.url)
    const context = { module: { exports: {} }, exports: {}, require: createRequire(import.meta.url), process,
      __dirname: fileURLToPath(directory), console, setTimeout, clearTimeout, TextDecoder, WebAssembly }
    runInNewContext(await readFile(new URL('basis_transcoder.js', directory), 'utf8'), context)
    const basis = await context.module.exports({ wasmBinary: await readFile(new URL('basis_transcoder.wasm', directory)) })
    basis.initializeBasis()
    return basis
  })()
  return basisPromise
}
export async function decodeKTX2(path) {
  const basis = await basisModule()
  const texture = new basis.KTX2File(new Uint8Array(await readFile(path)))
  try {
    if (!texture.isValid() || !texture.startTranscoding()) throw new Error(`Invalid KTX2 payload: ${path}`)
    const width = texture.getWidth(), height = texture.getHeight()
    // Basis transcoder RGBA32 (13) is the browser loader's uncompressed fallback.
    const bytes = new Uint8Array(texture.getImageTranscodedSizeInBytes(0, 0, 0, 13))
    if (!texture.transcodeImage(bytes, 0, 0, 0, 13, 0, -1, -1)) throw new Error('KTX2 RGBA32 transcoding failed')
    return { width, height, bytes: Buffer.from(bytes) }
  } finally { texture.close(); texture.delete() }
}
export function comparePixels(reference, candidate) {
  if (reference.length !== candidate.length) throw new Error('Pixel arrays must have identical dimensions')
  let sum = 0, squared = 0, maximum = 0
  for (let index = 0; index < reference.length; index++) {
    const error = Math.abs(reference[index] - candidate[index])
    sum += error; squared += error * error; maximum = Math.max(maximum, error)
  }
  return { meanAbsoluteError: sum / reference.length, maxAbsoluteError: maximum, rootMeanSquareError: Math.sqrt(squared / reference.length) }
}
export async function reviewTexture(sourceBytes, ktxPath) {
  const decoded = await decodeKTX2(ktxPath)
  const full = await sharp(sourceBytes).toColourspace('srgb').ensureAlpha().raw().toBuffer({ resolveWithObject: true })
  const sampled = await sharp(sourceBytes).toColourspace('srgb').ensureAlpha()
    .resize({ width: decoded.width, height: decoded.height, fit: 'fill', kernel: 'lanczos3' }).raw().toBuffer()
  const enlarged = await sharp(decoded.bytes, { raw: { width: decoded.width, height: decoded.height, channels: 4 } })
    .resize({ width: full.info.width, height: full.info.height, kernel: 'lanczos3' }).raw().toBuffer()
  return { source: { width: full.info.width, height: full.info.height, bytes: sourceBytes.length },
    encoded: { width: decoded.width, height: decoded.height, bytes: (await readFile(ktxPath)).length },
    compressionError: comparePixels(sampled, decoded.bytes), fullResolutionError: comparePixels(full.data, enlarged) }
}
