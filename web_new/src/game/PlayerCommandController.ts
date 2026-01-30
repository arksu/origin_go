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

export class PlayerCommandController {
  sendMoveTo(x: number, y: number, modifiers: number): void {
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
