package systems

import (
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
	"time"

	"go.uber.org/zap"

	"origin/internal/eventbus"
	"origin/internal/network/proto"
)

type TransformUpdateSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	eventBus     *eventbus.EventBus
	logger       *zap.Logger
}

func NewTransformUpdateSystem(world *ecs.World, chunkManager core.ChunkManager, eventBus *eventbus.EventBus, logger *zap.Logger) *TransformUpdateSystem {
	return &TransformUpdateSystem{
		BaseSystem:   ecs.NewBaseSystem("TransformUpdateSystem", 300),
		chunkManager: chunkManager,
		eventBus:     eventBus,
		logger:       logger,
	}
}

func (s *TransformUpdateSystem) Update(w *ecs.World, dt float64) {
	movedEntities := w.MovedEntities()
	// Process entities that moved this frame (from movedEntities buffer)
	for i := 0; i < movedEntities.Count; i++ {
		h := movedEntities.Handles[i]
		if !w.Alive(h) {
			continue
		}

		transform, ok := ecs.GetComponent[components.Transform](w, h)
		if !ok {
			continue
		}

		// Check for collision result
		collisionResult, hasCollision := ecs.GetComponent[components.CollisionResult](w, h)

		var finalX, finalY float64
		if hasCollision {
			if collisionResult.PerpendicularOscillation {
				// stop movement
				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.ClearTarget()
				})
			}

			// Apply collision-adjusted position
			finalX = collisionResult.FinalX
			finalY = collisionResult.FinalY
		} else {
			// No collision result - should not happen if movedEntities is properly managed
			continue
		}

		// Get chunk for spatial hash update
		chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, h)
		if hasChunkRef {
			chunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
			chunk := s.chunkManager.GetChunk(chunkCoord)
			if chunk != nil {
				// Update spatial hash if position changed
				oldX := int(transform.X)
				oldY := int(transform.Y)
				newX := int(finalX)
				newY := int(finalY)

				// TODO migrate chunks
				if oldX != newX || oldY != newY {
					chunk.Spatial().UpdateDynamic(h, oldX, oldY, newX, newY)
				}
			}
		}

		// Apply final position to transform
		ecs.WithComponent(w, h, func(t *components.Transform) {
			t.X = finalX
			t.Y = finalY
		})

		// Send to client event with current data
		if entityID, ok := w.GetExternalID(h); ok {
			// Get entity layer from EntityInfo component
			entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](w, h)
			layer := 0 // default layer
			if hasEntityInfo {
				layer = entityInfo.Layer
			}

			// Visibility guard: only publish move events if entity is visible to at least one observer
			visState := w.VisibilityState()
			if visState != nil {
				// Check if any observer can see this entity
				observers, hasObservers := visState.ObserversByVisibleTarget[h]
				if !hasObservers || len(observers) == 0 {
					// No observers can see this entity, skip publishing move event
					continue
				}
			}

			// Get movement component for velocity data
			movement, hasMovement := ecs.GetComponent[components.Movement](w, h)

			// Create movement data for packet
			moveMode := constt.Walk
			isMoving := false
			var velocity proto.Vector2
			var moveSeq uint32

			if hasMovement {
				moveMode = movement.Mode

				isMoving = movement.State == constt.StateMoving
				velocity = proto.Vector2{
					X: int32(movement.VelocityX),
					Y: int32(movement.VelocityY),
				}
				moveSeq = movement.MoveSeq

				// Increment MoveSeq for next movement
				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.MoveSeq++
				})
			}

			// Prepare target position as pointers
			var targetX, targetY *int
			if hasMovement && movement.TargetType == constt.TargetPoint {
				tx := int(movement.TargetX)
				ty := int(movement.TargetY)
				targetX = &tx
				targetY = &ty
			}

			// Get server time in milliseconds
			serverTimeMs := time.Now().UnixMilli()

			// Determine if this is a teleport (for now, always false - can be set by teleport system)
			isTeleport := false

			// Publish movement event with raw data
			s.eventBus.PublishAsync(
				ecs.NewObjectMoveEvent(
					entityID,
					int(finalX), int(finalY), int(transform.Direction),
					int(velocity.X), int(velocity.Y),
					moveMode, isMoving,
					targetX, targetY,
					layer,
					serverTimeMs, moveSeq, isTeleport,
				),
				eventbus.PriorityMedium,
			)
		}

		// Save collision state for next frame (for oscillation detection)
		ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
			// Save current collision position for next frame
			cr.PrevFinalX = cr.FinalX
			cr.PrevFinalY = cr.FinalY
			if cr.CollidedWith != 0 {
				cr.PrevCollidedWith = cr.CollidedWith
			}
			// Clear collision result for next frame
			cr.HasCollision = false
			cr.CollidedWith = 0
			cr.CollisionNormalX = 0
			cr.CollisionNormalY = 0
			cr.IsPhantom = false
			cr.PerpendicularOscillation = false
		})
	}
}
