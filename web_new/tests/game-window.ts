import { createApp, defineComponent, h, nextTick, ref, type VNode } from 'vue'
import { createPinia } from 'pinia'
import '../src/assets/main.scss'
import referenceUrl from '../../art_source/ui/window-forest/preview-320.png'
import GameWindow from '../src/components/ui/GameWindow.vue'
import InventoryWindow from '../src/components/ui/InventoryWindow.vue'
import NestedInventoryWindow from '../src/components/ui/NestedInventoryWindow.vue'
import EquipmentWindow from '../src/components/ui/EquipmentWindow.vue'
import CharacterSheetWindow from '../src/components/ui/CharacterSheetWindow.vue'
import PlayerStatsWindow from '../src/components/ui/PlayerStatsWindow.vue'
import CraftWindow from '../src/components/ui/CraftWindow.vue'
import BuildWindow from '../src/components/ui/BuildWindow.vue'
import BuildStateWindow from '../src/components/ui/BuildStateWindow.vue'
import SettingsWindow from '../src/components/ui/SettingsWindow.vue'
import KnockoutWindow from '../src/components/ui/KnockoutWindow.vue'
import { getWindowLayout } from '../src/constants/windowSkin'
import { useGameStore } from '../src/stores/gameStore'
import { useActionCooldownStore } from '../src/stores/actionCooldownStore'
import { gameConnection } from '../src/network/GameConnection'
import { timeSync } from '../src/network/TimeSync'
import { proto } from '../src/network/proto/packets.js'

const closed = ref<string[]>([])
const generation = ref(0)
const messages = ref<proto.IClientMessage[]>([])
const storageReads = ref<string[]>([])
const storageWrites = ref<string[]>([])
const hotkeys = ref(0)
const checks = ref<string[]>([])
const contrastBackground = ref(false)
const ids = [91001, 91002, 91003, 91004, 2000001, 7001, 7002, 7003, 7101, 7102, 7103, 9901]
const positions = new Map(ids.flatMap(id => [[`wnd_${id}_left`, '8'], [`wnd_${id}_top`, '8']]))
positions.set('wnd_-60_left', '1'); positions.set('wnd_-60_top', '2')

// This adapter exercises persistence without reading or writing the browser profile.
Object.defineProperty(window, 'localStorage', { value: {
  getItem(key: string) { storageReads.value.push(key); return positions.get(key) ?? null },
  setItem(key: string, value: string) { storageWrites.value.push(key); positions.set(key, value) },
} })
gameConnection.send = packet => { messages.value.push(packet) }
document.addEventListener('keydown', event => {
  if (event.key === 'Enter' || event.key === ' ') hotkeys.value++
})

