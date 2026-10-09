import assert from 'node:assert/strict'
import { after, test } from 'node:test'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import { compileScript, parse } from '@vue/compiler-sfc'
import { createRenderer, markRaw } from 'vue'

const clientRoot = fileURLToPath(new URL('..', import.meta.url))
const temporaryRoot = join(clientRoot, 'node_modules', '.tmp')
await mkdir(temporaryRoot, { recursive: true })
const temporaryDirectory = await mkdtemp(join(temporaryRoot, 'inventory-item-'))
after(() => rm(temporaryDirectory, { recursive: true, force: true }))
const componentPath = join(clientRoot, 'src/components/ui/InventoryItem.vue')
const { descriptor } = parse(await readFile(componentPath, 'utf8'), { filename: componentPath })
const script = compileScript(descriptor, { id: 'inventory-item-test', inlineTemplate: true })
const compiledPath = join(temporaryDirectory, 'InventoryItem.mjs')
await build({
  stdin: { contents: script.content, sourcefile: componentPath, resolveDir: dirname(componentPath), loader: 'ts' },
  outfile: compiledPath, bundle: true, platform: 'node', format: 'esm', external: ['vue'],
  alias: { '@': join(clientRoot, 'src') },
})
const { default: InventoryItem } = await import(pathToFileURL(compiledPath).href)

function createNode(type, text = '') {
  return markRaw({
    type, text, textContent: '', props: {}, style: {}, children: [], parent: null,
    appendChild(node) { node.parent = this; this.children.push(node) },
    removeChild(node) { this.children.splice(this.children.indexOf(node), 1); node.parent = null },
    set innerHTML(value) { throw new Error(`Unsafe HTML insertion: ${value}`) },
  })
}
const renderer = createRenderer({
  createElement: createNode, createText: text => createNode('#text', text), createComment: text => createNode('#comment', text),
  setText: (node, text) => { node.text = text }, setElementText: (node, text) => { node.text = text },
  parentNode: node => node.parent, nextSibling: () => null,
  patchProp: (node, key, previous, value) => { node.props[key] = value },
  insert: (node, parent) => parent.appendChild(node),
  remove: node => node.parent?.removeChild(node),
})

for (const [label, ownerId] of [['main', 7], ['nested', 700]]) {
  test(`skull tooltip in ${label} inventory renders nickname as literal text and cleans up`, () => {
    const originalDocument = globalThis.document
    const body = createNode('body')
    globalThis.document = { createElement: createNode, body }
    const host = createNode('root')
    const nick = '<img src=x onerror=alert(1)>'
    const hintExt = `${nick} died on January 10, 2026\r\nRemembered forever`
    const app = renderer.createApp(InventoryItem, {
      item: { x: 0, y: 0, instance: { name: 'Skull', quality: 12, resource: 'items/skull.png', hintExt } },
      inventoryRef: { kind: 0, ownerId, key: 0 },
    })
    try {
      app.mount(host)
      const item = host.children[0]
      item.props.onMouseenter({ clientX: 20, clientY: 30 })
      assert.equal(body.children.length, 1)
      const tooltip = body.children[0]
      assert.equal(tooltip.className, 'item-tooltip-global')
      assert.equal(tooltip.children.length, 1)
      assert.equal(tooltip.children[0].type, 'pre')
      assert.equal(tooltip.children[0].textContent, `Skull, quality: 12\n${nick} died on January 10, 2026\nRemembered forever`)
      assert.equal(tooltip.style.left, '30px')
      assert.equal(tooltip.style.top, '40px')
      item.props.onMouseleave()
      assert.equal(body.children.length, 0)
      item.props.onMouseenter({ clientX: 20, clientY: 30 })
      app.unmount()
      assert.equal(body.children.length, 0)
    } finally {
      app.unmount()
      if (originalDocument === undefined) delete globalThis.document
      else globalThis.document = originalDocument
    }
  })
}

test('ordinary item without memorial metadata keeps its existing tooltip', () => {
  const originalDocument = globalThis.document
  const body = createNode('body')
  globalThis.document = { createElement: createNode, body }
  const host = createNode('root')
  const app = renderer.createApp(InventoryItem, {
    item: { x: 0, y: 0, instance: { name: 'Skull', quality: 0 } },
    inventoryRef: { kind: 0, ownerId: 7, key: 0 },
  })
  try {
    app.mount(host)
    host.children[0].props.onMouseenter({ clientX: 0, clientY: 0 })
    assert.equal(body.children[0].children[0].textContent, 'Skull, quality: 0')
  } finally {
    app.unmount()
    if (originalDocument === undefined) delete globalThis.document
    else globalThis.document = originalDocument
  }
})
