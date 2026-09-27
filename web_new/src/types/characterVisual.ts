import type { proto } from '../network/proto/packets'
import { EQUIPMENT_SLOT_BY_ID, type EquipmentSlot } from './equipmentSlots'
import { decodeGeneration, decodeUint64, compareUint64 } from './networkIdentity'
export { EQUIPMENT_SLOT_BY_ID, type EquipmentSlot } from './equipmentSlots'
export interface EquippedVisual { readonly slot: EquipmentSlot; readonly visualKey: string }
export interface CharacterVisualState {
  readonly generation: string
  // Decimal strings preserve protobuf uint64 revisions beyond Number.MAX_SAFE_INTEGER.
  readonly revision: string
  readonly equipment: readonly EquippedVisual[]
}

export function decodeCharacterVisual(input: proto.ICharacterVisualState): CharacterVisualState {
  const generation = decodeGeneration(input.generation)
  const revision = decodeUint64(input.revision)
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
    compareUint64(incoming.revision, current.revision) > 0
}
