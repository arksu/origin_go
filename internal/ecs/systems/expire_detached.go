package systems

import (
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

// ExpireDetachedSystem handles expiration of detached player entities
// When a player disconnects, their entity remains in the world for DisconnectDelay seconds.
// This system checks for expired detached entities and despawns them.
type ExpireDetachedSystem struct {
	ecs.BaseSystem
	logger *zap.Logger

	// Callback to perform cleanup outside ECS (spatial index, AOI, etc.)
	onExpire func(entityID types.EntityID, handle types.Handle)
	// Batch callback for AOI cleanup/recalculation after per-entity work is done.
	onExpireBatch func(entityIDs []types.EntityID)

	// CharacterSaver for saving character data before despawn
	characterSaver *CharacterSaver

	// Reusable buffer for expired entities to avoid allocations
	expiredBuffer []types.EntityID
	// Deferred AOI-unregister buffer to avoid expensive chunk-state recalculation on every small batch.
	pendingUnregister []types.EntityID
	// Safety valve: limit detached despawns done in one tick to avoid long frame stalls.
	maxPerTick int
	// Max entities in one AOI-unregister flush call.
	unregisterFlushChunkSize int
	// Max AOI-unregister flush calls per tick.
	maxUnregisterFlushesPerTick int
}

// NewExpireDetachedSystem creates a new ExpireDetachedSystem
// onExpire callback is called for each expired entity before despawn to allow cleanup
func NewExpireDetachedSystem(
	logger *zap.Logger,
	characterSaver *CharacterSaver,
	onExpire func(entityID types.EntityID, handle types.Handle),
	onExpireBatch func(entityIDs []types.EntityID),
) *ExpireDetachedSystem {
	return &ExpireDetachedSystem{
		BaseSystem:        ecs.NewBaseSystem("ExpireDetachedSystem", 950), // Run after ChunkSystem
		logger:            logger,
		onExpire:          onExpire,
		onExpireBatch:     onExpireBatch,
		characterSaver:    characterSaver,
		expiredBuffer:     make([]types.EntityID, 0, 64),
		pendingUnregister: make([]types.EntityID, 0, 512),
		// Throughput guard: with 10Hz ticks, 16/tick caps at ~160 despawns/sec.
		// Use a higher value now that expensive AOI recalculation is deferred.
		maxPerTick: 256,
		// Keep each recalc bounded; large single-shot recalcs can freeze shard for seconds.
		unregisterFlushChunkSize:    128,
		maxUnregisterFlushesPerTick: 1,
	}
}

func (s *ExpireDetachedSystem) Update(w *ecs.World, dt float64) {
	now := ecs.GetResource[ecs.TimeState](w).Now
	detachedEntities := ecs.GetResource[ecs.DetachedEntities](w)

	// Even when detached map is empty we may still need to flush deferred AOI-unregister work.
	if len(detachedEntities.Map) == 0 && len(s.pendingUnregister) == 0 {
		return
	}

	// Reset buffer
	s.expiredBuffer = s.expiredBuffer[:0]

	// Collect expired entities
	for entityID, entity := range detachedEntities.Map {
		if now.After(entity.ExpirationTime) {
			s.expiredBuffer = append(s.expiredBuffer, entityID)
		}
	}

	limit := len(s.expiredBuffer)
	if s.maxPerTick > 0 && limit > s.maxPerTick {
		limit = s.maxPerTick
	}

	expiredIDs := make([]types.EntityID, 0, limit)

	var maxDetachedDuration time.Duration
	var minDetachedDuration time.Duration
	if limit > 0 {
		minDetachedDuration = time.Duration(1<<63 - 1)
	}

	// Process expired entities outside the iteration
	for i := 0; i < limit; i++ {
		entityID := s.expiredBuffer[i]
		entity, ok := detachedEntities.Map[entityID]
		if !ok {
			continue
		}

		handle := entity.Handle
		detachedDuration := now.Sub(entity.DetachedAt)
		if detachedDuration > maxDetachedDuration {
			maxDetachedDuration = detachedDuration
		}
		if detachedDuration < minDetachedDuration {
			minDetachedDuration = detachedDuration
		}

		// Save character data before despawn
		if s.characterSaver != nil {
			s.characterSaver.SaveDetached(w, entityID, handle)
		}

		// Call cleanup callback before despawn (for spatial index, AOI, etc.)
		if s.onExpire != nil {
			s.onExpire(entityID, handle)
		}

		// Detached/despawned player must behave like an unlink to downstream systems.
		if _, _, err := ecs.BreakLinkForPlayer(w, entityID, ecs.LinkBreakDespawn); err != nil {
			s.logger.Warn("Failed to publish LinkBroken for expired detached entity",
				zap.Error(err),
				zap.Uint64("entity_id", uint64(entityID)),
				zap.Int("layer", w.Layer),
			)
		}

		// Despawn the entity
		w.Despawn(handle)

		// Remove from CharacterEntities (stop periodic saves while detached)
		ecs.GetResource[ecs.CharacterEntities](w).Remove(entityID)

		// Remove from detachedEntities map
		detachedEntities.RemoveDetachedEntity(entityID)
		expiredIDs = append(expiredIDs, entityID)
	}

	// Defer expensive AOI/chunk-state updates and flush in bounded chunks.
	if len(expiredIDs) > 0 {
		s.pendingUnregister = append(s.pendingUnregister, expiredIDs...)
	}

	// Batch post-cleanup hook (e.g. AOI unregister + chunk-state recalc).
	flushedUnregister := 0
	if s.onExpireBatch != nil && len(s.pendingUnregister) > 0 {
		flushChunkSize := s.unregisterFlushChunkSize
		if flushChunkSize <= 0 {
			flushChunkSize = len(s.pendingUnregister)
		}
		maxFlushes := s.maxUnregisterFlushesPerTick
		if maxFlushes <= 0 {
			maxFlushes = 1
		}

		remainingDetached := len(detachedEntities.Map)
		for flushes := 0; flushes < maxFlushes && len(s.pendingUnregister) > 0; flushes++ {
			// If detached queue is still active, flush only when enough work accumulated.
			if remainingDetached > 0 && len(s.pendingUnregister) < flushChunkSize {
				break
			}

			n := flushChunkSize
			if len(s.pendingUnregister) < n {
				n = len(s.pendingUnregister)
			}

			s.onExpireBatch(s.pendingUnregister[:n])
			flushedUnregister += n

			copy(s.pendingUnregister, s.pendingUnregister[n:])
			s.pendingUnregister = s.pendingUnregister[:len(s.pendingUnregister)-n]
		}
	}

	if len(expiredIDs) > 0 {
		s.logger.Info("Detached entities expired batch processed",
			zap.Int("processed", len(expiredIDs)),
			zap.Int("remaining", len(detachedEntities.Map)),
			zap.Duration("min_detached_duration", minDetachedDuration),
			zap.Duration("max_detached_duration", maxDetachedDuration),
			zap.Int("layer", w.Layer),
			zap.Int("pending_unregister", len(s.pendingUnregister)),
			zap.Int("flushed_unregister", flushedUnregister),
		)
	}
}

// DespawnDetachedEntity manually despawns a detached entity (e.g., when killed)
// This is called from outside the system when a detached entity dies
func DespawnDetachedEntity(w *ecs.World, entityID types.EntityID, logger *zap.Logger) bool {
	detachedEntities := ecs.GetResource[ecs.DetachedEntities](w)
	entity, ok := detachedEntities.GetDetachedEntity(entityID)
	if !ok {
		return false
	}

	handle := entity.Handle
	if !w.Alive(handle) {
		detachedEntities.RemoveDetachedEntity(entityID)
		return false
	}

	detachedDuration := time.Since(entity.DetachedAt)
	logger.Info("Detached entity manually despawned",
		zap.Uint64("entity_id", uint64(entityID)),
		zap.Duration("detached_duration", detachedDuration),
		zap.Int("layer", w.Layer),
	)

	if _, _, err := ecs.BreakLinkForPlayer(w, entityID, ecs.LinkBreakDespawn); err != nil {
		logger.Warn("Failed to publish LinkBroken for manually despawned detached entity",
			zap.Error(err),
			zap.Uint64("entity_id", uint64(entityID)),
			zap.Int("layer", w.Layer),
		)
	}

	// Despawn the entity
	w.Despawn(handle)

	// Remove from detached map
	detachedEntities.RemoveDetachedEntity(entityID)

	return true
}

// StopMovementForDetached clears movement target for a detached entity
func StopMovementForDetached(w *ecs.World, handle types.Handle) {
	ecs.MutateComponent[components.Movement](w, handle, func(m *components.Movement) bool {
		m.ClearTarget()
		return true
	})
}
