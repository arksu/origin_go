import assert from 'node:assert/strict'
import { build } from 'esbuild'
import { compileScript, parse } from '@vue/compiler-sfc'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createRenderer, h, nextTick, provide, ref, ssrContextKey } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

const rootDirectory = fileURLToPath(new URL('..', import.meta.url))
const temporaryRoot = join(rootDirectory, 'node_modules', '.tmp')
await mkdir(temporaryRoot, { recursive: true })
const directory = await mkdtemp(join(temporaryRoot, 'actions-'))
const outfile = join(directory, 'components.mjs')
await build({
  stdin: {
    contents: `export { default as ActionsMenu } from './src/components/ui/ActionsMenu.vue';
export { default as Hotbar } from './src/components/ui/HotbarPlaceholder.vue';
export { useHotbarAssignments } from './src/composables/useHotbarAssignments.ts';
export { useActionsPanel } from './src/composables/useActionsPanel.ts';
export { requestGameAction } from './src/game/hud/actionCatalog.ts';
export { useGameStore } from './src/stores/gameStore.ts';
export { proto } from './src/network/proto/packets.js';
export { actionCursorCss } from './src/game/cursorCatalog.ts';
export { cancelActiveActionOnEscape } from './src/game/hud/actionState.ts';
export { CursorManager } from './src/game/CursorManager.ts';`,
    resolveDir: rootDirectory,
    sourcefile: 'actions-test.ts',
    loader: 'ts',
  },
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  external: ['vue', 'pinia'],
  alias: { '@': join(rootDirectory, 'src') },
  plugins: [{
    name: 'vue-inline-template',
    setup(pluginBuild) {
      pluginBuild.onLoad({ filter: /\.vue$/ }, async args => {
        const source = await readFile(args.path, 'utf8')
        const { descriptor } = parse(source, { filename: args.path })
        const script = compileScript(descriptor, { id: 'actions-test', inlineTemplate: true })
        return { contents: script.content, loader: 'ts', resolveDir: dirname(args.path) }
      })
    },
  }],
})

function withSsrContext(render) {
  return { setup() { provide(ssrContextKey, { modules: new Set() }); return render } }
}

function hostNode(type, text = '') {
  return { type, text, props: {}, children: [], parent: null }
}

const teleportTarget = hostNode('body')

const renderer = createRenderer({
  createElement: type => hostNode(type),
  createText: text => hostNode('#text', text),
  createComment: text => hostNode('#comment', text),
  setText: (node, text) => { node.text = text },
  setElementText: (node, text) => { node.children = [hostNode('#text', text)] },
  patchProp: (node, key, _oldValue, value) => { node.props[key] = value },
  insert: (node, parent, anchor = null) => {
    if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1)
    node.parent = parent
    const index = anchor ? parent.children.indexOf(anchor) : -1
    if (index < 0) parent.children.push(node)
    else parent.children.splice(index, 0, node)
  },
  remove: node => {
    if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1)
    node.parent = null
  },
  parentNode: node => node.parent,
  nextSibling: node => {
    if (!node.parent) return null
    return node.parent.children[node.parent.children.indexOf(node) + 1] || null
  },
  querySelector: selector => selector === 'body' ? teleportTarget : null,
})

function descendants(node, type) {
  return [node, ...node.children.flatMap(child => descendants(child, type))].filter(child => child.type === type)
}

