import { proto } from './proto/packets.js'
import { messageDispatcher } from './MessageDispatcher'
import { useGameStore, type EntityMovement } from '@/stores/gameStore'
import { gameFacade, coordGame2Screen } from '@/game'

function toNumber(value: number | Long): number {
  if (typeof value === 'number') return value
  return value.toNumber()
}

export function registerMessageHandlers(): void {
  const gameStore = useGameStore()

  messageDispatcher.on('playerEnterWorld', (msg: proto.IS2C_PlayerEnterWorld) => {
    const coordPerTile = msg.coordPerTile || 32
    const chunkSize = msg.chunkSize || 128

    console.log(`[Handlers] playerEnterWorld: coordPerTile=${coordPerTile}, chunkSize=${chunkSize}`)

    gameStore.setPlayerEnterWorld(
      toNumber(msg.entityId!),
      msg.name || '',
      coordPerTile,
      chunkSize,
      msg.streamEpoch || 0,
    )

    gameFacade.setWorldParams(coordPerTile, chunkSize)
  })

  messageDispatcher.on('playerLeaveWorld', () => {
    gameStore.setPlayerLeaveWorld()
  })

  messageDispatcher.on('chunkLoad', (msg: proto.IS2C_ChunkLoad) => {
    if (msg.chunk) {
      const x = msg.chunk.coord?.x || 0
      const y = msg.chunk.coord?.y || 0
      const tiles = msg.chunk.tiles instanceof Uint8Array ? msg.chunk.tiles : new Uint8Array(msg.chunk.tiles || [])

      console.log(`[Handlers] chunkLoad: x=${x}, y=${y}, tiles.length=${tiles.length}`)

      gameStore.loadChunk(x, y, tiles, msg.chunk.version || 0)
      gameFacade.loadChunk(x, y, tiles)
    }
  })

  messageDispatcher.on('chunkUnload', (msg: proto.IS2C_ChunkUnload) => {
    if (msg.coord) {
      const x = msg.coord.x || 0
      const y = msg.coord.y || 0
      console.log(`[Handlers] chunkUnload: x=${x}, y=${y}`)
      gameStore.unloadChunk(x, y)
      gameFacade.unloadChunk(x, y)
    }
  })

  messageDispatcher.on('objectSpawn', (msg: proto.IS2C_ObjectSpawn) => {
    const entityId = toNumber(msg.entityId!)
    const posX = msg.position?.position?.x || 0
    const posY = msg.position?.position?.y || 0
    const resourcePath = msg.resourcePath || ''

    console.log(`[Handlers] objectSpawn: entityId=${entityId}, type=${msg.objectType}, resource="${resourcePath}", pos=(${posX}, ${posY}), playerEntityId=${gameStore.playerEntityId}`)

    const objectData = {
      entityId,
      objectType: msg.objectType || 0,
      resourcePath,
      position: { x: posX, y: posY },
      size: {
        x: msg.position?.size?.x || 0,
        y: msg.position?.size?.y || 0,
      },
    }

    gameStore.spawnEntity(objectData)
    gameFacade.spawnObject(objectData)

    // If this is the player entity, set initial camera position
    if (entityId === gameStore.playerEntityId) {
      const screenPos = coordGame2Screen(posX, posY)
      console.log(`[Handlers] Player spawned at game(${posX}, ${posY}) -> screen(${screenPos.x.toFixed(0)}, ${screenPos.y.toFixed(0)})`)
      gameFacade.setCamera(screenPos.x, screenPos.y)
    }
  })

  messageDispatcher.on('objectDespawn', (msg: proto.IS2C_ObjectDespawn) => {
    const entityId = toNumber(msg.entityId!)
    console.log(`[Handlers] objectDespawn: entityId=${entityId}`)
    gameStore.despawnEntity(entityId)
    gameFacade.despawnObject(entityId)
  })

  messageDispatcher.on('objectMove', (msg: proto.IS2C_ObjectMove) => {
    const entityId = toNumber(msg.entityId!)
    console.log(`[Handlers] objectMove: entityId=${entityId}, playerEntityId=${gameStore.playerEntityId}, hasMovement=${!!msg.movement}`)

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

        // Update camera to follow player
        const screenPos = coordGame2Screen(movement.position.x, movement.position.y)
        console.log(`[Handlers] Player moved: game(${movement.position.x}, ${movement.position.y}) -> screen(${screenPos.x.toFixed(0)}, ${screenPos.y.toFixed(0)})`)
        gameFacade.setCamera(screenPos.x, screenPos.y)
      }

      gameStore.updateEntityMovement(entityId, movement)
      gameFacade.updateObjectPosition(entityId, movement.position.x, movement.position.y)
    }
  })

  messageDispatcher.on('error', (msg: proto.IS2C_Error) => {
    console.error('[Game] Server error:', msg.code, msg.message)
  })

  messageDispatcher.on('warning', (msg: proto.IS2C_Warning) => {
    console.warn('[Game] Server warning:', msg.code, msg.message)
  })
}
