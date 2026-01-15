package systems

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
	"time"

	"go.uber.org/zap"

	"origin/internal/eventbus"
	"origin/internal/network/proto"
)

// ObjectMoveEvent represents an object movement event for network transmission
type ObjectMoveEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  types.EntityID
	Movement  *proto.EntityMovement
}

func (e *ObjectMoveEvent) Topic() string { return e.topic }

func NewObjectMoveEvent(entityID types.EntityID, movement *proto.EntityMovement) *ObjectMoveEvent {
	return &ObjectMoveEvent{
		topic:     "gameplay.object.move",
		Timestamp: time.Now(),
		EntityID:  entityID,
		Movement:  movement,
	}
}

type TransformUpdateSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	eventBus     *eventbus.EventBus
	logger       *zap.Logger
}

func NewTransformUpdateSystem(chunkManager core.ChunkManager, eventBus *eventbus.EventBus, logger *zap.Logger) *TransformUpdateSystem {
	return &TransformUpdateSystem{
		BaseSystem:   ecs.NewBaseSystem("TransformUpdateSystem", 300),
		chunkManager: chunkManager,
		eventBus:     eventBus,
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
				if collisionResult.PerpendicularOscillation {
					// stop movement
					ecs.WithComponent(w, h, func(m *components.Movement) {
						m.ClearTarget()
					})
				}

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

			// Send to client S2C_ObjectMove with current data
			if entityID, ok := w.GetExternalID(h); ok {
				// Get movement component for velocity data
				movement, hasMovement := ecs.GetComponent[components.Movement](w, h)

				// Create movement data for packet
				moveMode := proto.MovementMode_MOVE_MODE_WALK
				isMoving := false
				var velocity proto.Vector2
				var targetPosition *proto.Vector2

				if hasMovement {
					// Convert movement mode
					switch movement.Mode {
					case components.Walk:
						moveMode = proto.MovementMode_MOVE_MODE_WALK
					case components.Run:
						moveMode = proto.MovementMode_MOVE_MODE_RUN
					case components.FastRun:
						moveMode = proto.MovementMode_MOVE_MODE_FAST_RUN
					case components.Swim:
						moveMode = proto.MovementMode_MOVE_MODE_SWIM
					}

					isMoving = movement.State == components.StateMoving
					velocity = proto.Vector2{
						X: int32(movement.VelocityX),
						Y: int32(movement.VelocityY),
					}

					if movement.TargetType == components.TargetPoint {
						targetPosition = &proto.Vector2{
							X: int32(movement.TargetX),
							Y: int32(movement.TargetY),
						}
					}
				}

				// Create entity movement packet
				entityMovement := &proto.EntityMovement{
					Position: &proto.Position{
						X:       int32(finalX),
						Y:       int32(finalY),
						Heading: 0, // TODO: get from transform component
					},
					Velocity:       &velocity,
					MoveMode:       moveMode,
					TargetPosition: targetPosition,
					IsMoving:       isMoving,
				}

				// Publish network event for S2C_ObjectMove
				s.eventBus.PublishAsync(
					NewObjectMoveEvent(entityID, entityMovement),
					eventbus.PriorityMedium,
				)
			}

			// Save collision state for next frame (for oscillation detection)
			if hasCollision {
				ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
					// Save current collision position for next frame
					cr.PrevFinalX = cr.FinalX
					cr.PrevFinalY = cr.FinalY
					if len(cr.CollidedWith) > 0 {
						cr.PrevCollidedWith = cr.CollidedWith[0]
					}
					// Clear collision result for next frame
					cr.HasCollision = false
					cr.CollidedWith = nil
					cr.CollisionNormalX = 0
					cr.CollisionNormalY = 0
					cr.IsPhantom = false
					cr.PerpendicularOscillation = false
				})
			}
		}
	}
}
