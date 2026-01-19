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

	// CharacterSaver for saving character data before despawn
	characterSaver *CharacterSaver

	// Reusable buffer for expired entities to avoid allocations
	expiredBuffer []types.EntityID
}

// NewExpireDetachedSystem creates a new ExpireDetachedSystem
// onExpire callback is called for each expired entity before despawn to allow cleanup
func NewExpireDetachedSystem(logger *zap.Logger, characterSaver *CharacterSaver, onExpire func(entityID types.EntityID, handle types.Handle)) *ExpireDetachedSystem {
	return &ExpireDetachedSystem{
		BaseSystem:     ecs.NewBaseSystem("ExpireDetachedSystem", 950), // Run after ChunkSystem
		logger:         logger,
		onExpire:       onExpire,
		characterSaver: characterSaver,
		expiredBuffer:  make([]types.EntityID, 0, 64),
	}
}

func (s *ExpireDetachedSystem) Update(w *ecs.World, dt float64) {
	now := time.Now()
	detachedEntities := w.DetachedEntities()

	if len(detachedEntities.Map) == 0 {
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

	// Process expired entities outside the iteration
	for _, entityID := range s.expiredBuffer {
		entity, ok := detachedEntities.Map[entityID]
		if !ok {
			continue
		}

		handle := entity.Handle
		detachedDuration := now.Sub(entity.DetachedAt)

		s.logger.Info("Detached entity expired, despawning",
			zap.Uint64("entity_id", uint64(entityID)),
			zap.Duration("detached_duration", detachedDuration),
			zap.Int("layer", w.Layer),
		)

		// Save character data before despawn
		if s.characterSaver != nil {
			s.characterSaver.Save(w, entityID, handle)
		}

		// Call cleanup callback before despawn (for spatial index, AOI, etc.)
		if s.onExpire != nil {
			s.onExpire(entityID, handle)
		}

		// Despawn the entity
		w.Despawn(handle)

		// Remove from CharacterEntities (stop periodic saves while detached)
		w.CharacterEntities().Remove(entityID)

		// Remove from detachedEntities map
		detachedEntities.RemoveDetachedEntity(entityID)
	}
}

// DespawnDetachedEntity manually despawns a detached entity (e.g., when killed)
// This is called from outside the system when a detached entity dies
func DespawnDetachedEntity(w *ecs.World, entityID types.EntityID, logger *zap.Logger) bool {
	detachedEntities := w.DetachedEntities()
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
