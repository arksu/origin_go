import assert from 'node:assert/strict'
import { after, test } from 'node:test'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import { compileScript, parse } from '@vue/compiler-sfc'
import { createRenderer, markRaw, nextTick } from 'vue'

const clientRoot = fileURLToPath(new URL('..', import.meta.url))
const temporaryRoot = join(clientRoot, 'node_modules', '.tmp')
await mkdir(temporaryRoot, { recursive: true })
const temporaryDirectory = await mkdtemp(join(temporaryRoot, 'daytime-hud-'))
after(() => rm(temporaryDirectory, { recursive: true, force: true }))

const componentPath = join(clientRoot, 'src/components/ui/DayTime.vue')
const { descriptor } = parse(await readFile(componentPath, 'utf8'), { filename: componentPath })
const script = compileScript(descriptor, { id: 'daytime-hud-test', inlineTemplate: true })
const compiledPath = join(temporaryDirectory, 'DayTime.mjs')
// Stub clocks in the component/helper bundle; Vue's own development measurements
// may read Date.now, so the external test renderer keeps its normal environment.
const timeApis = ['Date.now', 'performance.now', 'setTimeout', 'setInterval', 'clearTimeout', 'clearInterval',
  'requestAnimationFrame', 'cancelAnimationFrame']
const clockApis = [...timeApis, ...timeApis.map(api => `globalThis.${api}`), ...timeApis.map(api => `window.${api}`)]
await build({
  stdin: {
    contents: `${script.content}\nexport { gameCalendarSync } from '@/network/GameCalendarSync'\nexport { serverProfile } from ${JSON.stringify(join(clientRoot, 'tests/serverConstantsFixture'))}`,
    sourcefile: componentPath,
    resolveDir: dirname(componentPath),
    loader: 'ts',
  },
  outfile: compiledPath,
  bundle: true,
  platform: 'node',
  format: 'esm',
  external: ['vue'],
  alias: { '@': join(clientRoot, 'src') },
  loader: { '.png': 'file' },
  define: Object.fromEntries(clockApis.map((api, index) => [api, `__daytimeForbiddenClock${index}`])),
  banner: { js: `
    export const clockCalls = []
    ${clockApis.map((api, index) => `const __daytimeForbiddenClock${index} = () => {
      clockCalls.push(${JSON.stringify(api)})
      throw new Error(${JSON.stringify(`DayTime must not call ${api}`)})
    }`).join('\n')}
    export const clockProbe = __daytimeForbiddenClock0
  ` },
})
const { default: DayTime, gameCalendarSync, serverProfile, clockCalls, clockProbe } = await import(pathToFileURL(compiledPath).href)

function createNode(type, text = '') {
  return markRaw({ type, text, props: {}, children: [], parent: null })
}

// Exercise the real SFC and calendar singleton without adding a DOM dependency.
const renderer = createRenderer({
  createElement: createNode,
  createText: text => createNode('#text', text),
  createComment: text => createNode('#comment', text),
  setText: (node, text) => { node.text = text },
  setElementText: (node, text) => { node.text = text },
  parentNode: node => node.parent,
  nextSibling(node) { return node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null },
  patchProp: (node, key, previous, value) => { node.props[key] = value },
  insert(node, parent, anchor = null) {
    if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1)
    node.parent = parent
    const index = anchor ? parent.children.indexOf(anchor) : -1
    parent.children.splice(index < 0 ? parent.children.length : index, 0, node)
  },
  remove(node) {
    if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1)
    node.parent = null
  },
})

function descendants(node) { return [node, ...node.children.flatMap(descendants)] }

function mount() {
  gameCalendarSync.reset()
  const container = createNode('root')
  const app = renderer.createApp(DayTime)
  app.mount(container)
  return {
    app,
    get root() { return container.children[0] },
    images: () => descendants(container).filter(node => node.type === 'img'),
    time: () => descendants(container).find(node => node.props.class === 'daytime__time').text,
    cleanup() { app.unmount(); gameCalendarSync.reset() },
  }
}

function closeTo(actual, expected) { assert.ok(Math.abs(actual - expected) < 1e-10, `${actual} != ${expected}`) }

test('placeholder and the same HUD component survive synchronization, reset, and reconnect', async () => {
  const view = mount()
  const root = view.root
  try {
    assert.equal(root.props.role, 'img')
    assert.equal(root.props['aria-label'], 'Game time unavailable')
    assert.equal(view.time(), '--:--')
    assert.equal(view.images().length, 0, 'no graphical layer invents an unsynchronized midnight')

    assert.equal(gameCalendarSync.acceptSample(364686, 1000), true)
    await nextTick()
    assert.equal(view.time(), '--:--', 'received runtime still requires server constants')
    gameCalendarSync.configure(serverProfile)
    await nextTick()
    assert.equal(view.time(), '15:54')
    assert.equal(root.props['aria-label'], 'Game time: 15:54')
    assert.equal(view.images().length, 5)
    assert.strictEqual(view.root, root)

    gameCalendarSync.reset()
    await nextTick()
    assert.strictEqual(view.root, root)
    assert.equal(view.time(), '--:--')
    assert.equal(view.images().length, 0)
    gameCalendarSync.configure(serverProfile)
    assert.equal(gameCalendarSync.acceptSample(0, 2000), true)
    await nextTick()
    assert.equal(view.time(), '00:00', 'a new connection may report lower runtime')
    assert.strictEqual(view.root, root)
  } finally { view.cleanup() }
})

