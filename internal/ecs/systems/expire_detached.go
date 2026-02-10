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
		BaseSystem:     ecs.NewBaseSystem("ExpireDetachedSystem", 950), // Run after ChunkSystem
		logger:         logger,
		onExpire:       onExpire,
		onExpireBatch:  onExpireBatch,
		characterSaver: characterSaver,
		expiredBuffer:  make([]types.EntityID, 0, 64),
		pendingUnregister: make([]types.EntityID, 0, 512),
		// Throughput guard: with 10Hz ticks, 16/tick caps at ~160 despawns/sec.
		// Use a higher value now that expensive AOI recalculation is deferred.
		maxPerTick:     256,
		// Keep each recalc bounded; large single-shot recalcs can freeze shard for seconds.
		unregisterFlushChunkSize: 128,
		maxUnregisterFlushesPerTick: 1,
	}
}

func (s *ExpireDetachedSystem) Update(w *ecs.World, dt float64) {
	batchStartedAt := time.Now()
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
	var saveTotal time.Duration
	var onExpireTotal time.Duration
	var despawnTotal time.Duration
	var removeCharTotal time.Duration
	var maxSave time.Duration
	var maxOnExpire time.Duration
	var maxDespawn time.Duration
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
			saveStartedAt := time.Now()
			s.characterSaver.SaveDetached(w, entityID, handle)
			d := time.Since(saveStartedAt)
			saveTotal += d
			if d > maxSave {
				maxSave = d
			}
		}

		// Call cleanup callback before despawn (for spatial index, AOI, etc.)
		if s.onExpire != nil {
			onExpireStartedAt := time.Now()
			s.onExpire(entityID, handle)
			d := time.Since(onExpireStartedAt)
			onExpireTotal += d
			if d > maxOnExpire {
				maxOnExpire = d
			}
		}

		// Despawn the entity
		despawnStartedAt := time.Now()
		w.Despawn(handle)
		d := time.Since(despawnStartedAt)
		despawnTotal += d
		if d > maxDespawn {
			maxDespawn = d
		}

		// Remove from CharacterEntities (stop periodic saves while detached)
		removeStartedAt := time.Now()
		ecs.GetResource[ecs.CharacterEntities](w).Remove(entityID)
		removeCharTotal += time.Since(removeStartedAt)

		// Remove from detachedEntities map
		detachedEntities.RemoveDetachedEntity(entityID)
		expiredIDs = append(expiredIDs, entityID)
	}

	// Defer expensive AOI/chunk-state updates and flush in bounded chunks.
	if len(expiredIDs) > 0 {
		s.pendingUnregister = append(s.pendingUnregister, expiredIDs...)
	}

	// Batch post-cleanup hook (e.g. AOI unregister + chunk-state recalc).
	onExpireBatchTotal := time.Duration(0)
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

			onExpireBatchStartedAt := time.Now()
			s.onExpireBatch(s.pendingUnregister[:n])
			onExpireBatchTotal += time.Since(onExpireBatchStartedAt)
			flushedUnregister += n

			copy(s.pendingUnregister, s.pendingUnregister[n:])
			s.pendingUnregister = s.pendingUnregister[:len(s.pendingUnregister)-n]
		}
	}

	if len(expiredIDs) > 0 {
		avgOrZero := func(total time.Duration, n int) time.Duration {
			if n <= 0 {
				return 0
			}
			return time.Duration(int64(total) / int64(n))
		}

		fields := []zap.Field{
			zap.Int("processed", len(expiredIDs)),
			zap.Int("remaining", len(detachedEntities.Map)),
			zap.Duration("min_detached_duration", minDetachedDuration),
			zap.Duration("max_detached_duration", maxDetachedDuration),
			zap.Int("layer", w.Layer),
			zap.Duration("batch_duration", time.Since(batchStartedAt)),
			zap.Duration("save_total", saveTotal),
			zap.Duration("save_avg", avgOrZero(saveTotal, len(expiredIDs))),
			zap.Duration("save_max", maxSave),
			zap.Duration("on_expire_total", onExpireTotal),
			zap.Duration("on_expire_avg", avgOrZero(onExpireTotal, len(expiredIDs))),
			zap.Duration("on_expire_max", maxOnExpire),
			zap.Duration("despawn_total", despawnTotal),
			zap.Duration("despawn_avg", avgOrZero(despawnTotal, len(expiredIDs))),
			zap.Duration("despawn_max", maxDespawn),
			zap.Duration("remove_char_total", removeCharTotal),
			zap.Duration("remove_char_avg", avgOrZero(removeCharTotal, len(expiredIDs))),
			zap.Duration("on_expire_batch_total", onExpireBatchTotal),
			zap.Int("pending_unregister", len(s.pendingUnregister)),
			zap.Int("flushed_unregister", flushedUnregister),
		}
		if s.characterSaver != nil {
			enqueued, dropped := s.characterSaver.SnapshotStats()
			fields = append(fields,
				zap.Int("save_queue_len", s.characterSaver.QueueLen()),
				zap.Uint64("save_enqueued_total", enqueued),
				zap.Uint64("save_dropped_total", dropped),
			)
		}

		s.logger.Info("Detached entities expired batch processed",
			fields...,
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
