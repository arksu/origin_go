package systems

import (
	"origin/internal/ecs"

	"go.uber.org/zap"
)

// ResetSystem clears temporary data structures at the beginning of each frame
type ResetSystem struct {
	ecs.BaseSystem
	movedEntities *MovedEntities
	logger        *zap.Logger
}

func NewResetSystem(movedEntities *MovedEntities, logger *zap.Logger) *ResetSystem {
	return &ResetSystem{
		BaseSystem:    ecs.NewBaseSystem("ResetSystem", 0), // Run first
		movedEntities: movedEntities,
		logger:        logger,
	}
}

func (s *ResetSystem) Update(w *ecs.World, dt float64) {
	// Clear moved entities tracking for next frame
	//clear(s.movedEntities.Handles)
	s.movedEntities.Count = 0
}
