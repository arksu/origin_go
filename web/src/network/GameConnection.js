import { proto } from '../proto/packets.js'
import { networkConfig } from '../config/network.js'
import { useGameStore } from '../stores/game.js'
import { useRouter } from 'vue-router'

class GameConnection {
  constructor() {
    this.ws = null
    this.pingInterval = null
    this.sequenceNumber = 0
    this.handlers = new Map()
    this.router = null
  }

  setRouter(router) {
    this.router = router
  }

  connect() {
    const gameStore = useGameStore()

    if (this.ws) {
      this.disconnect()
    }

    gameStore.setConnectionState('connecting')

    this.ws = new WebSocket(networkConfig.wsUrl)
    this.ws.binaryType = 'arraybuffer'

    this.ws.onopen = () => {
      this.sendAuth()
    }

    this.ws.onmessage = (event) => {
      this.handleMessage(event.data)
    }

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error)
      gameStore.setError('Connection error')
    }

    this.ws.onclose = () => {
      this.stopPing()
      gameStore.setConnectionState('disconnected')
    }
  }

  disconnect() {
    this.stopPing()
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    const gameStore = useGameStore()
    gameStore.reset()
  }

  sendAuth() {
    const gameStore = useGameStore()
    const authMsg = proto.C2S_Auth.create({
      token: gameStore.wsToken,
      clientVersion: networkConfig.clientVersion
    })

    this.send({ auth: authMsg })
  }

  sendPlayerAction(worldX, worldY, modifiers = 0) {
    console.debug('Sending player action:', { worldX, worldY, modifiers })
    const playerActionMsg = proto.C2S_PlayerAction.create({
      moveTo: proto.MoveTo.create({
        x: worldX,
        y: worldY,
      }),
      modifiers: modifiers
    })

    this.send({ playerAction: playerActionMsg })
  }

  send(payload) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn('WebSocket not connected')
      return
    }

    const message = proto.ClientMessage.create({
      sequence: ++this.sequenceNumber,
      ...payload
    })

    const buffer = proto.ClientMessage.encode(message).finish()
    this.ws.send(buffer)
  }

  handleMessage(data) {
    const gameStore = useGameStore()

    try {
      const buffer = new Uint8Array(data)
      const message = proto.ServerMessage.decode(buffer)
      // console.debug('Received message:', message)

      if (message.authResult) {
        this.handleAuthResult(message.authResult)
      } else if (message.pong) {
        this.handlePong(message.pong)
      } else if (message.playerEnterWorld) {
        this.handlePlayerEnterWorld(message.playerEnterWorld)
      } else if (message.chunkLoad) {
        this.handleChunkLoad(message.chunkLoad)
      } else if (message.chunkUnload) {
        this.handleChunkUnload(message.chunkUnload)
      } else if (message.objectSpawn) {
        this.handleObject(message.objectSpawn)
      } else if (message.objectDespawn) {
        this.handleObjectDespawn(message.objectDespawn)
      } else if (message.objectMove) {
        this.handleObjectMove(message.objectMove)
      } else if (message.error) {
        this.handleError(message.error)
      } else {
        console.warn("Unknown message type:", message)
      }

      // Notify registered handlers
      const payloadType = message.payload
      if (payloadType && this.handlers.has(payloadType)) {
        this.handlers.get(payloadType)(message[payloadType])
      }
    } catch (error) {
      console.error('Failed to decode message:', error)
      gameStore.setError('Protocol error')
    }
  }

  handleAuthResult(authResult) {
    const gameStore = useGameStore()

    if (authResult.success) {
      gameStore.setConnectionState('connected')
      gameStore.setPlayerState(authResult.playerState)
      this.startPing()
    } else {
      console.error('Authentication failed:', authResult.errorMessage)
      gameStore.setError(authResult.errorMessage || 'Authentication failed')
      this.disconnect()
      if (this.router) {
        this.router.push('/characters')
      }
    }
  }

  handlePong(pong) {
    const latency = Date.now() - Number(pong.clientTimeMs)
    console.debug(`Pong received, latency: ${latency}ms, server time: ${pong.serverTimeMs}`)
  }

  startPing() {
    this.stopPing()
    this.pingInterval = setInterval(() => {
      this.sendPing()
    }, networkConfig.pingIntervalMs)
  }

  stopPing() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
  }

  sendPing() {
    const pingMsg = proto.C2S_Ping.create({
      clientTimeMs: Date.now()
    })
    this.send({ ping: pingMsg })
  }

  handlePlayerEnterWorld(enterWorld) {
    const gameStore = useGameStore()
    gameStore.setPlayerPosition({
      x: enterWorld.movement?.position?.x || 0,
      y: enterWorld.movement?.position?.y || 0,
      heading: enterWorld.movement?.position?.heading || 0
    })
    gameStore.setWorldReady(true)
    console.log('Player entered world:', enterWorld)
  }

  handleChunkLoad(chunkLoad) {
    const gameStore = useGameStore()
    if (chunkLoad.chunk) {
      console.log('Chunk loaded:', chunkLoad.chunk.coord, 'tiles length:', chunkLoad.chunk.tiles?.length)
      gameStore.addChunk(chunkLoad.chunk.coord, chunkLoad.chunk.tiles)
    }
  }

  handleChunkUnload(chunkUnload) {
    const gameStore = useGameStore()
    gameStore.removeChunk(chunkUnload.coord)
    console.log('Chunk unloaded:', chunkUnload.coord)
  }

  handleObject(object) {
    const gameStore = useGameStore()

    // Check if this is the player's entity
    if (object.entityId === gameStore.characterId) {
      // Update player position from initial object data
      if (object.position && object.position.position) {
        const newPosition = {
          x: object.position.position.x || 0,
          y: object.position.position.y || 0,
          heading: object.position.position.heading || 0
        }
        gameStore.setPlayerPosition(newPosition)
        console.log('Set initial player position:', newPosition)
      }

      // Store player size from collider
      if (object.position && object.position.size) {
        const playerSize = {
          x: object.position.size.x || 10,
          y: object.position.size.y || 10
        }
        gameStore.setPlayerSize(playerSize)
        console.log('Set player size:', playerSize)
      }
      return // Don't add player as a game object
    }

    // Add object type to the game object for rendering
    const gameObject = {
      ...object,
      objectType: object.objectType || 0
    }

    gameStore.addGameObject(object.entityId, gameObject)
    console.log('Added game object:', object.entityId, 'type:', object.objectType)
  }

  handleObjectMove(objectMove) {
    const gameStore = useGameStore()
    // console.log('Object move received:', objectMove)

    // Check if this is the player's entity
    if (objectMove.entityId === gameStore.characterId) {
      // Update player position
      if (objectMove.movement && objectMove.movement.position) {
        const newPosition = {
          x: objectMove.movement.position.x || 0,
          y: objectMove.movement.position.y || 0,
          heading: objectMove.movement.position.heading || 0
        }
        gameStore.setPlayerPosition(newPosition)
        console.log('Updated player position:', newPosition)
      }

      // Store player movement data for target rendering
      gameStore.setPlayerMovement(objectMove.movement)
      console.log('Updated player movement:', objectMove.movement)

      return // Don't add player as a game object
    }

    // Update existing game object with new movement data
    const existingObject = gameStore.gameObjects.get(objectMove.entityId)
    if (existingObject) {
      // Merge the movement data with existing object data
      const updatedObject = {
        ...existingObject,
        movement: objectMove.movement
      }
      gameStore.updateGameObject(objectMove.entityId, updatedObject)
    } else {
      // If object doesn't exist, create it with basic info
      const newObject = {
        entityId: objectMove.entityId,
        objectType: objectMove.objectType || 0,
        movement: objectMove.movement,
        position: objectMove.movement.position
      }
      gameStore.addGameObject(objectMove.entityId, newObject)
    }
  }

  handleObjectDespawn(objectDespawn) {
    const gameStore = useGameStore()
    console.debug("objectDespawn", objectDespawn)

    // Check if this is the player's entity (shouldn't happen, but handle gracefully)
    if (objectDespawn.entityId === gameStore.characterId) {
      console.warn('Received despawn for player entity:', objectDespawn.entityId)
      return // Don't remove player entity
    }

    // Check if object exists before removing
    if (gameStore.gameObjects.has(objectDespawn.entityId)) {
      gameStore.removeGameObject(objectDespawn.entityId)
    } else {
      console.warn('Attempted to remove non-existent game object:', objectDespawn.entityId, objectDespawn)
    }
  }

  handleError(error) {
    const gameStore = useGameStore()
    const errorMessage = error.message || 'An error occurred'
    console.error('Server error:', error.code, errorMessage)
    gameStore.setError(errorMessage)
    this.disconnect()
    if (this.router) {
      this.router.push('/characters')
    }
  }

  onMessage(type, handler) {
    this.handlers.set(type, handler)
  }

  offMessage(type) {
    this.handlers.delete(type)
  }
}

export const gameConnection = new GameConnection()
