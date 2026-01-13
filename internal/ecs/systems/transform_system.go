package systems

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"

	"go.uber.org/zap"
)

type TransformUpdateSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	logger       *zap.Logger
}

func NewTransformUpdateSystem(chunkManager core.ChunkManager, logger *zap.Logger) *TransformUpdateSystem {
	return &TransformUpdateSystem{
		BaseSystem:   ecs.NewBaseSystem("TransformUpdateSystem", 120), // After CollisionSystem (110)
		chunkManager: chunkManager,
		logger:       logger,
	}
}

func (s *TransformUpdateSystem) Update(w *ecs.World, dt float64) {
	activeChunks := s.chunkManager.ActiveChunks()

	for _, chunk := range activeChunks {
		dynamicHandles := chunk.GetDynamicHandles()

		for _, h := range dynamicHandles {
			if !w.Alive(h) {
				continue
			}

			// We need Transform to update it
			transform, ok := ecs.GetComponent[components.Transform](w, h)
			if !ok {
				continue
			}

			newX := transform.IntentX
			newY := transform.IntentY

			// If there was a collision result, use it
			if result, ok := ecs.GetComponent[components.CollisionResult](w, h); ok {
				// Only use result if it's valid/recent?
				// The CollisionSystem runs every frame and resets/sets the result.
				// If HasCollision is true, or even if false, FinalX/Y should be respected
				// because CollisionSystem might have processed sliding even without "collision"
				// (e.g. just movement without hitting anything).
				// Actually CollisionSystem sets FinalX/Y to IntentX/Y initially and then modifies it.
				// So we should always use FinalX/Y if CollisionResult exists.
				newX = result.FinalX
				newY = result.FinalY
			}

			// Check if position changed
			if newX != transform.X || newY != transform.Y {
				oldX := int(transform.X)
				oldY := int(transform.Y)
				iNewX := int(newX)
				iNewY := int(newY)

				// Update SpatialHash
				chunk.Spatial().UpdateDynamic(h, oldX, oldY, iNewX, iNewY)

				// Update Transform component
				ecs.WithComponent(w, h, func(t *components.Transform) {
					t.X = newX
					t.Y = newY
					// Intent is already consumed, maybe reset it to current pos?
					// Or leave it. MovementSystem will overwrite it next frame.
					t.IntentX = newX
					t.IntentY = newY
				})
			}
		}
	}
}
