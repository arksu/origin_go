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

  sendPlayerAction(worldX, worldY, layer = 0, modifiers = 0) {
    console.debug('Sending player action:', { worldX, worldY, layer, modifiers })
    const playerActionMsg = proto.C2S_PlayerAction.create({
      moveTo: proto.MoveTo.create({
        x: worldX,
        y: worldY,
        layer: layer
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

      if (message.authResult) {
        this.handleAuthResult(message.authResult)
      } else if (message.pong) {
        this.handlePong(message.pong)
      } else if (message.playerEnterWorld) {
        this.handlePlayerEnterWorld(message.playerEnterWorld)
      } else if (message.loadChunk) {
        this.handleLoadChunk(message.loadChunk)
      } else if (message.unloadChunk) {
        this.handleUnloadChunk(message.unloadChunk)
      } else if (message.error) {
        this.handleError(message.error)
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
      gameStore.setError(authResult.errorMessage || 'Authentication failed')
      this.disconnect()
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

  handleLoadChunk(loadChunk) {
    const gameStore = useGameStore()
    if (loadChunk.chunk) {
      console.log('Chunk loaded:', loadChunk.chunk.coord, 'tiles length:', loadChunk.chunk.tiles?.length)
      gameStore.addChunk(loadChunk.chunk.coord, loadChunk.chunk.tiles)
    }
  }

  handleUnloadChunk(unloadChunk) {
    const gameStore = useGameStore()
    gameStore.removeChunk(unloadChunk.coord)
    console.log('Chunk unloaded:', unloadChunk.coord)
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
