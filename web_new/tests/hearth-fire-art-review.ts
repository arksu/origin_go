import { Application, Assets, Container, TextureStyle, Ticker, type Spritesheet } from 'pixi.js'
import { Chunk } from '../src/game/Chunk'
import { ObjectManager } from '../src/game/ObjectManager'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { cullingController } from '../src/game/culling'
import { initTileSets } from '../src/game/tiles/tileSetLoader'
import { TILE_DIRT, TILE_GRASS } from '../src/game/tiles/tileIds'
import { setWorldParams } from './serverConstantsFixture'

const FRAME_ORDER = [1, 3, 2, 5, 4, 6, 7] as const
const CHUNK_SIZE = 32
const CENTER_TILE = 16.5
const RESOURCE_ORIGINAL = 'campfire/burning'
const RESOURCE_CANDIDATE = 'hearth_fire'
const status = document.querySelector<HTMLParagraphElement>('#status')!
const playback = document.querySelector<HTMLButtonElement>('#playback')!
const ground = document.querySelector<HTMLSelectElement>('#ground')!

interface Scene {
  app: Application
  host: HTMLElement
  world: Container
  chunk: Chunk
  objects: ObjectManager
  entityId: number
  resourcePath: string
  scale: number
  observer: ResizeObserver
}

declare global {
  interface Window {
	hearthArtReview?: {
	  ready: boolean
	  frameOrder: readonly number[]
	  fps: number
	  frameSize: readonly [number, number]
	  anchor: readonly [number, number]
	  scales: readonly number[]
	  renderedFrames: number
	}
  }
}

const scenes: Scene[] = []
let playing = true
let elapsedMs = 0
let animationEpoch = performance.now()
let spritesheet: Spritesheet

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
  const isHearth = scene.resourcePath === RESOURCE_CANDIDATE
  const size = isHearth ? 12 : 10
  scene.objects.spawnObject({
	entityId: scene.entityId,
	typeId: isHearth ? 36 : 16,
	resourcePath: scene.resourcePath,
	position: { x: CENTER_TILE * 12, y: CENTER_TILE * 12 },
	size: { x: size, y: size },
  })
}

async function createScene(id: string, entityId: number, resourcePath: string, scale: number): Promise<Scene> {
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
  host.append(app.canvas)
  const world = new Container()
  world.scale.set(scale)
  const chunk = new Chunk(0, 0)
  chunk.buildTiles(buildTiles(), spritesheet)
  const objectsContainer = new Container({ sortableChildren: true })
  world.addChild(chunk.getContainer(), objectsContainer)
  app.stage.addChild(world)
  const objects = new ObjectManager()
  objects.setParentContainer(objectsContainer)
  const resize = () => {
	app.renderer.resize(host.clientWidth, host.clientHeight)
	const groundY = host.clientHeight * (scale === 1 ? 0.65 : 0.79)
	world.position.set(host.clientWidth / 2, groundY - CENTER_TILE * 32 * scale)
	app.render()
  }
  const observer = new ResizeObserver(resize)
  const scene = { app, host, world, chunk, objects, entityId, resourcePath, scale, observer }
  spawn(scene)
  resize()
  observer.observe(host)
  return scene
}

async function main(): Promise<void> {
  TextureStyle.defaultOptions.scaleMode = 'nearest'
  setWorldParams(12, CHUNK_SIZE)
  initTileSets()
  Ticker.shared.stop()
  const base = ResourceLoader.getResourceDef(RESOURCE_ORIGINAL)
  if (!base) throw new Error('Campfire resource is missing')
  const hearth = ResourceLoader.getResourceDef(RESOURCE_CANDIDATE)
  const flame = hearth?.layers.find((layer) => layer.frames)
  if (!hearth || !flame?.frames) throw new Error('Hearth Fire resource is missing')
  const candidateTextures = await Promise.all(flame.frames.map((frame) => ResourceLoader.loadTexture(frame.img)))
  for (const texture of candidateTextures) {
	if (texture.width !== 96 || texture.height !== 96) throw new Error('Candidate flame frames must be 96 x 96')
  }
  await Promise.all([base, hearth].flatMap((resource) => resource.layers).flatMap((layer) => layer.frames?.map((frame) => ResourceLoader.loadTexture(frame.img)) ??
	(layer.img ? [ResourceLoader.loadTexture(layer.img)] : [])))
  spritesheet = await Assets.load<Spritesheet>('/assets/game/tiles.json')
  for (const [id, resourcePath, scale] of [
	['original', RESOURCE_ORIGINAL, 1],
	['candidate', RESOURCE_CANDIDATE, 1],
	['original-detail', RESOURCE_ORIGINAL, 4],
	['candidate-detail', RESOURCE_CANDIDATE, 4],
  ] as const) {
	scenes.push(await createScene(id, scenes.length + 101, resourcePath, scale))
  }
  animationEpoch = performance.now()
  window.hearthArtReview = {
	ready: true,
	frameOrder: FRAME_ORDER,
	fps: 4,
	frameSize: [96, 96],
	anchor: [48, 74],
	scales: [1, 4],
	renderedFrames: 0,
  }
  const driver = scenes[0]!.app
  driver.ticker.add((ticker) => {
	if (!playing) return
	elapsedMs += ticker.deltaMS
	for (const scene of scenes) {
	  scene.objects.update(animationEpoch + elapsedMs)
	  if (scene.app !== driver) scene.app.render()
	}
	window.hearthArtReview!.renderedFrames++
  })
  playback.addEventListener('click', () => {
	playing = !playing
	playback.title = playing ? 'Pause' : 'Play'
	playback.setAttribute('aria-label', playback.title)
	playback.firstElementChild!.className = playing ? 'pause' : 'play'
	if (playing) {
	  Ticker.shared.start()
	  driver.start()
	} else {
	  Ticker.shared.stop()
	  driver.stop()
	}
  })
  ground.addEventListener('change', () => {
	for (const scene of scenes) {
	  scene.chunk.buildTiles(buildTiles(), spritesheet)
	  scene.app.render()
	}
  })
  playback.disabled = false
  status.textContent = ''
  document.body.dataset.status = 'ready'
  Ticker.shared.start()
  driver.start()
}

window.addEventListener('pagehide', () => {
  playing = false
  Ticker.shared.stop()
  for (const scene of scenes) {
	scene.observer.disconnect()
	scene.objects.destroy()
	scene.chunk.destroy()
	scene.app.destroy(true, { children: true })
  }
  cullingController.clear()
})

void main().catch((error: unknown) => {
  status.textContent = error instanceof Error ? error.message : String(error)
  document.body.dataset.status = 'error'
  console.error('[HearthArtReview]', error)
})
