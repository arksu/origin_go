/**
 * PlayerCommandController - sends player commands to the server.
 * 
 * Responsibilities:
 * - Send MapClick input with coordinates and the object under the pointer
 * - Include modifiers (Shift/Ctrl/Alt) with commands
 */

import { useGameStore } from '@/stores/gameStore'
import { combatAttempt } from '@/types/combat'
import { gameConnection } from '@/network/GameConnection'
import { sendInventoryOp } from '@/network'
import { proto } from '@/network/proto/packets.js'
import { DEBUG_MOVEMENT } from '@/constants/game'
import { moveController } from './MoveController'
import { gameFacade } from './GameFacade'

export class PlayerCommandController {
  private playerId: number | null = null

  setPlayerId(playerId: number): void {
    this.playerId = playerId
  }

  sendMoveDirection(x: number, y: number, inputRevision: number, streamEpoch: number): void {
    gameConnection.send({
      playerAction: proto.C2S_PlayerAction.create({
        moveDirection: proto.MoveDirection.create({ x, y, inputRevision, streamEpoch }),
      }),
    })
  }

  sendMapClick(
    x: number,
    y: number,
    targetEntityId: number,
    modifiers: number,
    button: proto.MapClickButton = proto.MapClickButton.MAP_CLICK_BUTTON_PRIMARY,
  ): void {
    if (DEBUG_MOVEMENT) {
      let currentPos = 'unknown'
      if (this.playerId !== null) {
        const pos = moveController.getRenderPosition(this.playerId)
        if (pos) {
          currentPos = `(${pos.x.toFixed(2)}, ${pos.y.toFixed(2)})`
        }
      }

      console.log(`[PlayerCommandController] Sending MapClick:`, {
        currentPos,
        target: `(${Math.round(x)}, ${Math.round(y)})`,
        modifiers,
        button,
        timestamp: Date.now(),
      })
    }

    const store = useGameStore()
    const state = store.gameActionState
    const definition = store.gameActions.find(action => action.id === state.actionId)
    const attempt = button === proto.MapClickButton.MAP_CLICK_BUTTON_PRIMARY
      ? combatAttempt(state, definition, store.worldParams?.combatSupported === true, store.combat)
      : undefined
    gameConnection.send({
      playerAction: proto.C2S_PlayerAction.create({
        mapClick: proto.MapClick.create({
          x: Math.round(x),
          y: Math.round(y),
          targetEntityId,
          button,
          ...(attempt ? { combatAttempt: proto.CombatAttempt.fromObject(attempt) } : {}),
        }),
        modifiers,
      }),
    })
  }

  sendSelectContextAction(entityId: number, actionId: string): void {
    if (!actionId) {
      return
    }

    gameFacade.releaseKeyboardMovement()
    gameConnection.send({
      playerAction: proto.C2S_PlayerAction.create({
        selectContextAction: proto.SelectContextAction.create({
          entityId,
          actionId,
        }),
      }),
    })
  }

  sendDropToWorld(
    handRef: proto.IInventoryRef,
    handRevision: number,
    itemId: number | Long,
    opId: number,
  ): void {
    console.log(`[PlayerCommandController] Sending DropToWorld:`, {
      handRef,
      itemId,
      opId,
      timestamp: Date.now(),
    })

    sendInventoryOp({
      opId,
      expected: [
        { ref: handRef, expectedRevision: handRevision },
      ],
      dropToWorld: {
        src: handRef,
        itemId,
      },
    })
  }
}

export const playerCommandController = new PlayerCommandController()
