package systems

import (
	"origin/internal/ecs"

	"go.uber.org/zap"
)

// ResetSystem clears temporary data structures at the beginning of each frame
type ResetSystem struct {
	ecs.BaseSystem
	logger *zap.Logger
}

func NewResetSystem(logger *zap.Logger) *ResetSystem {
	return &ResetSystem{
		BaseSystem: ecs.NewBaseSystem("ResetSystem", 10), // Run after NetworkCommandSystem
		logger:     logger,
	}
}

func (s *ResetSystem) Update(w *ecs.World, dt float64) {
	// Clear moved entities tracking for next frame
	movedEntities := w.MovedEntities()
	movedEntities.Count = 0
}
