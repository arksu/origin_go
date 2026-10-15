import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useInventoryDrag } from '../src/composables/useInventoryDrag'
import { useInventoryOps } from '../src/composables/useInventoryOps'
import { soundManager } from '../src/game/SoundManager'
import { gameConnection } from '../src/network/GameConnection'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { setWorldParams } from './serverConstantsFixture'

const gridRef = (ownerId: number): proto.IInventoryRef => ({ kind: proto.InventoryKind.INVENTORY_KIND_GRID, ownerId, inventoryKey: 0 })
const buildRef = (ownerId: number): proto.IInventoryRef => ({ kind: proto.InventoryKind.INVENTORY_KIND_BUILD, ownerId, inventoryKey: 0 })
const material = { itemId: 101, w: 3, h: 2, resource: 'items/branch.png', quality: 14 }

class Element extends EventTarget {
  dataset: { inventoryDropKey?: string } = {}
  captured = new Set<number>()
  target: Element | null = this
  rect = { left: 0, top: 0, width: 94, height: 63 }
  closest() { return this.target }
  getBoundingClientRect() { return this.rect }
  setPointerCapture(pointerId: number) { this.captured.add(pointerId) }
  hasPointerCapture(pointerId: number) { return this.captured.has(pointerId) }
  releasePointerCapture(pointerId: number) {
    this.captured.delete(pointerId)
    this.dispatchEvent(input('lostpointercapture', { pointerId }))
  }
}

function input(type: string, details: object = {}): PointerEvent {
  const event = new Event(type, { cancelable: true })
  Object.assign(event, { pointerId: 1, pointerType: 'mouse', isPrimary: true, button: 0, clientX: 40, clientY: 40, ...details })
  return event as PointerEvent
}

function fixture(t: TestContext) {
  t.mock.method(console, 'log', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  setActivePinia(createPinia())
  setWorldParams(12, 128)
  const store = useGameStore()
  store.setConnectionState('connected')
  store.setPlayerEnterWorld(1, 'Player', 1)
  const sourceRef = gridRef(1)
  const destinationRef = gridRef(20)
  const inventory = (ref: proto.IInventoryRef, revision = 7, items: proto.IGridItem[] = []): proto.IInventoryState => ({
    ref, revision, grid: { width: 8, height: 8, items },
  })
  store.updateInventory(inventory(sourceRef, 7, [{ x: 0, y: 0, item: material }]))
  store.setPlayerInventoryVisible(true)
  store.onContainerOpened(inventory(destinationRef, 9))
  const sent: proto.IInventoryOp[] = []
  t.mock.method(gameConnection, 'send', (packet: proto.IClientMessage) => {
    if (packet.inventoryOp?.op) sent.push(packet.inventoryOp.op)
  })
  const browser = new EventTarget()
  const document = Object.assign(new EventTarget(), { hidden: false, elementFromPoint: () => topElement })
  const oldWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const oldDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
  Object.defineProperty(globalThis, 'window', { configurable: true, value: browser })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: document })
  const source = new Element()
  source.rect = { left: 10, top: 20, width: 94, height: 63 }
  const destination = new Element()
  destination.dataset.inventoryDropKey = store.refKey(destinationRef)
  destination.rect = { left: 200, top: 300, width: 249, height: 249 }
  let topElement: Element | null = destination
  const drag = useInventoryDrag()
  const begin = (details: object = {}) => {
    // A genuine new pointerdown clears any previous delayed-click guard first.
    browser.dispatchEvent(input('pointerdown', details))
    const event = input('pointerdown', details)
    Object.defineProperty(event, 'currentTarget', { value: source })
    drag.beginGridDrag(event, sourceRef, { x: 0, y: 0, instance: material })
    return event
  }
  const beginBuild = (slot = 0, details: object = {}) => {
    browser.dispatchEvent(input('pointerdown', details))
    const event = input('pointerdown', details)
    Object.defineProperty(event, 'currentTarget', { value: source })
    drag.beginBuildDrag(event, 50, slot, '/assets/game/items/branch.png')
    return event
  }
  const dispatch = (type: string, details: object = {}) => {
    const event = input(type, details)
    browser.dispatchEvent(event)
    return event
  }
  const move = (details: object = {}) => dispatch('pointermove', { clientX: 80, clientY: 80, ...details })
  const drop = (details: object = {}) => dispatch('pointerup', { clientX: 300, clientY: 400, ...details })
  t.after(() => {
    drag.cancelDrag()
    drag.consumeClick()
    store.reset()
    if (oldWindow) Object.defineProperty(globalThis, 'window', oldWindow)
    else Reflect.deleteProperty(globalThis, 'window')
    if (oldDocument) Object.defineProperty(globalThis, 'document', oldDocument)
    else Reflect.deleteProperty(globalThis, 'document')
  })
  return { store, sent, drag, source, sourceRef, destination, destinationRef, browser, document, inventory, begin, beginBuild, move, drop, dispatch,
    setTopElement: (element: Element | null) => { topElement = element } }
}

