/**
 * PlayerCommandController - sends player commands to the server.
 * 
 * Responsibilities:
 * - Send MoveTo commands (click on ground)
 * - Send MoveToEntity commands (click on object)
 * - Include modifiers (Shift/Ctrl/Alt) with commands
 */

import { gameConnection } from '@/network/GameConnection'
import { proto } from '@/network/proto/packets.js'
import { DEBUG_MOVEMENT } from '@/constants/game'
import { moveController } from './MoveController'

export class PlayerCommandController {
  private playerId: number | null = null

  setPlayerId(playerId: number): void {
    this.playerId = playerId
  }

  sendMoveTo(x: number, y: number, modifiers: number): void {
    if (DEBUG_MOVEMENT) {
      let currentPos = 'unknown'
      if (this.playerId !== null) {
        const pos = moveController.getRenderPosition(this.playerId)
        if (pos) {
          currentPos = `(${pos.x.toFixed(2)}, ${pos.y.toFixed(2)})`
        }
      }

      console.log(`[PlayerCommandController] Sending MoveTo:`, {
        currentPos,
        target: `(${Math.round(x)}, ${Math.round(y)})`,
        modifiers,
        timestamp: Date.now(),
      })
    }

    gameConnection.send({
      playerAction: proto.C2S_PlayerAction.create({
        moveTo: proto.MoveTo.create({
          x: Math.round(x),
          y: Math.round(y),
        }),
        modifiers,
      }),
    })
  }

  sendMoveToEntity(entityId: number, autoInteract: boolean, modifiers: number): void {
    if (DEBUG_MOVEMENT) {
      console.log(`[PlayerCommandController] Sending MoveToEntity:`, {
        entityId,
        autoInteract,
        modifiers,
        timestamp: Date.now(),
      })
    }

    gameConnection.send({
      playerAction: proto.C2S_PlayerAction.create({
        moveToEntity: proto.MoveToEntity.create({
          entityId,
          autoInteract,
        }),
        modifiers,
      }),
    })
  }

  sendInteract(entityId: number, interactionType: proto.InteractionType = proto.InteractionType.AUTO): void {
    if (DEBUG_MOVEMENT) {
      console.log(`[PlayerCommandController] Sending Interact:`, {
        entityId,
        interactionType,
        timestamp: Date.now(),
      })
    }

    gameConnection.send({
      playerAction: proto.C2S_PlayerAction.create({
        interact: proto.Interact.create({
          entityId,
          type: interactionType,
        }),
      }),
    })
  }
}

export const playerCommandController = new PlayerCommandController()
