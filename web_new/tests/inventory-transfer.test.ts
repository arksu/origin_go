import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useInventoryOps } from '../src/composables/useInventoryOps'
import { soundManager } from '../src/game/SoundManager'
import { gameConnection } from '../src/network/GameConnection'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { setWorldParams } from './serverConstantsFixture'

const gridRef = (ownerId: number, inventoryKey = 0): proto.IInventoryRef => ({
  kind: proto.InventoryKind.INVENTORY_KIND_GRID, ownerId, inventoryKey,
})
const buildRef = (ownerId: number): proto.IInventoryRef => ({
  kind: proto.InventoryKind.INVENTORY_KIND_BUILD, ownerId, inventoryKey: 0,
})
const material: proto.IItemInstance = { itemId: 101, quantity: 5 }
function grid(ref: proto.IInventoryRef, revision = 1, items: proto.IItemInstance[] = []): proto.IInventoryState {
  return { ref, revision, grid: { width: 4, height: 4, items: items.map((item, x) => ({ x, y: 0, item })) } }
}

function fixture(t: TestContext) {
  t.mock.method(console, 'log', () => {})
  t.mock.method(console, 'warn', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  setActivePinia(createPinia())
  setWorldParams(12, 128)
  const store = useGameStore()
  store.setConnectionState('connected')
  store.setPlayerEnterWorld(1, 'Player', 1)
  const sent: proto.IInventoryOp[] = []
  const packets: proto.IClientMessage[] = []
  t.mock.method(gameConnection, 'send', (packet: proto.IClientMessage) => {
    packets.push(packet)
    if (packet.inventoryOp?.op) sent.push(packet.inventoryOp.op)
  })
  registerMessageHandlers()
  t.after(() => store.reset())
  const ops = useInventoryOps()
  const reply = (result: proto.IS2C_InventoryOpResult) => {
    messageDispatcher.dispatch(proto.ServerMessage.create({ inventoryOpResult: result }))
  }
  const openPlayer = (items: proto.IItemInstance[] = [material]) => {
    store.updateInventory(grid(gridRef(1), 7, items))
    store.setPlayerInventoryVisible(true)
  }
  return { store, ops, sent, packets, reply, openPlayer }
}

test('open inventory order includes builds and bags; snapshots retain order and reopen appends', t => {
  const { store, openPlayer } = fixture(t)
  store.setBuildStateSnapshot({ entityId: 50, buildName: 'Build' })
  store.onContainerOpened(grid(gridRef(20)))
  openPlayer()
  store.onContainerOpened(grid(gridRef(30)))
  const keys = () => store.openInventoryRefs.map(store.refKey)
  assert.deepEqual(keys(), ['4_50_0', '0_20_0', '0_1_0', '0_30_0'])
  store.setBuildStateSnapshot({ entityId: 50, buildName: 'Updated Build' })
  store.onContainerOpened(grid(gridRef(20), 8))
  store.updateInventory(grid(gridRef(1), 9))
  assert.deepEqual(keys(), ['4_50_0', '0_20_0', '0_1_0', '0_30_0'])
  store.onContainerClosed(gridRef(20))
  store.onContainerOpened(grid(gridRef(20), 9))
  store.setPlayerInventoryVisible(false)
  store.setPlayerInventoryVisible(true)
  assert.deepEqual(keys(), ['4_50_0', '0_30_0', '0_20_0', '0_1_0'])
  store.closeBuildStateWindowIfEntity(999)
  assert.equal(keys()[0], '4_50_0')
  store.setBuildStateSnapshot({ entityId: 51 })
  assert.deepEqual(keys(), ['0_30_0', '0_20_0', '0_1_0', '4_51_0'])
  store.closeBuildStateWindowIfEntity(51)
  store.closeAllNestedInventories()
  assert.deepEqual(keys(), ['0_1_0'])
})

test('late initial player snapshot registers its window; cached closed grids produce no destination', t => {
  const { store, ops, sent } = fixture(t)
  store.setPlayerInventoryVisible(true)
  store.updateInventory(grid(gridRef(1), 7, [material]))
  store.onContainerOpened(grid(gridRef(20)))
  store.onContainerClosed(gridRef(20))
  assert.equal(store.inventories.has('0_20_0'), true)
  assert.deepEqual(store.openInventoryRefs.map(store.refKey), ['0_1_0'])
  ops.quickTransferItem(gridRef(1), material)
  assert.equal(sent.length, 0)
  assert.equal(store.quickInventoryTransfer, null)
  store.setPlayerInventoryVisible(false)
  assert.equal(store.openInventoryRefs.length, 0)
})

test('grid Shift sends one server command with source and own nested refs; occupied hand is unchanged', t => {
  const { store, ops, sent, packets, openPlayer } = fixture(t)
  const bag = { ...material, nestedRef: gridRef(10) }
  openPlayer([bag])
  store.onContainerOpened(grid(gridRef(10), 11))
  store.setBuildStateSnapshot({ entityId: 50 })
  store.onContainerOpened(grid(gridRef(20), 12))
  const hand: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1, inventoryKey: 0 },
    revision: 13, hand: { item: { itemId: 500, quantity: 2 }, handPos: { mouseOffsetX: 10, mouseOffsetY: 20 } },
  }
  store.updateInventory(hand)
  ops.quickTransferItem(gridRef(1), bag)
  assert.equal(packets.length, 1)
  assert.deepEqual(sent[0]!.move, {
    src: gridRef(1), itemId: 101, modifiers: 1, allowSwapOrMerge: false,
    openInventories: [gridRef(1), gridRef(10), buildRef(50), gridRef(20)],
  })
  assert.deepEqual(sent[0]!.expected, [{ ref: gridRef(1), expectedRevision: 7 }])
  assert.deepEqual(store.handInventoryState, hand)
  assert.equal(store.getPlayerInventory()!.grid!.items![0]!.item!.quantity, 5, 'await the authoritative result')
})