for (const pointerType of ['mouse', 'touch']) {
  test(`${pointerType} waits beyond 8 px, then drops once at the grabbed multi-cell anchor`, t => {
    const { drag, begin, move, drop, sent, source } = fixture(t)
    begin({ pointerType, clientX: 75, clientY: 58 }) // (65, 38) inside the image: cell (2, 1).
    assert.equal(drag.activeDrag.value, null)
    move({ pointerType, clientX: 83, clientY: 58 })
    assert.equal(drag.activeDrag.value, null, 'the shared threshold is strictly exceeded')
    move({ pointerType, clientX: 84, clientY: 58 })
    assert.deepEqual(drag.activeDrag.value, {
      imageUrl: '/assets/game/items/branch.png', clientX: 84, clientY: 58, offsetX: 65, offsetY: 38, qualityText: '14',
    })
    assert.equal(sent.length, 0, 'arming and moving never pick up, deposit, or quick-transfer')
    drop({ pointerType, clientX: 200 + 1 + 5 * 31, clientY: 300 + 1 + 4 * 31 })
    assert.equal(sent.length, 1)
    assert.deepEqual(sent[0]!.move!.dstPos, { x: 3, y: 3 })
    assert.equal(sent[0]!.move!.modifiers, undefined)
    assert.equal(sent[0]!.move!.handPos, undefined)
    assert.equal(sent[0]!.move!.allowSwapOrMerge, false)
    assert.equal(drag.activeDrag.value, null)
    assert.equal(source.captured.size, 0)
    assert.equal(drag.consumeClick(), true)
    assert.equal(drag.consumeClick(), false)
  })
}

test('short pointer gesture preserves the ordinary click including Shift', t => {
  const { begin, drop, drag, sent } = fixture(t)
  begin({ shiftKey: true })
  drop({ clientX: 42, clientY: 41, shiftKey: true })
  assert.equal(sent.length, 0)
  assert.equal(drag.consumeClick(), false)
})

test('grid edge pitch and inset preserve the image cell boundary', t => {
  const { begin, move, drop, sent } = fixture(t)
  begin({ clientX: 10 + 31, clientY: 20 + 30 })
  move()
  drop({ clientX: 200 + 1 + 3 * 31, clientY: 300 + 1 + 2 * 31 })
  assert.deepEqual(sent[0]!.move!.dstPos, { x: 2, y: 2 })
})

for (const target of ['outside', 'unmarked top window', 'closed cached inventory', 'outside cells', 'negative anchor']) {
  test(`invalid drop on ${target} cancels and suppresses its click`, t => {
    const { begin, move, drop, sent, drag, setTopElement, destination, store, destinationRef } = fixture(t)
    begin({ clientX: 75, clientY: 58 })
    move()
    if (target === 'outside') setTopElement(null)
    if (target === 'unmarked top window') {
      const blocker = new Element()
      blocker.target = null
      setTopElement(blocker)
    }
    if (target === 'closed cached inventory') store.onContainerClosed(destinationRef)
    if (target === 'outside cells') destination.rect.left = 1000
    drop(target === 'negative anchor' ? { clientX: 201, clientY: 301 } : {})
    assert.equal(sent.length, 0)
    assert.equal(drag.activeDrag.value, null)
    assert.equal(drag.consumeClick(), true)
  })
}

