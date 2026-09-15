import type { proto } from '../network/proto/packets'

export const EQUIPMENT_SLOT_BY_ID = {
  1: 'head', 2: 'chest', 3: 'legs', 4: 'feet', 6: 'left_hand', 7: 'right_hand',
  8: 'back', 9: 'neck', 10: 'ring1', 11: 'ring2',
} as const
export type EquipmentSlot = typeof EQUIPMENT_SLOT_BY_ID[keyof typeof EQUIPMENT_SLOT_BY_ID]
export interface EquippedVisual { readonly slot: EquipmentSlot; readonly visualKey: string }
export interface CharacterVisualState {
  readonly generation: string
  // Decimal strings preserve protobuf uint64 revisions beyond Number.MAX_SAFE_INTEGER.
  readonly revision: string
  readonly equipment: readonly EquippedVisual[]
}

export function decodeCharacterVisual(input: proto.ICharacterVisualState): CharacterVisualState {
  const generation = input.generation ?? ''
  if (!/^-?\d+:\d+$/.test(generation) || generation.length > 64) throw new Error('Invalid character visual generation')
  if (typeof input.revision === 'number' && !Number.isSafeInteger(input.revision)) throw new Error('Unsafe character visual revision')
  const revision = (input.revision ?? 0).toString()
  if (!/^(0|[1-9]\d{0,19})$/.test(revision) || (revision.length === 20 && revision > '18446744073709551615')) throw new Error('Invalid character visual revision')
  const equipment: EquippedVisual[] = []
  const slots = new Set<EquipmentSlot>()
  for (const item of input.equipment ?? []) {
    const slot = EQUIPMENT_SLOT_BY_ID[item.slot as keyof typeof EQUIPMENT_SLOT_BY_ID]
    const visualKey = item.visualKey ?? ''
    if (!slot || slots.has(slot) || !/^[a-zA-Z0-9_-]{1,128}$/.test(visualKey)) throw new Error('Invalid character equipment snapshot')
    slots.add(slot)
    equipment.push({ slot, visualKey })
  }
  return { generation, revision, equipment }
}

export function isNewerCharacterVisual(current: CharacterVisualState, incoming: CharacterVisualState): boolean {
  return incoming.generation === current.generation &&
    (incoming.revision.length > current.revision.length || (incoming.revision.length === current.revision.length && incoming.revision > current.revision))
}
