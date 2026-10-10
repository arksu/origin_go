import { MOVEMENT_DIRECTIONS, ResourceLoader } from '../src/game/ResourceLoader'

const DIRECTIONS = MOVEMENT_DIRECTIONS
const FRAME_COUNT = 8
const FRAME_SIZE = 64
const ANCHOR_X = 32
const ANCHOR_Y = 48

interface ReviewCard {
  direction: string
  frames: Array<HTMLImageElement | undefined>
  idle?: HTMLImageElement
  walkCanvases: HTMLCanvasElement[]
  idleCanvases: HTMLCanvasElement[]
  reference: HTMLElement
  status: HTMLElement
}

const cardsElement = document.querySelector<HTMLElement>('#cards')!
const overviewCanvas = document.querySelector<HTMLCanvasElement>('#overview')!
const statusElement = document.querySelector<HTMLElement>('#status')!
const playButton = document.querySelector<HTMLButtonElement>('#play')!
const fpsInput = document.querySelector<HTMLInputElement>('#fps')!
const fpsOutput = document.querySelector<HTMLOutputElement>('#fps-value')!
const frameInput = document.querySelector<HTMLInputElement>('#frame')!
const frameOutput = document.querySelector<HTMLOutputElement>('#frame-value')!
const sizeInput = document.querySelector<HTMLSelectElement>('#size')!
const idleInput = document.querySelector<HTMLInputElement>('#idle')!
const anchorsInput = document.querySelector<HTMLInputElement>('#anchors')!
const errors: string[] = []
let playing = true
let frameIndex = 0
let fps = 8
let lastTime: number | undefined
let elapsed = 0

function addViews(parent: HTMLElement, label: string, direction: string): HTMLCanvasElement[] {
  const views = document.createElement('div')
  views.className = 'views'
  parent.append(views)
  return [1, 4].map(scale => {
    const figure = document.createElement('figure')
    figure.className = scale === 1 ? 'view-native' : 'view-enlarged'
    const caption = document.createElement('figcaption')
    caption.textContent = `${label} · ${scale === 1 ? 'Native' : '4×'}`
    const canvas = document.createElement('canvas')
    canvas.width = FRAME_SIZE
    canvas.height = FRAME_SIZE
    canvas.style.width = `${FRAME_SIZE * scale}px`
    canvas.style.height = `${FRAME_SIZE * scale}px`
    canvas.setAttribute('role', 'img')
    canvas.setAttribute('aria-label', `${direction} ${label.toLowerCase()} at ${scale}×`)
    figure.append(caption, canvas)
    views.append(figure)
    return canvas
  })
}

const cards: ReviewCard[] = DIRECTIONS.map((direction, index) => {
  const article = document.createElement('article')
  article.className = 'card'
  const heading = document.createElement('h2')
  heading.textContent = `${index} · ${direction}`
  const status = document.createElement('p')
  status.className = 'frame-status'
  article.append(heading, status)
  const walkCanvases = addViews(article, 'Walk', direction)
  const reference = document.createElement('section')
  reference.className = 'reference'
  reference.hidden = true
  const idleCanvases = addViews(reference, 'Idle reference', direction)
  article.append(reference)
  cardsElement.append(article)
  return {
    direction,
    frames: Array.from({ length: FRAME_COUNT }, () => undefined),
    walkCanvases,
    idleCanvases,
    reference,
    status,
  }
})

function draw(canvas: HTMLCanvasElement, image: HTMLImageElement | undefined): void {
  const context = canvas.getContext('2d')!
  context.imageSmoothingEnabled = false
  context.clearRect(0, 0, FRAME_SIZE, FRAME_SIZE)
  context.fillStyle = '#334132'
  context.fillRect(0, 0, FRAME_SIZE, FRAME_SIZE)
  if (image) context.drawImage(image, 0, 0)
  if (anchorsInput.checked) {
    context.strokeStyle = '#d56a55'
    context.lineWidth = 1
    context.beginPath()
    context.moveTo(ANCHOR_X - 3 + 0.5, ANCHOR_Y + 0.5)
    context.lineTo(ANCHOR_X + 3 + 0.5, ANCHOR_Y + 0.5)
    context.moveTo(ANCHOR_X + 0.5, ANCHOR_Y - 3 + 0.5)
    context.lineTo(ANCHOR_X + 0.5, ANCHOR_Y + 3 + 0.5)
    context.stroke()
  }
}

function render(): void {
  const frameLabel = String(frameIndex).padStart(2, '0')
  frameInput.value = String(frameIndex)
  frameOutput.value = `${frameLabel} / 07`
  playButton.textContent = playing ? 'Pause' : 'Play'
  drawOverview()
  for (const card of cards) {
    const image = card.frames[frameIndex]
    const loaded = card.frames.filter(Boolean).length
    card.status.textContent = image
      ? `Frame ${frameLabel} · ${loaded}/8 frames loaded`
      : `Frame ${frameLabel} missing · ${loaded}/8 frames loaded`
    for (const canvas of card.walkCanvases) draw(canvas, image)
    card.reference.hidden = !idleInput.checked
    if (idleInput.checked) {
      for (const canvas of card.idleCanvases) draw(canvas, card.idle)
    }
  }
}

