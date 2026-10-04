package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/playerstate"
	"origin/internal/types"
)

const digItemQuality uint32 = 10

type digTerrain interface {
	GetTileID(tileX, tileY int) (byte, bool)
}

type digTileActionHandler struct {
	terrain  digTerrain
	giveItem contracts.GiveItemFn
}

func (handler *digTileActionHandler) UnavailableReason(*ecs.World, types.EntityID, types.Handle) string {
	if handler.terrain == nil || handler.giveItem == nil {
		return "ACTION_UNAVAILABLE"
	}
	return ""
}

func (handler *digTileActionHandler) itemForTarget(target ActionTarget) (string, string) {
	if handler.terrain == nil {
		return "", "ACTION_UNAVAILABLE"
	}
	tileX, tileY := tileCoordinates(target)
	tileID, found := handler.terrain.GetTileID(tileX, tileY)
	if !found {
		return "", "ACTION_INVALID_TARGET"
	}
	switch tileID {
	case types.TileGrass:
		return "soil", ""
	case types.TileShallowWater:
		return "clay", ""
	case types.TileMountain:
		return "stone", ""
	case types.TileSand:
		return "sand", ""
	default:
		return "", "BAD_TERRAIN"
	}
}

func (handler *digTileActionHandler) ValidateTarget(_ *ecs.World, _ types.EntityID, _ types.Handle, target ActionTarget) string {
	_, reason := handler.itemForTarget(target)
	return reason
}

func (handler *digTileActionHandler) Start(world *ecs.World, playerID types.EntityID, player types.Handle, target ActionTarget, _ uint64) ActionResult {
	if playerstate.ItemsLocked(world, player) {
		return ActionResult{Outcome: ActionRejected, Reason: playerstate.ItemsLockedReason}
	}

	itemKey, reason := handler.itemForTarget(target)
	if reason != "" {
		return ActionResult{Outcome: ActionFailed, Reason: reason}
	}
	if handler.giveItem == nil {
		return ActionResult{Outcome: ActionFailed, Reason: "ACTION_UNAVAILABLE"}
	}
	outcome := handler.giveItem(world, playerID, player, itemKey, 1, digItemQuality)
	if !outcome.Success || outcome.GrantedCount != 1 {
		return ActionResult{Outcome: ActionFailed, Reason: "DIG_GIVE_FAILED"}
	}
	return ActionResult{Outcome: ActionSucceeded, StopAfterCycle: outcome.PlacedInHand}
}

func (*digTileActionHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
}

func (*digTileActionHandler) RequiresItemMutation() bool { return true }
