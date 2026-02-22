import { proto } from './proto/packets.js'
import { messageDispatcher } from './MessageDispatcher'
import { useGameStore, type EntityMovement } from '@/stores/gameStore'
import { gameFacade, moveController, playerCommandController, soundManager } from '@/game'
import { DEBUG_MOVEMENT } from '@/constants/game'
import { distanceAttenuation, SoundAttenuationModel } from '@/game/soundAttenuation'

function toNumber(value: number | Long): number {
  if (typeof value === 'number') return value
  return value.toNumber()
}

function distance2D(ax: number, ay: number, bx: number, by: number): number {
  const dx = ax - bx
  const dy = ay - by
  return Math.sqrt((dx * dx) + (dy * dy))
}

const SOUND_ATTENUATION_MODEL = SoundAttenuationModel.Smoothstep

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
    gameStore.markPlayerEnterWorldBootstrap()

    // Set stream epoch for MoveController to validate incoming movement packets
    moveController.setStreamEpoch(streamEpoch, tickRate)

    // Set player ID for command controller (camera target is deferred to objectSpawn
    // to avoid camera sitting at (0,0) before the player entity actually spawns)
    playerCommandController.setPlayerId(toNumber(msg.entityId!))

    gameFacade.setWorldParams(coordPerTile, chunkSize)
  })

  messageDispatcher.on('playerLeaveWorld', () => {
    gameStore.setPlayerLeaveWorld()
    gameFacade.setPlayerEntityId(null)
    gameFacade.resetWorld()
    moveController.clear()
  })

  messageDispatcher.on('characterProfile', (msg: proto.IS2C_CharacterProfile) => {
    gameStore.setCharacterProfileSnapshot(msg)
  })

  messageDispatcher.on('playerStats', (msg: proto.IS2C_PlayerStats) => {
    console.log('[Handlers] playerStats:', {
      stamina: msg.stamina,
      staminaMax: msg.staminaMax,
      energy: msg.energy,
      energyMax: msg.energyMax,
    })
    gameStore.setPlayerStats(msg)
  })

  messageDispatcher.on('fx', (msg: proto.IS2C_Fx) => {
    const targetEntityId = gameStore.playerEntityId
    if (targetEntityId == null) return

    console.log('[Handlers] S2C_Fx received:', {
      fxKey: msg.fxKey,
      position: msg.position ? { x: msg.position.x, y: msg.position.y } : null
    })

    if (msg.fxKey) {
      gameFacade.playFx(targetEntityId, msg.fxKey)
    }
  })

  messageDispatcher.on('craftList', (msg: proto.IS2C_CraftList) => {
    console.log('[Handlers] craftList:', {
      recipes: msg.recipes?.length || 0,
    })
    gameStore.setCraftListSnapshot(msg)
  })

  messageDispatcher.on('expGained', (msg: proto.IS2C_ExpGained) => {
    console.log('[Handlers] S2C_ExpGained received:', {
      entityId: toNumber(msg.entityId || 0),
      lp: toNumber(msg.lp || 0),
      nature: toNumber(msg.nature || 0),
      industry: toNumber(msg.industry || 0),
      combat: toNumber(msg.combat || 0),
    })

    const targetEntityId = toNumber(msg.entityId || 0)
    if (gameStore.playerEntityId == null || targetEntityId !== gameStore.playerEntityId) {
      return
    }

    gameStore.applyExpGained(msg)
  })

  messageDispatcher.on('movementMode', (msg: proto.IS2C_MovementMode) => {
    const entityId = toNumber(msg.entityId!)
    const movementMode = msg.movementMode || 0
    gameStore.setPlayerMoveMode(entityId, movementMode)
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
      gameStore.markBootstrapFirstChunkLoaded()
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

    // console.log(`[Handlers] objectSpawn: entityId=${entityId}, type=${msg.typeId}, resource="${resourcePath}", pos=(${posX}, ${posY}), playerEntityId=${gameStore.playerEntityId}`)

    const objectData = {
      entityId,
      typeId: msg.typeId || 0,
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

    // If this is the player entity, set camera target and position together
    // to avoid the camera being at (0,0) between playerEnterWorld and objectSpawn
    if (entityId === gameStore.playerEntityId) {
      console.log(`[Handlers] Player entity spawned: entityId=${entityId}, pos=(${posX}, ${posY})`)
      gameFacade.setPlayerEntityId(entityId)
      gameFacade.setCamera(posX, posY)
      gameStore.markBootstrapPlayerSpawned()
    }
  })

  messageDispatcher.on('objectDespawn', (msg: proto.IS2C_ObjectDespawn) => {
    const entityId = toNumber(msg.entityId!)
    // console.log(`[Handlers] objectDespawn: entityId=${entityId}`)
    gameStore.despawnEntity(entityId)
    gameFacade.despawnObject(entityId)
    moveController.removeEntity(entityId)
  })

  messageDispatcher.on('objectMove', (msg: proto.IS2C_ObjectMove) => {
    const entityId = toNumber(msg.entityId!)

    if (!msg.movement) return

    const serverTimeMs = Number(msg.serverTimeMs || 0)
    const moveSeq = msg.moveSeq || 0
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

  messageDispatcher.on('inventoryOpResult', (msg: proto.IS2C_InventoryOpResult) => {
    if (!msg.success) {
      console.warn('[Handlers] inventoryOpResult FAILED:', {
        opId: msg.opId,
        error: msg.error,
        message: msg.message,
      })

      const message = (msg.message || '').trim()
      const errorCode = Number(msg.error || 0)
      const errorName = proto.ErrorCode[errorCode] || 'INVENTORY_OP_FAILED'
      const reasonCode = errorName === 'ERROR_CODE_NONE' ? 'INVENTORY_OP_FAILED' : errorName

      gameStore.pushMiniAlert({
        reasonCode,
        message: message || undefined,
        severity: proto.AlertSeverity.ALERT_SEVERITY_ERROR,
        ttlMs: 0,
      })
    } else {
      console.log('[Handlers] inventoryOpResult OK:', { msg: msg })
    }

    if (msg.updated) {
      for (const inventoryState of msg.updated) {
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

  messageDispatcher.on('contextMenu', (msg: proto.IS2C_ContextMenu) => {
    console.log('[Handlers] contextMenu RAW:', {
      entityId: msg.entityId,
      actions: msg.actions,
      actionsCount: msg.actions?.length || 0,
    })

    if (!msg.entityId || !msg.actions || msg.actions.length === 0) {
      console.log('[Handlers] contextMenu ignored: empty payload')
      return
    }

    const normalizedActions = msg.actions.map((a) => ({
      actionId: a.actionId || '',
      title: a.title || a.actionId || '',
    })).filter((a) => a.actionId !== '')

    console.log('[Handlers] contextMenu NORMALIZED:', {
      entityId: toNumber(msg.entityId),
      actions: normalizedActions,
      actionsCount: normalizedActions.length,
    })

    gameStore.openContextMenu(
      toNumber(msg.entityId),
      normalizedActions,
    )
  })

  messageDispatcher.on('miniAlert', (msg: proto.IS2C_MiniAlert) => {
    const reasonCode = (msg.reasonCode || '').trim()
    if (!reasonCode) {
      return
    }
    gameStore.pushMiniAlert({
      reasonCode,
      severity: msg.severity ?? proto.AlertSeverity.ALERT_SEVERITY_INFO,
      ttlMs: msg.ttlMs ? Number(msg.ttlMs) : 0,
    })
  })

  messageDispatcher.on('cyclicActionProgress', (msg: proto.IS2C_CyclicActionProgress) => {
    const totalTicks = Number(msg.totalTicks || 0)
    const elapsedTicks = Number(msg.elapsedTicks || 0)
    if (totalTicks <= 0) {
      gameStore.clearActionProgress()
      return
    }
    gameStore.setActionProgress(totalTicks, elapsedTicks)
  })

  messageDispatcher.on('cyclicActionFinished', (msg: proto.IS2C_CyclicActionFinished) => {
    console.log('[Handlers] cyclicActionFinished:', {
      actionId: msg.actionId,
      targetEntityId: msg.targetEntityId,
      cycleIndex: msg.cycleIndex,
      result: msg.result,
      reasonCode: msg.reasonCode,
    })
    gameStore.clearActionProgress()
  })

  messageDispatcher.on('sound', (msg: proto.IS2C_Sound) => {
    console.log('[Handlers] S2C_Sound received:', {
      soundKey: msg.soundKey,
      maxHearDistance: msg.maxHearDistance,
      x: msg.x,
      y: msg.y
    })

    const soundKey = (msg.soundKey || '').trim()
    if (!soundKey) {
      return
    }

    const maxHearDistance = Number(msg.maxHearDistance || 0)
    const sourceX = Number(msg.x || 0)
    const sourceY = Number(msg.y || 0)
    const playerPosition = gameStore.playerPosition
    const distance = distance2D(playerPosition.x, playerPosition.y, sourceX, sourceY)
    const attenuation = distanceAttenuation(distance, maxHearDistance, SOUND_ATTENUATION_MODEL)
    if (attenuation <= 0) {
      return
    }

    console.log('[Handlers] sound -> play', {
      soundKey,
      sourceX,
      sourceY,
      maxHearDistance,
      distance,
      attenuation,
    })
    soundManager.play(soundKey, attenuation)
  })

  messageDispatcher.on('error', (msg: proto.IS2C_Error) => {
    console.error('[Game] Server error:', msg.code, msg.message)
    const code = String(msg.code ?? '').trim()
    const message = (msg.message || '').trim()
    if (message) {
      gameStore.setLastServerErrorMessage(message)
    }
    gameStore.pushMiniAlert({
      reasonCode: code || message || 'SERVER_ERROR',
      message: message || undefined,
      severity: proto.AlertSeverity.ALERT_SEVERITY_ERROR,
      ttlMs: 0,
    })
  })

  messageDispatcher.on('warning', (msg: proto.IS2C_Warning) => {
    console.warn('[Game] Server warning:', msg.code, msg.message)
    const code = String(msg.code ?? '').trim()
    const message = (msg.message || '').trim()
    gameStore.pushMiniAlert({
      reasonCode: code || message || 'SERVER_WARNING',
      message: message || undefined,
      severity: proto.AlertSeverity.ALERT_SEVERITY_WARNING,
      ttlMs: 0,
    })
  })
}
