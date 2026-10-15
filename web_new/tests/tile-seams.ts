import { Application, Assets, Container, Graphics, Rectangle, Sprite, TextureStyle, type Spritesheet } from 'pixi.js'
import { Chunk } from '../src/game/Chunk'
import { getGroundTextureName, getRegisteredTileIdsBelow, getTileSet } from '../src/game/tiles/TileSet'
import { initTileSets } from '../src/game/tiles/tileSetLoader'
import { TILE_WIDTH_HALF, TILE_HEIGHT_HALF } from '../src/game/tiles/Tile'
import { setWorldParams } from './serverConstantsFixture'
import * as tileIds from '../src/game/tiles/tileIds'
import { PIECE_OFFSETS, pieceNeighborhood, transitionPieces, type TransitionPiece } from './tile-seam-pieces'

const CHUNK_SIZE = 16
const ZOOM_LEVELS = [0.25, 0.5, 1, 2, 3, 4, 6]
const TILE_NAMES: Record<number, string> = {
  [tileIds.TILE_DEEP_WATER]: 'Deep Water',
  [tileIds.TILE_SHALLOW_WATER]: 'Shallow Water',
  [tileIds.TILE_STONE_PAVING]: 'Stone Paving',
  [tileIds.TILE_PLOWED]: 'Plowed Land',
  [tileIds.TILE_CONIFEROUS_FOREST]: 'Coniferous Forest',
  [tileIds.TILE_BROADLEAF_FOREST]: 'Broadleaf Forest',
  [tileIds.TILE_THICKET]: 'Thicket',
  [tileIds.TILE_GRASS]: 'Grass',
  [tileIds.TILE_HEATH]: 'Heath',
  [tileIds.TILE_MOOR]: 'Moor',
  [tileIds.TILE_SWAMP_1]: 'Swamp 1',
  [tileIds.TILE_SWAMP_2]: 'Swamp 2',
  [tileIds.TILE_SWAMP_3]: 'Swamp 3',
  [tileIds.TILE_DIRT]: 'Dirt',
  [tileIds.TILE_CLAY]: 'Clay',
  [tileIds.TILE_SAND]: 'Sand',
  [tileIds.TILE_MOUNTAIN]: 'Mountain',
}

function element<T extends HTMLElement>(id: string): T {
  const found = document.getElementById(id)
  if (!found) throw new Error(`Missing control: ${id}`)
  return found as T
}

const controls = {
  view: element<HTMLSelectElement>('view'),
  target: element<HTMLSelectElement>('target'),
  direction: element<HTMLSelectElement>('direction'),
  neighbor: element<HTMLSelectElement>('neighbor'),
  pattern: element<HTMLSelectElement>('pattern'),
  third: element<HTMLSelectElement>('third'),
  size: element<HTMLSelectElement>('size'),
  zoom: element<HTMLSelectElement>('zoom'),
  background: element<HTMLInputElement>('background'),
  grid: element<HTMLInputElement>('grid'),
  chunks: element<HTMLInputElement>('chunks'),
}
const preview = element<HTMLElement>('preview')
const gallery = element<HTMLElement>('gallery')
const piecesOutput = element<HTMLElement>('pieces')
const pieceKind = element<HTMLSelectElement>('piece-kind')
const pieceZoom = element<HTMLSelectElement>('piece-zoom')
const pieceContext = element<HTMLInputElement>('piece-context')
const status = element<HTMLElement>('status')
const inspect = element<HTMLElement>('inspect')
const errorOutput = element<HTMLElement>('error')
const tileLabel = (id: number) => `${TILE_NAMES[id] ?? `Type ${id}`} [${id}]`

function reportError(error: unknown): void {
  errorOutput.textContent = error instanceof Error ? error.message : String(error)
  document.title = 'Error — tile seams'
  console.error(error)
}