function drawOverview(): void {
  const context = overviewCanvas.getContext('2d')!
  const scale = 4
  const cellSize = FRAME_SIZE * scale
  context.imageSmoothingEnabled = false
  context.fillStyle = '#334132'
  context.fillRect(0, 0, overviewCanvas.width, overviewCanvas.height)
  for (const [index, card] of cards.entries()) {
    const x = index % 4 * cellSize
    const y = Math.floor(index / 4) * cellSize
    const image = card.frames[frameIndex]
    if (image) context.drawImage(image, x, y, cellSize, cellSize)
    context.strokeStyle = '#52634d'
    context.lineWidth = 1
    context.strokeRect(x + 0.5, y + 0.5, cellSize - 1, cellSize - 1)
    context.fillStyle = '#d8e3cc'
    context.font = '16px monospace'
    context.fillText(`${index} · ${card.direction}`, x + 12, y + 24)
    if (!image) {
      context.fillStyle = '#b8c5ad'
      context.font = '12px monospace'
      context.fillText('Frame not loaded', x + 12, y + 44)
    }
    if (anchorsInput.checked) {
      const anchorX = x + ANCHOR_X * scale + 0.5
      const anchorY = y + ANCHOR_Y * scale + 0.5
      context.strokeStyle = '#d56a55'
      context.beginPath()
      context.moveTo(anchorX - 8, anchorY)
      context.lineTo(anchorX + 8, anchorY)
      context.moveTo(anchorX, anchorY - 8)
      context.lineTo(anchorX, anchorY + 8)
      context.stroke()
    }
  }
}

function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => {
      if (image.naturalWidth !== FRAME_SIZE || image.naturalHeight !== FRAME_SIZE) {
        reject(new Error(`Expected 64×64 image: ${url}`))
        return
      }
      resolve(image)
    }
    image.onerror = () => reject(new Error(`Could not load image: ${url}`))
    image.src = url
  })
}

function publishedFrameUrls(directionIndex: number): string[] {
  const resourcePath = `rabbit/walk/${directionIndex}`
  const resource = ResourceLoader.getResourceDef(resourcePath)
  if (!resource) throw new Error(`Missing published resource: ${resourcePath}`)
  if (resource.size?.[0] !== FRAME_SIZE || resource.size?.[1] !== FRAME_SIZE ||
      resource.offset?.[0] !== ANCHOR_X || resource.offset?.[1] !== ANCHOR_Y) {
    throw new Error(`Invalid published canvas or ground origin: ${resourcePath}`)
  }
  const layer = resource.layers[0]
  if (resource.layers.length !== 1 || !layer || layer.fps !== 8 ||
      layer.loop !== true || layer.frames?.length !== FRAME_COUNT) {
    throw new Error(`Expected one looping eight-frame layer at 8 FPS: ${resourcePath}`)
  }
  return layer.frames.map((frame, index) => {
    const expectedPath = `animals/rabbit/walk/${directionIndex}/${String(index).padStart(2, '0')}.png`
    const position = ResourceLoader.resolveLayerPosition(layer, resource, frame.offset)
    if (frame.img !== expectedPath || position.x !== -ANCHOR_X || position.y !== -ANCHOR_Y) {
      throw new Error(`Invalid published frame path or placement: ${resourcePath}, frame ${index}`)
    }
    return `/assets/game/${frame.img}`
  })
}

function publishedIdleUrl(directionIndex: number): string {
  const resourcePath = `rabbit/idle/${directionIndex}`
  const resource = ResourceLoader.getResourceDef(resourcePath)
  const image = resource?.layers[0]?.img
  if (!image) throw new Error(`Missing published idle reference: ${resourcePath}`)
  return `/assets/game/${image}`
}

async function loadPublishedFrames(): Promise<void> {
  const loads: Promise<void>[] = []
  for (const [directionIndex, card] of cards.entries()) {
    try {
      for (const [index, url] of publishedFrameUrls(directionIndex).entries()) {
        loads.push(loadImage(url).then(image => {
          card.frames[index] = image
        }).catch(error => {
          errors.push(String(error))
        }))
      }
      loads.push(loadImage(publishedIdleUrl(directionIndex)).then(image => {
        card.idle = image
      }).catch(error => {
        errors.push(String(error))
      }))
    } catch (error) {
      errors.push(String(error))
    }
  }
  await Promise.all(loads)
  const loaded = cards.reduce((total, card) => total + card.frames.filter(Boolean).length, 0)
  statusElement.textContent = loaded === DIRECTIONS.length * FRAME_COUNT
    ? 'All 64 approved, published frames loaded from rabbit/walk/0–7. Catalog validated: 8 FPS, loop enabled, 64×64 canvas, ground origin (32, 48).'
    : `${loaded}/64 published frames loaded. Check the registered resources and published assets.`
  if (errors.length) statusElement.textContent += `\n${errors.join('\n')}`
  render()
}

playButton.addEventListener('click', () => {
  playing = !playing
  elapsed = 0
  render()
})
fpsInput.addEventListener('input', () => {
  fps = Number(fpsInput.value)
  fpsOutput.value = String(fps)
  elapsed = 0
})
frameInput.addEventListener('input', () => {
  frameIndex = Number(frameInput.value)
  playing = false
  elapsed = 0
  render()
})
sizeInput.addEventListener('change', () => {
  cardsElement.dataset.size = sizeInput.value
})
idleInput.addEventListener('change', render)
anchorsInput.addEventListener('change', render)

function animate(now: number): void {
  if (lastTime !== undefined && playing) {
    elapsed += Math.min(now - lastTime, 250)
    const frameDuration = 1000 / fps
    const steps = Math.floor(elapsed / frameDuration)
    if (steps > 0) {
      frameIndex = (frameIndex + steps) % FRAME_COUNT
      elapsed %= frameDuration
      render()
    }
  }
  lastTime = now
  requestAnimationFrame(animate)
}

render()
void loadPublishedFrames()
requestAnimationFrame(animate)
