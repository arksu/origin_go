import { readonly, shallowRef, watch, type WatchStopHandle } from 'vue'
import { CLICK_DRAG_THRESHOLD_PX } from '@/constants/game'
import { INVENTORY_GRID } from '@/constants/inventoryLayout'
import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import { useInventoryOps, type InventoryDragSource } from '@/composables/useInventoryOps'
import { toNonNegativeProtoInt } from '@/utils/protoNumbers'

interface GridDragItem {
  x: number
  y: number
  instance: proto.IItemInstance
}

interface InventoryDragGhost {
  imageUrl: string
  clientX: number
  clientY: number
  offsetX: number
  offsetY: number
  qualityText?: string
}

interface DragGesture {
  pointerId: number
  element: HTMLElement
  source: InventoryDragSource
  store: ReturnType<typeof useGameStore>
  isCurrent: () => boolean
  startX: number
  startY: number
  cellOffsetX: number
  cellOffsetY: number
  ghost: InventoryDragGhost
  stopWatch?: WatchStopHandle
}

// DOM capture and gesture bookkeeping stay outside the data-only game store.
const activeDrag = shallowRef<InventoryDragGhost | null>(null)
let gesture: DragGesture | null = null
let suppressClick = false
let clickGuardInstalled = false

function stopEvent(event: Event) {
  event.preventDefault()
  event.stopImmediatePropagation()
}

function removeClickGuard() {
  if (!clickGuardInstalled) return
  window.removeEventListener('click', onGlobalClick, true)
  window.removeEventListener('pointerdown', onNextPointerDown, true)
  clickGuardInstalled = false
}

function consumeClick(event?: MouseEvent): boolean {
  if (!suppressClick && !activeDrag.value) return false
  suppressClick = false
  if (event) stopEvent(event)
  if (!gesture) removeClickGuard()
  return true
}

function onGlobalClick(event: MouseEvent) {
  consumeClick(event)
}

function onNextPointerDown(event: PointerEvent) {
  // A fresh gesture cannot inherit a cancelled touch drag's delayed-click guard.
  if (gesture) {
    stopEvent(event)
    return
  }
  suppressClick = false
  removeClickGuard()
}

function installClickGuard() {
  if (clickGuardInstalled) return
  window.addEventListener('click', onGlobalClick, true)
  window.addEventListener('pointerdown', onNextPointerDown, true)
  clickGuardInstalled = true
}

function finishGesture(cancelled: boolean) {
  const previous = gesture
  if (!previous) return
  if (cancelled || activeDrag.value) suppressClick = true
  gesture = null
  activeDrag.value = null
  previous.stopWatch?.()
  window.removeEventListener('pointermove', onPointerMove, true)
  window.removeEventListener('pointerup', onPointerUp, true)
  window.removeEventListener('pointercancel', onPointerCancel, true)
  window.removeEventListener('keydown', onKeyDown, true)
  window.removeEventListener('contextmenu', stopEvent, true)
  window.removeEventListener('blur', cancelDrag)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  previous.element.removeEventListener('lostpointercapture', onLostPointerCapture)
  if (previous.element.hasPointerCapture(previous.pointerId)) {
    previous.element.releasePointerCapture(previous.pointerId)
  }
  if (!suppressClick) removeClickGuard()
}

function cancelDrag() {
  finishGesture(true)
}

function onKeyDown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !gesture) return
  stopEvent(event)
  cancelDrag()
}

function onVisibilityChange() {
  if (document.hidden) cancelDrag()
}

function onPointerCancel(event: PointerEvent) {
  if (event.pointerId !== gesture?.pointerId) return
  stopEvent(event)
  cancelDrag()
}

function onLostPointerCapture(event: PointerEvent) {
  if (event.pointerId === gesture?.pointerId) cancelDrag()
}

function onPointerMove(event: PointerEvent) {
  const current = gesture
  if (!current || event.pointerId !== current.pointerId) return
  if (!current.isCurrent()) {
    cancelDrag()
    return
  }
  if (!activeDrag.value && Math.hypot(event.clientX - current.startX, event.clientY - current.startY) <= CLICK_DRAG_THRESHOLD_PX) return
  stopEvent(event)
  activeDrag.value = { ...current.ghost, clientX: event.clientX, clientY: event.clientY }
}

function dropAt(current: DragGesture, clientX: number, clientY: number) {
  const target = document.elementFromPoint(clientX, clientY)?.closest<HTMLElement>('[data-inventory-drop-key]')
  if (!target) return
  const key = target.dataset.inventoryDropKey
  const destination = current.store.openInventoryRefs.find(ref => current.store.refKey(ref) === key && current.store.isInventoryOpen(ref))
  if (!destination) return

  if ((destination.kind ?? 0) === proto.InventoryKind.INVENTORY_KIND_BUILD) {
    if (current.source.kind === 'grid') useInventoryOps().transferDraggedItem(current.source, destination)
    return
  }

  if ((destination.kind ?? 0) !== proto.InventoryKind.INVENTORY_KIND_GRID) return
  const grid = current.store.getOpenInventoryState(destination)?.grid
  if (!grid) return
  const rect = target.getBoundingClientRect()
  const slotX = Math.floor((clientX - rect.left - INVENTORY_GRID.itemInset) / INVENTORY_GRID.cellPitch)
  const slotY = Math.floor((clientY - rect.top - INVENTORY_GRID.itemInset) / INVENTORY_GRID.cellPitch)
  if (slotX < 0 || slotY < 0 || slotX >= (grid.width ?? 0) || slotY >= (grid.height ?? 0)) return
  useInventoryOps().transferDraggedItem(current.source, destination, {
    x: slotX - current.cellOffsetX,
    y: slotY - current.cellOffsetY,
  })
}

