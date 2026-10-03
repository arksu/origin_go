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
export { default as ActionIcon } from './src/components/ui/ActionIcon.vue';
export { useActionCooldownStore } from './src/stores/actionCooldownStore.ts';
export { timeSync } from './src/network/TimeSync.ts';
export { default as Hotbar } from './src/components/ui/HotbarPlaceholder.vue';
export { useHotbarAssignments } from './src/composables/useHotbarAssignments.ts';
export { useActionsPanel } from './src/composables/useActionsPanel.ts';
export { useActionPresentation } from './src/composables/useActionPresentation.ts';
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
  const { ActionsMenu, ActionIcon, useActionCooldownStore, timeSync, Hotbar, useHotbarAssignments, useActionsPanel, useActionPresentation, requestGameAction, useGameStore, proto, actionCursorCss, cancelActiveActionOnEscape, CursorManager } = await import(pathToFileURL(outfile).href)
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
  gameStore.setGameActionList(actions)
  const cooldownStore = useActionCooldownStore()
  const actionApp = renderer.createApp(withSsrContext(() => h(ActionsMenu, {
    actions, activeActionId: 'lift', activePhase: 'selecting',
    onActivate: id => {
      events.push(['activate', id])
      panel.select(id, candidate => requestGameAction(candidate, actions, true, sendAction, cooldownStore.isCoolingDown))
    },
    onDragStart: id => events.push(['drag', id]),
    onTouchDragStart: payload => events.push(['touch', payload.actionId]),
    onTouchDragEnd: () => events.push(['touchEnd']),
  })))
  actionApp.mount(root)
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
  assert.equal(requestGameAction('lift', actions, false, sendAction, cooldownStore.isCoolingDown), false)
  assert.equal(requestGameAction('retired', actions, true, sendAction, cooldownStore.isCoolingDown), false)

  const hotbarRoot = hostNode('root')
  const activated = []
  const drops = []
  const hotbarApp = renderer.createApp(withSsrContext(() => h(Hotbar, {
    assignments: assignments.assignments.value,
    onActivate: slot => {
      activated.push(slot)
      const id = assignments.get(slot)
      if (id?.startsWith('game:')) requestGameAction(id.slice(5), actions, true, sendAction, cooldownStore.isCoolingDown)
    },
    onDrop: (slot, id) => drops.push([slot, id]),
  })))
  hotbarApp.mount(hotbarRoot)
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
  // All mounted icons share one RAF and derive their angle from server timestamps.
  let monotonicMs = 0
  let nextFrameId = 1
  const frames = new Map()
  const originalPerformance = globalThis.performance
  globalThis.performance = { now: () => monotonicMs }
  globalThis.requestAnimationFrame = callback => {
    const id = nextFrameId++
    frames.set(id, callback)
    return id
  }
  globalThis.cancelAnimationFrame = id => frames.delete(id)
  const advanceFrame = async ms => {
    monotonicMs = ms
    const callbacks = [...frames.values()]
    frames.clear()
    for (const callback of callbacks) callback(ms)
    await nextTick()
  }
  const overlays = root => descendants(root, 'span').filter(node => node.props.class === 'action-icon__cooldown')
  assignments.assign(4, 'game:lift')
  gameStore.setActionProgress(20, 10)
  gameStore.setGameActionState({ actionId: 'lift', phase: 'cooldown_wait', serverTimeMs: 10000,
    cooldowns: [{ actionId: 'lift', startedAtMs: 10000, expiresAtMs: 12000 }] })
  await nextTick()
  assert.deepEqual(gameStore.actionProgress, { total: 0, current: 0 })
  assert.equal(frames.size, 1)
  assert.equal(overlays(root).length, 1)
  assert.equal(overlays(hotbarRoot).length, 2)
  const beforeBlockedClicks = requests.length
  buttons[0].props.onClick()
  slots[0].props.onClick()
  assert.equal(requestGameAction('lift', actions, true, sendAction, cooldownStore.isCoolingDown), false)
  assert.equal(requests.length, beforeBlockedClicks)
  assert.equal(buttons[0].props['aria-disabled'], true)
  assert.equal(slots[0].props['aria-disabled'], true)
  // A cooldown does not disable assignment or clearing.
  buttons[0].props.onDragstart({ dataTransfer: { setData: (type, value) => transferred.set(type, value) } })
  assert.equal(transferred.get('application/x-origin-action-id'), 'game:lift')
  const lateRoot = hostNode('root')
  await advanceFrame(1000)
  const lateApp = renderer.createApp(withSsrContext(() => h(ActionIcon, { actionId: 'game:lift' })))
  lateApp.mount(lateRoot)
  await nextTick()
  for (const icon of [...overlays(root), ...overlays(hotbarRoot), ...overlays(lateRoot)]) {
    assert.match(icon.props.style.background, /0\.5turn/)
  }
  assert.equal(frames.size, 1)
  lateApp.unmount()
  assert.equal(frames.size, 1)
  // Closing/reopening a panel must not restart its cooldown.
  actionApp.unmount()
  await advanceFrame(1500)
  const reopenedRoot = hostNode('root')
  const reopenedApp = renderer.createApp(withSsrContext(() => h(ActionIcon, { actionId: 'game:lift' })))
  reopenedApp.mount(reopenedRoot)
  await nextTick()
  assert.match(overlays(reopenedRoot)[0].props.style.background, /0\.75turn/)
  // Input checks remain current while a hidden tab has not run its next frame.
  monotonicMs = 2000
  assert.equal(cooldownStore.isCoolingDown('lift'), false)
  await advanceFrame(2000)
  assert.equal(overlays(hotbarRoot).length, 0)
  assert.equal(overlays(reopenedRoot).length, 0)
  assert.equal(frames.size, 0)
  // A delayed snapshot uses elapsed time rather than restarting a full duration.
  gameStore.setGameActionState({ phase: 'idle', serverTimeMs: 11500,
    cooldowns: [{ actionId: 'lift', startedAtMs: 10000, expiresAtMs: 12000 }] })
  await nextTick()
  assert.match(overlays(reopenedRoot)[0].props.style.background, /0\.75turn/)
  assert.equal(frames.size, 1)
  // Initialized TimeSync wins over the packet fallback.
  const originalDateNow = Date.now
  Date.now = () => 50000
  timeSync.onPong(50000, 11750)
  await advanceFrame(2000)
  assert.equal(cooldownStore.progress('lift'), 0.875)
  Date.now = originalDateNow
  timeSync.reset()
  gameStore.reset()
  await nextTick()
  assert.equal(frames.size, 0)
  assert.equal(overlays(reopenedRoot).length, 0)
  reopenedApp.unmount()
  hotbarApp.unmount()

  // Both UI entry points must arm local direction selection through the same activation path.
  gameStore.setConnectionState('connected')
  gameStore.setPlayerEnterWorld(1, 'Player', 12, 100, 7)
  const directionalActions = [
    { id: 'axe_sweep', label: 'Axe sweep', menuIcon: '/assets/cursor/atk.png', targetKind: 'direction', sector: { range: 18, sectorAngle: Math.PI / 2 } },
    { id: 'axe_strike', label: 'Axe strike', menuIcon: '/assets/cursor/atk.png', targetKind: 'direction', sector: { range: 18, sectorAngle: Math.PI / 2 } },
  ]
  gameStore.setGameActionList(directionalActions)
  const aimPresentation = useActionPresentation()
  const aimRequests = []
  const activateAim = id => aimPresentation.activate(id, candidate => aimRequests.push(candidate))
  const aimMenuRoot = hostNode('root')
  const aimMenuApp = renderer.createApp(withSsrContext(() => h(ActionsMenu, {
    actions: directionalActions, activeActionId: gameStore.directionAim?.actionId || '', activePhase: gameStore.directionAim ? 'selecting' : 'idle',
    onActivate: activateAim,
  })))
  aimMenuApp.mount(aimMenuRoot)
  await nextTick()
  descendants(aimMenuRoot, 'button')[0].props.onClick()
  assert.deepEqual(gameStore.directionAim, { actionId: 'axe_sweep', streamEpoch: 7 })
  const aimHotbarRoot = hostNode('root')
  const aimHotbarApp = renderer.createApp(withSsrContext(() => h(Hotbar, {
    assignments: ['game:axe_sweep', 'game:axe_strike', null, null, null, null, null, null, null, null],
    onActivate: slot => activateAim(directionalActions[slot].id),
  })))
  aimHotbarApp.mount(aimHotbarRoot)
  await nextTick()
  descendants(aimHotbarRoot, 'button')[1].props.onClick()
  assert.deepEqual(gameStore.directionAim, { actionId: 'axe_strike', streamEpoch: 7 })
  assert.deepEqual(aimRequests, [])
  aimMenuApp.unmount()
  aimHotbarApp.unmount()
  gameStore.reset()

  globalThis.performance = originalPerformance
  delete globalThis.requestAnimationFrame
  delete globalThis.cancelAnimationFrame
  console.log('Action menu, activation, drag, hotbar, cursor, and synchronized cooldown tests passed')
} finally {
  gameStore?.reset()
  await rm(directory, { recursive: true, force: true })
}
