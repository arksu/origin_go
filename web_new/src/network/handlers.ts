import { proto } from './proto/packets.js'
import { messageDispatcher } from './MessageDispatcher'
import { useGameStore, type EntityMovement } from '@/stores/gameStore'

function toNumber(value: number | Long): number {
  if (typeof value === 'number') return value
  return value.toNumber()
}

export function registerMessageHandlers(): void {
  const gameStore = useGameStore()

  messageDispatcher.on('playerEnterWorld', (msg: proto.IS2C_PlayerEnterWorld) => {
    gameStore.setPlayerEnterWorld(
      toNumber(msg.entityId!),
      msg.name || '',
      msg.coordPerTile || 32,
      msg.chunkSize || 128,
      msg.streamEpoch || 0,
    )
  })

  messageDispatcher.on('playerLeaveWorld', () => {
    gameStore.setPlayerLeaveWorld()
  })

  messageDispatcher.on('chunkLoad', (msg: proto.IS2C_ChunkLoad) => {
    if (msg.chunk) {
      gameStore.loadChunk(
        msg.chunk.coord?.x || 0,
        msg.chunk.coord?.y || 0,
        msg.chunk.tiles instanceof Uint8Array ? msg.chunk.tiles : new Uint8Array(msg.chunk.tiles || []),
        msg.chunk.version || 0,
      )
    }
  })

  messageDispatcher.on('chunkUnload', (msg: proto.IS2C_ChunkUnload) => {
    if (msg.coord) {
      gameStore.unloadChunk(msg.coord.x || 0, msg.coord.y || 0)
    }
  })

  messageDispatcher.on('objectSpawn', (msg: proto.IS2C_ObjectSpawn) => {
    gameStore.spawnEntity({
      entityId: toNumber(msg.entityId!),
      objectType: msg.objectType || 0,
      position: {
        x: msg.position?.position?.x || 0,
        y: msg.position?.position?.y || 0,
      },
      size: {
        x: msg.position?.size?.x || 0,
        y: msg.position?.size?.y || 0,
      },
    })
  })

  messageDispatcher.on('objectDespawn', (msg: proto.IS2C_ObjectDespawn) => {
    gameStore.despawnEntity(toNumber(msg.entityId!))
  })

  messageDispatcher.on('objectMove', (msg: proto.IS2C_ObjectMove) => {
    if (msg.movement) {
      const movement: EntityMovement = {
        position: {
          x: msg.movement.position?.x || 0,
          y: msg.movement.position?.y || 0,
          heading: msg.movement.position?.heading || 0,
        },
        velocity: {
          x: msg.movement.velocity?.x || 0,
          y: msg.movement.velocity?.y || 0,
        },
        moveMode: msg.movement.moveMode || 0,
        isMoving: msg.movement.isMoving || false,
      }

      if (msg.movement.targetPosition) {
        movement.targetPosition = {
          x: msg.movement.targetPosition.x || 0,
          y: msg.movement.targetPosition.y || 0,
        }
      }

      const entityId = toNumber(msg.entityId!)

      // Update player position if this is the player entity
      if (entityId === gameStore.playerEntityId) {
        gameStore.updatePlayerPosition(movement.position)
      }

      gameStore.updateEntityMovement(entityId, movement)
    }
  })

  messageDispatcher.on('error', (msg: proto.IS2C_Error) => {
    console.error('[Game] Server error:', msg.code, msg.message)
  })

  messageDispatcher.on('warning', (msg: proto.IS2C_Warning) => {
    console.warn('[Game] Server warning:', msg.code, msg.message)
  })
}