async function main(): Promise<void> {
  // Use the same atlas sampler and Chunk mesh builder as the game, including
  // its descending-ID border/corner passes and cross-chunk neighbor lookup.
  TextureStyle.defaultOptions.scaleMode = 'nearest'
  initTileSets()
  setWorldParams(32, CHUNK_SIZE)
  const supportedIds = getRegisteredTileIdsBelow(tileIds.TILE_VOID).reverse()
  if (!supportedIds.length) throw new Error('The client did not register any tile types')
  const app = new Application()
  await app.init({
    width: Math.max(1, preview.clientWidth), height: Math.max(1, preview.clientHeight), resolution: 1,
    background: controls.background.value, antialias: false, preference: 'webgl',
  })
  app.stop()
  preview.append(app.canvas)
  const scene = new Container()
  const terrain = new Container({ sortableChildren: true })
  const grid = new Graphics()
  const highlight = new Graphics()
  scene.addChild(terrain, grid, highlight)
  app.stage.addChild(scene)
  let chunks: Chunk[] = []
  let originChunkX = 0
  let originChunkY = 0
  let fieldSize = Number(controls.size.value)
  let fieldTiles = new Uint8Array()
  let drag: { pointerId: number; x: number; y: number } | null = null

  for (const select of [controls.target, controls.third]) {
    for (const id of supportedIds) select.add(new Option(tileLabel(id), String(id)))
  }
  controls.target.value = String(tileIds.TILE_SHALLOW_WATER)
  controls.third.value = String(tileIds.TILE_SHALLOW_WATER)

  function updateNeighbors(): void {
    const target = Number(controls.target.value)
    const previous = Number(controls.neighbor.value) || tileIds.TILE_SAND
    const direction = controls.direction.value
    controls.neighbor.replaceChildren()
    for (const [above, label] of [[false, 'Target overlays these types'], [true, 'These types overlay the target']] as const) {
      if (direction === 'target-over' && above || direction === 'neighbor-over' && !above) continue
      const group = document.createElement('optgroup')
      group.label = label
      for (const id of supportedIds) {
        if (id === target || (id < target) !== above) continue
        group.append(new Option(tileLabel(id), String(id)))
      }
      if (group.children.length) controls.neighbor.append(group)
    }
    const available = Array.from(controls.neighbor.options).map(option => Number(option.value))
    if (available.includes(previous)) controls.neighbor.value = String(previous)
    if (!available.length) controls.neighbor.add(new Option('No matching types', ''))
  }

  function sampleType(x: number, y: number): number {
    const target = Number(controls.target.value)
    const neighbor = Number(controls.neighbor.value) || target
    const center = fieldSize / 2
    const inside = x >= fieldSize / 4 && x < fieldSize * 3 / 4 && y >= fieldSize / 4 && y < fieldSize * 3 / 4
    switch (controls.pattern.value) {
      case 'solid': return target
      case 'target-island': return inside ? target : neighbor
      case 'neighbor-island': return inside ? neighbor : target
      case 'split-x': return x < center ? target : neighbor
      case 'split-y': return y < center ? target : neighbor
      case 'stripes': return Math.floor(x / 4) % 2 === 0 ? target : neighbor
      case 'checkerboard': return (Math.floor(x / 4) + Math.floor(y / 4)) % 2 === 0 ? target : neighbor
      case 'single-cells': return x % 4 === 2 && y % 4 === 2 ? neighbor : target
      case 'three-way': return x < center ? target : y < center ? neighbor : Number(controls.third.value)
      default: throw new Error('Unknown field pattern')
    }
  }

  function render(): void { app.render() }

  function project(x: number, y: number): [number, number] {
    const globalX = originChunkX * CHUNK_SIZE + x
    const globalY = originChunkY * CHUNK_SIZE + y
    return [(globalX - globalY) * TILE_WIDTH_HALF, (globalX + globalY) * TILE_HEIGHT_HALF]
  }

  function drawGrid(): void {
    grid.clear()
    const zoom = scene.scale.x
    for (let index = 0; index <= fieldSize; index++) {
      const chunkBoundary = controls.chunks.checked && index % CHUNK_SIZE === 0
      if (!controls.grid.checked && !chunkBoundary) continue
      const color = chunkBoundary ? 0xffcd70 : 0xe1f5dd
      const width = (chunkBoundary ? 2 : 1) / zoom
      const alpha = chunkBoundary ? 0.85 : 0.22
      grid.moveTo(...project(index, 0)).lineTo(...project(index, fieldSize)).stroke({ color, width, alpha })
      grid.moveTo(...project(0, index)).lineTo(...project(fieldSize, index)).stroke({ color, width, alpha })
    }
  }

  function centerScene(): void {
    const [centerX, centerY] = project(fieldSize / 2, fieldSize / 2)
    scene.position.set(Math.round(app.screen.width / 2 - centerX * scene.scale.x), Math.round(app.screen.height / 2 - centerY * scene.scale.y))
  }

  function setZoom(value: number, anchorX = app.screen.width / 2, anchorY = app.screen.height / 2): void {
    const localX = (anchorX - scene.x) / scene.scale.x
    const localY = (anchorY - scene.y) / scene.scale.y
    scene.scale.set(value)
    scene.position.set(Math.round(anchorX - localX * value), Math.round(anchorY - localY * value))
    controls.zoom.value = String(value)
    highlight.clear()
    drawGrid()
    render()
  }

  function rebuild(): void {
    errorOutput.textContent = ''
    highlight.clear()
    for (const chunk of chunks) chunk.destroy()
    chunks = []
    const showingPieces = controls.view.value === 'pieces'
    preview.hidden = showingPieces
    gallery.hidden = !showingPieces
    for (const input of [controls.pattern, controls.size, controls.zoom, controls.grid, controls.chunks]) input.disabled = showingPieces
    for (const id of ['shuffle', 'fit', 'center']) element<HTMLButtonElement>(id).disabled = showingPieces
    controls.third.disabled = showingPieces || controls.pattern.value !== 'three-way'
    controls.neighbor.disabled = !controls.neighbor.value || (!showingPieces && controls.pattern.value === 'solid')
    if (showingPieces) {
      rebuildPieces()
      return
    }
    app.renderer.resize(preview.clientWidth, preview.clientHeight)
    fieldSize = Number(controls.size.value)
    const target = Number(controls.target.value)
    const mixed = controls.pattern.value !== 'solid'
    const noNeighbor = !controls.neighbor.value
    controls.neighbor.disabled = !mixed || noNeighbor
    controls.third.disabled = controls.pattern.value !== 'three-way'
    if (mixed && noNeighbor) {
      fieldTiles = new Uint8Array()
      grid.clear()
      status.textContent = 'No neighboring types are available for this direction. Choose another direction or a solid field.'
      inspect.textContent = 'Field not built: no matching neighbor type.'
      render()
      return
    }
    const chunksAcross = fieldSize / CHUNK_SIZE
    const neighbors = new Map<string, Uint8Array>()
    // A guard ring gives perimeter cells real neighbors without drawing more
    // field; it also keeps every transition identical across chunk boundaries.
    for (let cy = -1; cy <= chunksAcross; cy++) for (let cx = -1; cx <= chunksAcross; cx++) {
      const tiles = new Uint8Array(CHUNK_SIZE * CHUNK_SIZE)
      for (let y = 0; y < CHUNK_SIZE; y++) for (let x = 0; x < CHUNK_SIZE; x++) {
        tiles[y * CHUNK_SIZE + x] = sampleType(cx * CHUNK_SIZE + x, cy * CHUNK_SIZE + y)
      }
      neighbors.set(`${originChunkX + cx},${originChunkY + cy}`, tiles)
    }
    fieldTiles = new Uint8Array(fieldSize * fieldSize)
    const presentIds = new Set<number>()
    let transitionCells = 0
    for (let cy = 0; cy < chunksAcross; cy++) for (let cx = 0; cx < chunksAcross; cx++) {
      const key = `${originChunkX + cx},${originChunkY + cy}`
      const tiles = neighbors.get(key)!
      const chunk = new Chunk(originChunkX + cx, originChunkY + cy)
      chunks.push(chunk)
      const result = chunk.buildTiles(tiles, spritesheet, neighbors)
      terrain.addChild(chunk.getContainer())
      for (let y = 0; y < CHUNK_SIZE; y++) for (let x = 0; x < CHUNK_SIZE; x++) {
        const id = tiles[y * CHUNK_SIZE + x]!
        fieldTiles[(cy * CHUNK_SIZE + y) * fieldSize + cx * CHUNK_SIZE + x] = id
        presentIds.add(id)
        if (result.hasBordersOrCorners[x]![y]) transitionCells++
      }
    }
    const order = [...presentIds].sort((first, second) => first - second).map(tileLabel).join(' → ')
    status.textContent = `${fieldSize} × ${fieldSize} (${fieldTiles.length} tiles), ${chunks.length} chunks; ${transitionCells} transition tiles. ${presentIds.size > 1 ? `Edge order from top to bottom: ${order}.` : `Solid field: ${tileLabel(target)}.`}`
    inspect.textContent = 'Hover over a tile to see its type and texture variant.'
    document.title = `${tileLabel(target)} — tile seams`
    drawGrid()
    render()
  }

  // Optional local candidate atlas for art review before publishing the game atlas.
  const atlasParam = new URLSearchParams(window.location.search).get('atlas')
  const atlasUrl = atlasParam ? new URL(atlasParam, window.location.href) : new URL('/assets/game/tiles.json', window.location.href)
  if (atlasUrl.origin !== window.location.origin || !atlasUrl.pathname.endsWith('.json')) throw new Error('Review atlas must be a local JSON atlas')
  const spritesheet = await Assets.load<Spritesheet>(atlasUrl.href)
  // Fail instead of silently displaying Chunk's fallback sand when an atlas
  // frame is missing: a fallback would invalidate this visual comparison.
  for (const id of supportedIds) {
    const set = getTileSet(id)!
    for (const array of [set.ground, ...set.borders, ...set.corners]) {
      for (const name of array.getTextureNames()) {
        if (!spritesheet.textures[name]) throw new Error(`Texture missing from atlas: ${name} (${tileLabel(id)})`)
      }
    }
  }

  function pieceSprite(name: string): Sprite {
    const texture = spritesheet.textures[name]!
    const sprite = new Sprite(texture)
    // Same full-canvas scale as VertexBuffer, retaining trim and rotated UVs.
    sprite.scale.set(64 / texture.orig.width, 32 / texture.orig.height)
    return sprite
  }

  function imageFigure(target: Container, frame: Rectangle, label: string): HTMLElement {
    const figure = document.createElement('figure')
    const caption = document.createElement('figcaption')
    caption.textContent = label
    const canvas = app.renderer.extract.canvas({ target, frame, resolution: 1, antialias: false }) as HTMLCanvasElement
    const zoom = Number(pieceZoom.value)
    canvas.style.width = `${frame.width * zoom}px`
    canvas.style.height = `${frame.height * zoom}px`
    canvas.setAttribute('role', 'img')
    canvas.setAttribute('aria-label', label)
    figure.append(caption, canvas)
    return figure
  }

  function contextFigure(piece: TransitionPiece, overlay: number, base: number): HTMLElement {
    const tiles = pieceNeighborhood(piece.kind, piece.mask, overlay, base, CHUNK_SIZE)
    const set = getTileSet(overlay)!
    const arrays = piece.kind === 'border' ? set.borders : set.corners
    // Force this exact variant for the requested mask; all remaining selections
    // and suppression of diagonal corners are handled by the real Chunk code.
    const sheet = Object.create(spritesheet) as Spritesheet
    sheet.textures = { ...spritesheet.textures }
    for (const name of arrays[piece.mask - 1]!.getTextureNames()) sheet.textures[name] = spritesheet.textures[piece.texture]!
    const chunk = new Chunk(0, 0)
    const target = new Container({ sortableChildren: true })
    try {
      chunk.buildTiles(tiles, sheet)
      target.addChild(chunk.getContainer())
      const centerY = Math.floor(CHUNK_SIZE / 2) * 32
      const outline = new Graphics().poly([0, centerY, 32, centerY + 16, 0, centerY + 32, -32, centerY + 16])
        .stroke({ color: 0xffe1a0, width: 0.5, alpha: 0.8 })
      outline.zIndex = Number.MAX_SAFE_INTEGER
      target.addChild(outline)
      return imageFigure(target, new Rectangle(-96, centerY - 32, 192, 96), 'In game · center tile outlined')
    } finally {
      chunk.destroy()
      target.destroy({ children: true })
    }
  }

  function rebuildPieces(): void {
    element('review-sheet').hidden = true
    piecesOutput.replaceChildren()
    if (!controls.neighbor.value) {
      status.textContent = 'No neighboring types are available. Choose another overlay direction.'
      return
    }
    const first = Number(controls.target.value)
    const second = Number(controls.neighbor.value)
    const overlay = Math.min(first, second)
    const base = Math.max(first, second)
    const pieces = transitionPieces(getTileSet(overlay)!).filter(piece => pieceKind.value === 'all' || piece.kind === pieceKind.value)
    const fragment = document.createDocumentFragment()
    for (const piece of pieces) {
      const card = document.createElement('article')
      card.className = 'piece'
      card.dataset.texture = piece.texture
      const heading = document.createElement('h2')
      heading.textContent = `${piece.kind === 'border' ? 'Border' : 'Corner'} ${piece.mask} · mask ${piece.mask.toString(2).padStart(4, '0')} · variant ${piece.variant}`
      const diagram = document.createElement('div')
      diagram.className = 'mask'
      diagram.setAttribute('aria-label', 'Neighbor mask in world X/Y axes')
      for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) {
        const cell = document.createElement('span')
        const bit = PIECE_OFFSETS[piece.kind].findIndex(([x, y]) => x === dx && y === dy)
        if (bit >= 0 && piece.mask & (1 << bit)) cell.className = 'overlay'
        if (dx === 0 && dy === 0) cell.className = 'center'
        cell.title = `(${dx}, ${dy})`
        diagram.append(cell)
      }
      const images = document.createElement('div')
      images.className = 'piece-images'
      const tile = new Container()
      try {
        tile.addChild(pieceSprite(piece.texture))
        images.append(imageFigure(tile, new Rectangle(0, 0, 64, 32), 'Piece · transparent canvas'))
        const ground = pieceSprite(getGroundTextureName(base, 8, 8)!)
        tile.addChildAt(ground, 0)
        images.append(imageFigure(tile, new Rectangle(0, 0, 64, 32), `Over ${TILE_NAMES[base]}`))
      } finally {
        tile.destroy({ children: true })
      }
      if (pieceContext.checked) images.append(contextFigure(piece, overlay, base))
      const path = document.createElement('code')
      path.textContent = `art_source/tiles/${piece.texture}`
      card.append(heading, diagram, images, path)
      fragment.append(card)
    }
    piecesOutput.append(fragment)
    status.textContent = `${tileLabel(overlay)} overlays ${tileLabel(base)} · ${pieces.length} texture variants · ${new Set(pieces.filter(piece => piece.kind === 'border').map(piece => piece.mask)).size} border masks · ${new Set(pieces.filter(piece => piece.kind === 'corner').map(piece => piece.mask)).size} corner masks.`
    if (!pieces.length) status.textContent += ' This tileset has no pieces for the selected filter.'
    inspect.textContent = 'All registered variants are shown. Select a source path to copy it for editing.'
    document.title = `${tileLabel(overlay)} — transition pieces`
  }

  function exportReview(): void {
    const cards = [...piecesOutput.querySelectorAll<HTMLElement>('.piece')]
    if (!cards.length) return
    const columns = 6
    const width = pieceContext.checked ? 192 : 64
    const height = pieceContext.checked ? 96 : 32
    const sheet = document.createElement('canvas')
    sheet.width = columns * width
    sheet.height = Math.ceil(cards.length / columns) * (height + 20)
    const context = sheet.getContext('2d')!
    context.fillStyle = '#14231b'
    context.fillRect(0, 0, sheet.width, sheet.height)
    context.font = '11px monospace'
    context.fillStyle = '#e0eadc'
    cards.forEach((card, index) => {
      const x = index % columns * width
      const y = Math.floor(index / columns) * (height + 20)
      const canvases = card.querySelectorAll('canvas')
      const canvas = canvases[pieceContext.checked ? 2 : 1]!
      context.fillText(card.dataset.texture!, x + 4, y + 13)
      context.drawImage(canvas, x, y + 20)
    })
    const link = element<HTMLAnchorElement>('review-download')
    link.href = sheet.toDataURL('image/png')
    element<HTMLImageElement>('review-image').src = link.href
    element('review-sheet').hidden = false
    element('review-sheet').scrollIntoView({ block: 'start' })
  }

  function safely(action: () => void): () => void {
    return () => { try { action() } catch (error) { reportError(error) } }
  }

  controls.target.addEventListener('change', safely(() => { updateNeighbors(); rebuild() }))
  controls.view.addEventListener('change', safely(() => { rebuild(); centerScene(); render() }))
  for (const control of [pieceKind, pieceZoom, pieceContext]) control.addEventListener('change', safely(rebuildPieces))
  element('reload').addEventListener('click', () => window.location.reload())
  element('export-review').addEventListener('click', safely(exportReview))
  controls.direction.addEventListener('change', safely(() => { updateNeighbors(); rebuild() }))
  for (const select of [controls.neighbor, controls.pattern, controls.third]) select.addEventListener('change', safely(rebuild))
  controls.size.addEventListener('change', safely(() => { rebuild(); centerScene(); render() }))
  controls.zoom.addEventListener('change', safely(() => setZoom(Number(controls.zoom.value))))
  for (const input of [controls.grid, controls.chunks]) input.addEventListener('change', safely(() => { drawGrid(); render() }))
  controls.background.addEventListener('input', () => { app.renderer.background.color = controls.background.value; render() })
  element('shuffle').addEventListener('click', safely(() => {
    originChunkX += 7
    originChunkY += 11
    rebuild()
    centerScene()
    render()
  }))
  element('center').addEventListener('click', () => { centerScene(); render() })
  element('fit').addEventListener('click', () => {
    const fit = Math.min((app.screen.width - 40) / (fieldSize * 64), (app.screen.height - 40) / (fieldSize * 32))
    setZoom(ZOOM_LEVELS.filter(level => level <= fit).at(-1) ?? ZOOM_LEVELS[0]!)
    centerScene()
    render()
  })

  function inspectTile(canvasX: number, canvasY: number): void {
    const screenX = (canvasX - scene.x) / scene.scale.x
    const screenY = (canvasY - scene.y) / scene.scale.y
    const globalX = Math.floor((screenX / TILE_WIDTH_HALF + screenY / TILE_HEIGHT_HALF) / 2)
    const globalY = Math.floor((screenY / TILE_HEIGHT_HALF - screenX / TILE_WIDTH_HALF) / 2)
    const x = globalX - originChunkX * CHUNK_SIZE
    const y = globalY - originChunkY * CHUNK_SIZE
    highlight.clear()
    if (x < 0 || y < 0 || x >= fieldSize || y >= fieldSize || !fieldTiles.length) {
      inspect.textContent = 'Outside the field'
    } else {
      const id = fieldTiles[y * fieldSize + x]!
      const texture = getGroundTextureName(id, globalX, globalY)
      inspect.textContent = `${tileLabel(id)} · coordinates (${globalX}, ${globalY}) · ${texture}`
      highlight.poly([...project(x, y), ...project(x + 1, y), ...project(x + 1, y + 1), ...project(x, y + 1)]).stroke({ color: 0xffebac, width: 1 / scene.scale.x, alpha: 0.85 })
    }
    render()
  }

  app.canvas.addEventListener('pointerdown', event => {
    if (event.button !== 0 && event.button !== 1) return
    drag = { pointerId: event.pointerId, x: event.clientX, y: event.clientY }
    app.canvas.setPointerCapture(event.pointerId)
    app.canvas.classList.add('dragging')
  })
  app.canvas.addEventListener('pointermove', event => {
    if (drag?.pointerId === event.pointerId) {
      scene.x += event.clientX - drag.x
      scene.y += event.clientY - drag.y
      drag.x = event.clientX
      drag.y = event.clientY
    }
    const rect = app.canvas.getBoundingClientRect()
    inspectTile(event.clientX - rect.left, event.clientY - rect.top)
  })
  function stopDrag(): void { drag = null; app.canvas.classList.remove('dragging') }
  for (const eventName of ['pointerup', 'pointercancel', 'lostpointercapture']) app.canvas.addEventListener(eventName, stopDrag)
  app.canvas.addEventListener('pointerleave', () => { highlight.clear(); render() })
  app.canvas.addEventListener('wheel', event => {
    event.preventDefault()
    if (!event.deltaY) return
    const index = ZOOM_LEVELS.indexOf(scene.scale.x)
    const next = Math.max(0, Math.min(ZOOM_LEVELS.length - 1, index + (event.deltaY < 0 ? 1 : -1)))
    const rect = app.canvas.getBoundingClientRect()
    setZoom(ZOOM_LEVELS[next]!, event.clientX - rect.left, event.clientY - rect.top)
  }, { passive: false })

  const observer = new ResizeObserver(() => {
    if (preview.hidden) return
    app.renderer.resize(preview.clientWidth, preview.clientHeight)
    centerScene()
    render()
  })
  observer.observe(preview)
  window.addEventListener('pagehide', () => {
    observer.disconnect()
    for (const chunk of chunks) chunk.destroy()
    app.destroy(true, { children: true })
  }, { once: true })
  updateNeighbors()
  scene.scale.set(Number(controls.zoom.value))
  rebuild()
  centerScene()
  render()
  element<HTMLFieldSetElement>('controls').disabled = false
}

void main().catch(reportError)
