import { MinimapRenderer } from '../../src/game/minimap/MinimapRenderer'
import type { MinimapChunk, MinimapFrame } from '../../src/game/minimap/types'
import { RENDERABLE_TILE_IDS, TILE_PLOWED, TILE_VOID } from '../../src/game/tiles/tileIds'
import { MINIMAP_DEFAULT_ZOOM, MINIMAP_MANIFEST_URL } from '../../src/constants/minimap'

const chunkSize = 64
const materialNames = [
  'Deep water', 'Shallow water', 'Stone paving', 'Plowed ground', 'Coniferous forest',
  'Broadleaf forest', 'Thicket', 'Grass', 'Heath', 'Moor', 'Swamp 1', 'Swamp 2',
  'Swamp 3', 'Dirt', 'Clay', 'Sand', 'Mountain',
]
const worldCanvas = document.querySelector<HTMLCanvasElement>('#world')!
const status = document.querySelector<HTMLOutputElement>('#status')!
const assetStatus = document.querySelector<HTMLOutputElement>('#asset-status')!
const diagnostics = document.querySelector<HTMLPreElement>('#diagnostics')!
const materials = document.querySelector<HTMLDivElement>('#materials')!
const worldChunks = new Map<string, MinimapChunk>()
const nativeFetch = window.fetch.bind(window)
const nativeWarn = console.warn.bind(console)
const nativeDpr = window.devicePixelRatio
const dprDescriptor = Object.getOwnPropertyDescriptor(window, 'devicePixelRatio')
let zoom: number = MINIMAP_DEFAULT_ZOOM
let headingIndex = 0
let selectedDpr: number | null = null
let layer = 0
let playerPresent = true
let forceFailure = false
let diagnosticCount = 0
let worldRenderer: MinimapRenderer

const samples = RENDERABLE_TILE_IDS.map((tileId, index) => {
  const figure = document.createElement('figure')
  const canvas = document.createElement('canvas')
  canvas.setAttribute('aria-label', `${materialNames[index]} tile ID ${tileId}`)
  const caption = document.createElement('figcaption')
  caption.textContent = `${tileId} · ${materialNames[index]}`
  figure.append(canvas, caption)
  materials.append(figure)
  return { canvas, tiles: new Uint8Array(chunkSize * chunkSize).fill(tileId), renderer: null as MinimapRenderer | null }
})

console.warn = (...arguments_: unknown[]) => {
  nativeWarn(...arguments_)
  if (String(arguments_[0]).startsWith('[Minimap]')) {
    diagnosticCount++
    diagnostics.textContent = `Minimap diagnostic count: ${diagnosticCount}\n${arguments_.map(String).join(' ')}`
  }
}

function loadLayer(): void {
  worldRenderer?.clear()
  worldChunks.clear()
  layer++
  for (let chunkY = -4; chunkY < 4; chunkY++) {
    for (let chunkX = -4; chunkX < 4; chunkX++) {
      if (chunkX === 1 && chunkY === 0) continue
      const tiles = new Uint8Array(chunkSize * chunkSize)
      for (let localY = 0; localY < chunkSize; localY++) {
        for (let localX = 0; localX < chunkSize; localX++) {
          const worldX = chunkX * chunkSize + localX
          const worldY = chunkY * chunkSize + localY
          const band = Math.floor(worldX / 8) + layer - 1
          const tileId = RENDERABLE_TILE_IDS[((band % RENDERABLE_TILE_IDS.length) + RENDERABLE_TILE_IDS.length) % RENDERABLE_TILE_IDS.length]!
          tiles[localY * chunkSize + localX] = worldY >= 24 && worldY < 32 ? TILE_VOID : tileId
        }
      }
      worldChunks.set(`${chunkX},${chunkY}`, { x: chunkX, y: chunkY, tiles, version: layer })
    }
  }
  playerPresent = true
}

function createRenderers(failManifest: boolean): void {
  worldRenderer?.destroy()
  for (const sample of samples) sample.renderer?.destroy()
  forceFailure = failManifest
  diagnosticCount = 0
  diagnostics.textContent = ''
  // Fault injection is restricted to renderer construction in this synthetic page.
  window.fetch = (input, init) => {
    if (failManifest && String(input).endsWith(MINIMAP_MANIFEST_URL)) {
      return Promise.reject(new Error('Synthetic fixture: forced missing minimap manifest'))
    }
    return nativeFetch(input, init)
  }
  try {
    worldRenderer = new MinimapRenderer(worldCanvas)
    worldRenderer.setZoom(zoom)
    for (const sample of samples) {
      sample.renderer = new MinimapRenderer(sample.canvas)
      sample.renderer.setZoom(zoom)
    }
  } finally {
    window.fetch = nativeFetch
  }
  updateButtons()
}

