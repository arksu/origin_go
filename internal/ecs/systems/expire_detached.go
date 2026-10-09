package systems

import (
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

// ExpireDetachedSystem performs bounded, authoritative logout checks after
// combat and death processing. Only an accepted final snapshot permits cleanup.
type ExpireDetachedSystem struct {
	ecs.BaseSystem
	logger *zap.Logger

	// Callback to perform cleanup outside ECS (spatial index, AOI, etc.)
	onExpire func(entityID types.EntityID, handle types.Handle)
	// Batch callback for AOI cleanup/recalculation after per-entity work is done.
	onExpireBatch func(entityIDs []types.EntityID)

	// CharacterSaver for saving character data before despawn
	characterSaver *CharacterSaver

	// Read-only policy callback. The legacy default enforces base delay only.
	checkLogout func(ecs.LogoutContext) (ecs.LogoutDecision, error)
	// Fixed candidate budget: pop before evaluating so one retry is checked once.
	dueBuffer [256]ecs.DetachedCandidate
	// Deferred AOI-unregister buffer to avoid expensive chunk-state recalculation on every small batch.
	pendingUnregister   []types.EntityID
	pendingUnregisterAt time.Time
	unregisterBuffer    [128]types.EntityID
	// Limits all candidate checks, including blocked and invalid bodies.
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
	checkLogout ...func(ecs.LogoutContext) (ecs.LogoutDecision, error),
) *ExpireDetachedSystem {
	system := &ExpireDetachedSystem{
		BaseSystem:        ecs.NewBaseSystem("ExpireDetachedSystem", 950), // Run after ChunkSystem
		logger:            logger,
		onExpire:          onExpire,
		onExpireBatch:     onExpireBatch,
		characterSaver:    characterSaver,
		pendingUnregister: make([]types.EntityID, 0, 512),
		// Every due candidate consumes budget even if it cannot leave yet.
		maxPerTick: 256,
		// Keep each recalc bounded; large single-shot recalcs can freeze shard for seconds.
		unregisterFlushChunkSize:    128,
		maxUnregisterFlushesPerTick: 1,
	}
	if len(checkLogout) > 0 {
		system.checkLogout = checkLogout[0]
	}
	return system
}

func (s *ExpireDetachedSystem) Update(w *ecs.World, dt float64) {
	clock := *ecs.GetResource[ecs.TimeState](w)
	now := clock.Now
	detachedEntities := ecs.GetResource[ecs.DetachedEntities](w)

	// Even when detached map is empty we may still need to flush deferred AOI-unregister work.
	if len(detachedEntities.Map) == 0 && len(s.pendingUnregister) == 0 {
		return
	}

	limit := len(s.dueBuffer)
	if s.maxPerTick > 0 {
		limit = min(limit, s.maxPerTick)
	}
	candidates := detachedEntities.PopDueInto(now, s.dueBuffer[:0:limit])
	processed := 0
	minDelay := clock.TickPeriod
	if minDelay <= 0 {
		minDelay = 100 * time.Millisecond
	}

	var maxDetachedDuration time.Duration
	var minDetachedDuration time.Duration
	if len(candidates) > 0 {
		minDetachedDuration = time.Duration(1<<63 - 1)
	}

	// Process expired entities outside the iteration
	for _, candidate := range candidates {
		entityID := candidate.EntityID
		entity, ok := detachedEntities.Map[entityID]
		if !ok || entity.Handle != candidate.Handle {
			continue
		}

		handle := entity.Handle
		if !w.Alive(handle) {
			if characters := ecs.GetResource[ecs.CharacterEntities](w); characters.Map[entityID].Handle == handle {
				characters.Remove(entityID)
			}
			detachedEntities.RemoveDetachedEntity(entityID)
			detachedEntities.Release(handle)
			continue
		}
		identity, validIdentity := ecs.GetComponent[ecs.ExternalID](w, handle)
		if !validIdentity || identity.ID != entityID || w.GetHandleByEntityID(entityID) != handle {
			// A corrupted identity cannot authorize capture or cleanup of a
			// different character. Keep the body and retry outside this tick.
			detachedEntities.ScheduleNext(entityID, handle, now, time.Second, minDelay)
			continue
		}
		if ecs.InventoryHandleReserved(w, handle) {
			detachedEntities.ScheduleNext(entityID, handle, now, time.Second, minDelay)
			continue
		}
		if now.Before(entity.SaveRetryAt) {
			detachedEntities.SetSaveRetryAt(entityID, handle, entity.SaveRetryAt)
			continue
		}
		decision := ecs.LogoutDecision{}
		var checkErr error
		if s.checkLogout != nil {
			decision, checkErr = s.checkLogout(ecs.LogoutContext{EntityID: entityID, Handle: handle, Detached: entity, Time: clock})
		} else if now.Before(entity.ExpirationTime) {
			decision = ecs.LogoutDecision{Blocked: true, RetryAfter: entity.ExpirationTime.Sub(now)}
		}
		if checkErr != nil || decision.Blocked {
			hint := decision.RetryAfter
			if checkErr != nil {
				hint = time.Second
			}
			detachedEntities.ScheduleNext(entityID, handle, now, hint, minDelay)
			if checkErr != nil {
				s.logger.Error("Detached logout policy rejected state; retaining entity for retry", zap.Uint64("entity_id", uint64(entityID)), zap.Error(checkErr))
			}
			continue
		}
		detachedDuration := now.Sub(entity.DetachedAt)
		if detachedDuration > maxDetachedDuration {
			maxDetachedDuration = detachedDuration
		}
		if detachedDuration < minDetachedDuration {
			minDetachedDuration = detachedDuration
		}

		// Save character data before despawn
		if s.characterSaver != nil {
			if err := s.characterSaver.SaveDetached(w, entityID, handle); err != nil {
				retryAt := now.Add(CharacterSaveCaptureRetryInterval)
				detachedEntities.SetSaveRetryAt(entityID, handle, retryAt)
				if characters := ecs.GetResource[ecs.CharacterEntities](w); characters.Map[entityID].Handle == handle {
					characters.RescheduleSave(entityID, retryAt)
				}
				s.logger.Error("Detached character snapshot rejected; retaining entity for retry",
					zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
				continue
			}
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
		detachedEntities.Release(handle)
		if s.onExpireBatch != nil {
			if len(s.pendingUnregister) == 0 {
				s.pendingUnregisterAt = now
			}
			s.pendingUnregister = append(s.pendingUnregister, entityID)
		}
		processed++
	}

	// Batch post-cleanup hook (e.g. AOI unregister + chunk-state recalc).
	flushedUnregister := 0
	if s.onExpireBatch != nil && len(s.pendingUnregister) > 0 {
		flushChunkSize := s.unregisterFlushChunkSize
		if flushChunkSize <= 0 {
			flushChunkSize = len(s.unregisterBuffer)
		}
		flushChunkSize = min(flushChunkSize, len(s.unregisterBuffer))
		maxFlushes := s.maxUnregisterFlushesPerTick
		if maxFlushes <= 0 {
			maxFlushes = 1
		}

		remainingDetached := len(detachedEntities.Map)
		for flushes := 0; flushes < maxFlushes && len(s.pendingUnregister) > 0; flushes++ {
			// A policy may block another body indefinitely. Bound partial-batch
			// latency independently rather than waiting for all bodies to leave.
			if remainingDetached > 0 && len(s.pendingUnregister) < flushChunkSize && now.Before(s.pendingUnregisterAt.Add(time.Second)) {
				break
			}

			n := flushChunkSize
			if len(s.pendingUnregister) < n {
				n = len(s.pendingUnregister)
			}

			eligible := s.unregisterBuffer[:0]
			for _, entityID := range s.pendingUnregister[:n] {
				// A new body may already own this protocol ID. The old body's
				// delayed unregister must not remove its fresh AOI registration.
				if w.GetHandleByEntityID(entityID) == types.InvalidHandle {
					eligible = append(eligible, entityID)
				}
			}
			if len(eligible) > 0 {
				s.onExpireBatch(eligible)
			}
			flushedUnregister += n

			copy(s.pendingUnregister, s.pendingUnregister[n:])
			s.pendingUnregister = s.pendingUnregister[:len(s.pendingUnregister)-n]
			if len(s.pendingUnregister) == 0 {
				s.pendingUnregisterAt = time.Time{}
			}
		}
	}

	if processed > 0 {
		s.logger.Info("Detached entities expired batch processed",
			zap.Int("processed", processed),
			zap.Int("remaining", len(detachedEntities.Map)),
			zap.Duration("min_detached_duration", minDetachedDuration),
			zap.Duration("max_detached_duration", maxDetachedDuration),
			zap.Int("layer", w.Layer),
			zap.Int("pending_unregister", len(s.pendingUnregister)),
			zap.Int("flushed_unregister", flushedUnregister),
		)
	}
}

// StopMovementForDetached clears movement target for a detached entity
func StopMovementForDetached(w *ecs.World, handle types.Handle) {
	ecs.MutateComponent[components.Movement](w, handle, func(m *components.Movement) bool {
		m.ClearTarget()
		return true
	})
}
