package game

import (
	"math"

	"origin/internal/ecs"
	"origin/internal/ecs/components"

	"go.uber.org/zap"
)

type MovementSystem struct {
	ecs.BaseSystem
	chunkManager *ChunkManager
	logger       *zap.Logger
}

func NewMovementSystem(chunkManager *ChunkManager, logger *zap.Logger) *MovementSystem {
	return &MovementSystem{
		BaseSystem:   ecs.NewBaseSystem("MovementSystem", 100),
		chunkManager: chunkManager,
		logger:       logger,
	}
}

func (s *MovementSystem) Update(w *ecs.World, dt float64) {

	activeChunks := s.chunkManager.ActiveChunks()

	for _, chunk := range activeChunks {
		dynamicHandles := chunk.GetDynamicHandles()

		for _, h := range dynamicHandles {
			if !w.Alive(h) {
				continue
			}

			movement, ok := ecs.GetComponent[components.Movement](w, h)
			if !ok {
				continue
			}

			if movement.State != components.StateMoving {
				continue
			}

			transform, ok := ecs.GetComponent[components.Transform](w, h)
			if !ok {
				continue
			}

			if movement.TargetType == components.TargetEntity {
				targetTransform, ok := ecs.GetComponent[components.Transform](w, movement.TargetHandle)
				if !ok {
					ecs.WithComponent(w, h, func(m *components.Movement) {
						m.ClearTarget()
					})
					continue
				}
				movement.TargetX = targetTransform.X
				movement.TargetY = targetTransform.Y
			}

			if movement.HasReachedTarget(transform.X, transform.Y) {
				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.ClearTarget()
				})
				continue
			}

			dx := movement.TargetX - transform.X
			dy := movement.TargetY - transform.Y
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist > 0.001 {
				speed := movement.GetCurrentSpeed()
				step := speed * dt

				// Clamp step to prevent overshoot oscillation
				if step >= dist {
					// Reached target, snap to exact position
					//oldX := transform.X
					//oldY := transform.Y
					ecs.WithComponent(w, h, func(t *components.Transform) {
						t.X = movement.TargetX
						t.Y = movement.TargetY
						t.Direction = math.Atan2(dy, dx)
					})
					ecs.WithComponent(w, h, func(m *components.Movement) {
						m.ClearTarget()
					})
					// TODO
					//spatial := chunk.Spatial()
					//spatial.UpdateDynamic(h, oldX, oldY, movement.TargetX, movement.TargetY)
					continue
				}

				// Normal movement
				velocityX := (dx / dist) * speed
				velocityY := (dy / dist) * speed

				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.VelocityX = velocityX
					m.VelocityY = velocityY
					m.TargetX = movement.TargetX
					m.TargetY = movement.TargetY
				})

				oldX := transform.X
				oldY := transform.Y
				newX := math.Round(transform.X + velocityX*dt)
				newY := math.Round(transform.Y + velocityY*dt)

				ecs.WithComponent(w, h, func(t *components.Transform) {
					t.IntentX = newX
					t.IntentY = newY
					// Direction based on actual velocity vector
					t.Direction = math.Atan2(velocityY, velocityX)
				})

				s.logger.Debug("Entity movement",
					zap.Uint64("handle", uint64(h)),
					zap.Float64("old_x", oldX),
					zap.Float64("old_y", oldY),
					zap.Float64("new_x", newX),
					zap.Float64("new_y", newY),
					zap.Float64("velocity_x", velocityX),
					zap.Float64("velocity_y", velocityY),
					zap.Float64("dt", dt))

				// TODO проверить миграции между гридами
				//spatial := chunk.Spatial()
				//spatial.UpdateDynamic(h, oldX, oldY, newX, newY)
			}
		}
	}
}