test('nested source preserves opening order without client rotation and each command owns its list snapshot', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  openPlayer([])
  store.onContainerOpened(grid(gridRef(20)))
  store.onContainerOpened(grid(gridRef(30), 3, [material]))
  ops.quickTransferItem(gridRef(30), material)
  assert.deepEqual(sent[0]!.move!.openInventories, [gridRef(1), gridRef(20), gridRef(30)])
  assert.deepEqual(sent[0]!.expected, [{ ref: gridRef(30), expectedRevision: 3 }])
  store.onContainerClosed(gridRef(20))
  store.onContainerOpened(grid(gridRef(40)))
  assert.deepEqual(sent[0]!.move!.openInventories, [gridRef(1), gridRef(20), gridRef(30)])
  reply({ opId: sent[0]!.opId, success: true })
  ops.quickTransferItem(gridRef(30), material)
  assert.deepEqual(sent[1]!.move!.openInventories, [gridRef(1), gridRef(30), gridRef(40)])
})

test('build row zero sends unified Shift without item ID, destination, revisions or hand hop', t => {
  const { store, ops, sent, packets, openPlayer } = fixture(t)
  openPlayer([])
  store.setBuildStateSnapshot({ entityId: 50, list: [{ putCount: 3 }] })
  store.onContainerOpened(grid(gridRef(20)))
  const hand: proto.IInventoryState = {
    ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1, inventoryKey: 0 },
    revision: 13, hand: { item: { itemId: 500 } },
  }
  store.updateInventory(hand)
  ops.quickTransferBuildItem(50, 0)
  assert.equal(packets.length, 1)
  assert.deepEqual(sent[0]!.move, {
    src: buildRef(50), buildSlot: 0, modifiers: 1, allowSwapOrMerge: false,
    openInventories: [gridRef(1), buildRef(50), gridRef(20)],
  })
  assert.deepEqual(sent[0]!.expected, [])
  const decoded = proto.InventoryOp.decode(proto.InventoryOp.encode(sent[0]!).finish())
  assert.equal(decoded.move!.buildSlot, 0)
  assert.equal(Object.hasOwn(decoded.move!, 'buildSlot'), true, 'row zero retains optional presence on the wire')
  assert.equal(Object.hasOwn(decoded.move!, 'itemId'), false)
  assert.deepEqual(store.handInventoryState, hand)
})

