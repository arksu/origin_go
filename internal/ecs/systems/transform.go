package systems

import (
	"math"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/types"

	"go.uber.org/zap"

	"origin/internal/eventbus"
)

type TransformUpdateSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	eventBus     *eventbus.EventBus
	logger       *zap.Logger
	moveBatch    []ecs.MoveBatchEntry // reused across ticks

	// Cached storages for the hot path: direct sparse-array access instead of
	// per-call registry lock + map lookup, and no capturing closures on writes.
	transformStorage       *ecs.ComponentStorage[components.Transform]
	collisionResultStorage *ecs.ComponentStorage[components.CollisionResult]
	movementStorage        *ecs.ComponentStorage[components.Movement]
	chunkRefStorage        *ecs.ComponentStorage[components.ChunkRef]
	entityStatsStorage     *ecs.ComponentStorage[components.EntityStats]
	liftCarryStorage       *ecs.ComponentStorage[components.LiftCarryState]
}

func NewTransformUpdateSystem(world *ecs.World, chunkManager core.ChunkManager, eventBus *eventbus.EventBus, logger *zap.Logger) *TransformUpdateSystem {
	return &TransformUpdateSystem{
		BaseSystem:             ecs.NewBaseSystem("TransformUpdateSystem", 300),
		chunkManager:           chunkManager,
		eventBus:               eventBus,
		logger:                 logger,
		transformStorage:       ecs.GetOrCreateStorage[components.Transform](world),
		collisionResultStorage: ecs.GetOrCreateStorage[components.CollisionResult](world),
		movementStorage:        ecs.GetOrCreateStorage[components.Movement](world),
		chunkRefStorage:        ecs.GetOrCreateStorage[components.ChunkRef](world),
		entityStatsStorage:     ecs.GetOrCreateStorage[components.EntityStats](world),
		liftCarryStorage:       ecs.GetOrCreateStorage[components.LiftCarryState](world),
	}
}

func (s *TransformUpdateSystem) Update(w *ecs.World, dt float64) {
	movedEntities := ecs.GetResource[ecs.MovedEntities](w)
	serverTimeMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	// Invariant across the whole tick: fetch once, not per moved entity.
	visState := ecs.GetResource[ecs.VisibilityState](w)
	s.moveBatch = s.moveBatch[:0]

	// Process entities that moved this frame (from movedEntities buffer)
	for i := 0; i < movedEntities.Count; i++ {
		h := movedEntities.Handles[i]
		if !w.Alive(h) {
			continue
		}

		transform, ok := s.transformStorage.Get(h)
		if !ok {
			continue
		}

		// Check for collision result
		collisionResult, hasCollision := s.collisionResultStorage.Get(h)

		var finalX, finalY float64
		if hasCollision {
			if collisionResult.PerpendicularOscillation {
				// stop movement
				if m, ok := s.movementStorage.Get(h); ok {
					m.ClearTarget()
					s.movementStorage.Set(h, m)
				}
			}

			// Apply collision-adjusted position
			finalX = collisionResult.FinalX
			finalY = collisionResult.FinalY
		} else {
			// No collision result - should not happen if movedEntities is properly managed
			continue
		}

		// Get chunk for spatial hash update
		chunkRef, hasChunkRef := s.chunkRefStorage.Get(h)
		s.applyMovementStaminaTick(w, h, transform.X, transform.Y, finalX, finalY)
		if hasChunkRef {
			chunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
			chunk := s.chunkManager.GetChunk(chunkCoord)
			if chunk != nil {
				// Update spatial hash if position changed
				oldX := int(transform.X)
				oldY := int(transform.Y)
				newX := int(finalX)
				newY := int(finalY)

				// Move the entry to the final position's cell even when that
				// position crosses a chunk border: ChunkSystem (400) decides
				// migration on the final position and removes the entry from
				// this grid using these same coords. Skipping the update here
				// would strand the entry at the pre-move cell.
				if oldX != newX || oldY != newY {
					chunk.Spatial().UpdateDynamic(h, oldX, oldY, newX, newY)
				}
			}
		}

		// Apply final position to transform
		transform.X = finalX
		transform.Y = finalY
		s.transformStorage.Set(h, transform)

		// Accumulate movement data for batch event
		if entityID, ok := w.GetExternalID(h); ok {
			// Visibility guard: only include if entity is visible to at least one observer
			if visState != nil {
				observers, hasObservers := visState.ObserversByVisibleTarget[h]
				if !hasObservers || len(observers) == 0 {
					goto saveCollision
				}
			}

			// Get movement component for velocity data
			{
				movement, hasMovement := s.movementStorage.Get(h)

				moveMode := constt.Walk
				isMoving := false
				var velX, velY int
				var moveSeq uint32

				if hasMovement {
					moveMode = movement.Mode
					isMoving = movement.State == constt.StateMoving
					velX = int(movement.VelocityX)
					velY = int(movement.VelocityY)
					moveSeq = movement.MoveSeq

					movement.MoveSeq++
					s.movementStorage.Set(h, movement)
				}

				var targetX, targetY *int
				if hasMovement && s.broadcastMoveTarget(&movement) {
					tx := int(movement.TargetX)
					ty := int(movement.TargetY)
					targetX = &tx
					targetY = &ty
				}

				s.moveBatch = append(s.moveBatch, ecs.MoveBatchEntry{
					EntityID:     entityID,
					Handle:       h,
					X:            int(finalX),
					Y:            int(finalY),
					Heading:      transform.Direction,
					VelocityX:    velX,
					VelocityY:    velY,
					MoveMode:     moveMode,
					IsMoving:     isMoving,
					TargetX:      targetX,
					TargetY:      targetY,
					ServerTimeMs: serverTimeMs,
					MoveSeq:      moveSeq,
					IsTeleport:   false,
				})
			}
		}

	saveCollision:
		// Save collision state for next frame (for oscillation detection)
		collisionResult.PrevFinalX = collisionResult.FinalX
		collisionResult.PrevFinalY = collisionResult.FinalY
		collisionResult.PrevCollidedWith = collisionResult.CollidedWith
		// Clear collision result for next frame
		collisionResult.HasCollision = false
		collisionResult.CollidedWith = 0
		collisionResult.CollisionNormalX = 0
		collisionResult.CollisionNormalY = 0
		collisionResult.IsPhantom = false
		collisionResult.PerpendicularOscillation = false
		s.collisionResultStorage.Set(h, collisionResult)
	}

	// Publish single batch event for all movements this tick
	if len(s.moveBatch) > 0 {
		s.eventBus.PublishAsync(
			ecs.NewObjectMoveBatchEvent(w.Layer, s.moveBatch),
			eventbus.PriorityMedium,
		)
	}
}

