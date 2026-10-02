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
const temporaryDirectory = await mkdtemp(join(temporaryRoot, 'minimap-hud-'))
after(() => rm(temporaryDirectory, { recursive: true, force: true }))

const componentPath = join(clientRoot, 'src/components/ui/MinimapWindow.vue')
const { descriptor } = parse(await readFile(componentPath, 'utf8'), { filename: componentPath })
const script = compileScript(descriptor, { id: 'minimap-hud-test', inlineTemplate: true })
const compiledPath = join(temporaryDirectory, 'MinimapWindow.mjs')
await build({
  stdin: {
    contents: `${script.content}\nexport { calls } from '@/game'`,
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
  plugins: [{
    name: 'minimap-facade',
    setup(builder) {
      builder.onResolve({ filter: /^@\/game$/ }, () => ({ path: 'facade', namespace: 'minimap-test' }))
      builder.onLoad({ filter: /.*/, namespace: 'minimap-test' }, () => ({
        contents: `
          export const calls = []
          export const gameFacade = Object.fromEntries(
            ['attachMinimap', 'detachMinimap', 'setMinimapZoom'].map(
              name => [name, (...args) => calls.push({ name, args })],
            ),
          )
        `,
      }))
    },
  }],
})
const { default: MinimapWindow, calls } = await import(pathToFileURL(compiledPath).href)

function createNode(type, text = '') {
  return markRaw({ type, text, props: {}, children: [], parent: null })
}

// A Vue host exercises the real component without adding a browser DOM package.
const renderer = createRenderer({
  createElement: createNode,
  createText: text => createNode('#text', text),
  createComment: text => createNode('#comment', text),
  setText: (node, text) => { node.text = text },
  setElementText: (node, text) => { node.text = text },
  parentNode: node => node.parent,
  nextSibling: () => null,
  patchProp: (node, key, previous, value) => { node.props[key] = value },
  insert(node, parent) {
    node.parent = parent
    parent.children.push(node)
  },
  remove(node) {
    const parent = node.parent
    if (parent) parent.children.splice(parent.children.indexOf(node), 1)
    node.parent = null
  },
})

function descendants(node) {
  return [node, ...node.children.flatMap(descendants)]
}

function dispatch(node, eventName, details = {}) {
  const event = {
    ...details,
    stopped: false,
    defaultPrevented: false,
    stopPropagation() { this.stopped = true },
    preventDefault() { this.defaultPrevented = true },
  }
  for (let target = node; target; target = target.parent) {
    target.props[eventName]?.(event)
    if (event.stopped) break
  }
  return event
}

test('minimap attaches once, zooms locally, isolates input, and detaches its canvas', async () => {
  const container = createNode('root')
  const app = renderer.createApp(MinimapWindow)
  app.mount(container)
  try {
    const nodes = descendants(container)
    const panel = nodes.find(node => node.type === 'section')
    const canvas = nodes.find(node => node.type === 'canvas')
    const zoomOut = nodes.find(node => node.props['aria-label'] === 'Zoom out minimap')
    const zoomIn = nodes.find(node => node.props['aria-label'] === 'Zoom in minimap')
    const scale = nodes.find(node => node.type === 'output')
    assert.equal(panel.props['aria-label'], 'Minimap')
    assert.equal(canvas.props.role, 'img')
    assert.equal(calls[0].name, 'attachMinimap')
    assert.equal(calls[0].args[0], canvas)
    assert.deepEqual(calls[1], { name: 'setMinimapZoom', args: [1] })

    for (const expectedZoom of [2, 4, 4]) {
      assert.equal(dispatch(zoomIn, 'onClick').stopped, true)
      await nextTick()
      assert.equal(scale.text, `${expectedZoom}×`)
    }
    assert.equal(zoomIn.props.disabled, true)
    for (const expectedZoom of [2, 1, 0.5, 0.5]) {
      const event = dispatch(canvas, 'onWheel', { deltaY: 1 })
      assert.equal(event.stopped, true)
      assert.equal(event.defaultPrevented, true)
      await nextTick()
      assert.equal(scale.text, `${expectedZoom}×`)
    }
    assert.equal(zoomOut.props.disabled, true)
    const callsBeforeHorizontalWheel = calls.length
    dispatch(canvas, 'onWheel', { deltaY: 0, deltaX: 1 })
    assert.equal(calls.length, callsBeforeHorizontalWheel)
    dispatch(canvas, 'onWheel', { deltaY: -1 })
    await nextTick()
    assert.equal(scale.text, '1×')

    for (const name of ['onPointerdown', 'onPointermove', 'onPointerup', 'onPointercancel',
      'onMousedown', 'onMouseup', 'onClick', 'onKeydown']) {
      const event = dispatch(canvas, name)
      assert.equal(event.stopped, true, name)
    }
    for (const name of ['onDblclick', 'onAuxclick', 'onContextmenu']) {
      const event = dispatch(canvas, name)
      assert.equal(event.stopped, true, name)
      assert.equal(event.defaultPrevented, true, name)
    }
    // A movement key pressed before zoom gained focus must still reach its release handler.
    assert.equal(dispatch(zoomIn, 'onKeyup', { code: 'KeyW' }).stopped, false)
    assert.equal(calls.filter(call => call.name === 'attachMinimap').length, 1)
  } finally {
    app.unmount()
  }
  assert.equal(calls.at(-1).name, 'detachMinimap')
  assert.equal(calls.at(-1).args[0], calls[0].args[0])
})
