import { Bone, Group, PropertyBinding, type Object3D } from 'three'
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
  constructor(model: Object3D) {
    for (const [name, boneName] of Object.entries(SOCKET_BONES) as [SocketId, string][]) {
      const bone = findRigBone(model, boneName)
      const authored = bone.getObjectByName(name)
      if (authored) {
        this.sockets.set(name, authored)
      } else {
        const socket = new Group()
        socket.name = name
        bone.add(socket)
        this.sockets.set(name, socket)
      }
    }
  }
  get(name: SocketId): Object3D {
    const socket = this.sockets.get(name)
    if (!socket) throw new Error(`Character socket missing: ${name}`)
    return socket
  }
}
