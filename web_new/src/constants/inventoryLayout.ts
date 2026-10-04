export const INVENTORY_GRID = { cellPitch: 31, slotSize: 32, itemInset: 1 } as const

export function getInventoryGridExtent(cellCount: number) {
  if (!Number.isInteger(cellCount) || cellCount < 0) {
    throw new Error('Inventory grid: cell count must be a non-negative integer')
  }
  // Adjacent slots share a border, but the final outside border still takes 1 px.
  return cellCount === 0 ? 0 : (cellCount - 1) * INVENTORY_GRID.cellPitch + INVENTORY_GRID.slotSize
}