test('build Shift requires an open active build and a valid row with deposited material', t => {
  const { store, ops, sent, openPlayer } = fixture(t)
  openPlayer([])
  store.setBuildStateSnapshot({ entityId: 50, list: [{ putCount: 0 }, { putCount: 1 }] })
  for (const [entityId, slot] of [[0, 1], [51, 1], [50, -1], [50, 0.5], [50, 9], [50, 0], [NaN, 1]]) {
    ops.quickTransferBuildItem(entityId!, slot!)
  }
  assert.equal(sent.length, 0)
  store.closeBuildStateWindow()
  ops.quickTransferBuildItem(50, 1)
  assert.equal(sent.length, 0)
})

test('all failures are final server results: one alert and no client fallback sends', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  const alerts = t.mock.method(store, 'pushMiniAlert')
  openPlayer()
  store.setBuildStateSnapshot({ entityId: 50 })
  store.onContainerOpened(grid(gridRef(20)))
  store.onContainerOpened(grid(gridRef(30)))
  for (const error of [proto.ErrorCode.ERROR_CODE_INVENTORY_FULL, proto.ErrorCode.ERROR_CODE_INVALID_REQUEST,
    proto.ErrorCode.ERROR_CODE_INTERNAL_ERROR, proto.ErrorCode.ERROR_CODE_NOT_AUTHENTICATED]) {
    const before = sent.length
    ops.quickTransferItem(gridRef(1), material)
    reply({ opId: sent[before]!.opId, success: false, error, message: `Final failure ${error}` })
    assert.equal(sent.length, before + 1)
    assert.equal(store.quickInventoryTransfer, null)
    assert.equal(alerts.mock.callCount(), before + 1)
  }
})

test('matching results apply snapshots before releasing pending; source refreshes never send again', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  openPlayer()
  store.onContainerOpened(grid(gridRef(20)))
  ops.quickTransferItem(gridRef(1), material)
  store.updateInventory(grid(gridRef(1), 8, [material]))
  assert.equal(store.quickInventoryTransfer!.cancelled, false)
  const complete = store.completeQuickInventoryTransfer
  t.mock.method(store, 'completeQuickInventoryTransfer', (opId: proto.IS2C_InventoryOpResult['opId']) => {
    assert.equal(store.getPlayerInventory()!.revision, 9)
    assert.equal(store.getNestedInventoryData('0_20_0')!.revision, 10)
    return complete(opId)
  })
  reply({ opId: sent[0]!.opId, success: false, error: proto.ErrorCode.ERROR_CODE_INVALID_REQUEST,
    updated: [grid(gridRef(1), 9, [material]), grid(gridRef(20), 10)] })
  assert.equal(sent.length, 1)
  assert.equal(store.quickInventoryTransfer, null)
  assert.equal(store.miniAlerts.length, 1)
})

test('additional grid/build clicks and unrelated results do not overlap or release the pending operation', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  openPlayer()
  store.setBuildStateSnapshot({ entityId: 50, list: [{ putCount: 3 }] })
  ops.quickTransferItem(gridRef(1), material)
  ops.quickTransferItem(gridRef(1), material)
  ops.quickTransferBuildItem(50, 0)
  assert.equal(sent.length, 1)
  const opId = sent[0]!.opId
  reply({ opId: 999, success: true })
  assert.equal(store.quickInventoryTransfer!.opId, opId)
  reply({ opId, success: true })
  assert.equal(store.quickInventoryTransfer, null)
  ops.quickTransferBuildItem(50, 0)
  assert.equal(sent.length, 2)
})

test('source close/reopen stays cancelled and late authoritative snapshots apply without feedback', t => {
  const { store, ops, sent, reply } = fixture(t)
  store.onContainerOpened(grid(gridRef(20), 7, [material]))
  store.onContainerOpened(grid(gridRef(30)))
  ops.quickTransferItem(gridRef(20), material)
  store.onContainerClosed(gridRef(20))
  store.onContainerOpened(grid(gridRef(20), 7, [material]))
  ops.quickTransferItem(gridRef(20), material)
  assert.equal(sent.length, 1)
  reply({ opId: sent[0]!.opId, success: false, error: proto.ErrorCode.ERROR_CODE_INVENTORY_FULL, updated: [grid(gridRef(30), 90)] })
  assert.equal(store.getNestedInventoryData('0_30_0')!.revision, 90)
  assert.equal(sent.length, 1)
  assert.equal(store.miniAlerts.length, 0)
  assert.equal(store.quickInventoryTransfer, null)
})

