import assert from 'node:assert/strict'
import { after, test } from 'node:test'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { createRequire as createTestRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { createRenderer, h, markRaw, nextTick, ref } from 'vue'
import {
  ECONOMY_RENDERING_STORAGE_KEY,
  loadEconomyRendering,
  persistEconomyRendering,
  useEconomyRendering,
} from '../src/composables/useEconomyRendering'
import { GameFacade } from '../src/game/GameFacade'
import { Render } from '../src/game/Render'
import { resolveRenderResolution } from '../src/game/renderResolution'
import { chunkCache } from '../src/game/cache/ChunkCache'

after(() => chunkCache.destroy())

function memoryStorage() {
  const values = new Map<string, string>()
  return {
    values,
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
  }
}

test('economy rendering is opt-in and accepts only a saved true value', () => {
  const storage = memoryStorage()
  assert.equal(loadEconomyRendering(storage), false)
  assert.equal(loadEconomyRendering(null), false)
  for (const malformed of ['false', '1', 'enabled', 'TRUE', ' true ', '{}', '']) {
    storage.values.set(ECONOMY_RENDERING_STORAGE_KEY, malformed)
    assert.equal(loadEconomyRendering(storage), false, malformed)
  }
  storage.values.set(ECONOMY_RENDERING_STORAGE_KEY, 'true')
  assert.equal(loadEconomyRendering(storage), true)
})

test('both preference states persist and blocked storage stays usable', () => {
  const storage = memoryStorage()
  persistEconomyRendering(true, storage)
  assert.equal(storage.values.get(ECONOMY_RENDERING_STORAGE_KEY), 'true')
  persistEconomyRendering(false, storage)
  assert.equal(storage.values.get(ECONOMY_RENDERING_STORAGE_KEY), 'false')
  const unavailable = {
    getItem() { throw new Error('Storage disabled') },
    setItem() { throw new Error('Storage full') },
  }
  assert.equal(loadEconomyRendering(unavailable), false)
  assert.doesNotThrow(() => persistEconomyRendering(true, unavailable))
  assert.doesNotThrow(() => persistEconomyRendering(false, null))
})

test('composable loads, updates and persists; even browser storage access may throw', context => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
  context.after(() => {
    if (previous) Object.defineProperty(globalThis, 'localStorage', previous)
    else Reflect.deleteProperty(globalThis, 'localStorage')
  })
  const storage = memoryStorage()
  storage.values.set(ECONOMY_RENDERING_STORAGE_KEY, 'true')
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: storage })
  const settings = useEconomyRendering()
  assert.equal(settings.enabled.value, true)
  settings.setEnabled(false)
  assert.equal(settings.enabled.value, false)
  assert.equal(storage.values.get(ECONOMY_RENDERING_STORAGE_KEY), 'false')
  settings.setEnabled(true)
  assert.equal(storage.values.get(ECONOMY_RENDERING_STORAGE_KEY), 'true')

  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    get() { throw new Error('Browser denies access') },
  })
  const blocked = useEconomyRendering()
  assert.equal(blocked.enabled.value, false)
  assert.doesNotThrow(() => blocked.setEnabled(true))
  assert.equal(blocked.enabled.value, true)
})

test('facade retains the preference before init, forwards live changes and retains it after destroy', () => {
  const facade = new GameFacade()
  const state = facade as unknown as {
    economyRendering: boolean
    render: { setEconomyRendering(enabled: boolean): void; destroy(): void } | null
  }
  assert.equal(state.economyRendering, false)
  facade.setEconomyRendering(true)
  assert.equal(state.economyRendering, true)
  const calls: (boolean | string)[] = []
  state.render = {
    setEconomyRendering: enabled => { calls.push(enabled) },
    destroy: () => { calls.push('destroy') },
  }
  facade.setEconomyRendering(false)
  facade.setEconomyRendering(true)
  facade.destroy()
  assert.deepEqual(calls, [false, true, 'destroy'])
  assert.equal(state.render, null)
  assert.equal(state.economyRendering, true)
  facade.setEconomyRendering(false)
  assert.equal(state.economyRendering, false)
})

test('economy density is one; normal density remains capped with safe invalid defaults', () => {
  for (const density of [0.75, 1, 1.5, 2, 3, Number.NaN, Number.POSITIVE_INFINITY, 0, -1]) {
    assert.equal(resolveRenderResolution(true, density), 1)
  }
  assert.equal(resolveRenderResolution(false, 0.75), 0.75)
  assert.equal(resolveRenderResolution(false, 1), 1)
  assert.equal(resolveRenderResolution(false, 1.5), 1.5)
  assert.equal(resolveRenderResolution(false, 3), 2)
  for (const invalid of [Number.NaN, Number.POSITIVE_INFINITY, 0, -1]) {
    assert.equal(resolveRenderResolution(false, invalid), 1)
  }
})

test('facade applies the preference before each renderer initialization', async context => {
  const initialPreferences: boolean[] = []
  context.mock.method(Render.prototype, 'init', async function(this: Render) {
    initialPreferences.push((this as unknown as { economyRendering: boolean }).economyRendering)
  })
  context.mock.method(Render.prototype, 'getApp', () => ({
    renderer: { events: { cursorStyles: {}, rootBoundary: { cursor: 'default' } } },
  }))
  context.mock.method(Render.prototype, 'destroy', () => {})
  const facade = new GameFacade()
  context.after(() => facade.destroy())
  const canvas = { style: {} } as HTMLCanvasElement
  facade.setEconomyRendering(true)
  await facade.init(canvas)
  facade.destroy()
  await facade.init(canvas)
  facade.setEconomyRendering(false)
  await facade.init(canvas)
  assert.deepEqual(initialPreferences, [true, true, false])
})