test('only the topmost keyed target receives a drop', t => {
  const { begin, move, drop, sent, store, inventory, setTopElement } = fixture(t)
  const upperRef = gridRef(30)
  store.onContainerOpened(inventory(upperRef))
  const upper = new Element()
  upper.dataset.inventoryDropKey = store.refKey(upperRef)
  upper.rect = { left: 200, top: 300, width: 249, height: 249 }
  const child = new Element()
  child.target = upper
  setTopElement(child)
  begin()
  move()
  drop()
  assert.deepEqual(sent[0]!.move!.dst, upperRef)
})

for (const cancellation of ['Escape', 'pointercancel', 'lostpointercapture', 'blur', 'hidden', 'source close', 'revision', 'world reset', 'source unmount']) {
  test(`${cancellation} cancels without commands and releases capture`, t => {
    const { begin, move, drop, drag, sent, source, sourceRef, store, inventory, dispatch, document } = fixture(t)
    begin()
    move()
    if (cancellation === 'Escape') {
      const event = dispatch('keydown', { key: 'Escape' })
      assert.equal(event.defaultPrevented, true)
    }
    if (cancellation === 'pointercancel') dispatch('pointercancel')
    if (cancellation === 'lostpointercapture') source.dispatchEvent(input('lostpointercapture'))
    if (cancellation === 'blur') dispatch('blur')
    if (cancellation === 'hidden') {
      document.hidden = true
      document.dispatchEvent(new Event('visibilitychange'))
    }
    if (cancellation === 'source close') store.setPlayerInventoryVisible(false)
    if (cancellation === 'revision') store.updateInventory(inventory(sourceRef, 8, [{ x: 0, y: 0, item: material }]))
    if (cancellation === 'world reset') store.reset()
    if (cancellation === 'source unmount') drag.cancelSourceDrag(sourceRef, material.itemId)
    assert.equal(drag.activeDrag.value, null, 'cancellation is immediate')
    assert.equal(source.captured.size, 0)
    move()
    drop()
    assert.equal(sent.length, 0)
    assert.equal(drag.consumeClick(), true)
  })
}

test('extra pointer and secondary input never replace the primary gesture', t => {
  const { begin, move, drop, drag, sent, dispatch } = fixture(t)
  begin({ button: 2 })
  move()
  drop()
  assert.equal(drag.activeDrag.value, null)
  begin({ isPrimary: false, pointerId: 2 })
  move({ pointerId: 2 })
  assert.equal(drag.activeDrag.value, null)
  begin()
  begin({ isPrimary: false, pointerId: 2 })
  assert.equal(dispatch('pointerdown', { pointerId: 2, isPrimary: false }).defaultPrevented, true)
  move({ pointerId: 2 })
  assert.equal(drag.activeDrag.value, null)
  move()
  dispatch('pointercancel', { pointerId: 2 })
  drop({ pointerId: 2 })
  assert.ok(drag.activeDrag.value)
  drop()
  assert.equal(sent.length, 1)
})

test('Escape while armed cancels the ordinary click and native context menus cannot act during a gesture', t => {
  const { begin, dispatch, drag, source, sent } = fixture(t)
  begin()
  assert.equal(dispatch('contextmenu').defaultPrevented, true)
  assert.equal(dispatch('keydown', { key: 'Escape' }).defaultPrevented, true)
  assert.equal(source.captured.size, 0)
  assert.equal(drag.consumeClick(), true)
  assert.equal(sent.length, 0)
})

test('delayed touch click is swallowed globally after an invalid drop, then a fresh pointerdown resets suppression', t => {
  const { begin, move, drop, browser, setTopElement, dispatch, drag } = fixture(t)
  begin({ pointerType: 'touch' })
  move({ pointerType: 'touch' })
  setTopElement(null)
  drop({ pointerType: 'touch' })
  const click = input('click')
  let reachedGameplay = false
  browser.addEventListener('click', () => { reachedGameplay = true }, { once: true })
  browser.dispatchEvent(click)
  assert.equal(click.defaultPrevented, true)
  assert.equal(reachedGameplay, false)
  begin({ pointerType: 'touch' })
  move({ pointerType: 'touch' })
  drop({ pointerType: 'touch' })
  dispatch('pointerdown')
  assert.equal(drag.consumeClick(), false, 'new input does not inherit the previous touch drag')
})

