import { gameConnection } from './GameConnection'
import { messageDispatcher } from './MessageDispatcher'
import { registerMessageHandlers } from './handlers'
import { useGameStore } from '@/stores/gameStore'

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

export { gameConnection, messageDispatcher }