const clientRoot = process.cwd()
const temporaryRoot = join(clientRoot, 'node_modules', '.tmp')
await mkdir(temporaryRoot, { recursive: true })
const temporaryDirectory = await mkdtemp(join(temporaryRoot, 'economy-rendering-'))
after(() => rm(temporaryDirectory, { recursive: true, force: true }))

// Compile the real Settings SFC and exercise its events without a DOM dependency.
const requireModule = createTestRequire(import.meta.url)
const { compileScript, parse } = requireModule('@vue/compiler-sfc') as typeof import('@vue/compiler-sfc')
const { build } = requireModule('esbuild') as typeof import('esbuild')
const componentPath = join(clientRoot, 'src/components/ui/SettingsWindow.vue')
const { descriptor } = parse(await readFile(componentPath, 'utf8'), { filename: componentPath })
const script = compileScript(descriptor, { id: 'economy-settings-test', inlineTemplate: true })
const compiledPath = join(temporaryDirectory, 'SettingsWindow.mjs')
await build({
  stdin: { contents: script.content, sourcefile: componentPath, resolveDir: dirname(componentPath), loader: 'ts' },
  outfile: compiledPath,
  bundle: true,
  platform: 'node',
  format: 'esm',
  external: ['vue'],
  plugins: [{
    name: 'window-shell',
    setup(builder) {
      builder.onResolve({ filter: /GameWindow\.vue$/ }, () => ({ path: 'window-shell', namespace: 'window-shell' }))
      builder.onLoad({ filter: /.*/, namespace: 'window-shell' }, () => ({ contents: `
        import { defineComponent, h } from 'vue'
        export default defineComponent({
          inheritAttrs: false,
          setup(_, { slots }) { return () => h('section', slots.default?.()) },
        })
      ` }))
    },
  }],
})
const { default: SettingsWindow } = await import(pathToFileURL(compiledPath).href)

interface TestNode {
  type: string
  text: string
  props: Record<string, unknown>
  children: TestNode[]
  parent: TestNode | null
}
function createNode(type: string, text = ''): TestNode {
  return markRaw({ type, text, props: {}, children: [], parent: null })
}
const renderer = createRenderer<TestNode, TestNode>({
  createElement: type => createNode(type),
  createText: text => createNode('#text', text),
  createComment: text => createNode('#comment', text),
  setText: (node, text) => { node.text = text },
  setElementText: (node, text) => { node.text = text },
  parentNode: node => node.parent,
  nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
  patchProp: (node, key, _previous, value) => { node.props[key] = value },
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
function descendants(node: TestNode): TestNode[] { return [node, ...node.children.flatMap(descendants)] }

test('real Settings checkbox reflects state, emits both choices and keeps radio/debug controls', async () => {
  const economy = ref(false)
  const choices: boolean[] = []
  const container = createNode('root')
  const app = renderer.createApp({
    setup() {
      return () => h(SettingsWindow, {
        mode: 'hybrid3d',
        debugEnabled: false,
        economyRendering: economy.value,
        onUpdateEconomyRendering(enabled: boolean) { choices.push(enabled); economy.value = enabled },
      })
    },
  })
  app.mount(container)
  try {
    const label = descendants(container).find(node => node.type === 'label' &&
      descendants(node).some(child => child.text === 'Economy rendering'))
    assert.ok(label)
    const checkbox = descendants(label).find(node => node.type === 'input')
    assert.ok(checkbox)
    assert.equal(checkbox.props.type, 'checkbox')
    assert.equal(checkbox.props.checked, false)
    const change = checkbox.props.onChange as (event: { target: { checked: boolean } }) => void
    change({ target: { checked: true } })
    await nextTick()
    assert.equal(checkbox.props.checked, true)
    change({ target: { checked: false } })
    await nextTick()
    assert.equal(checkbox.props.checked, false)
    assert.deepEqual(choices, [true, false])
    assert.equal(descendants(container).filter(node => node.props.type === 'radio').length, 2)
    assert.equal(descendants(container).filter(node => node.props.type === 'checkbox').length, 2)
  } finally { app.unmount() }
})

test('GameView applies saved state before initialization and forwards Settings updates', async () => {
  const view = await readFile(join(clientRoot, 'src/views/GameView.vue'), 'utf8')
  assert.match(view, /gameFacade\.setEconomyRendering\(economyRendering\.value\)[\s\S]*?await ensureCanvasInitialized\(\)/)
  assert.match(view, /function setEconomyRendering\(enabled: boolean\): void\s*\{\s*saveEconomyRendering\(enabled\)\s*gameFacade\?\.setEconomyRendering\(enabled\)/)
  assert.match(view, /:economy-rendering="economyRendering"/)
  assert.match(view, /@update-economy-rendering="setEconomyRendering"/)
  const facade = await readFile(join(clientRoot, 'src/game/GameFacade.ts'), 'utf8')
  assert.match(facade, /this\.render = new Render\([^\n]*\)\s*this\.render\.setEconomyRendering\(this\.economyRendering\)[\s\S]*?await this\.render\.init\(canvas\)/)
  assert.match(descriptor.styles.map(style => style.content).join('\n'), /\.settings-window\s*\{[^}]*overflow-y:\s*auto;/s)
})