function onPointerUp(event: PointerEvent) {
  const current = gesture
  if (!current || event.pointerId !== current.pointerId) return
  if (activeDrag.value) {
    stopEvent(event)
    if (current.isCurrent()) dropAt(current, event.clientX, event.clientY)
  }
  finishGesture(false)
}

function arm(event: PointerEvent, details: Omit<DragGesture, 'pointerId' | 'element' | 'startX' | 'startY' | 'stopWatch'>) {
  const element = event.currentTarget as HTMLElement | null
  if (!element) return
  const current: DragGesture = {
    ...details, element, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY,
  }
  if (!current.isCurrent()) return
  gesture = current
  suppressClick = false
  installClickGuard()
  window.addEventListener('pointermove', onPointerMove, true)
  window.addEventListener('pointerup', onPointerUp, true)
  window.addEventListener('pointercancel', onPointerCancel, true)
  window.addEventListener('keydown', onKeyDown, true)
  window.addEventListener('contextmenu', stopEvent, true)
  window.addEventListener('blur', cancelDrag)
  document.addEventListener('visibilitychange', onVisibilityChange)
  element.addEventListener('lostpointercapture', onLostPointerCapture)
  current.stopWatch = watch(current.isCurrent, valid => { if (!valid) cancelDrag() }, { flush: 'sync' })
  try {
    element.setPointerCapture(event.pointerId)
  } catch {
    cancelDrag()
  }
}

function canBegin(event: PointerEvent): boolean {
  return !gesture && event.isPrimary && event.button === 0 && !!event.currentTarget
}

function beginGridDrag(event: PointerEvent, ref: proto.IInventoryRef, item: GridDragItem) {
  if (!canBegin(event) || (ref.kind ?? 0) !== proto.InventoryKind.INVENTORY_KIND_GRID) return
  const store = useGameStore()
  const state = store.getOpenInventoryState(ref)
  const itemId = item.instance.itemId
  if (!store.isInGame || store.quickInventoryTransfer || !state || !itemId || String(itemId) === '0') return
  const revision = state.revision ?? 0
  const world = store.worldParams
  const source: InventoryDragSource = { kind: 'grid', ref: { ...ref }, itemId, revision }
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const offsetX = Math.max(0, Math.min(event.clientX - rect.left, rect.width))
  const offsetY = Math.max(0, Math.min(event.clientY - rect.top, rect.height))
  const width = Math.max(1, toNonNegativeProtoInt(item.instance.w))
  const height = Math.max(1, toNonNegativeProtoInt(item.instance.h))
  arm(event, {
    store, source,
    isCurrent: () => store.isInGame && store.worldParams === world &&
      String(store.getOpenInventoryState(source.ref)?.revision ?? 0) === String(revision) &&
      !!store.getOpenInventoryState(source.ref)?.grid?.items?.some(entry => String(entry.item?.itemId ?? '') === String(itemId)),
    cellOffsetX: Math.min(width - 1, Math.floor(offsetX / INVENTORY_GRID.cellPitch)),
    cellOffsetY: Math.min(height - 1, Math.floor(offsetY / INVENTORY_GRID.cellPitch)),
    ghost: { imageUrl: `/assets/game/${item.instance.resource ?? ''}`, clientX: event.clientX, clientY: event.clientY,
      offsetX, offsetY, qualityText: String(item.instance.quality ?? 0) },
  })
}

function buildRowSignature(row: proto.IBuildStateItem | undefined): string {
  return row ? JSON.stringify([row.itemKey, row.itemTag, row.resource, row.requiredCount, row.putCount, row.buildCount]) : ''
}

function beginBuildDrag(event: PointerEvent, entityId: number, slot: number, imageUrl: string) {
  if (!canBegin(event) || !Number.isInteger(entityId) || entityId <= 0 || !Number.isInteger(slot) || slot < 0) return
  const store = useGameStore()
  const ref = { kind: proto.InventoryKind.INVENTORY_KIND_BUILD, ownerId: entityId, inventoryKey: 0 }
  const row = store.buildStateList[slot]
  if (!store.isInGame || store.quickInventoryTransfer || !store.isInventoryOpen(ref) || toNonNegativeProtoInt(row?.putCount) <= 0) return
  const signature = buildRowSignature(row)
  const world = store.worldParams
  arm(event, {
    store, source: { kind: 'build', ref, buildSlot: slot },
    isCurrent: () => store.isInGame && store.worldParams === world && store.isInventoryOpen(ref) &&
      buildRowSignature(store.buildStateList[slot]) === signature,
    cellOffsetX: 0, cellOffsetY: 0,
    ghost: { imageUrl, clientX: event.clientX, clientY: event.clientY, offsetX: 0, offsetY: 0 },
  })
}

function cancelSourceDrag(ref: proto.IInventoryRef, itemId?: proto.IItemInstance['itemId'], buildSlot?: number) {
  const current = gesture
  if (!current || current.store.refKey(current.source.ref) !== current.store.refKey(ref)) return
  if (current.source.kind === 'grid' && itemId !== undefined && String(current.source.itemId) !== String(itemId)) return
  if (current.source.kind === 'build' && buildSlot !== undefined && current.source.buildSlot !== buildSlot) return
  cancelDrag()
}

export function useInventoryDrag() {
  return { activeDrag: readonly(activeDrag), beginGridDrag, beginBuildDrag, cancelDrag, cancelSourceDrag, consumeClick }
}
