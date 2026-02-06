import { gameConnection } from './GameConnection'
import { messageDispatcher } from './MessageDispatcher'
import { registerMessageHandlers } from './handlers'
import { useGameStore } from '@/stores/gameStore'
import { proto } from './proto/packets.js'

let initialized = false

export function initNetwork(): void {
  if (initialized) return
  initialized = true

  const gameStore = useGameStore()

  // Register message handlers
  registerMessageHandlers()

  // Connect dispatcher to connection
  gameConnection.onMessage((message) => {
    messageDispatcher.dispatch(message)
  })

  // Sync connection state to store
  gameConnection.onStateChange((state, error) => {
    gameStore.setConnectionState(state, error)

    if (state === 'disconnected' || state === 'error') {
      gameStore.setPlayerLeaveWorld()
    }
  })
}

export function connectToGame(authToken: string): void {
  initNetwork()
  gameConnection.connect(authToken)
}

export function disconnectFromGame(): void {
  gameConnection.disconnect()
}

export function sendChatMessage(text: string): void {
  if (!text.trim()) return

  gameConnection.send({
    chat: proto.C2S_ChatMessage.create({
      text: text.trim(),
      channel: proto.ChatChannel.CHAT_CHANNEL_LOCAL
    })
  })
}

export function sendInventoryOp(op: proto.IInventoryOp): void {
  gameConnection.send({
    inventoryOp: proto.C2S_InventoryOp.create({ op })
  })
}

export function sendOpenContainer(ref: proto.IInventoryRef): void {
  gameConnection.send({
    openContainer: proto.C2S_OpenContainer.create({ ref })
  })
}

export function sendCloseContainer(ref: proto.IInventoryRef): void {
  gameConnection.send({
    closeContainer: proto.C2S_CloseContainer.create({ ref })
  })
}

export { gameConnection, messageDispatcher }