test('build source replacement cancels late feedback and session reset clears operation/window state', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  openPlayer([])
  store.setBuildStateSnapshot({ entityId: 50, list: [{ putCount: 3 }] })
  ops.quickTransferBuildItem(50, 0)
  store.setBuildStateSnapshot({ entityId: 51, list: [{ putCount: 1 }] })
  assert.equal(store.quickInventoryTransfer!.cancelled, true)
  reply({ opId: sent[0]!.opId, success: false, error: proto.ErrorCode.ERROR_CODE_INVENTORY_FULL })
  assert.equal(store.miniAlerts.length, 0)
  ops.quickTransferBuildItem(51, 0)
  store.setPlayerLeaveWorld()
  assert.equal(store.quickInventoryTransfer, null)
  assert.equal(store.openInventoryRefs.length, 0)
  reply({ opId: sent[1]!.opId, success: true })
  assert.equal(sent.length, 2)
})

test('Long IDs and revisions retain exact identities in unified command and opening list', t => {
  const { store, ops, sent } = fixture(t)
  const sourceRef = proto.InventoryRef.fromObject({ kind: 0, ownerId: '9007199254740992', inventoryKey: 0 })
  const otherRef = proto.InventoryRef.fromObject({ kind: 0, ownerId: '9007199254740993', inventoryKey: 0 })
  const item = proto.ItemInstance.fromObject({ itemId: '9007199254740993', quantity: 5 })
  const source = proto.InventoryState.fromObject({ ...grid(sourceRef, 0, [item]), revision: '9007199254740993' })
  store.onContainerOpened(source)
  store.onContainerOpened(grid(otherRef))
  ops.quickTransferItem(sourceRef, item)
  const decoded = proto.InventoryOp.decode(proto.InventoryOp.encode(sent[0]!).finish())
  assert.equal(String(decoded.move!.src!.ownerId), '9007199254740992')
  assert.equal(String(decoded.move!.itemId), '9007199254740993')
  assert.deepEqual(decoded.move!.openInventories!.map(r => String(r.ownerId)), ['9007199254740992', '9007199254740993'])
  assert.equal(String(decoded.expected![0]!.expectedRevision), '9007199254740993')
})

test('ordinary pickup and placement keep explicit destinations/revisions without Shift metadata', t => {
  const { store, ops, sent, openPlayer } = fixture(t)
  openPlayer()
  const handRef = store.getPlayerHandRef()!
  store.updateInventory({ ref: handRef, revision: 3, hand: {} })
  ops.pickUpItem(gridRef(1), store.getPlayerInventory(), 101, 5, 6)
  assert.equal(sent[0]!.move!.modifiers, undefined)
  assert.equal(sent[0]!.move!.openInventories, undefined)
  assert.deepEqual(sent[0]!.move!.dst, handRef)
  assert.deepEqual(sent[0]!.move!.handPos, { mouseOffsetX: 5, mouseOffsetY: 6 })
  assert.equal(sent[0]!.expected!.length, 2)
  store.updateInventory({ ref: handRef, revision: 4, hand: { item: material } })
  store.onContainerOpened(grid(gridRef(20), 8))
  ops.placeItem(gridRef(20), store.getNestedInventoryData('0_20_0'), 1, 2)
  assert.deepEqual(sent[1]!.move!.dstPos, { x: 1, y: 2 })
  assert.equal(sent[1]!.move!.modifiers, undefined)
  assert.equal(sent[1]!.expected!.length, 2)
  assert.equal(store.quickInventoryTransfer, null)
})

