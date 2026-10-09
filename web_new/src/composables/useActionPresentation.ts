import { ACTION_CATALOG, requestGameAction, type HotbarActionId } from '@/game/hud/actionCatalog'
import { useGameStore } from '@/stores/gameStore'
import { useActionCooldownStore } from '@/stores/actionCooldownStore'
import { getDirectionSector } from '@/game/hud/directionAim'

const shortcuts = new Map(ACTION_CATALOG.map(entry => [entry.id as string, entry]))

export function useActionPresentation() {
  const game = useGameStore()
  const cooldowns = useActionCooldownStore()

  function presentation(id: HotbarActionId) {
    const gameplay = id.startsWith('game:')
    const actionId = gameplay ? id.slice(5) : id
    const definition = gameplay ? game.gameActionsById.get(actionId) : undefined
    const shortcut = gameplay ? undefined : shortcuts.get(actionId)
    const label = definition?.label || shortcut?.label || (game.gameActionListLoaded ? 'Unavailable action' : 'Loading action')
    const cooldownProgress = gameplay ? cooldowns.progress(actionId) : 1
    return {
      label,
      shortLabel: shortcut?.shortLabel || label.slice(0, 3).toUpperCase(),
      iconPath: definition?.menuIcon || shortcut?.iconPath || '',
      available: !!definition || !!shortcut,
      cooldownProgress,
      coolingDown: cooldownProgress < 1,
    }
  }

  function canActivate(id: HotbarActionId): boolean {
    return presentation(id).available && (!id.startsWith('game:') || !cooldowns.isCoolingDown(id.slice(5)))
  }

  function activate(actionId: string, send: (id: string) => void): boolean {
    let accepted = true
    const requested = requestGameAction(actionId, game.gameActions, game.gameActionListLoaded, id => {
      if (game.gameActionsById.get(id)?.targetKind === 'direction') {
        const epoch = game.worldParams?.streamEpoch
        if (!game.isInGame || !Number.isInteger(epoch) || !epoch || epoch < 0 || epoch > 0xffffffff ||
            !getDirectionSector(game.gameActionsById.get(id))) {
          accepted = false
          return
        }
        game.clearBuildPlacement()
        game.closeContextMenu()
      }
      send(id)
    }, cooldowns.isCoolingDown)
    return requested && accepted
  }

  return { presentation, canActivate, activate }
}
