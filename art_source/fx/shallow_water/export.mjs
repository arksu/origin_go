import { createRequire } from 'node:module'
import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

// Reuse the asset pipeline's image encoder for a deterministic runtime export.
const require = createRequire(new URL('../../../tools/asset_pipeline/package.json', import.meta.url))
const sharp = require('sharp')
const source = fileURLToPath(new URL('./review/ripples-v1.png', import.meta.url))
const destinationDirectory = new URL('../../../web_new/public/assets/game/fx/shallow_water/', import.meta.url)
await mkdir(destinationDirectory, { recursive: true })
await sharp(source)
  .extract({ left: 360, top: 288, width: 1056, height: 384 })
  .resize(80, 40, { fit: 'fill', kernel: 'nearest' })
  .png()
  .toFile(fileURLToPath(new URL('ripples.png', destinationDirectory)))
