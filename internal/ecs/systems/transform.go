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
		BaseSystem:   ecs.NewBaseSystem("TransformUpdateSystem", 300),
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

			transform, ok := ecs.GetComponent[components.Transform](w, h)
			if !ok {
				continue
			}

			// Skip if no movement intent
			if !transform.WasMoved {
				continue
			}

			// Check for collision result
			collisionResult, hasCollision := ecs.GetComponent[components.CollisionResult](w, h)

			var finalX, finalY float64
			if hasCollision {
				if collisionResult.HasCollision {
					s.logger.Debug("Collision detected")
				}
				// Apply collision-adjusted position
				finalX = collisionResult.FinalX
				finalY = collisionResult.FinalY
			} else {
				// No collision result - apply intent directly
				finalX = transform.IntentX
				finalY = transform.IntentY
			}

			// Update spatial hash if position changed
			oldX := int(transform.X)
			oldY := int(transform.Y)
			newX := int(finalX)
			newY := int(finalY)

			// TODO migrate chunks
			if oldX != newX || oldY != newY {
				chunk.Spatial().UpdateDynamic(h, oldX, oldY, newX, newY)
			}

			// Apply final position to transform
			ecs.WithComponent(w, h, func(t *components.Transform) {
				t.X = finalX
				t.Y = finalY
				t.IntentX = finalX
				t.IntentY = finalY
				t.WasMoved = false
			})

			// Clear collision result for next frame
			if hasCollision {
				ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
					cr.HasCollision = false
					cr.CollidedWith = nil
					cr.CollisionNormalX = 0
					cr.CollisionNormalY = 0
					cr.IsPhantom = false
				})
			}
		}
	}
}
