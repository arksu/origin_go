import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import { sendInventoryOp } from '@/network'

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
   * Find the InventoryState for a given ref from the store.
   */
  function findInventoryState(ref: proto.IInventoryRef): proto.IInventoryState | undefined {
    const key = `${ref.kind ?? 0}_${ref.ownerId ?? 0}_${ref.inventoryKey ?? 0}`
    return gameStore.inventories.get(key) ?? gameStore.openNestedInventories.get(key)
  }

  return {
    pickUpItem,
    placeItem,
    placeOrSwapItem,
    findInventoryState,
  }
}
