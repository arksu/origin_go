import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import { sendInventoryOp } from '@/network'
import { Modifiers } from '@/game/InputController'
import { toNonNegativeProtoInt } from '@/utils/protoNumbers'

const INVENTORY_SLOT_SIZE_PX = 32

export type InventoryDragSource =
  | {
    kind: 'grid'
    ref: proto.IInventoryRef
    itemId: NonNullable<proto.IItemInstance['itemId']>
    revision: NonNullable<proto.IInventoryState['revision']>
  }
  | { kind: 'build'; ref: proto.IInventoryRef; buildSlot: number }

/**
 * Builds expected revisions for optimistic concurrency.
 * Includes src and dst containers (deduped if same).
 */
function buildExpected(
  srcState: proto.IInventoryState | undefined,
  dstState: proto.IInventoryState | undefined,
): proto.IInventoryExpected[] {
  const expected: proto.IInventoryExpected[] = []

  if (srcState?.ref) {
    expected.push({
      ref: srcState.ref,
      expectedRevision: srcState.revision ?? 0,
    })
  }

  if (dstState?.ref && dstState !== srcState) {
    expected.push({
      ref: dstState.ref,
      expectedRevision: dstState.revision ?? 0,
    })
  }

  return expected
}

export function useInventoryOps() {
  const gameStore = useGameStore()

  function transferDraggedItem(
    source: InventoryDragSource,
    dstRef: proto.IInventoryRef,
    dstPos?: { x: number; y: number },
  ): boolean {
    if (!gameStore.isInGame || gameStore.quickInventoryTransfer ||
      !gameStore.isInventoryOpen(source.ref) || !gameStore.isInventoryOpen(dstRef)) return false

    const srcKey = gameStore.refKey(source.ref)
    const dstKey = gameStore.refKey(dstRef)
    const expected: proto.IInventoryExpected[] = []
    let gridItem: proto.IGridItem | undefined
    if (source.kind === 'grid') {
      const state = gameStore.getOpenInventoryState(source.ref)
      if (!state || String(state.revision ?? 0) !== String(source.revision)) return false
      gridItem = state.grid?.items?.find(entry => String(entry.item?.itemId ?? 0) === String(source.itemId))
      const item = gridItem?.item
      if (!item || (item.nestedRef && gameStore.refKey(item.nestedRef) === dstKey)) return false
      expected.push({ ref: source.ref, expectedRevision: source.revision })
    } else {
      if (!Number.isInteger(source.buildSlot) || source.buildSlot < 0 ||
        toNonNegativeProtoInt(gameStore.buildStateList[source.buildSlot]?.putCount) <= 0) return false
    }

    if ((dstRef.kind ?? 0) === proto.InventoryKind.INVENTORY_KIND_GRID) {
      const destination = gameStore.getOpenInventoryState(dstRef)
      if (!destination?.grid || !dstPos || !Number.isInteger(dstPos.x) || !Number.isInteger(dstPos.y) ||
        dstPos.x < 0 || dstPos.y < 0 || dstPos.x >= (destination.grid.width ?? 0) ||
        dstPos.y >= (destination.grid.height ?? 0)) return false
      if (gridItem?.item) {
        if (dstKey === srcKey && dstPos.x === (gridItem.x ?? 0) && dstPos.y === (gridItem.y ?? 0)) return false
        if (dstPos.x + Math.max(1, gridItem.item.w ?? 1) > (destination.grid.width ?? 0) ||
          dstPos.y + Math.max(1, gridItem.item.h ?? 1) > (destination.grid.height ?? 0)) return false
      }
      if (source.kind === 'build' || dstKey !== srcKey) {
        expected.push({ ref: dstRef, expectedRevision: destination.revision ?? 0 })
      }
    } else if ((dstRef.kind ?? 0) !== proto.InventoryKind.INVENTORY_KIND_BUILD || source.kind === 'build' || dstPos) {
      return false
    }

    const opId = gameStore.allocOpId()
    gameStore.quickInventoryTransfer = { opId, sourceRef: { ...source.ref }, cancelled: false }
    sendInventoryOp({
      opId,
      expected,
      move: {
        src: source.ref,
        dst: dstRef,
        ...(source.kind === 'grid' ? { itemId: source.itemId } : { buildSlot: source.buildSlot }),
        ...(dstPos ? { dstPos } : {}),
        allowSwapOrMerge: false,
      },
    })
    return true
  }

  function sendQuickTransfer(move: proto.IInventoryMoveSpec, srcState?: proto.IInventoryState) {
    const srcRef = move.src
    if (!gameStore.isInGame || gameStore.quickInventoryTransfer || !srcRef || !gameStore.isInventoryOpen(srcRef)) return
    const openInventories = gameStore.openInventoryRefs
      .filter(r => gameStore.isInventoryOpen(r)).map(r => ({ ...r }))
    if (openInventories.length < 2 || !openInventories.some(r => gameStore.refKey(r) === gameStore.refKey(srcRef))) return

    const opId = gameStore.allocOpId()
    gameStore.quickInventoryTransfer = { opId, sourceRef: { ...srcRef }, cancelled: false }
    sendInventoryOp({
      opId,
      expected: buildExpected(srcState, undefined),
      move: { ...move, modifiers: Modifiers.SHIFT, openInventories, allowSwapOrMerge: false },
    })
  }

  function quickTransferItem(srcRef: proto.IInventoryRef, item: proto.IItemInstance) {
    if ((srcRef.kind ?? 0) !== proto.InventoryKind.INVENTORY_KIND_GRID || !item.itemId || String(item.itemId) === '0') return
    const source = gameStore.getOpenInventoryState(srcRef)
    if (!source) return
    sendQuickTransfer({ src: { ...srcRef }, itemId: item.itemId }, source)
  }

  function quickTransferBuildItem(entityId: number, slot: number) {
    if (!Number.isInteger(entityId) || entityId <= 0 || !Number.isInteger(slot) || slot < 0) return
    const row = gameStore.buildStateList[slot]
    if (!row || toNonNegativeProtoInt(row.putCount) <= 0) return
    sendQuickTransfer({
      src: { kind: proto.InventoryKind.INVENTORY_KIND_BUILD, ownerId: entityId, inventoryKey: 0 },
      buildSlot: slot,
    })
  }

  function getPlacePositionFromSlotClick(dstX: number, dstY: number): { x: number; y: number } {
    const handPos = gameStore.handState?.handPos
    if (!handPos) {
      return { x: dstX, y: dstY }
    }

    const offsetX = Math.max(0, Math.floor(Number(handPos.mouseOffsetX ?? 0)))
    const offsetY = Math.max(0, Math.floor(Number(handPos.mouseOffsetY ?? 0)))
    const slotShiftX = Math.floor(offsetX / INVENTORY_SLOT_SIZE_PX)
    const slotShiftY = Math.floor(offsetY / INVENTORY_SLOT_SIZE_PX)

    return {
      x: dstX - slotShiftX,
      y: dstY - slotShiftY,
    }
  }

  /**
   * Pick up item from a grid container into the hand.
   * @param srcRef - source container ref (the grid)
   * @param srcState - source container inventory state (for revision)
   * @param itemId - entity ID of the item to pick up
   * @param clickOffsetX - mouse click offset X within the item image
   * @param clickOffsetY - mouse click offset Y within the item image
   */
  function pickUpItem(
    srcRef: proto.IInventoryRef,
    srcState: proto.IInventoryState | undefined,
    itemId: number,
    clickOffsetX: number,
    clickOffsetY: number,
  ) {
    const handRef = gameStore.getPlayerHandRef()
    if (!handRef) return

    const handState = gameStore.handInventoryState

    const op: proto.IInventoryOp = {
      opId: gameStore.allocOpId(),
      expected: buildExpected(srcState, handState),
      move: {
        src: srcRef,
        dst: handRef,
        itemId,
        handPos: {
          mouseOffsetX: Math.round(clickOffsetX),
          mouseOffsetY: Math.round(clickOffsetY),
        },
        allowSwapOrMerge: false,
      },
    }

    console.log('[InventoryOps] pickUpItem:', op)
    sendInventoryOp(op)
  }

  /**
   * Place item from hand into a grid container at a specific position.
   * @param dstRef - destination container ref
   * @param dstState - destination container inventory state (for revision)
   * @param dstX - grid X position
   * @param dstY - grid Y position
   */
  function placeItem(
    dstRef: proto.IInventoryRef,
    dstState: proto.IInventoryState | undefined,
    dstX: number,
    dstY: number,
  ) {
    const handRef = gameStore.getPlayerHandRef()
    if (!handRef) return

    const hand = gameStore.handState
    if (!hand?.item) return

    const handState = gameStore.handInventoryState
    const itemId = Number(hand.item.itemId ?? 0)

    const op: proto.IInventoryOp = {
      opId: gameStore.allocOpId(),
      expected: buildExpected(handState, dstState),
      move: {
        src: handRef,
        dst: dstRef,
        itemId,
        dstPos: { x: dstX, y: dstY },
        allowSwapOrMerge: false,
      },
    }

    console.log('[InventoryOps] placeItem:', op)
    sendInventoryOp(op)
  }

  /**
   * Place item from hand onto an existing item (swap or merge).
   * @param dstRef - destination container ref
   * @param dstState - destination container inventory state (for revision)
   * @param dstX - grid X position of the target item
   * @param dstY - grid Y position of the target item
   */
  function placeOrSwapItem(
    dstRef: proto.IInventoryRef,
    dstState: proto.IInventoryState | undefined,
    dstX: number,
    dstY: number,
  ) {
    const handRef = gameStore.getPlayerHandRef()
    if (!handRef) return

    const hand = gameStore.handState
    if (!hand?.item) return

    const handState = gameStore.handInventoryState
    const itemId = Number(hand.item.itemId ?? 0)

    const op: proto.IInventoryOp = {
      opId: gameStore.allocOpId(),
      expected: buildExpected(handState, dstState),
      move: {
        src: handRef,
        dst: dstRef,
        itemId,
        dstPos: { x: dstX, y: dstY },
        allowSwapOrMerge: true,
      },
    }

    console.log('[InventoryOps] placeOrSwapItem:', op)
    sendInventoryOp(op)
  }

  /**
   * Place item from hand into an equipment container slot.
   * @param dstRef - destination equipment container ref
   * @param dstState - destination container inventory state (for revision)
   * @param slot - target equipment slot
   */
  function placeItemToEquipmentSlot(
    dstRef: proto.IInventoryRef,
    dstState: proto.IInventoryState | undefined,
    slot: proto.EquipSlot,
  ) {
    const handRef = gameStore.getPlayerHandRef()
    if (!handRef) return

    const hand = gameStore.handState
    if (!hand?.item) return

    const handState = gameStore.handInventoryState
    const itemId = Number(hand.item.itemId ?? 0)

    const op: proto.IInventoryOp = {
      opId: gameStore.allocOpId(),
      expected: buildExpected(handState, dstState),
      move: {
        src: handRef,
        dst: dstRef,
        itemId,
        dstEquipSlot: slot,
        allowSwapOrMerge: false,
      },
    }

    console.log('[InventoryOps] placeItemToEquipmentSlot:', op)
    sendInventoryOp(op)
  }

  /**
   * Place item from hand onto an equipment slot (swap allowed).
   * @param dstRef - destination equipment container ref
   * @param dstState - destination container inventory state (for revision)
   * @param slot - target equipment slot
   */
  function placeOrSwapItemInEquipmentSlot(
    dstRef: proto.IInventoryRef,
    dstState: proto.IInventoryState | undefined,
    slot: proto.EquipSlot,
  ) {
    const handRef = gameStore.getPlayerHandRef()
    if (!handRef) return

    const hand = gameStore.handState
    if (!hand?.item) return

    const handState = gameStore.handInventoryState
    const itemId = Number(hand.item.itemId ?? 0)

    const op: proto.IInventoryOp = {
      opId: gameStore.allocOpId(),
      expected: buildExpected(handState, dstState),
      move: {
        src: handRef,
        dst: dstRef,
        itemId,
        dstEquipSlot: slot,
        allowSwapOrMerge: true,
      },
    }

    console.log('[InventoryOps] placeOrSwapItemInEquipmentSlot:', op)
    sendInventoryOp(op)
  }

  function placeItemIntoBuild(buildEntityId: number) {
    const targetEntityId = Math.trunc(buildEntityId)
    if (!Number.isFinite(targetEntityId) || targetEntityId <= 0) return

    const handRef = gameStore.getPlayerHandRef()
    if (!handRef) return

    const hand = gameStore.handState
    if (!hand?.item) return

    const handState = gameStore.handInventoryState
    const itemId = Number(hand.item.itemId ?? 0)
    if (!Number.isFinite(itemId) || itemId <= 0) return

    const op: proto.IInventoryOp = {
      opId: gameStore.allocOpId(),
      expected: buildExpected(handState, undefined),
      move: {
        src: handRef,
        dst: {
          kind: proto.InventoryKind.INVENTORY_KIND_BUILD,
          ownerId: targetEntityId,
          inventoryKey: 0,
        },
        itemId,
        allowSwapOrMerge: false,
      },
    }

    console.log('[InventoryOps] placeItemIntoBuild:', op)
    sendInventoryOp(op)
  }

  /**
   * Find the InventoryState for a given ref from the store.
   */
  function findInventoryState(ref: proto.IInventoryRef): proto.IInventoryState | undefined {
    const key = `${ref.kind ?? 0}_${ref.ownerId ?? 0}_${ref.inventoryKey ?? 0}`
    return gameStore.inventories.get(key) ?? gameStore.openNestedInventories.get(key)
  }

  return {
    transferDraggedItem,
    quickTransferItem,
    quickTransferBuildItem,
    pickUpItem,
    placeItem,
    placeOrSwapItem,
    placeItemToEquipmentSlot,
    placeOrSwapItemInEquipmentSlot,
    placeItemIntoBuild,
    findInventoryState,
    getPlacePositionFromSlotClick,
  }
}