test('drag sends one exact grid move with two revisions and preserves the occupied hand', t => {
  const { store, ops, sent, packets, openPlayer } = fixture(t)
  openPlayer([{ ...material, w: 2, h: 1 }])
  store.onContainerOpened(grid(gridRef(20), 11))
  const hand: proto.IInventoryState = { ref: store.getPlayerHandRef(), revision: 12,
    hand: { item: { itemId: 500 }, handPos: { mouseOffsetX: 40, mouseOffsetY: 20 } } }
  store.updateInventory(hand)
  assert.equal(ops.transferDraggedItem({ kind: 'grid', ref: gridRef(1), itemId: 101, revision: 7 }, gridRef(20), { x: 2, y: 1 }), true)
  assert.equal(packets.length, 1)
  assert.deepEqual(sent[0]!.move, { src: gridRef(1), dst: gridRef(20), itemId: 101,
    dstPos: { x: 2, y: 1 }, allowSwapOrMerge: false })
  assert.deepEqual(sent[0]!.expected, [{ ref: gridRef(1), expectedRevision: 7 }, { ref: gridRef(20), expectedRevision: 11 }])
  assert.deepEqual(store.handInventoryState, hand)
  assert.equal(store.getPlayerInventory()!.grid!.items![0]!.item!.quantity, 5)
})

test('drag within the same grid deduplicates revision and ignores unchanged position', t => {
  const { store, ops, sent, openPlayer } = fixture(t)
  openPlayer()
  const source = { kind: 'grid' as const, ref: gridRef(1), itemId: 101, revision: 7 }
  assert.equal(ops.transferDraggedItem(source, gridRef(1), { x: 0, y: 0 }), false)
  assert.equal(ops.transferDraggedItem(source, gridRef(1), { x: 2, y: 1 }), true)
  assert.deepEqual(sent[0]!.expected, [{ ref: gridRef(1), expectedRevision: 7 }])
  assert.deepEqual(sent[0]!.move!.dstPos, { x: 2, y: 1 })
  assert.equal(store.getPlayerInventory()!.grid!.items![0]!.x, 0)
})

test('drag from build row zero uses exact destination and drag into build uses normal deposit', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  openPlayer()
  store.setBuildStateSnapshot({ entityId: 50, list: [{ putCount: 3 }] })
  assert.equal(ops.transferDraggedItem({ kind: 'build', ref: buildRef(50), buildSlot: 0 }, gridRef(1), { x: 2, y: 1 }), true)
  assert.deepEqual(sent[0]!.move, { src: buildRef(50), dst: gridRef(1), buildSlot: 0,
    dstPos: { x: 2, y: 1 }, allowSwapOrMerge: false })
  assert.deepEqual(sent[0]!.expected, [{ ref: gridRef(1), expectedRevision: 7 }])
  assert.equal(store.buildStateList[0]!.putCount, 3)
  reply({ opId: sent[0]!.opId, success: true })
  assert.equal(ops.transferDraggedItem({ kind: 'grid', ref: gridRef(1), itemId: 101, revision: 7 }, buildRef(50)), true)
  assert.deepEqual(sent[1]!.move, { src: gridRef(1), dst: buildRef(50), itemId: 101, allowSwapOrMerge: false })
  assert.deepEqual(sent[1]!.expected, [{ ref: gridRef(1), expectedRevision: 7 }])
})

test('invalid, stale, closed or own-nested drag targets never send a hand or transfer operation', t => {
  const { store, ops, sent, openPlayer } = fixture(t)
  openPlayer([{ ...material, w: 2, h: 1 }])
  store.onContainerOpened(grid(gridRef(20)))
  const source = { kind: 'grid' as const, ref: gridRef(1), itemId: 101, revision: 7 }
  for (const position of [undefined, { x: -1, y: 0 }, { x: 0.5, y: 0 }, { x: NaN, y: 0 },
    { x: 256, y: 0 }, { x: 0, y: 4 }, { x: 3, y: 0 }]) {
    assert.equal(ops.transferDraggedItem(source, gridRef(20), position), false)
  }
  assert.equal(ops.transferDraggedItem({ ...source, revision: 6 }, gridRef(20), { x: 1, y: 1 }), false)
  assert.equal(ops.transferDraggedItem({ ...source, itemId: 999 }, gridRef(20), { x: 1, y: 1 }), false)
  store.updateInventory(grid(gridRef(1), 7, [{ ...material, nestedRef: gridRef(20) }]))
  assert.equal(ops.transferDraggedItem(source, gridRef(20), { x: 1, y: 1 }), false)
  store.onContainerClosed(gridRef(20))
  assert.equal(ops.transferDraggedItem(source, gridRef(20), { x: 1, y: 1 }), false)
  store.setPlayerInventoryVisible(false)
  assert.equal(ops.transferDraggedItem(source, gridRef(1), { x: 1, y: 1 }), false)
  assert.equal(sent.length, 0)
  assert.equal(store.quickInventoryTransfer, null)
})

