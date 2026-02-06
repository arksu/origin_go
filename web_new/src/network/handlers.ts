import { proto } from './proto/packets.js'
import { messageDispatcher } from './MessageDispatcher'
import { useGameStore, type EntityMovement } from '@/stores/gameStore'
import { gameFacade, moveController } from '@/game'
import { DEBUG_MOVEMENT } from '@/constants/game'

function toNumber(value: number | Long): number {
  if (typeof value === 'number') return value
  return value.toNumber()
}

export function registerMessageHandlers(): void {
  const gameStore = useGameStore()

  messageDispatcher.on('playerEnterWorld', (msg: proto.IS2C_PlayerEnterWorld) => {
    const coordPerTile = msg.coordPerTile || 32
    const chunkSize = msg.chunkSize || 128
    const streamEpoch = msg.streamEpoch || 0
    const tickRate = msg.tickRate || 10 // Default to 10 ticks/sec

    console.log(`[Handlers] playerEnterWorld: coordPerTile=${coordPerTile}, chunkSize=${chunkSize}, streamEpoch=${streamEpoch}, tickRate=${tickRate}`)

    gameStore.setPlayerEnterWorld(
      toNumber(msg.entityId!),
      msg.name || '',
      coordPerTile,
      chunkSize,
      streamEpoch,
    )

    // Set stream epoch for MoveController to validate incoming movement packets
    moveController.setStreamEpoch(streamEpoch, tickRate)

    // Set player entity ID for camera following
    gameFacade.setPlayerEntityId(toNumber(msg.entityId!))

    gameFacade.setWorldParams(coordPerTile, chunkSize)
  })

  messageDispatcher.on('playerLeaveWorld', () => {
    gameStore.setPlayerLeaveWorld()
    gameFacade.setPlayerEntityId(null)
    moveController.clear()
  })

  messageDispatcher.on('chunkLoad', (msg: proto.IS2C_ChunkLoad) => {
    if (msg.chunk) {
      const x = msg.chunk.coord?.x || 0
      const y = msg.chunk.coord?.y || 0
      const tiles = msg.chunk.tiles instanceof Uint8Array ? msg.chunk.tiles : new Uint8Array(msg.chunk.tiles || [])

      console.log(`[Handlers] chunkLoad: x=${x}, y=${y}, tiles.length=${tiles.length}`)

      const version = msg.chunk.version || 0
      gameStore.loadChunk(x, y, tiles, version)
      gameFacade.loadChunk(x, y, tiles, version)
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
    const heading = msg.position?.position?.heading || 0
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

    // Initialize entity in MoveController for smooth movement
    moveController.initEntity(entityId, posX, posY, heading)

    // If this is the player entity, set initial camera position
    if (entityId === gameStore.playerEntityId) {
      console.log(`[Handlers] Player spawned at game(${posX}, ${posY})`)
      gameFacade.setCamera(posX, posY)
    }
  })

  messageDispatcher.on('objectDespawn', (msg: proto.IS2C_ObjectDespawn) => {
    const entityId = toNumber(msg.entityId!)
    console.log(`[Handlers] objectDespawn: entityId=${entityId}`)
    gameStore.despawnEntity(entityId)
    gameFacade.despawnObject(entityId)
    moveController.removeEntity(entityId)
  })

  messageDispatcher.on('objectMove', (msg: proto.IS2C_ObjectMove) => {
    const entityId = toNumber(msg.entityId!)

    if (!msg.movement) return

    const serverTimeMs = Number(msg.serverTimeMs || 0)
    const moveSeq = msg.moveSeq || 0
    const streamEpoch = msg.streamEpoch || 0
    const isTeleport = msg.isTeleport || false

    const x = msg.movement.position?.x || 0
    const y = msg.movement.position?.y || 0
    const heading = msg.movement.position?.heading || 0
    const vx = msg.movement.velocity?.x || 0
    const vy = msg.movement.velocity?.y || 0
    const isMoving = msg.movement.isMoving || false
    const moveMode = msg.movement.moveMode || 0

    // Log every objectMove packet for debugging
    if (DEBUG_MOVEMENT) {
      console.log(`[Handlers] IS2C_ObjectMove:`, {
        entityId,
        serverTimeMs,
        moveSeq,
        streamEpoch,
        isTeleport,
        position: `(${x}, ${y})`,
        velocity: `(${vx}, ${vy})`,
        isMoving,
        moveMode,
        heading,
        timestamp: Date.now(),
      })
    }

    // Feed movement data to MoveController for interpolation
    moveController.onObjectMove(
      entityId,
      serverTimeMs,
      moveSeq,
      streamEpoch,
      isTeleport,
      x, y,
      vx, vy,
      isMoving,
      moveMode,
      heading,
    )

    // Update store with server data (source of truth)
    const movement: EntityMovement = {
      position: { x, y, heading },
      velocity: { x: vx, y: vy },
      moveMode,
      isMoving,
    }

    if (msg.movement.targetPosition) {
      movement.targetPosition = {
        x: msg.movement.targetPosition.x || 0,
        y: msg.movement.targetPosition.y || 0,
      }
    }

    gameStore.updateEntityMovement(entityId, movement)

    // Update player position in store if this is the player entity
    if (entityId === gameStore.playerEntityId) {
      gameStore.updatePlayerPosition(movement.position)
    }
  })

  messageDispatcher.on('inventoryUpdate', (msg: proto.IS2C_InventoryUpdate) => {
    console.log('[Handlers] inventoryUpdate:', msg.updated?.length || 0, 'inventories')

    if (msg.updated) {
      for (const inventoryState of msg.updated) {
        console.log('[Handlers] Full inventory state:', {
          ref: inventoryState.ref,
          revision: inventoryState.revision,
          hasGrid: !!inventoryState.grid,
          hasEquipment: !!inventoryState.equipment,
          hasHand: !!inventoryState.hand,
          grid: inventoryState.grid,
          equipment: inventoryState.equipment,
          hand: inventoryState.hand
        })
        gameStore.updateInventory(inventoryState)
      }
    }
  })

  messageDispatcher.on('containerOpened', (msg: proto.IS2C_ContainerOpened) => {
    console.log('[Handlers] containerOpened:', msg.state)
    if (msg.state) {
      gameStore.onContainerOpened(msg.state)
    }
  })

  messageDispatcher.on('containerClosed', (msg: proto.IS2C_ContainerClosed) => {
    console.log('[Handlers] containerClosed:', msg.ref)
    if (msg.ref) {
      gameStore.onContainerClosed(msg.ref)
    }
  })

  messageDispatcher.on('chat', (msg: proto.IS2C_ChatMessage) => {
    console.log('[Game] Chat message:', msg.fromName, msg.text, msg.channel)

    // Add message to store
    gameStore.addChatMessage(
      msg.fromName || 'Unknown',
      msg.text || '',
      msg.channel || proto.ChatChannel.CHAT_CHANNEL_LOCAL
    )
  })

  messageDispatcher.on('error', (msg: proto.IS2C_Error) => {
    console.error('[Game] Server error:', msg.code, msg.message)
  })

  messageDispatcher.on('warning', (msg: proto.IS2C_Warning) => {
    console.warn('[Game] Server warning:', msg.code, msg.message)
  })
}