function updateButtons(): void {
  for (const button of document.querySelectorAll<HTMLButtonElement>('[data-zoom]')) {
    button.setAttribute('aria-pressed', String(Number(button.dataset.zoom) === zoom))
  }
  for (const button of document.querySelectorAll<HTMLButtonElement>('[data-heading]')) {
    button.setAttribute('aria-pressed', String(Number(button.dataset.heading) === headingIndex))
  }
  for (const button of document.querySelectorAll<HTMLButtonElement>('[data-dpr]')) {
    button.setAttribute('aria-pressed', String(button.dataset.dpr === 'native' ? selectedDpr === null : Number(button.dataset.dpr) === selectedDpr))
  }
  document.querySelector('#load-atlas')!.setAttribute('aria-pressed', String(!forceFailure))
  document.querySelector('#fail-atlas')!.setAttribute('aria-pressed', String(forceFailure))
}

for (const button of document.querySelectorAll<HTMLButtonElement>('[data-zoom]')) {
  button.addEventListener('click', () => {
    zoom = Number(button.dataset.zoom)
    worldRenderer.setZoom(zoom)
    for (const sample of samples) sample.renderer!.setZoom(zoom)
    updateButtons()
  })
}
for (const button of document.querySelectorAll<HTMLButtonElement>('[data-heading]')) {
  button.addEventListener('click', () => {
    headingIndex = Number(button.dataset.heading)
    updateButtons()
  })
}
for (const button of document.querySelectorAll<HTMLButtonElement>('[data-dpr]')) {
  button.addEventListener('click', () => {
    selectedDpr = button.dataset.dpr === 'native' ? null : Number(button.dataset.dpr)
    Object.defineProperty(window, 'devicePixelRatio', { configurable: true, get: () => selectedDpr ?? nativeDpr })
    updateButtons()
  })
}
document.querySelector('#load-atlas')!.addEventListener('click', () => createRenderers(false))
document.querySelector('#fail-atlas')!.addEventListener('click', () => createRenderers(true))
document.querySelector('#reset-world')!.addEventListener('click', () => {
  worldChunks.clear()
  playerPresent = false
  worldRenderer.clear()
})
document.querySelector('#load-world')!.addEventListener('click', loadLayer)
document.querySelector('#remove-chunks')!.addEventListener('click', () => worldChunks.clear())
document.querySelector('#edit-terrain')!.addEventListener('click', () => {
  worldChunks.set('0,0', { x: 0, y: 0, tiles: new Uint8Array(chunkSize * chunkSize).fill(TILE_PLOWED), version: layer + 1 })
})

function draw(): void {
  const player = { x: 0, y: 0, heading: headingIndex * Math.PI / 2 }
  const frame: MinimapFrame = {
    player: playerPresent ? player : null,
    chunkSize,
    coordPerTile: 1,
    getChunk: (chunkX, chunkY) => worldChunks.get(`${chunkX},${chunkY}`),
  }
  worldRenderer.render(frame)
  for (const sample of samples) {
    sample.renderer!.render({
      player, chunkSize, coordPerTile: 1,
      getChunk: (chunkX, chunkY) => ({ x: chunkX, y: chunkY, tiles: sample.tiles, version: 1 }),
    })
  }
  const headingName = ['East →', 'South ↓', 'West ←', 'North ↑'][headingIndex]
  status.textContent = `Layer ${layer} · ${worldChunks.size} available synthetic chunks · ${playerPresent ? 'player at (0, 0)' : 'no player pose'}\nZoom ${zoom}× · heading ${headingName} · effective DPR ${window.devicePixelRatio} (${selectedDpr === null ? 'native' : 'fixture override'})\nWorld canvas backing: ${worldCanvas.width}×${worldCanvas.height}; CSS: 200×200`
  const firstSample = samples[0]!.canvas
  const row = firstSample.getContext('2d')!.getImageData(0, 0, firstSample.width, 1).data
  const colors = new Set<string>()
  for (let index = 0; index < row.length; index += 4) colors.add(`${row[index]},${row[index + 1]},${row[index + 2]}`)
  assetStatus.textContent = forceFailure
    ? `Forced manifest failure · fallback palette active · ${diagnosticCount} diagnostic(s) across ${samples.length + 1} renderers; count must stop growing.`
    : `Real atlas requested · ${colors.size > 1 ? 'textured water pixels observed' : 'waiting for texture variation'} · ${diagnosticCount} diagnostic(s).`
}

loadLayer()
createRenderers(false)
draw()
const timer = window.setInterval(draw, 100)
window.addEventListener('pagehide', () => {
  window.clearInterval(timer)
  worldRenderer.destroy()
  for (const sample of samples) sample.renderer?.destroy()
  console.warn = nativeWarn
  if (dprDescriptor) Object.defineProperty(window, 'devicePixelRatio', dprDescriptor)
  else Reflect.deleteProperty(window, 'devicePixelRatio')
})
