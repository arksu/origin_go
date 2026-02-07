package world

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

// DeleteDroppedObject removes a dropped item entity from ECS, InventoryRefIndex, and spatial index.
// DB soft-delete should be handled separately by the caller.
func DeleteDroppedObject(w *ecs.World, entityID types.EntityID, handle types.Handle, logger *zap.Logger) {
	// Remove inventory container from InventoryRefIndex
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	containerHandle, found := refIndex.Lookup(constt.InventoryDroppedItem, entityID, 0)
	if found {
		refIndex.Remove(constt.InventoryDroppedItem, entityID, 0)
		w.Despawn(containerHandle)
	}

	// Despawn the dropped entity itself
	if !w.Despawn(handle) {
		logger.Warn("DeleteDroppedObject: entity already despawned",
			zap.Uint64("entity_id", uint64(entityID)))
	}
}

// DeleteDroppedObjectFull removes a dropped item from ECS + InventoryRefIndex + DB.
func DeleteDroppedObjectFull(
	w *ecs.World,
	entityID types.EntityID,
	handle types.Handle,
	persister interface{ DeleteDroppedObject(region int, entityID types.EntityID) error },
	logger *zap.Logger,
) {
	// Get region before despawn
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	region := 0
	if hasInfo {
		region = info.Region
	}

	DeleteDroppedObject(w, entityID, handle, logger)

	if persister != nil && region > 0 {
		if err := persister.DeleteDroppedObject(region, entityID); err != nil {
			logger.Error("Failed to soft-delete dropped object from DB",
				zap.Uint64("entity_id", uint64(entityID)),
				zap.Error(err))
		}
	}
}