function checkLayout() {
  const failures: string[] = []
  let windowCount = 0, itemCount = 0
  for (const stage of document.querySelectorAll<HTMLElement>('.stage')) {
    const container = stage.querySelector<HTMLElement>('.window-container')
    const content = stage.querySelector<HTMLElement>('.content')
    const interior = stage.querySelector<HTMLElement>('.window-interior')
    if (!container || !content || !interior) continue
    windowCount++
    const innerWidth = Number(stage.dataset.innerWidth), innerHeight = Number(stage.dataset.innerHeight)
    const contentRect = content.getBoundingClientRect(), windowRect = container.getBoundingClientRect()
    const interiorRect = interior.getBoundingClientRect()
    const label = stage.closest('article')?.dataset.fixture ?? 'window'
    if (contentRect.width !== innerWidth || contentRect.height !== innerHeight) failures.push(`${label}: content size`)
    if (windowRect.width < 192 || windowRect.height < 96) failures.push(`${label}: minimum outer size`)
    const margins = [contentRect.left - interiorRect.left, interiorRect.right - contentRect.right,
      contentRect.top - interiorRect.top, interiorRect.bottom - contentRect.bottom]
    if (margins[0]! < 8 || margins.some(margin => Math.abs(margin - margins[0]!) > 0.1)) failures.push(`${label}: unequal content margins (${margins.join(', ')})`)
    if (interiorRect.left - windowRect.left !== 20 || interiorRect.top - windowRect.top !== 29 || windowRect.right - interiorRect.right !== 11 || windowRect.bottom - interiorRect.bottom !== 22) failures.push(`${label}: interior does not follow frame`)
    if (!Number.isInteger(content.offsetLeft) || !Number.isInteger(content.offsetTop)) failures.push(`${label}: fractional content position`)
    for (const corner of container.querySelectorAll<HTMLElement>('.frame-part--top-left, .frame-part--top-right, .frame-part--bottom-right')) {
      if (corner.offsetWidth !== 48 || corner.offsetHeight !== 48) failures.push(`${label}: corner stretched`)
    }
    const closeControl = container.querySelector<HTMLElement>('.close-btn')
    if (closeControl && (closeControl.offsetWidth !== 28 || closeControl.offsetHeight !== 28)) failures.push(`${label}: close target size`)
    const slots = [...stage.querySelectorAll<HTMLElement>('.item-back')].map(slot => slot.getBoundingClientRect())
    if (['player', 'small', 'nested', 'tiny'].includes(label) && slots.length) {
      const gridBounds = { left: Math.min(...slots.map(slot => slot.left)), top: Math.min(...slots.map(slot => slot.top)),
        right: Math.max(...slots.map(slot => slot.right)), bottom: Math.max(...slots.map(slot => slot.bottom)) }
      if (Math.abs(gridBounds.left - contentRect.left) > 0.1 || Math.abs(gridBounds.top - contentRect.top) > 0.1 || Math.abs(gridBounds.right - contentRect.right) > 0.1 || Math.abs(gridBounds.bottom - contentRect.bottom) > 0.1) failures.push(`${label}: grid outside content bounds`)
    }
    for (const item of stage.querySelectorAll<HTMLElement>('.item-container')) {
      itemCount++
      const itemRect = item.getBoundingClientRect()
      if (!slots.some(slot => Math.abs(itemRect.left - slot.left - 1) < 0.1 && Math.abs(itemRect.top - slot.top - 1) < 0.1)) failures.push(`${label}: item and slot alignment`)
    }
  }
  const knockoutStage = document.querySelector<HTMLElement>('[data-fixture="knockout"] .stage')
  const knockout = knockoutStage?.querySelector<HTMLElement>('.window-container')
  if (knockout && knockoutStage) {
    if (knockout.querySelector('.close-btn')) failures.push('knockout: close control present')
    if (knockout.offsetLeft !== Math.max(0, Math.round((knockoutStage.clientWidth - knockout.offsetWidth) / 2)) || knockout.offsetTop !== Math.max(0, Math.round((knockoutStage.clientHeight - knockout.offsetHeight) / 2))) failures.push('knockout: not centered')
  }
  if (storageReads.value.some(key => key.startsWith('wnd_-60')) || storageWrites.value.some(key => key.startsWith('wnd_-60'))) failures.push('knockout: persistence used')
  checks.value = failures.length ? failures.map(reason => `FAIL: ${reason}`) : [`PASS: ${windowCount} window layouts and ${itemCount} item positions`]
}

function touchDragSmall() {
  const header = document.querySelector<HTMLElement>('[data-fixture="small"] .header')
  if (!header) return
  const bounds = header.getBoundingClientRect()
  const touch = (clientX: number, clientY: number) => new Touch({ identifier: 9, target: header, clientX, clientY })
  header.dispatchEvent(new TouchEvent('touchstart', { touches: [touch(bounds.left + 40, bounds.top + 12)], bubbles: true, cancelable: true }))
  document.dispatchEvent(new TouchEvent('touchmove', { touches: [touch(bounds.left + 64, bounds.top + 26)], bubbles: true, cancelable: true }))
  document.dispatchEvent(new TouchEvent('touchend', { touches: [], bubbles: true }))
}