test('drag and Shift share one pending operation; rejected drop never picks another target', t => {
  const { store, ops, sent, reply, openPlayer } = fixture(t)
  openPlayer()
  store.onContainerOpened(grid(gridRef(20)))
  store.onContainerOpened(grid(gridRef(30)))
  const source = { kind: 'grid' as const, ref: gridRef(1), itemId: 101, revision: 7 }
  ops.quickTransferItem(gridRef(1), material)
  assert.equal(ops.transferDraggedItem(source, gridRef(20), { x: 1, y: 1 }), false)
  reply({ opId: sent[0]!.opId, success: true })
  assert.equal(ops.transferDraggedItem(source, gridRef(20), { x: 1, y: 1 }), true)
  ops.quickTransferItem(gridRef(1), material)
  assert.equal(ops.transferDraggedItem(source, gridRef(30), { x: 1, y: 1 }), false)
  assert.equal(sent.length, 2)
  reply({ opId: sent[1]!.opId, success: false, error: proto.ErrorCode.ERROR_CODE_INVENTORY_FULL })
  assert.equal(sent.length, 2)
  assert.equal(store.miniAlerts.length, 1)
  assert.equal(store.quickInventoryTransfer, null)
})

test('cancelled pending drag still applies late snapshots without a second command or alert', t => {
  const { store, ops, sent, reply } = fixture(t)
  store.onContainerOpened(grid(gridRef(20), 7, [material]))
  store.onContainerOpened(grid(gridRef(30)))
  assert.equal(ops.transferDraggedItem({ kind: 'grid', ref: gridRef(20), itemId: 101, revision: 7 }, gridRef(30), { x: 1, y: 1 }), true)
  store.onContainerClosed(gridRef(20))
  reply({ opId: sent[0]!.opId, success: false, error: proto.ErrorCode.ERROR_CODE_INVENTORY_FULL,
    updated: [grid(gridRef(30), 90)] })
  assert.equal(store.getNestedInventoryData('0_30_0')!.revision, 90)
  assert.equal(store.miniAlerts.length, 0)
  assert.equal(store.quickInventoryTransfer, null)
  assert.equal(sent.length, 1)
})

test('drag packet preserves Long IDs and source/destination revisions exactly', t => {
  const { store, ops, sent } = fixture(t)
  const sourceRef = proto.InventoryRef.fromObject({ kind: 0, ownerId: '9007199254740992' })
  const destinationRef = proto.InventoryRef.fromObject({ kind: 0, ownerId: '9007199254740993' })
  const item = proto.ItemInstance.fromObject({ itemId: '9007199254740993', quantity: 1, w: 1, h: 1 })
  const source = proto.InventoryState.fromObject({ ...grid(sourceRef, 0, [item]), revision: '9007199254740993' })
  const destination = proto.InventoryState.fromObject({ ...grid(destinationRef), revision: '9007199254740995' })
  store.onContainerOpened(source)
  store.onContainerOpened(destination)
  assert.equal(ops.transferDraggedItem({ kind: 'grid', ref: sourceRef, itemId: item.itemId, revision: source.revision }, destinationRef, { x: 1, y: 1 }), true)
  const decoded = proto.InventoryOp.decode(proto.InventoryOp.encode(sent[0]!).finish())
  assert.equal(String(decoded.move!.itemId), '9007199254740993')
  assert.equal(String(decoded.move!.dst!.ownerId), '9007199254740993')
  assert.deepEqual(decoded.expected!.map(entry => String(entry.expectedRevision)), ['9007199254740993', '9007199254740995'])
})