// broadcastMoveTarget reports whether the entity's move target should be sent
// to clients. Point targets are stable, so they are always sent. Entity
// targets are withheld while the target itself is moving: the destination
// changes every tick during a chase, and clients use this field to render a
// move-target marker.
func (s *TransformUpdateSystem) broadcastMoveTarget(movement *components.Movement) bool {
	switch movement.TargetType {
	case constt.TargetPoint:
		return true
	case constt.TargetEntity:
		targetMovement, ok := s.movementStorage.Get(movement.TargetHandle)
		return !ok || targetMovement.State != constt.StateMoving
	default:
		return false
	}
}

func (s *TransformUpdateSystem) applyMovementStaminaTick(
	w *ecs.World,
	handle types.Handle,
	fromX float64,
	fromY float64,
	toX float64,
	toY float64,
) {
	movement, hasMovement := s.movementStorage.Get(handle)
	if !hasMovement {
		return
	}
	stats, hasStats := s.entityStatsStorage.Get(handle)
	if !hasStats {
		return
	}

	con := resolveConForHandle(w, handle)
	maxStamina := entitystats.MaxStaminaFromCon(con)
	currentStamina := entitystats.ClampStamina(stats.Stamina, maxStamina)
	statsChanged := currentStamina != stats.Stamina
	currentEnergy := stats.Energy
	if currentEnergy < 0 {
		currentEnergy = 0
		statsChanged = true
	}

	dx := toX - fromX
	dy := toY - fromY
	moved := dx*dx+dy*dy > 0.000001
	if moved {
		tile := entitystats.MovementTileContext{}
		if entitystats.MovementCostNeedsTileContext() {
			tile = s.resolveMovementTileContext(fromX, fromY)
		}
		cost := entitystats.ResolveMovementStaminaCostPerTick(movement.Mode, con, tile)
		if cost > 0 {
			nextStamina := entitystats.ClampStamina(currentStamina-cost, maxStamina)
			if nextStamina != currentStamina {
				currentStamina = nextStamina
				statsChanged = true
			}
		}
	}

	isCarrying := s.liftCarryStorage.Has(handle)
	allowedMode, canMove := entitystats.ResolveAllowedMoveModeWithCarry(
		movement.Mode,
		currentStamina,
		maxStamina,
		currentEnergy,
		isCarrying,
	)
	modeChanged := movement.Mode != allowedMode
	forceStopped := false
	if !canMove && movement.State == constt.StateMoving {
		forceStopped = true
	}

	if statsChanged {
		stats.Stamina = currentStamina
		stats.Energy = currentEnergy
		s.entityStatsStorage.Set(handle, stats)
		ecs.MarkPlayerStatsDirtyByHandle(w, handle, ecs.ResolvePlayerStatsTTLms(w))
	}

	if modeChanged || !canMove {
		movement.Mode = allowedMode
		if !canMove && movement.State == constt.StateMoving {
			movement.ClearTarget()
		}
		s.movementStorage.Set(handle, movement)
		ecs.MarkMovementModeDirtyByHandle(w, handle)
	}

	if forceStopped && !moved {
		ecs.GetResource[ecs.MovedEntities](w).Add(handle, toX, toY)
	}
	if statsChanged {
		ecs.UpdateEntityStatsRegenSchedule(w, handle, currentStamina, currentEnergy, maxStamina)
	}
}

func (s *TransformUpdateSystem) resolveMovementTileContext(worldX float64, worldY float64) entitystats.MovementTileContext {
	tileSize := float64(constt.CoordPerTile)
	tileX := int(math.Floor(worldX / tileSize))
	tileY := int(math.Floor(worldY / tileSize))

	chunkSize := constt.ChunkSize
	chunkX := floorDiv(tileX, chunkSize)
	chunkY := floorDiv(tileY, chunkSize)
	localTileX := tileX - chunkX*chunkSize
	localTileY := tileY - chunkY*chunkSize

	chunk := s.chunkManager.GetChunk(types.ChunkCoord{X: chunkX, Y: chunkY})
	if chunk == nil {
		return entitystats.MovementTileContext{HasTile: false}
	}
	tileID, ok := chunk.TileID(localTileX, localTileY, chunkSize)
	if !ok {
		return entitystats.MovementTileContext{HasTile: false}
	}
	return entitystats.MovementTileContext{
		TileID:  tileID,
		HasTile: true,
	}
}

func floorDiv(value int, divisor int) int {
	if divisor <= 0 {
		return 0
	}
	if value >= 0 {
		return value / divisor
	}
	return -((-value + divisor - 1) / divisor)
}
