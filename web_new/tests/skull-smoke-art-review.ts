import { Application, Assets, Container, Sprite, TextureStyle, Ticker, type Texture, type Spritesheet } from 'pixi.js'
import { Chunk } from '../src/game/Chunk'
import { ObjectManager } from '../src/game/ObjectManager'
import { ResourceLoader, type ResourceDef } from '../src/game/ResourceLoader'
import { cullingController } from '../src/game/culling'
import { SKULL_SMOKE } from '../src/game/fx/assets/skullSmoke'
import { initTileSets } from '../src/game/tiles/tileSetLoader'
import { TILE_DIRT, TILE_GRASS } from '../src/game/tiles/tileIds'
import { setWorldParams } from './serverConstantsFixture'

const CHUNK_SIZE = 32
const CENTER_TILE = 16.5
const FRAME_COUNT = SKULL_SMOKE.frames.length
const FRAME_DURATION_MS = SKULL_SMOKE.frameDurationMs
const EFFECT_DURATION_MS = SKULL_SMOKE.lifetimeMs
const PAUSE_DURATION_MS = 1000
const CYCLE_DURATION_MS = EFFECT_DURATION_MS + PAUSE_DURATION_MS
const [FRAME_WIDTH, FRAME_HEIGHT] = SKULL_SMOKE.frameSize
const ANCHOR = SKULL_SMOKE.anchor
const BASE_OFFSET_Y = -8
const CAMPFIRE_PREVIEW = 'skull-art-review/campfire'
const status = document.querySelector<HTMLParagraphElement>('#status')!
const playback = document.querySelector<HTMLButtonElement>('#playback')!
const replay = document.querySelector<HTMLButtonElement>('#replay')!
const fire = document.querySelector<HTMLSelectElement>('#fire')!
const ground = document.querySelector<HTMLSelectElement>('#ground')!
const frame = document.querySelector<HTMLInputElement>('#frame')!
const frameNumber = document.querySelector<HTMLOutputElement>('#frame-number')!
const originalGetResourceDef = ResourceLoader.getResourceDef

interface Scene {
  app: Application
  host: HTMLElement
  world: Container
  chunk: Chunk
  objects: ObjectManager
  smoke: Sprite
  entityId: number
  scale: number
  observer: ResizeObserver
}

declare global {
  interface Window {
	skullSmokeArtReview?: {
	  ready: boolean
	  frameSize: readonly [number, number]
	  anchor: readonly [number, number]
	  baseOffsetY: number
	  frameDurationMs: number
	  effectDurationMs: number
	  pauseDurationMs: number
	  scales: readonly number[]
	  frame: number
	  phaseMs: number
	  visible: boolean
	  playing: boolean
	  fire: string
	  renderedFrames: number
	}
  }
}

const scenes: Scene[] = []
let playing = true
let disposed = false
let elapsedMs = 0
let lastTickMs = 0
let animationNowMs = 0
let manualFrame: number | null = null
let spritesheet: Spritesheet
let smokeTextures: Texture[] = []

function buildTiles(): Uint8Array {
  const tiles = new Uint8Array(CHUNK_SIZE * CHUNK_SIZE)
  tiles.fill(ground.value === 'dirt' ? TILE_DIRT : TILE_GRASS)
  if (ground.value !== 'dirt') {
	for (let y = 14; y <= 18; y++) {
	  for (let x = 14; x <= 18; x++) {
		if (Math.hypot(x - 16, y - 16) <= 1.5) tiles[y * CHUNK_SIZE + x] = TILE_DIRT
	  }
	}
  }
  return tiles
}

function spawn(scene: Scene): void {
  const isHearth = fire.value === 'hearth_fire'
  const size = isHearth ? 12 : 10
  scene.objects.spawnObject({
	entityId: scene.entityId,
	typeId: isHearth ? 36 : 16,
	resourcePath: fire.value === 'campfire' ? CAMPFIRE_PREVIEW : fire.value,
	position: { x: CENTER_TILE * 12, y: CENTER_TILE * 12 },
	size: { x: size, y: size },
  })
}

