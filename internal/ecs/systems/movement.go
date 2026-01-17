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
	chunkManager core.ChunkManager
	logger       *zap.Logger
	movingQuery  *ecs.PreparedQuery
}

func NewMovementSystem(world *ecs.World, chunkManager core.ChunkManager, logger *zap.Logger) *MovementSystem {
	// Query for entities with Transform and Movement components
	movingQuery := ecs.NewPreparedQuery(
		world,
		0|
			(1<<components.TransformComponentID)|
			(1<<components.MovementComponentID),
		0, // no exclusions
	)

	return &MovementSystem{
		BaseSystem:   ecs.NewBaseSystem("MovementSystem", 100),
		chunkManager: chunkManager,
		logger:       logger,
		movingQuery:  movingQuery,
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
					t.IntentX = movement.TargetX
					t.IntentY = movement.TargetY
					t.Direction = math.Atan2(dy, dx)
				})
				ecs.WithComponent(w, h, func(m *components.Movement) {
					m.ClearTarget()
				})
				// Add MoveTag to indicate real movement occurred
				ecs.AddComponent(w, h, components.MoveTag{})
				// TODO впереди еще проверка коллизий, поэтому пишем просто в Intent, и только после будет фактическая смена позиции
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
				t.IntentX = newX
				t.IntentY = newY
				// Direction based on actual velocity vector
				t.Direction = math.Atan2(velocityY, velocityX)
			})
			// Add MoveTag to indicate real movement occurred
			ecs.AddComponent(w, h, components.MoveTag{})

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
