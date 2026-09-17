import { Bone, PropertyBinding, type Object3D } from 'three'
import type { SocketId } from './equipment'

const SOCKET_BONES: Record<SocketId, string> = {
  grip_l: 'hand.l', grip_r: 'hand.r', forearm_l: 'forearm.l', forearm_r: 'forearm.r',
}

export function findRigBone(model: Object3D, name: string): Bone {
  // GLTFLoader sanitizes dots in exported names; Blender keeps the original names.
  const bone = model.getObjectByName(name) ?? model.getObjectByName(PropertyBinding.sanitizeNodeName(name))
  if (!(bone instanceof Bone)) throw new Error(`Character bone missing: ${name}`)
  return bone
}

export class ActorSockets {
  private readonly sockets = new Map<SocketId, Object3D>()
  constructor(model: Object3D, names: Partial<Record<SocketId, string>> = { grip_l: 'grip_l', grip_r: 'grip_r', forearm_l: 'forearm_l', forearm_r: 'forearm_r' }) {
    for (const [name, boneName] of Object.entries(SOCKET_BONES) as [SocketId, string][]) {
      const bone = findRigBone(model, boneName)
      const authored = names[name] && bone.getObjectByName(names[name])
      if (authored) {
        this.sockets.set(name, authored)
      } else {
        throw new Error(`Character socket missing: ${name}`)
      }
    }
  }
  get(name: SocketId): Object3D {
    const socket = this.sockets.get(name)
    if (!socket) throw new Error(`Character socket missing: ${name}`)
    return socket
  }
}