async function createScene(id: string, entityId: number, scale: number): Promise<Scene | undefined> {
  const host = document.getElementById(id)!
  const app = new Application()
  await app.init({
	width: host.clientWidth,
	height: host.clientHeight,
	resolution: 1,
	background: '#283c2a',
	antialias: false,
	preference: 'webgl',
  })
  app.stop()
  if (disposed) {
	app.destroy(true, { children: true })
	return undefined
  }
  host.append(app.canvas)
  const world = new Container()
  world.scale.set(scale)
  const chunk = new Chunk(0, 0)
  chunk.buildTiles(buildTiles(), spritesheet)
  const objectsContainer = new Container({ sortableChildren: true })
  const smoke = new Sprite(smokeTextures[0]!)
  smoke.anchor.set(ANCHOR[0] / FRAME_WIDTH, ANCHOR[1] / FRAME_HEIGHT)
  smoke.position.set(0, CENTER_TILE * 32 + BASE_OFFSET_Y)
  world.addChild(chunk.getContainer(), objectsContainer, smoke)
  app.stage.addChild(world)
  const objects = new ObjectManager()
  objects.setParentContainer(objectsContainer)
  const resize = () => {
	if (disposed) return
	app.renderer.resize(host.clientWidth, host.clientHeight)
	const groundY = host.clientHeight - (scale === 1 ? 57 : 60)
	world.position.set(host.clientWidth / 2, groundY - CENTER_TILE * 32 * scale)
	app.render()
  }
  const observer = new ResizeObserver(resize)
  const scene = { app, host, world, chunk, objects, smoke, entityId, scale, observer }
  spawn(scene)
  resize()
  observer.observe(host)
  return scene
}

function advanceClock(): void {
  const now = performance.now()
  if (playing) {
	elapsedMs = (elapsedMs + Math.max(0, now - lastTickMs)) % CYCLE_DURATION_MS
	animationNowMs = now
  }
  lastTickMs = now
}

function updatePlaybackControl(): void {
  playback.title = playing ? 'Pause' : 'Play'
  playback.setAttribute('aria-label', playback.title)
  playback.firstElementChild!.className = playing ? 'pause' : 'play'
}

function renderPreview(renderDriver = false): void {
  const visible = manualFrame !== null || elapsedMs < EFFECT_DURATION_MS
  const frameIndex = manualFrame ?? Math.min(FRAME_COUNT - 1, Math.floor(elapsedMs / FRAME_DURATION_MS))
  const driver = scenes[0]?.app
  for (const scene of scenes) {
	scene.smoke.texture = smokeTextures[frameIndex]!
	scene.smoke.visible = visible
	scene.objects.update(animationNowMs)
	if (renderDriver || scene.app !== driver) scene.app.render()
  }
  frame.value = String(frameIndex + 1)
  frameNumber.value = `${frameIndex + 1} / ${FRAME_COUNT}`
  if (window.skullSmokeArtReview) {
	Object.assign(window.skullSmokeArtReview, {
	  frame: frameIndex + 1,
	  phaseMs: elapsedMs,
	  visible,
	  playing,
	  fire: fire.value,
	})
	window.skullSmokeArtReview.renderedFrames++
  }
}

function cleanup(): void {
  if (disposed) return
  disposed = true
  playing = false
  ResourceLoader.getResourceDef = originalGetResourceDef
  Ticker.shared.stop()
  for (const scene of scenes) {
	scene.observer.disconnect()
	scene.app.stop()
	scene.objects.destroy()
	scene.chunk.destroy()
	scene.app.destroy(true, { children: true })
  }
  scenes.length = 0
  cullingController.clear()
  if (window.skullSmokeArtReview) window.skullSmokeArtReview.ready = false
}