async function mouseDragSmall() {
  const header = document.querySelector<HTMLElement>('[data-fixture="small"] .header')
  const container = header?.parentElement
  if (!header || !container) return
  const bounds = header.getBoundingClientRect(), previousLeft = container.offsetLeft, previousTop = container.offsetTop
  header.dispatchEvent(new MouseEvent('mousedown', { button: 0, clientX: bounds.left + 50, clientY: bounds.top + 12, bubbles: true, cancelable: true }))
  document.dispatchEvent(new MouseEvent('mousemove', { clientX: bounds.left + 62, clientY: bounds.top + 20, bubbles: true, cancelable: true }))
  document.dispatchEvent(new MouseEvent('mousemove', { clientX: bounds.left + 86, clientY: bounds.top + 30, bubbles: true, cancelable: true }))
  document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
  await nextTick()
  checks.value = [container.offsetLeft === previousLeft + 36 && container.offsetTop === previousTop + 18 && document.onmousemove === null && document.onmouseup === null
    ? 'PASS: mouse drag delta and handler cleanup' : 'FAIL: mouse drag delta or handler cleanup']
}

async function touchCloseSmall() {
  const button = document.querySelector<HTMLButtonElement>('[data-fixture="small"] .close-btn')
  if (!button) return
  const writesBefore = storageWrites.value.length
  const touch = new Touch({ identifier: 10, target: button, clientX: 0, clientY: 0 })
  button.dispatchEvent(new TouchEvent('touchstart', { touches: [touch], bubbles: true, cancelable: true }))
  button.dispatchEvent(new TouchEvent('touchend', { touches: [], bubbles: true }))
  button.click()
  document.dispatchEvent(new TouchEvent('touchend', { touches: [], bubbles: true }))
  await nextTick()
  checks.value = [closed.value.includes('small') && storageWrites.value.length === writesBefore ? 'PASS: touch close without dragging' : 'FAIL: touch close started dragging']
}

async function unmountDuringDrag() {
  const header = document.querySelector<HTMLElement>('[data-fixture="small"] .header')
  if (!header) return
  const writesBefore = storageWrites.value.length
  const touch = new Touch({ identifier: 11, target: header, clientX: 40, clientY: 12 })
  header.dispatchEvent(new MouseEvent('mousedown', { button: 0, clientX: 40, clientY: 12, bubbles: true, cancelable: true }))
  header.dispatchEvent(new TouchEvent('touchstart', { touches: [touch], bubbles: true, cancelable: true }))
  closed.value.push('small')
  await nextTick()
  document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
  document.dispatchEvent(new TouchEvent('touchend', { touches: [], bubbles: true }))
  checks.value = [storageWrites.value.length === writesBefore && document.onmousemove === null && document.onmouseup === null
    ? 'PASS: unmounted mouse and touch handlers removed' : 'FAIL: stale drag handlers after unmount']
}

