import { gameConnection } from './GameConnection'
import { messageDispatcher } from './MessageDispatcher'
import { registerMessageHandlers } from './handlers'
import { useGameStore } from '@/stores/gameStore'
import { gameFacade, moveController } from '@/game'
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
      gameFacade.resetWorld()
      moveController.clear()
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

export function sendMovementMode(mode: proto.MovementMode): void {
  gameConnection.send({
    movementMode: proto.C2S_MovementMode.create({
      mode,
    }),
  })
}

export function sendOpenWindow(name: string): void {
  const normalized = name.trim()
  if (!normalized) return

  gameConnection.send({
    openWindow: proto.C2S_OpenWindow.create({
      name: normalized,
    }),
  })
}

export function sendCloseWindow(name: string): void {
  const normalized = name.trim()
  if (!normalized) return

  gameConnection.send({
    closeWindow: proto.C2S_CloseWindow.create({
      name: normalized,
    }),
  })
}

export function sendStartCraftOne(craftKey: string): void {
  const normalized = craftKey.trim()
  if (!normalized) return

  gameConnection.send({
    startCraftOne: proto.C2S_StartCraftOne.create({
      craftKey: normalized,
    }),
  })
}

export function sendStartCraftMany(craftKey: string, cycles: number): void {
  const normalized = craftKey.trim()
  if (!normalized) return

  const safeCycles = Math.max(1, Math.floor(cycles))
  gameConnection.send({
    startCraftMany: proto.C2S_StartCraftMany.create({
      craftKey: normalized,
      cycles: safeCycles,
    }),
  })
}

export function sendStartBuild(buildKey: string, pos: { x: number; y: number }): void {
  const normalized = buildKey.trim()
  if (!normalized) return

  const x = Math.trunc(pos.x)
  const y = Math.trunc(pos.y)
  if (!Number.isFinite(x) || !Number.isFinite(y)) return

  gameConnection.send({
    buildStart: proto.C2S_BuildStart.create({
      buildKey: normalized,
      pos: proto.Vector2.create({
        x,
        y,
      }),
    }),
  })
}

export { gameConnection, messageDispatcher }