async function main(): Promise<void> {
  TextureStyle.defaultOptions.scaleMode = 'nearest'
  setWorldParams(12, CHUNK_SIZE)
  initTileSets()
  Ticker.shared.stop()
  const burning = originalGetResourceDef.call(ResourceLoader, 'campfire/burning')
  const hearth = originalGetResourceDef.call(ResourceLoader, 'hearth_fire')
  const unlit = originalGetResourceDef.call(ResourceLoader, 'campfire/unlit')
  if (!burning || !hearth || !unlit) throw new Error('Fire resource is missing')
  const campfirePreview: ResourceDef = structuredClone(burning)
  delete campfirePreview.fx
  // Only this preview lookup suppresses the campfire's continuous smoke.
  ResourceLoader.getResourceDef = (path) => path === CAMPFIRE_PREVIEW
	? campfirePreview : originalGetResourceDef.call(ResourceLoader, path)
  smokeTextures = await Promise.all(SKULL_SMOKE.frames.map(path => ResourceLoader.loadTexture(path)))
  for (const texture of smokeTextures) {
	if (texture.width !== FRAME_WIDTH || texture.height !== FRAME_HEIGHT) {
	  throw new Error(`Skull Smoke frames must be ${FRAME_WIDTH} x ${FRAME_HEIGHT}`)
	}
  }
  await Promise.all([campfirePreview, hearth, unlit].flatMap((resource) => resource.layers).flatMap((layer) =>
	layer.frames?.map((item) => ResourceLoader.loadTexture(item.img)) ??
	(layer.img ? [ResourceLoader.loadTexture(layer.img)] : [])))
  spritesheet = await Assets.load<Spritesheet>('/assets/game/tiles.json')
  if (disposed) return
  for (const [id, scale] of [['game-scale', 1], ['detail', 3]] as const) {
	const scene = await createScene(id, scenes.length + 201, scale)
	if (!scene) return
	scenes.push(scene)
  }
  lastTickMs = performance.now()
  animationNowMs = lastTickMs
  window.skullSmokeArtReview = {
	ready: true,
	frameSize: [FRAME_WIDTH, FRAME_HEIGHT],
	anchor: ANCHOR,
	baseOffsetY: BASE_OFFSET_Y,
	frameDurationMs: FRAME_DURATION_MS,
	effectDurationMs: EFFECT_DURATION_MS,
	pauseDurationMs: PAUSE_DURATION_MS,
	scales: [1, 3],
	frame: 1,
	phaseMs: 0,
	visible: true,
	playing,
	fire: fire.value,
	renderedFrames: 0,
  }
  const driver = scenes[0]!.app
  driver.ticker.add(() => {
	if (!playing || disposed) return
	advanceClock()
	renderPreview()
  })
  playback.addEventListener('click', () => {
	advanceClock()
	playing = !playing
	manualFrame = null
	updatePlaybackControl()
	renderPreview(true)
	if (playing) driver.start()
	else driver.stop()
  })
  replay.addEventListener('click', () => {
	elapsedMs = 0
	manualFrame = null
	playing = true
	lastTickMs = performance.now()
	animationNowMs = lastTickMs
	updatePlaybackControl()
	renderPreview(true)
	driver.start()
  })
  frame.addEventListener('input', () => {
	playing = false
	driver.stop()
	manualFrame = Math.max(0, Math.min(FRAME_COUNT - 1, Math.round(Number(frame.value)) - 1))
	elapsedMs = manualFrame * FRAME_DURATION_MS
	updatePlaybackControl()
	renderPreview(true)
  })
  fire.addEventListener('change', () => {
	animationNowMs = performance.now()
	for (const scene of scenes) spawn(scene)
	for (const caption of document.querySelectorAll('.fire-name')) {
	  caption.textContent = fire.selectedOptions[0]!.textContent
	}
	renderPreview(true)
	requestAnimationFrame(() => {
	  if (!disposed) renderPreview(true)
	})
  })
  ground.addEventListener('change', () => {
	for (const scene of scenes) scene.chunk.buildTiles(buildTiles(), spritesheet)
	renderPreview(true)
  })
  for (const control of [playback, replay, fire, ground, frame]) control.disabled = false
  status.textContent = ''
  document.body.dataset.status = 'ready'
  renderPreview(true)
  driver.start()
}

window.addEventListener('pagehide', cleanup)

void main().catch((error: unknown) => {
  cleanup()
  status.textContent = error instanceof Error ? error.message : String(error)
  document.body.dataset.status = 'error'
  console.error('[SkullSmokeArtReview]', error)
})