test('occupied hand is untouched and the pending lock is shared with Shift transfer', t => {
  const { begin, move, drop, store, sent, sourceRef, drag } = fixture(t)
  const hand = { ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1 }, revision: 4,
    hand: { item: { itemId: 500 }, handPos: { mouseOffsetX: 88, mouseOffsetY: 99 } } }
  store.updateInventory(hand)
  begin()
  move()
  drop()
  assert.equal(sent.length, 1)
  assert.deepEqual(store.handInventoryState, hand)
  useInventoryOps().quickTransferItem(sourceRef, material)
  assert.equal(sent.length, 1)
  drag.consumeClick()
  begin()
  move()
  drop()
  assert.equal(sent.length, 1)
  store.completeQuickInventoryTransfer(sent[0]!.opId)
  useInventoryOps().quickTransferItem(sourceRef, material)
  assert.equal(sent.length, 2)
  begin()
  move()
  drop()
  assert.equal(sent.length, 2)
})

test('same-source rearrangement uses direct placement and unchanged position is a no-op', t => {
  const { begin, move, drop, sent, sourceRef, destination, store } = fixture(t)
  destination.dataset.inventoryDropKey = store.refKey(sourceRef)
  begin({ clientX: 10, clientY: 20 })
  move()
  drop({ clientX: 201, clientY: 301 })
  assert.equal(sent.length, 0)
  begin({ clientX: 10, clientY: 20 })
  move()
  drop({ clientX: 232, clientY: 332 })
  assert.deepEqual(sent[0]!.move!.dstPos, { x: 1, y: 1 })
  assert.equal(sent[0]!.expected!.length, 1)
})

test('grid deposits into a build list without placement or hand fields', t => {
  const { begin, move, drop, destination, store, sent } = fixture(t)
  store.setBuildStateSnapshot({ entityId: 50, list: [{ itemKey: 'branch', putCount: 2 }] })
  destination.dataset.inventoryDropKey = store.refKey(buildRef(50))
  begin()
  move()
  drop()
  assert.deepEqual(sent[0]!.move, { src: gridRef(1), dst: buildRef(50), itemId: 101, allowSwapOrMerge: false })
})

test('build row zero drags one server-selected unit and anchors its top-left at the pointer', t => {
  const { beginBuild, move, drop, store, drag, sent } = fixture(t)
  store.setBuildStateSnapshot({ entityId: 50, list: [{ itemKey: 'branch', putCount: 2 }] })
  beginBuild()
  move()
  assert.equal(drag.activeDrag.value!.offsetX, 0)
  assert.equal(drag.activeDrag.value!.offsetY, 0)
  assert.equal(sent.length, 0)
  drop({ clientX: 200 + 1 + 2 * 31, clientY: 300 + 1 + 3 * 31 })
  assert.deepEqual(sent[0]!.move, {
    src: buildRef(50), dst: gridRef(20), buildSlot: 0, dstPos: { x: 2, y: 3 }, allowSwapOrMerge: false,
  })
})

for (const change of ['counts', 'row identity', 'build close', 'build replacement', 'build target', 'empty row']) {
  test(`build drag rejects ${change}`, t => {
    const { beginBuild, move, drop, store, drag, sent, destination } = fixture(t)
    store.setBuildStateSnapshot({ entityId: 50, list: [{ itemKey: 'branch', putCount: change === 'empty row' ? 0 : 2 }] })
    beginBuild()
    move()
    if (change === 'counts') store.buildStateList[0]!.putCount = 1
    if (change === 'row identity') store.buildStateList[0]!.itemKey = 'stone'
    if (change === 'build close') store.closeBuildStateWindow()
    if (change === 'build replacement') store.setBuildStateSnapshot({ entityId: 51, list: [{ itemKey: 'branch', putCount: 2 }] })
    if (change === 'build target') destination.dataset.inventoryDropKey = store.refKey(buildRef(50))
    else assert.equal(drag.activeDrag.value, null)
    drop()
    assert.equal(sent.length, 0)
  })
}
