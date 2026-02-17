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
	movedEntities := ecs.GetResource[ecs.MovedEntities](w)
	serverTimeMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	s.moveBatch = s.moveBatch[:0]

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

		// Accumulate movement data for batch event
		if entityID, ok := w.GetExternalID(h); ok {
			// Visibility guard: only include if entity is visible to at least one observer
			visState := ecs.GetResource[ecs.VisibilityState](w)
			if visState != nil {
				observers, hasObservers := visState.ObserversByVisibleTarget[h]
				if !hasObservers || len(observers) == 0 {
					goto saveCollision
				}
			}

			// Get movement component for velocity data
			{
				movement, hasMovement := ecs.GetComponent[components.Movement](w, h)

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

					ecs.WithComponent(w, h, func(m *components.Movement) {
						m.MoveSeq++
					})
				}

				var targetX, targetY *int
				if hasMovement && movement.TargetType == constt.TargetPoint {
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
		ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
			// Save current collision position for next frame
			cr.PrevFinalX = cr.FinalX
			cr.PrevFinalY = cr.FinalY
			cr.PrevCollidedWith = cr.CollidedWith
			// Clear collision result for next frame
			cr.HasCollision = false
			cr.CollidedWith = 0
			cr.CollisionNormalX = 0
			cr.CollisionNormalY = 0
			cr.IsPhantom = false
			cr.PerpendicularOscillation = false
		})
	}

	// Publish single batch event for all movements this tick
	if len(s.moveBatch) > 0 {
		s.eventBus.PublishAsync(
			ecs.NewObjectMoveBatchEvent(w.Layer, s.moveBatch),
			eventbus.PriorityMedium,
		)
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
	movement, hasMovement := ecs.GetComponent[components.Movement](w, handle)
	if !hasMovement {
		return
	}
	stats, hasStats := ecs.GetComponent[components.EntityStats](w, handle)
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

	allowedMode, canMove := entitystats.ResolveAllowedMoveMode(movement.Mode, currentStamina, maxStamina)
	modeChanged := movement.Mode != allowedMode
	forceStopped := false
	if !canMove && movement.State == constt.StateMoving {
		forceStopped = true
	}

	if statsChanged {
		ecs.WithComponent(w, handle, func(entityStats *components.EntityStats) {
			entityStats.Stamina = currentStamina
			entityStats.Energy = currentEnergy
		})
		ecs.MarkPlayerStatsDirtyByHandle(w, handle, ecs.ResolvePlayerStatsTTLms(w))
	}

	if modeChanged || !canMove {
		ecs.WithComponent(w, handle, func(m *components.Movement) {
			m.Mode = allowedMode
			if !canMove {
				if m.Mode != constt.Crawl {
					m.Mode = constt.Crawl
				}
				if m.State == constt.StateMoving {
					m.ClearTarget()
				}
			}
		})
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
