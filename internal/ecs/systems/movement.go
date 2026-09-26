package systems

import (
	"math"
	constt "origin/internal/const"
	"origin/internal/entitystats"
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

	// Cached storages for the hot path: direct sparse-array access instead of
	// per-call registry lock + map lookup, and no capturing closures on writes.
	movementStorage    *ecs.ComponentStorage[components.Movement]
	transformStorage   *ecs.ComponentStorage[components.Transform]
	entityStatsStorage *ecs.ComponentStorage[components.EntityStats]
	liftCarryStorage   *ecs.ComponentStorage[components.LiftCarryState]
	profileStorage     *ecs.ComponentStorage[components.CharacterProfile]
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
		BaseSystem:         ecs.NewBaseSystem("MovementSystem", 100),
		chunkManager:       chunkManager,
		logger:             logger,
		movingQuery:        movingQuery,
		movementStorage:    ecs.GetOrCreateStorage[components.Movement](world),
		transformStorage:   ecs.GetOrCreateStorage[components.Transform](world),
		entityStatsStorage: ecs.GetOrCreateStorage[components.EntityStats](world),
		liftCarryStorage:   ecs.GetOrCreateStorage[components.LiftCarryState](world),
		profileStorage:     ecs.GetOrCreateStorage[components.CharacterProfile](world),
	}
}

func (s *MovementSystem) Update(w *ecs.World, dt float64) {
	movedEntities := ecs.GetResource[ecs.MovedEntities](w)
	// Use prepared query to iterate over entities with Transform and Movement
	s.movingQuery.ForEach(func(h types.Handle) {
		movement, ok := s.movementStorage.Get(h)
		if !ok {
			return
		}

		if movement.State != constt.StateMoving {
			return
		}

		transform, ok := s.transformStorage.Get(h)
		if !ok {
			return
		}

		if stats, hasStats := s.entityStatsStorage.Get(h); hasStats {
			capability := resolveMovementCapability(s.profileStorage, h)
			maxStamina := capability.maxStamina
			clampedStamina := entitystats.ClampStamina(stats.Stamina, maxStamina)
			if clampedStamina != stats.Stamina {
				stats.Stamina = clampedStamina
				s.entityStatsStorage.Set(h, stats)
				ecs.MarkPlayerStatsDirtyByHandle(w, h, ecs.ResolvePlayerStatsTTLms(w))
				ecs.UpdateEntityStatsRegenSchedule(w, h, clampedStamina, stats.Energy, maxStamina)
			}

			isCarrying := s.liftCarryStorage.Has(h)
			allowedMode, canMove := entitystats.ResolveAllowedMoveModeWithCarry(
				movement.Mode,
				stats.Stamina,
				maxStamina,
				stats.Energy,
				isCarrying,
			)
			if !canMove {
				movement.Mode = constt.Crawl
				movement.StopAtPointTarget()
				s.movementStorage.Set(h, movement)
				ecs.MarkMovementModeDirtyByHandle(w, h)
				movedEntities.Add(h, transform.X, transform.Y)
				return
			}
			if movement.Mode != allowedMode {
				movement.Mode = allowedMode
				s.movementStorage.Set(h, movement)
				ecs.MarkMovementModeDirtyByHandle(w, h)
			}
		}

		if movement.TargetType == constt.TargetEntity {
			targetTransform, ok := s.transformStorage.Get(movement.TargetHandle)
			if !ok {
				movement.ClearTarget()
				s.movementStorage.Set(h, movement)
				movedEntities.Add(h, transform.X, transform.Y)
				return
			}
			movement.TargetX = targetTransform.X
			movement.TargetY = targetTransform.Y
		}

		if movement.HasReachedTarget(transform.X, transform.Y) {
			movement.StopAtPointTarget()
			s.movementStorage.Set(h, movement)
			movedEntities.Add(h, transform.X, transform.Y)
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
				transform.Direction = math.Atan2(dy, dx)
				s.transformStorage.Set(h, transform)
				movement.StopAtPointTarget()
				s.movementStorage.Set(h, movement)
				// Add to moved entities buffer
				movedEntities.Add(h, movement.TargetX, movement.TargetY)
				return
			}

			// Normal movement
			velocityX := (dx / dist) * speed
			velocityY := (dy / dist) * speed

			movement.VelocityX = velocityX
			movement.VelocityY = velocityY
			s.movementStorage.Set(h, movement)

			old := types.Vector2{X: transform.X, Y: transform.Y}
			newX := transform.X + velocityX*dt
			newY := transform.Y + velocityY*dt

			// Direction based on actual velocity vector
			transform.Direction = math.Atan2(velocityY, velocityX)
			s.transformStorage.Set(h, transform)
			// Add to moved entities buffer
			movedEntities.Add(h, newX, newY)

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
