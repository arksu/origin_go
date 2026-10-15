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
const dragFixtureSource = `
  import { ref } from 'vue'
  const activeDrag = ref(null)
  export const useInventoryDrag = () => ({
    activeDrag,
    beginGridDrag: (...args) => globalThis.inventoryDragFixture?.calls.push(['grid', ...args]),
    beginBuildDrag: (...args) => globalThis.inventoryDragFixture?.calls.push(['build', ...args]),
    cancelSourceDrag: (...args) => globalThis.inventoryDragFixture?.calls.push(['cancel', ...args]),
    consumeClick: () => !!globalThis.inventoryDragFixture?.consume,
  })
`
await build({
  stdin: { contents: script.content, sourcefile: componentPath, resolveDir: dirname(componentPath), loader: 'ts' },
  outfile: compiledPath, bundle: true, platform: 'node', format: 'esm', external: ['vue'],
  alias: { '@': join(clientRoot, 'src') },
  plugins: [{
    name: 'inventory-drag-fixture',
    setup(build) {
      build.onResolve({ filter: /^@\/composables\/useInventoryDrag$/ }, () => ({ path: 'drag', namespace: 'inventory-drag-fixture' }))
      build.onLoad({ filter: /.*/, namespace: 'inventory-drag-fixture' }, () => ({ contents: dragFixtureSource }))
    },
  }],
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

for (const shiftKey of [false, true]) {
  for (const targetType of ['img', 'span']) {
    test(`item click on ${targetType} preserves offsets and shift=${shiftKey}`, () => {
      const host = createNode('root')
      const clickedItem = { x: 2, y: 3, instance: { itemId: 19, quality: 12 } }
      const clicks = []
      const app = renderer.createApp(InventoryItem, {
        item: clickedItem,
        inventoryRef: { kind: 0, ownerId: 7, inventoryKey: 0 },
        onItemClick: (...args) => clicks.push(args),
      })
      try {
        app.mount(host)
        const container = host.children[0]
        const target = container.children.find(node => node.type === targetType)
        assert.ok(target)
        container.getBoundingClientRect = () => ({ left: 10, top: 20, width: 64, height: 32 })
        container.props.onClick({
          currentTarget: container, target, clientX: 25.8, clientY: 27.2,
          offsetX: 1, offsetY: 2, shiftKey, preventDefault() {},
        })
        assert.deepEqual(clicks, [[clickedItem, 15, 7, shiftKey]])
      } finally {
        app.unmount()
      }
    })
  }
}

test('item click without a current target preserves shift and event offsets', () => {
  const host = createNode('root')
  const clickedItem = { x: 0, y: 0, instance: { itemId: 19 } }
  const clicks = []
  const app = renderer.createApp(InventoryItem, {
    item: clickedItem,
    inventoryRef: { kind: 0, ownerId: 7, inventoryKey: 0 },
    onItemClick: (...args) => clicks.push(args),
  })
  try {
    app.mount(host)
    host.children[0].props.onClick({ currentTarget: null, offsetX: 5, offsetY: 6, shiftKey: true, preventDefault() {} })
    assert.deepEqual(clicks, [[clickedItem, 5, 6, true]])
  } finally {
    app.unmount()
  }
})

test('grid item arms pointer drag and suppresses the finishing click', () => {
  const originalFixture = globalThis.inventoryDragFixture
  const fixture = { calls: [], consume: true }
  globalThis.inventoryDragFixture = fixture
  const host = createNode('root')
  const item = { x: 0, y: 0, instance: { itemId: 19 } }
  const inventoryRef = { kind: 0, ownerId: 7, inventoryKey: 0 }
  const clicks = []
  const app = renderer.createApp(InventoryItem, { item, inventoryRef, onItemClick: (...args) => clicks.push(args) })
  try {
    app.mount(host)
    const container = host.children[0]
    const event = { pointerId: 1, stopPropagation() { assert.fail('initial pointerdown must reach the outside-panel listener') } }
    container.props.onPointerdown(event)
    assert.deepEqual(fixture.calls, [['grid', event, inventoryRef, item]])
    container.props.onClick({ preventDefault() {} })
    assert.deepEqual(clicks, [])
    app.unmount()
    assert.deepEqual(fixture.calls.at(-1), ['cancel', inventoryRef, 19])
  } finally {
    app.unmount()
    if (originalFixture === undefined) delete globalThis.inventoryDragFixture
    else globalThis.inventoryDragFixture = originalFixture
  }
})

// Compile the real window templates while replacing external controls and I/O.
// Invoking the rendered item's listener verifies both windows route Shift-click
// independently from the hand's ordinary pickup/swap behavior.
for (const name of ['InventoryWindow', 'NestedInventoryWindow', 'BuildStateWindow']) {
  const windowPath = join(clientRoot, `src/components/ui/${name}.vue`)
  const { descriptor } = parse(await readFile(windowPath, 'utf8'), { filename: windowPath })
  const script = compileScript(descriptor, { id: `${name}-test`, inlineTemplate: true })
  const outputPath = join(temporaryDirectory, `${name}.mjs`)
  await build({
    stdin: { contents: script.content, sourcefile: windowPath, resolveDir: dirname(windowPath), loader: 'ts' },
    outfile: outputPath, bundle: true, platform: 'node', format: 'esm', external: ['vue'],
    alias: { '@': join(clientRoot, 'src') },
    plugins: [{
      name: 'inventory-window-fixtures',
      setup(build) {
        build.onResolve({ filter: /^(@\/stores\/gameStore|@\/composables\/useInventoryOps|@\/composables\/useInventoryDrag|@\/network|\.\/.*\.vue)$/ }, args => ({
          path: args.path, namespace: 'inventory-window-fixture',
        }))
        build.onLoad({ filter: /.*/, namespace: 'inventory-window-fixture' }, args => {
          if (args.path === '@/stores/gameStore') return { contents: 'export const useGameStore = () => globalThis.inventoryWindowFixture.store' }
          if (args.path === '@/composables/useInventoryOps') return { contents: 'export const useInventoryOps = () => globalThis.inventoryWindowFixture.ops' }
          if (args.path === '@/composables/useInventoryDrag') return { contents: dragFixtureSource }
          if (args.path === '@/network') return { contents: 'export const sendOpenContainer = ref => globalThis.inventoryWindowFixture.opened.push(ref); export const sendCloseContainer = () => {}' }
          return { contents: `import { defineComponent, h } from 'vue'; export default defineComponent({ inheritAttrs: false, setup(_, { attrs, slots }) { return () => h('control', attrs, slots.default?.()) } })` }
        })
      },
    }],
  })
  const { default: Window } = await import(pathToFileURL(outputPath).href)

  if (name === 'BuildStateWindow') {
    for (const handHasItem of [false, true]) {
      test(`build row Shift-click bypasses the hand with hand occupied=${handHasItem}`, () => {
        const calls = []
        const host = createNode('root')
        const app = renderer.createApp(Window, {
          entityId: 50, handHasItem,
          list: [{ itemKey: 'branch', requiredCount: 5, putCount: 2 }, { itemKey: 'stone', requiredCount: 1, putCount: 0 }],
          onQuickTakeBack: (...args) => calls.push(['quick', ...args]),
          onTakeBack: (...args) => calls.push(['hand', ...args]),
          onPutFromHand: (...args) => calls.push(['put', ...args]),
        })
        try {
          app.mount(host)
          const rows = []
          const collectRows = node => {
            if (node.props.class === 'build-state-window__row') rows.push(node)
            for (const child of node.children) collectRows(child)
          }
          collectRows(host)
          assert.equal(rows.length, 2)
          rows[0].props.onClick({ shiftKey: true, stopPropagation() {} })
          assert.deepEqual(calls, [['quick', 50, 0]], 'Shift-click returns from slot zero regardless of the hand')
          calls.length = 0
          rows[1].props.onClick({ shiftKey: true, stopPropagation() {} })
          assert.deepEqual(calls, [], 'empty row must not fall through to hand deposit or take-back')
          rows[0].props.onClick({ shiftKey: false, stopPropagation() {} })
          assert.deepEqual(calls, handHasItem ? [['put', 50]] : [['hand', 50, 0]])
        } finally {
          app.unmount()
        }
      })
    }
    test('build material icon arms pointer drag and suppresses row/hand clicks', () => {
      const originalFixture = globalThis.inventoryDragFixture
      const fixture = { calls: [], consume: true }
      globalThis.inventoryDragFixture = fixture
      const host = createNode('root')
      const clicks = []
      const app = renderer.createApp(Window, {
        entityId: 50, handHasItem: true,
        list: [{ itemKey: 'branch', resource: 'branch.png', requiredCount: 5, putCount: 2 }],
        onPutFromHand: (...args) => clicks.push(args),
        onTakeBack: (...args) => clicks.push(args),
        onQuickTakeBack: (...args) => clicks.push(args),
      })
      try {
        app.mount(host)
        const findNode = (node, className) => node.props.class === className ? node : node.children.map(child => findNode(child, className)).find(Boolean)
        const icon = findNode(host, 'build-state-window__icon')
        const row = findNode(host, 'build-state-window__row')
        const list = findNode(host, 'build-state-window__list')
        const event = { pointerId: 1, stopPropagation() { assert.fail('initial pointerdown must reach the outside-panel listener') } }
        icon.props.onPointerdown(event)
        assert.deepEqual(fixture.calls, [['build', event, 50, 0, '/assets/game/branch.png']])
        row.props.onClick({ shiftKey: true, stopPropagation() {} })
        list.props.onClick({})
        assert.deepEqual(clicks, [])
        assert.equal(list.props['data-inventory-drop-key'], '4_50_0')
      } finally {
        app.unmount()
        if (originalFixture === undefined) delete globalThis.inventoryDragFixture
        else globalThis.inventoryDragFixture = originalFixture
      }
    })
    continue
  }

  for (const handHasItem of [false, true]) {
    test(`${name} routes Shift-click separately with hand occupied=${handHasItem}`, () => {
      const originalFixture = globalThis.inventoryWindowFixture
      const calls = []
      const sourceRef = { kind: 0, ownerId: 7, inventoryKey: 0 }
      const instance = { itemId: 19, nestedRef: { kind: 0, ownerId: 19, inventoryKey: 0 } }
      const inventory = { ref: sourceRef, title: 'Inventory', grid: { width: 1, height: 1, items: [{ x: 0, y: 0, item: instance }] } }
      const fixture = {
        store: {
          playerEntityId: 7,
          handState: handHasItem ? { item: { itemId: 20 } } : null,
          openNestedInventories: new Map(),
          refKey: ref => `${ref.kind ?? 0}_${ref.ownerId ?? 0}_${ref.inventoryKey ?? 0}`,
        },
        ops: {
          quickTransferItem: (...args) => { calls.push(['quick', ...args]); return false },
          pickUpItem: (...args) => calls.push(['pickup', ...args]),
          placeOrSwapItem: (...args) => calls.push(['swap', ...args]),
          findInventoryState: () => inventory,
        },
        opened: [],
      }
      globalThis.inventoryWindowFixture = fixture
      const host = createNode('root')
      const props = name === 'InventoryWindow' ? { inventory } : { windowKey: '0_7_0', inventoryState: inventory }
      const app = renderer.createApp(Window, props)
      try {
        app.mount(host)
        const findItemControl = node => node.props?.item ? node : node.children.map(findItemControl).find(Boolean)
        const control = findItemControl(host)
        assert.ok(control)
        control.props.onItemClick(control.props.item, 4, 5, true)
        assert.deepEqual(calls, [['quick', sourceRef, instance]], 'failed quick transfer must not fall through to pickup or swap')

        calls.length = 0
        control.props.onItemClick(control.props.item, 4, 5, false)
        assert.deepEqual(calls, handHasItem
          ? [['swap', sourceRef, inventory, 0, 0]]
          : [['pickup', sourceRef, inventory, 19, 4, 5]])

        control.props.onItemRightClick(control.props.item)
        assert.deepEqual(fixture.opened, [instance.nestedRef])
      } finally {
        app.unmount()
        if (originalFixture === undefined) delete globalThis.inventoryWindowFixture
        else globalThis.inventoryWindowFixture = originalFixture
      }
    })
  }
}
