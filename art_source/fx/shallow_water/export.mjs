import { createRequire } from 'node:module'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

// Reuse the asset pipeline's image encoder for a deterministic runtime export.
const require = createRequire(new URL('../../../tools/asset_pipeline/package.json', import.meta.url))
const sharp = require('sharp')
const frames = JSON.parse(await readFile(new URL('./animation/frames.json', import.meta.url), 'utf8'))
if (!Array.isArray(frames) || frames.length !== 5) throw new Error('Ripple animation requires five source frames')
const frameWidth = 80
const frameHeight = 40
const destinationDirectory = new URL('../../../web_new/public/assets/game/fx/shallow_water/', import.meta.url)
await mkdir(destinationDirectory, { recursive: true })
const frameBuffers = []
const frameKeys = []
const atlasFrames = {}
for (const [index, frame] of frames.entries()) {
  if (!/^frame-\d{2}\.png$/.test(frame.file)) throw new Error(`Invalid ripple source filename: ${frame.file}`)
  const source = fileURLToPath(new URL(`./animation/${frame.file}`, import.meta.url))
  const buffer = await sharp(source)
    .extract({ left: 360, top: 288, width: 1056, height: 384 })
    .resize(frameWidth, frameHeight, { fit: 'fill', kernel: 'nearest' })
    .png().toBuffer()
  frameBuffers.push(buffer)
  const key = `ripple-${index}`
  frameKeys.push(key)
  atlasFrames[key] = {
    frame: { x: index * frameWidth, y: 0, w: frameWidth, h: frameHeight },
    rotated: false, trimmed: false,
    spriteSourceSize: { x: 0, y: 0, w: frameWidth, h: frameHeight },
    sourceSize: { w: frameWidth, h: frameHeight },
  }
}
await sharp({ create: { width: frameWidth * frames.length, height: frameHeight, channels: 4,
  background: { r: 0, g: 0, b: 0, alpha: 0 } } })
  .composite(frameBuffers.map((input, index) => ({ input, left: index * frameWidth, top: 0 })))
  .png().toFile(fileURLToPath(new URL('ripples-strip.png', destinationDirectory)))
await writeFile(new URL('ripples.json', destinationDirectory), JSON.stringify({
  frames: atlasFrames,
  animations: { ripples: frameKeys },
  meta: { image: 'ripples-strip.png', format: 'RGBA8888',
    size: { w: frameWidth * frames.length, h: frameHeight }, scale: '1' },
}, null, 2) + '\n')
// Preserve the first-frame PNG for static texture reviews.
await writeFile(new URL('ripples.png', destinationDirectory), frameBuffers[0])
