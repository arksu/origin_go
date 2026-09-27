export const EQUIPMENT_SLOT_BY_ID = {
  1: 'head', 2: 'chest', 3: 'legs', 4: 'feet', 6: 'left_hand', 7: 'right_hand',
  8: 'back', 9: 'neck', 10: 'ring1', 11: 'ring2',
} as const
export type EquipmentSlot = typeof EQUIPMENT_SLOT_BY_ID[keyof typeof EQUIPMENT_SLOT_BY_ID]
