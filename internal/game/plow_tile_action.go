package game

import (
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type plowTerrain interface {
	GetTileID(tileX, tileY int) (byte, bool)
	SetTile(tileX, tileY int, tileID byte) bool
}

type plowTileActionHandler struct{ terrain plowTerrain }

func (handler *plowTileActionHandler) UnavailableReason(*ecs.World, types.EntityID, types.Handle) string {
	return ""
}

func plowTileCoordinates(target ActionTarget) (int, int) {
	return int(math.Floor(target.X / constt.CoordPerTile)), int(math.Floor(target.Y / constt.CoordPerTile))
}

func (handler *plowTileActionHandler) ValidateTarget(_ *ecs.World, _ types.EntityID, _ types.Handle, target ActionTarget) string {
	tileX, tileY := plowTileCoordinates(target)
	tileID, found := handler.terrain.GetTileID(tileX, tileY)
	if !found {
		return "ACTION_INVALID_TARGET"
	}
	switch tileID {
	case types.TileConiferousForest, types.TileBroadleafForest, types.TileThicket, types.TileGrass:
		return ""
	default:
		return "BAD_TERRAIN"
	}
}

func (handler *plowTileActionHandler) Start(world *ecs.World, playerID types.EntityID, player types.Handle, target ActionTarget, _ uint64) ActionResult {
	if reason := handler.ValidateTarget(world, playerID, player, target); reason != "" {
		return ActionResult{Outcome: ActionFailed, Reason: reason}
	}
	tileX, tileY := plowTileCoordinates(target)
	if !handler.terrain.SetTile(tileX, tileY, types.TilePlowed) {
		return ActionResult{Outcome: ActionFailed, Reason: "ACTION_INVALID_TARGET"}
	}
	return ActionResult{Outcome: ActionSucceeded}
}

func (handler *plowTileActionHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
}