test('original layers, sun geometry and night fades follow accepted server phases', async () => {
  const view = mount()
  try {
    gameCalendarSync.configure(serverProfile)
    const phases = [
      [0, '00:00', 1, 37.45846870965144, 30.56939460828641],
      [5, '05:00', 1, 37.45846870965144, 30.56939460828641],
      [5.5, '05:30', .5, 37.079321542835736, 27.908533222997693],
      [6, '06:00', 0],
      [13, '13:00', 0, 62.29616858287704, 3.114904198605405],
      [21, '21:00', 0, 81.18440286206636, 34.95662187309895],
      [21.5, '21:30', .5, 81.18440286206636, 34.95662187309895],
      [22, '22:00', 1, 81.18440286206636, 34.95662187309895],
      [23, '23:00', 1, 81.18440286206636, 34.95662187309895],
    ]
    for (const [hour, text, opacity, left, top] of phases) {
      assert.equal(gameCalendarSync.acceptSample(hour * 1200, 1000 + hour * 1000), true)
      await nextTick()
      const images = view.images()
      assert.equal(view.time(), text)
      assert.deepEqual(images.map(node => node.props.src.match(/(daysky|nightsky|sun|dayscape|nightscape)-/)[1]),
        ['daysky', 'nightsky', 'sun', 'dayscape', 'nightscape'])
      closeTo(images[1].props.style.opacity, opacity)
      closeTo(images[4].props.style.opacity, opacity)
      if (left !== undefined) {
        closeTo(parseFloat(images[2].props.style.left), left)
        closeTo(parseFloat(images[2].props.style.top), top)
      }
      for (const image of images) {
        assert.equal(image.props.alt, '')
        assert.equal(image.props['aria-hidden'], 'true')
        assert.equal(image.props.draggable, 'false')
        assert.equal(image.props.width, image === images[2] ? '17' : '134')
        assert.equal(image.props.height, image === images[2] ? '17' : '71')
      }
    }
  } finally { view.cleanup() }
})

test('fractional day phase changes the artwork between samples within the same hour', async () => {
  const view = mount()
  try {
    gameCalendarSync.configure(serverProfile)
    gameCalendarSync.acceptSample(6300, 1000)
    await nextTick()
    const first = view.images()[2].props.style
    assert.equal(view.time(), '05:15')
    closeTo(parseFloat(first.left), 37.229993969925204)
    closeTo(view.images()[1].props.style.opacity, .75)
    gameCalendarSync.acceptSample(6600, 2000)
    await nextTick()
    assert.equal(view.time(), '05:30')
    assert.notEqual(view.images()[2].props.style.left, first.left)
    assert.notEqual(view.images()[2].props.style.top, first.top)
    closeTo(view.images()[1].props.style.opacity, .5)
  } finally { view.cleanup() }
})

test('mount, updates and unmount use no local clock, timer or animation frame', async () => {
  assert.throws(clockProbe, /DayTime must not call Date.now/, 'the clock trap is active')
  clockCalls.length = 0
  let view
  try {
    view = mount()
    gameCalendarSync.configure(serverProfile)
    assert.equal(gameCalendarSync.acceptSample(28799, 1000), true)
    await nextTick()
    assert.equal(view.time(), '23:59')
    const snapshot = gameCalendarSync.getCalendar()
    const sunStyle = { ...view.images()[2].props.style }
    await nextTick()
    assert.strictEqual(gameCalendarSync.getCalendar(), snapshot)
    assert.deepEqual(view.images()[2].props.style, sunStyle)
    assert.equal(gameCalendarSync.acceptSample(28799, 1000), false)
    assert.equal(gameCalendarSync.acceptSample(28798, 2000), false)
    assert.equal(gameCalendarSync.acceptSample(undefined, 3000), false)
    await nextTick()
    assert.equal(view.time(), '23:59')
    assert.equal(gameCalendarSync.acceptSample(28800, 4000), true)
    await nextTick()
    assert.equal(view.time(), '00:00', 'midnight changes only with the next accepted sample')
    view.cleanup()
    view = null
    assert.deepEqual(clockCalls, [])
  } finally {
    view?.cleanup()
  }
})

test('HUD layout reserves the full artwork and clock height without animated styles', async () => {
  const styles = descriptor.styles.map(style => style.content).join('\n')
  assert.match(styles, /\.daytime\s*\{[^}]*width:\s*134px;[^}]*height:\s*80px;[^}]*pointer-events:\s*none;/s)
  assert.match(styles, /\.daytime__layer\s*\{[^}]*position:\s*absolute;[^}]*pointer-events:\s*none;/s)
  assert.match(styles, /\.daytime__time\s*\{[^}]*top:\s*66px;[^}]*color:\s*#3db67f;[^}]*font-size:\s*12px;[^}]*line-height:\s*14px;/s)
  assert.doesNotMatch(styles, /\b(?:animation(?:-[\w-]+)?|transition(?:-[\w-]+)?)\s*:|@keyframes/)
  const gameView = await readFile(join(clientRoot, 'src/views/GameView.vue'), 'utf8')
  assert.match(gameView, /<div class="hud-top-hotbar">\s*<HotbarPlaceholder[\s\S]*?\/>(\s*)<DayTime\s*\/>\s*<\/div>/)
  assert.match(gameView, /\.hud-top-hotbar\s*\{[^}]*top:\s*calc\(8px \+ env\(safe-area-inset-top\)\);[^}]*flex-direction:\s*column;[^}]*align-items:\s*center;[^}]*gap:\s*8px;/s)
})
