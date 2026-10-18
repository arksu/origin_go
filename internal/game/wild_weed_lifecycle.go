package game

import (
	"context"
	"fmt"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/types"
)

// Only managed WWW extend the ordinary destruction transaction. Other object
// persisters keep their existing contract.
type wildWeedDestructionPersister interface {
	ReplaceWildWeedWithDroppedItems(context.Context, int, gameworld.WildWeedObjectDeletion, types.EntityID,
		func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) (gameworld.SpotStateCommit, error)
}

type wildWeedObjectDeleter interface {
	DeleteWildWeedObject(context.Context, int, gameworld.WildWeedObjectDeletion) (gameworld.SpotStateCommit, error)
}

func managedWildWeedSpotID(w *ecs.World, handle types.Handle) (int64, error) {
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !ok {
		return 0, nil
	}
	weed, ok := components.GetBehaviorState[components.WildWeedBehaviorState](state, "wild_weed")
	if !ok || weed == nil {
		return 0, nil
	}
	if weed.SpotID < 0 {
		return 0, fmt.Errorf("wild weed has invalid spot ID %d", weed.SpotID)
	}
	return weed.SpotID, nil
}

func (s *Shard) applyCommittedSpotState(commit gameworld.SpotStateCommit) error {
	if s.spotManager == nil {
		return gameworld.ErrSpotNotFound
	}
	return s.spotManager.ApplyCommittedState(commit)
}

// Keep the first administrative delete synchronous. If its outcome is uncertain,
// the existing destruction slot retains the intent and pins the source for retry.
type wildWeedAdminDeleter struct {
	destruction *ObjectDestructionService
	fallback    gameworld.ObjectDeleter
}

func (d wildWeedAdminDeleter) DeleteObject(region int, entityID types.EntityID) error {
	s := d.destruction
	if s == nil {
		return ErrInvalidObjectDestructionService
	}
	handle := s.world.GetHandleByEntityID(entityID)
	spotID, err := managedWildWeedSpotID(s.world, handle)
	if err != nil {
		return err
	}
	if spotID == 0 {
		return d.fallback.DeleteObject(region, entityID)
	}
	persister, ok := s.deps.Persister.(wildWeedObjectDeleter)
	if !ok || s.deps.SpotStateCommitted == nil {
		return ErrInvalidObjectDestructionService
	}
	if region != s.deps.Region {
		return ErrObjectDestructionCapture
	}
	reservation, err := s.reserve(handle)
	if err != nil {
		return err
	}
	op := s.reservationOperation(reservation, destructionReserved)
	op.adminWildWeedDelete = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	commit, err := persister.DeleteWildWeedObject(ctx, region, op.spotDeletion)
	if err == nil {
		op.spotCommit, op.committed = commit, true
		err = s.deps.SpotStateCommitted(commit)
	}
	if err == nil {
		s.cancelReservation(reservation)
		return nil
	}
	// Returning an error leaves the entity alive in the admin caller. Quarantine
	// it first so neither gameplay nor an ordinary chunk save can reuse it.
	s.commitReservation(reservation)
	s.finalizeReservation(reservation)
	return err
}

func (op *objectDestructionOperation) runWildWeedAdminDeletion() error {
	if op.committed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var commit gameworld.SpotStateCommit
	err := op.service.deps.Chunks.WithPersistence(op.pinned, func() error {
		var err error
		commit, err = op.service.deps.Persister.(wildWeedObjectDeleter).DeleteWildWeedObject(ctx, op.region, op.spotDeletion)
		return err
	})
	if err != nil {
		return err
	}
	op.spotCommit, op.committed = commit, true
	return nil
}
