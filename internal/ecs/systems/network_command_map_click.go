package systems

import (
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// A pending command consumes the input before ordinary movement or pickup.
func (s *NetworkCommandSystem) consumeAdminMapClick(w *ecs.World, playerHandle types.Handle, playerID types.EntityID, click *netproto.MapClick) bool {
	if s.adminHandler == nil {
		return false
	}
	if ecs.GetResource[ecs.PendingAdminDestroy](w).Get(playerID) {
		s.adminHandler.ExecutePendingDestroy(w, playerID, types.EntityID(click.TargetEntityId))
		return true
	}
	if ecs.GetResource[ecs.PendingAdminObjectInfo](w).Get(playerID) {
		s.adminHandler.ExecutePendingObjectInfo(w, playerID, types.EntityID(click.TargetEntityId))
		return true
	}
	if ecs.GetResource[ecs.PendingAdminTeleport](w).Get(playerID) {
		s.adminHandler.ExecutePendingTeleport(w, playerID, playerHandle, float64(click.X), float64(click.Y))
		return true
	}
	if _, pending := ecs.GetResource[ecs.PendingAdminSpawn](w).Get(playerID); pending {
		s.adminHandler.ExecutePendingSpawn(w, playerID, playerHandle, float64(click.X), float64(click.Y))
		return true
	}
	return false
}
