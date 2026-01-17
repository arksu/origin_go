package systems

import (
	"math"
	"origin/internal/types"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"

	"go.uber.org/zap"
)

const debugEnabled = false

type MovementSystem struct {
	ecs.BaseSystem
	chunkManager  core.ChunkManager
	logger        *zap.Logger
	movingQuery   *ecs.PreparedQuery
	movedEntities *MovedEntities
}

func NewMovementSystem(world *ecs.World, chunkManager core.ChunkManager, movedEntities *MovedEntities, logger *zap.Logger) *MovementSystem {
	// Query for entities with Transform and Movement components
	movingQuery := ecs.NewPreparedQuery(
		world,
		0|
			(1<<components.TransformComponentID)|
			(1<<components.MovementComponentID),
		0, // no exclusions
	)

	return &MovementSystem{
		BaseSystem:    ecs.NewBaseSystem("MovementSystem", 100),
		chunkManager:  chunkManager,
		logger:        logger,
		movingQuery:   movingQuery,
		movedEntities: movedEntities,
	}
}

func (s *MovementSystem) Update(w *ecs.World, dt float64) {
	// Use prepared query to iterate over entities with Transform and Movement
	s.movingQuery.ForEach(func(h types.Handle) {
		movement, ok := ecs.GetComponent[components.Movement](w, h)
		if !ok {
			return
		}

		if movement.State != components.StateMoving {
			return
		}

		transform, ok := ecs.GetComponent[components.Transform](w, h)
		if !ok {
			return
		}

		if movement.TargetType == components.TargetEntity {
			targetTransform, ok := ecs.GetComponent[components.Transform](w, movement.TargetHandle)
			if !ok {
				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.ClearTarget()
				})
				return
			}
			movement.TargetX = targetTransform.X
			movement.TargetY = targetTransform.Y
		}

		if movement.HasReachedTarget(transform.X, transform.Y) {
			ecs.WithComponent(w, h, func(m *components.Movement) {
				m.ClearTarget()
			})
			return
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
				ecs.WithComponent(w, h, func(t *components.Transform) {
					t.Direction = math.Atan2(dy, dx)
				})
				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.ClearTarget()
				})
				// Add to moved entities buffer
				s.movedEntities.Add(h, movement.TargetX, movement.TargetY)
				return
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

			old := types.Vector2{X: transform.X, Y: transform.Y}
			newX := transform.X + velocityX*dt
			newY := transform.Y + velocityY*dt

			ecs.WithComponent(w, h, func(t *components.Transform) {
				// Direction based on actual velocity vector
				t.Direction = math.Atan2(velocityY, velocityX)
			})
			// Add to moved entities buffer
			s.movedEntities.Add(h, newX, newY)

			if debugEnabled {
				s.logger.Debug("Entity movement",
					zap.Uint64("handle", uint64(h)),
					zap.Any("old", old),
					zap.Any("new", types.Vector2{X: newX, Y: newY}),
					//zap.Float64("velocity_x", velocityX),
					//zap.Float64("velocity_y", velocityY),
					//zap.Float64("dt", dt),
				)
			}
		}
	})
}
