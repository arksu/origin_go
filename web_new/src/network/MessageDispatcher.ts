import { proto } from './proto/packets.js'
import { config } from '@/config'

type ServerMessageType = keyof Omit<proto.IServerMessage, 'sequence'>
type MessageHandler<T> = (message: T) => void

interface DebugMessage {
  timestamp: number
  type: string
  message: unknown
}

const DEBUG_BUFFER_SIZE = 100

export class MessageDispatcher {
  private handlers = new Map<ServerMessageType, MessageHandler<unknown>>()
  private debugBuffer: DebugMessage[] = []
  private unknownMessageCount = 0

  on<K extends ServerMessageType>(
    type: K,
    handler: MessageHandler<NonNullable<proto.IServerMessage[K]>>,
  ): void {
    this.handlers.set(type, handler as MessageHandler<unknown>)
  }

  off(type: ServerMessageType): void {
    this.handlers.delete(type)
  }

  dispatch(message: proto.ServerMessage): void {
    const type = this.getMessageType(message)

    if (!type) {
      this.unknownMessageCount++
      console.error('[MessageDispatcher] Unknown message type:', message)
      return
    }

    const payload = message[type]
    if (!payload) {
      return
    }

    if (config.DEBUG) {
      this.addToDebugBuffer(type, payload)
    }

    const handler = this.handlers.get(type)
    if (handler) {
      try {
        handler(payload)
      } catch (err) {
        console.error(`[MessageDispatcher] Handler error for ${type}:`, err)
      }
    } else {
      if (config.DEBUG) {
        console.error(`[MessageDispatcher] No handler for: ${type}`, payload)
      }
    }
  }

  getDebugBuffer(): readonly DebugMessage[] {
    return this.debugBuffer
  }

  getUnknownMessageCount(): number {
    return this.unknownMessageCount
  }

  clearDebugBuffer(): void {
    this.debugBuffer = []
  }

  private getMessageType(message: proto.ServerMessage): ServerMessageType | null {
    if (message.authResult) return 'authResult'
    if (message.pong) return 'pong'
    if (message.chunkLoad) return 'chunkLoad'
    if (message.chunkUnload) return 'chunkUnload'
    if (message.playerEnterWorld) return 'playerEnterWorld'
    if (message.playerLeaveWorld) return 'playerLeaveWorld'
    if (message.objectSpawn) return 'objectSpawn'
    if (message.objectDespawn) return 'objectDespawn'
    if (message.objectMove) return 'objectMove'
    if (message.movementMode) return 'movementMode'
    if (message.inventoryOpResult) return 'inventoryOpResult'
    if (message.inventoryUpdate) return 'inventoryUpdate'
    if (message.containerOpened) return 'containerOpened'
    if (message.containerClosed) return 'containerClosed'
    if (message.chat) return 'chat'
    if (message.contextMenu) return 'contextMenu'
    if (message.miniAlert) return 'miniAlert'
    if (message.cyclicActionProgress) return 'cyclicActionProgress'
    if (message.cyclicActionFinished) return 'cyclicActionFinished'
    if (message.sound) return 'sound'
    if (message.fx) return 'fx'
    if (message.craftList) return 'craftList'
    if (message.characterProfile) return 'characterProfile'
    if (message.playerStats) return 'playerStats'
    if (message.expGained) return 'expGained'
    if (message.error) return 'error'
    if (message.warning) return 'warning'
    return null
  }

  private addToDebugBuffer(type: string, message: unknown): void {
    this.debugBuffer.push({
      timestamp: Date.now(),
      type,
      message,
    })

    if (this.debugBuffer.length > DEBUG_BUFFER_SIZE) {
      this.debugBuffer.shift()
    }
  }
}

export const messageDispatcher = new MessageDispatcher()