createApp(defineComponent({ setup() {
  const game = useGameStore(), clock = useActionCooldownStore()
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(1, 'Window preview', 12, 4, 7, true)
  timeSync.reset(); clock.setSnapshot({ serverTimeMs: Date.now() })
  game.setPlayerStats({ isKnockedOut: true, isLying: true, canStandUp: false, koUntilMs: clock.serverNow() + 600000 })
  const brick = (itemId: number): proto.IItemInstance => ({ itemId, name: 'Brick', resource: 'items/brick.png', quality: 12 })
  const inventory: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_GRID, ownerId: 91001, inventoryKey: 0 }, title: 'Inventory', revision: 1,
    grid: { width: 2, height: 2, items: [{ x: 0, y: 0, item: brick(101) }, { x: 1, y: 1, item: brick(102) }] },
  }
  const nested: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_GRID, ownerId: 91002, inventoryKey: 0 }, title: 'Seed Bag', revision: 1,
    grid: { width: 4, height: 3, items: [{ x: 0, y: 0, item: brick(201) }, { x: 3, y: 2, item: brick(202) }] },
  }
  const playerInventory: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_GRID, ownerId: 91003, inventoryKey: 0 }, title: 'Player', revision: 1,
    grid: { width: 5, height: 5, items: Array.from({ length: 25 }, (_, index) => ({ x: index % 5, y: Math.floor(index / 5), item: brick(401 + index) })) },
  }
  const tinyInventory: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_GRID, ownerId: 91004, inventoryKey: 0 }, title: 'Tiny bag', revision: 1,
    grid: { width: 1, height: 1, items: [{ x: 0, y: 0, item: brick(501) }] },
  }
  const equipment: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_EQUIPMENT, ownerId: 1, inventoryKey: 0 }, revision: 1,
    equipment: { items: [{ slot: proto.EquipSlot.EQUIP_SLOT_HEAD, item: brick(301) }, { slot: proto.EquipSlot.EQUIP_SLOT_FEET, item: brick(302) }] },
  }
  game.inventories.set('0_91001_0', inventory)
  game.inventories.set('0_91002_0', nested)
  game.inventories.set('0_91003_0', playerInventory)
  game.inventories.set('0_91004_0', tinyInventory)
  game.inventories.set('2_1_0', equipment)
  game.openNestedInventories.set('0_91002_0', nested)
  game.setCraftListSnapshot({ recipes: Array.from({ length: 30 }, (_, index) => ({
    craftKey: `preview_${index}`, name: `Brick recipe ${index + 1}`, inputs: [{ itemKey: 'brick', count: 2 }],
    outputs: [{ itemKey: 'brick', count: 1 }], flags: { canStartNow: true, hasRequiredLinkedObject: true },
  })) })
  game.setBuildListSnapshot({ builds: Array.from({ length: 15 }, (_, index) => ({
    buildKey: `preview_${index}`, name: `Wooden structure ${index + 1}`, objectKey: 'crate', inputs: [{ itemKey: 'brick', count: 3 }],
  })) })

  const close = (id: string) => { closed.value.push(id) }
  const card = (id: string, title: string, width: number, height: number, render: () => VNode, centered = false) => {
    const layout = getWindowLayout(width, height)
    return h('article', { 'data-fixture': id, class: width > 400 ? 'wide' : '' }, [
      h('h2', title),
      h('div', { class: 'stage', 'data-inner-width': width, 'data-inner-height': height,
        style: { width: `${centered ? 420 : layout.width + 16}px`, height: `${centered ? 232 : layout.height + 16}px` } },
      closed.value.includes(id) ? h('p', { class: 'closed' }, 'Closed') : render()),
    ])
  }
  const control = (text: string, action: () => void) => h('button', { type: 'button', onClick: action }, text)
  return () => h('main', { key: generation.value, class: { contrast: contrastBackground.value } }, [
    h('h1', 'Forest carving — window acceptance'),
    h('nav', [
      control('Check layout', checkLayout),
      control('Reopen windows', () => { closed.value = []; generation.value++ }),
      control('Touch drag small', touchDragSmall),
      control('Mouse drag small', mouseDragSmall),
      control('Touch close small', touchCloseSmall),
      control('Unmount during drag', unmountDuringDrag),
      control('Put brick in hand', () => game.inventories.set('1_1_0', { ref: game.getPlayerHandRef(), revision: 1, hand: { item: brick(999) } })),
      control('Clear hand', () => game.inventories.delete('1_1_0')),
      control('Contrast background', () => { contrastBackground.value = !contrastBackground.value }),
    ]),
    h('output', { id: 'audit' }, `closed=${closed.value.join(',') || 'none'}; messages=${messages.value.length}; writes=${storageWrites.value.length}; game hotkeys=${hotkeys.value}`),
    h('p', { id: 'checks' }, checks.value.join('; ')),
    h('pre', { id: 'last-message' }, messages.value.length ? JSON.stringify(messages.value.at(-1)) : 'No gameplay command sent'),
    h('section', { class: 'gallery' }, [
      h('article', { class: 'reference' }, [h('h2', 'Selected reference'), h('img', { src: referenceUrl, width: 320, alt: 'Original forest carving reference' })]),
      card('player', 'Player inventory · 5 × 5', 156, 156, () => h(InventoryWindow, { inventory: playerInventory, onClose: () => close('player') })),
      card('small', 'Small inventory · 2 × 2', 63, 63, () => h(InventoryWindow, { inventory, onClose: () => close('small') })),
      card('nested', 'Nested container · 4 × 3', 125, 94, () => h(NestedInventoryWindow, { windowKey: '0_91002_0', inventoryState: nested, onClose: () => close('nested') })),
      card('tiny', 'Single cell · minimum size', 32, 32, () => h(InventoryWindow, { inventory: tinyInventory, onClose: () => close('tiny') })),
      card('equipment', 'Equipment', 218, 268, () => h(EquipmentWindow, { inventory: equipment, onClose: () => close('equipment') })),
      card('character', 'Character', 236, 292, () => h(CharacterSheetWindow, { onClose: () => close('character') })),
      card('stats', 'Player stats', 236, 190, () => h(PlayerStatsWindow, { onClose: () => close('stats') })),
      card('settings', 'Settings', 270, 230, () => h(SettingsWindow, { mode: 'hybrid3d', debugEnabled: false, onClose: () => close('settings') })),
      card('long-title', 'Long title', 180, 64, () => h(GameWindow, { id: 9901, title: 'A very long container title that must not overlap the close control', innerWidth: 180, innerHeight: 64, onClose: () => close('long-title') }, () => 'Exact content dimensions')),
      card('knockout', 'Knockout · centered, no close control', 280, 104, () => h(KnockoutWindow), true),
      card('build-state', 'Build state', 210, 140, () => h(BuildStateWindow, { entityId: 2, title: 'Crate', list: [{ resource: 'items/brick.png', putCount: 1, buildCount: 3 }, { resource: 'items/brick.png', putCount: 0, buildCount: 2 }], onClose: () => close('build-state') })),
      card('craft', 'Wide craft window · 680 × 360', 680, 360, () => h(CraftWindow, { onClose: () => close('craft') })),
      card('build', 'Build recipes', 490, 360, () => h(BuildWindow, { onClose: () => close('build') })),
    ]),
  ])
} })).use(createPinia()).mount('#app')

const stylesheet = document.createElement('style')
stylesheet.textContent = `
html, body, #app { height: auto; overflow: auto; }
body { background: #172414; color: #eee7d1; font-size: 13px; }
main { padding: 20px; }
h1 { font-size: 21px; margin-bottom: 14px; }
nav { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 12px; }
nav button { font-size: 12px; background: #3a4930; color: #eee7d1; padding: 6px 10px; }
output, #checks { display: block; margin: 8px 0; }
pre { max-width: 900px; overflow: auto; font: 11px monospace; margin-bottom: 20px; }
.gallery { display: flex; align-items: flex-start; flex-wrap: wrap; gap: 18px; }
article h2 { font-size: 12px; font-weight: normal; margin-bottom: 6px; }
.stage { position: relative; background: radial-gradient(ellipse at 30% 10%, #34452a, transparent 75%), #20341e; }
.contrast .stage { background: repeating-conic-gradient(#af5482 0% 25%, #427a9c 0% 50%) 0 0 / 12px 12px; }
.reference img { display: block; }
.wide { flex-basis: 100%; }
.closed { padding: 12px; color: #e3c178; }
`
document.head.appendChild(stylesheet)
