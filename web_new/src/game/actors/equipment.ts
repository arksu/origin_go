import { EQUIPMENT_SLOT_BY_ID, type EquipmentSlot, type EquippedVisual } from '../../types/characterVisual'

export type ArmSide = 'left' | 'right'
export type SocketId = 'grip_l' | 'grip_r' | 'forearm_l' | 'forearm_r'
export interface AttachmentTransform {
  readonly position?: readonly [number, number, number]
  readonly rotation?: readonly [number, number, number]
  readonly quaternion?: readonly [number, number, number, number]
  readonly scale?: number
}
export type ArmMotion =
  | { readonly kind: 'ordinary' }
  | { readonly kind: 'layered'; readonly idlePose: string; readonly walkPose?: string }
export interface EquipmentBinding {
  readonly socket: SocketId
  readonly transform?: AttachmentTransform
  // Ordinary is the default for one-handed weapons. Layered clips are opt-in
  // for equipment that genuinely constrains both hands, such as a rod or shield.
  readonly armMotion?: ArmMotion
}
export type EquipmentDefinition =
  | { readonly kind: 'deferred' }
  | { readonly kind: 'rigid'; readonly url: string; readonly bindings: Partial<Record<EquipmentSlot, EquipmentBinding>> }
  | { readonly kind: 'skinned'; readonly url: string; readonly slots: readonly EquipmentSlot[] }

// Grip transforms are authored against the commoner fist; quaternions avoid a
// Blender/Three Euler-order mismatch. Held objects follow the ordinary hand
// animation. Item-specific poses are reserved for deliberate actions, not carry.
export const EQUIPMENT: Readonly<Record<string, EquipmentDefinition>> = {
  stone_axe: {
    kind: 'rigid', url: '/assets/game/equipment/stone_axe/stone_axe.glb',
    bindings: {
      left_hand: { socket: 'grip_l', transform: {
        position: [-.019444086, .088261202, -.006447274],
        quaternion: [-.079406664, .163817227, .097031258, .978490412],
      } },
      right_hand: { socket: 'grip_r', transform: {
        position: [.019444138, .088261358, -.006447325],
        quaternion: [.097031206, .978490412, -.079406559, .163817227],
      } },
    },
  },
}
export const DEFAULT_EQUIPMENT: readonly EquippedVisual[] = []

export function armForSlot(slot: EquipmentSlot): ArmSide | undefined {
  return slot === 'left_hand' ? 'left' : slot === 'right_hand' ? 'right' : undefined
}

export function validateEquipment(items: readonly EquippedVisual[], catalog = EQUIPMENT): void {
  const slots = new Set<EquipmentSlot>()
  for (const item of items) {
    if (!item || !Object.values(EQUIPMENT_SLOT_BY_ID).includes(item.slot) || !/^[a-zA-Z0-9_-]{1,128}$/.test(item.visualKey) || slots.has(item.slot)) throw new Error('Invalid character equipment')
    slots.add(item.slot)
    // A client can receive items whose 3D representation is not available yet.
    // Preserve their state without leaving the previous weapon drawn.
    const definition = Object.hasOwn(catalog, item.visualKey) ? catalog[item.visualKey] : undefined
    if (!definition) continue
    if (definition.kind === 'rigid') {
      const binding = definition.bindings[item.slot]
      if (!binding) throw new Error(`No ${item.slot} binding for ${item.visualKey}`)
      const motion = binding.armMotion
      if (motion && (motion.kind === 'layered') && (!motion.idlePose || (motion.walkPose !== undefined && !motion.walkPose))) throw new Error(`Invalid arm motion for ${item.visualKey}`)
    }
    if (definition.kind === 'skinned' && !definition.slots.includes(item.slot)) throw new Error(`Invalid garment slot for ${item.visualKey}`)
  }
}
