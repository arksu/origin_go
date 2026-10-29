package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// RemoveTransient removes an inventory-free runtime object using the same
// quarantine cleanup as lethal damage. The owning shard lock must be held.
func (s *ObjectDestructionService) RemoveTransient(target types.Handle) error {
	if err := s.validateTransientRemoval(target); err != nil {
		return err
	}
	ecs.GetResource[ecs.ObjectDestructionState](s.world).Pending[target] = true
	s.finalizeTransientRemoval(target)
	return nil
}

func (s *ObjectDestructionService) validateTransientRemoval(target types.Handle) error {
	if s == nil || s.world == nil || !s.world.Alive(target) {
		return ErrInvalidTransientRemoval
	}
	id, hasID := s.world.GetExternalID(target)
	info, hasInfo := ecs.GetComponent[components.EntityInfo](s.world, target)
	if !hasID || id == 0 || s.world.GetHandleByEntityID(id) != target || !hasInfo ||
		!info.Transient || info.Region != s.deps.Region || info.Layer != s.world.Layer ||
		ecs.HasComponent[components.EntityHealth](s.world, target) {
		return ErrInvalidTransientRemoval
	}
	if owner, exists := ecs.GetComponent[components.InventoryOwner](s.world, target); exists && len(owner.Inventories) != 0 {
		return ErrInvalidTransientRemoval
	}
	if index, exists := ecs.TryGetResource[ecs.InventoryRefIndex](s.world); exists && index.OwnerEntryCount(id) != 0 {
		return ErrInvalidTransientRemoval
	}
	if ecs.ObjectDestructionPending(s.world, target) || ecs.InventoryHandleReserved(s.world, target) || s.reserved[target] != 0 {
		return ErrObjectDestructionPending
	}
	return nil
}

// Damage calls this only after every health write in its same-lock batch.
func (s *ObjectDestructionService) finalizeTransientRemoval(target types.Handle) {
	s.deps.Quarantine(target)
	s.world.Despawn(target)
}