let gameStore
try {
  const { ActionsMenu, Hotbar, useHotbarAssignments, useActionsPanel, requestGameAction, useGameStore, proto, actionCursorCss, cancelActiveActionOnEscape, CursorManager } = await import(pathToFileURL(outfile).href)
  setActivePinia(createPinia())
  gameStore = useGameStore()

  let cancels = 0
  let windowCloses = 0
  if (!cancelActiveActionOnEscape('approaching', () => { cancels++ })) windowCloses++
  assert.equal(cancels, 1)
  assert.equal(windowCloses, 0)
  if (!cancelActiveActionOnEscape('idle', () => { cancels++ })) windowCloses++
  assert.equal(cancels, 1)
  assert.equal(windowCloses, 1)

  const panel = useActionsPanel()
  panel.toggle()
  assert.equal(panel.isOpen.value, true)
  panel.closeOnOutsidePointer({ closest: () => ({}) })
  assert.equal(panel.isOpen.value, true)
  panel.closeOnOutsidePointer({ closest: () => null })
  assert.equal(panel.isOpen.value, false)
  panel.toggle()
  panel.toggle()
  assert.equal(panel.isOpen.value, false)

  const events = []
  const requests = []
  const sendAction = id => requests.push(id)
  const root = hostNode('root')
  const actions = [
    { id: 'lift', label: 'Lift', menuIcon: '/assets/cursor/lift.png' },
    { id: 'lift_down', label: 'Lift down', menuIcon: '/assets/cursor/lift_down.png' },
  ]
  renderer.createApp(withSsrContext(() => h(ActionsMenu, {
    actions, activeActionId: 'lift', activePhase: 'selecting',
    onActivate: id => {
      events.push(['activate', id])
      panel.select(id, candidate => requestGameAction(candidate, actions, true, sendAction))
    },
    onDragStart: id => events.push(['drag', id]),
    onTouchDragStart: payload => events.push(['touch', payload.actionId]),
    onTouchDragEnd: () => events.push(['touchEnd']),
  }))).mount(root)
  await nextTick()
  const buttons = descendants(root, 'button')
  assert.deepEqual(buttons.map(button => button.props['aria-label']), ['Lift', 'Lift down'])
  assert(buttons.every(button => button.props.type === 'button' && button.props['aria-disabled'] == null))
  assert.equal(buttons[0].props['aria-pressed'], true)
  assert(String(buttons[0].props.class).includes('actions-menu__action--active'))
  panel.toggle()
  buttons[0].props.onClick()
  assert.deepEqual(events.pop(), ['activate', 'lift'])
  assert.deepEqual(requests, ['lift'])
  assert.equal(panel.isOpen.value, false)
  panel.toggle()
  buttons[1].props.onClick()
  assert.deepEqual(events.pop(), ['activate', 'lift_down'])
  assert.deepEqual(requests, ['lift', 'lift_down'])
  assert.equal(panel.isOpen.value, false)
  gameStore.pushMiniAlert({ reasonCode: 'LIFT_NOT_CARRYING', severity: proto.AlertSeverity.ALERT_SEVERITY_WARNING, ttlMs: 3000 })
  assert.equal(gameStore.miniAlerts[0].reasonCode, 'LIFT_NOT_CARRYING')
  assert.equal(gameStore.miniAlerts[0].message, 'Lift Not Carrying')
  const transferred = new Map()
  buttons[0].props.onDragstart({ dataTransfer: { setData: (type, value) => transferred.set(type, value) } })
  assert.equal(transferred.get('application/x-origin-action-id'), 'game:lift')
  assert.deepEqual(events.pop(), ['drag', 'game:lift'])
  buttons[1].props.onDragstart({ dataTransfer: { setData: (type, value) => transferred.set(type, value) } })
  assert.equal(transferred.get('application/x-origin-action-id'), 'game:lift_down')
  assert.deepEqual(events.pop(), ['drag', 'game:lift_down'])
  buttons[0].props.onPointerdown({ pointerType: 'touch', pointerId: 9, clientX: 0, clientY: 0, currentTarget: { setPointerCapture() {} } })
  buttons[0].props.onPointermove({ pointerId: 9, clientX: 20, clientY: 0 })
  buttons[0].props.onPointerup({ pointerId: 9, clientX: 20, clientY: 0 })
  assert(events.some(event => event[0] === 'touch' && event[1] === 'game:lift'))
  assert(events.some(event => event[0] === 'touchEnd'))

  const storage = new Map([['hotbar_assignments_v1:account:7', JSON.stringify(['game:lift', 'game:retired', 'game:lift_down', null, null, null, null, null, null, null])]])
  globalThis.localStorage = {
    getItem: key => storage.get(key) || null,
    setItem: (key, value) => storage.set(key, value),
  }
  const assignments = useHotbarAssignments(ref('account'), ref(7))
  assert.equal(assignments.get(0), 'game:lift')
  assert.equal(assignments.get(1), 'game:retired')
  assert.equal(assignments.get(2), 'game:lift_down')
  assert.match(storage.get('hotbar_assignments_v1:account:7'), /game:retired/)
  assert.equal(requestGameAction('lift', actions, false, sendAction), false)
  assert.equal(requestGameAction('retired', actions, true, sendAction), false)

  const hotbarRoot = hostNode('root')
  const activated = []
  const drops = []
  renderer.createApp(withSsrContext(() => h(Hotbar, {
    assignments: assignments.assignments.value, serverActions: actions, actionListLoaded: true,
    onActivate: slot => {
      activated.push(slot)
      const id = assignments.get(slot)
      if (id?.startsWith('game:')) requestGameAction(id.slice(5), actions, true, sendAction)
    },
    onDrop: (slot, id) => drops.push([slot, id]),
  }))).mount(hotbarRoot)
  await nextTick()
  const slots = descendants(hotbarRoot, 'button')
  assert.equal(descendants(slots[0], 'img')[0].props.src, '/assets/cursor/lift.png')
  assert.equal(descendants(slots[1], 'img').length, 0)
  slots[0].props.onClick()
  slots[1].props.onClick()
  slots[2].props.onClick()
  assert.deepEqual(activated, [0, 2])
  assert.deepEqual(requests, ['lift', 'lift_down', 'lift', 'lift_down'])
  assert.equal(requests[0], requests[2])
  assert.equal(requests[1], requests[3])
  slots[3].props.onDrop({ preventDefault() {}, dataTransfer: { getData: type => transferred.get(type) || '' } })
  assert.deepEqual(drops, [[3, 'game:lift_down']])

  assert.equal(actionCursorCss(''), 'default')
  assert.equal(actionCursorCss('unknown'), 'help')
  assert.match(actionCursorCss('dig'), /dig\.png/)
  assert.match(actionCursorCss('lift_down'), /lift_down\.png/)
  const canvas = { style: { cursor: '' } }
  const pixiEvents = { cursorStyles: { default: 'inherit', pointer: 'pointer' }, rootBoundary: { cursor: 'pointer' } }
  const cursor = new CursorManager()
  cursor.attach(canvas, pixiEvents)
  cursor.set('lift')
  assert.match(canvas.style.cursor, /lift\.png/)
  assert.match(pixiEvents.cursorStyles.pointer, /lift\.png/)
  cursor.set('unknown')
  assert.equal(pixiEvents.cursorStyles.pointer, 'help')
  cursor.detach()
  assert.equal(pixiEvents.cursorStyles.pointer, 'pointer')
  console.log('Action menu, activation, mini-alert, drag, hotbar persistence, and cursor tests passed')
} finally {
  gameStore?.reset()
  await rm(directory, { recursive: true, force: true })
}
