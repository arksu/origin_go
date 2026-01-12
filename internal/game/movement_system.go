package game

import (
	"math"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

type MovementSystem struct {
	ecs.BaseSystem
	chunkManager *ChunkManager
}

func NewMovementSystem(chunkManager *ChunkManager) *MovementSystem {
	return &MovementSystem{
		BaseSystem:   ecs.NewBaseSystem("MovementSystem", 100),
		chunkManager: chunkManager,
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
			dist := float64(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist > 0.001 {
				speed := movement.GetCurrentSpeed()
				step := speed * dt

				// Clamp step to prevent overshoot oscillation
				if step >= dist {
					// Reached target, snap to exact position
					oldX := transform.X
					oldY := transform.Y
					ecs.WithComponent(w, h, func(t *components.Transform) {
						t.X = movement.TargetX
						t.Y = movement.TargetY
						t.Direction = float32(math.Atan2(float64(dy), float64(dx)))
					})
					ecs.WithComponent(w, h, func(m *components.Movement) {
						m.ClearTarget()
					})
					spatial := chunk.Spatial()
					spatial.UpdateDynamic(h, float64(oldX), float64(oldY), float64(movement.TargetX), float64(movement.TargetY))
					continue
				}

				// Normal movement
				velocityX := (float64(dx) / dist) * speed
				velocityY := (float64(dy) / dist) * speed

				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.VelocityX = velocityX
					m.VelocityY = velocityY
					m.TargetX = movement.TargetX
					m.TargetY = movement.TargetY
				})

				oldX := transform.X
				oldY := transform.Y
				newX := float64(transform.X) + velocityX*dt
				newY := float64(transform.Y) + velocityY*dt

				ecs.WithComponent(w, h, func(t *components.Transform) {
					t.IntentX = int(math.Round(newX))
					t.IntentY = int(math.Round(newY))
					// Direction based on actual velocity vector
					t.Direction = float32(math.Atan2(float64(velocityY), float64(velocityX)))
				})

				spatial := chunk.Spatial()
				// TODO log debug movement data

				// TODO проверить миграции между гридами
				spatial.UpdateDynamic(h, float64(oldX), float64(oldY), float64(newX), float64(newY))
			}
		}
	}
}
