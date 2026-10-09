package game

import (
	"errors"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const corpseDecayAdmissionBudget = 100

// Runtime deadlines run before cyclic completion, without a world scan.
type corpseDecaySystem struct {
	ecs.BaseSystem
	shard *Shard
}

func (s *corpseDecaySystem) Update(w *ecs.World, _ float64) {
	s.shard.updateCorpseDecay(w)
}

func corpseDecayDeadline(w *ecs.World, handle types.Handle) (int64, bool) {
	info, exists := ecs.GetComponent[components.EntityInfo](w, handle)
	if !exists {
		return 0, false
	}
	def, exists := objectdefs.Global().GetByID(int(info.TypeID))
	if !exists || def.Key != "player_dead" {
		return 0, false
	}
	state, exists := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !exists {
		return 0, false
	}
	decay, exists := components.GetBehaviorState[components.CorpseDecayBehaviorState](state, "player_dead")
	if !exists || decay == nil {
		return 0, false
	}
	return decay.DecayAtRuntimeSeconds, decay.DecayAtRuntimeSeconds > 0
}

func (s *Shard) beginCorpseDecay(w *ecs.World, handle types.Handle) error {
	if s == nil || w != s.world || s.objectDestruction == nil {
		return ErrInvalidObjectDestructionService
	}
	destination, exists := objectdefs.Global().GetByKey("player_skeleton")
	if !exists {
		return ErrObjectDestructionCapture
	}
	return s.objectDestruction.TransformWithLoot(handle, destination)
}

func (s *Shard) updateCorpseDecay(w *ecs.World) {
	schedule, exists := ecs.TryGetResource[ecs.CorpseDecaySchedule](w)
	if !exists || s.objectDestruction == nil {
		return
	}
	now := ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal
	for count := 0; count < corpseDecayAdmissionBudget; count++ {
		entry, exists := schedule.PopDue(now)
		if !exists {
			break
		}
		id, alive := w.GetExternalID(entry.Handle)
		if !alive || !w.Alive(entry.Handle) || id != entry.EntityID {
			continue
		}
		deadline, corpse := corpseDecayDeadline(w, entry.Handle)
		if !corpse || ecs.ObjectDestructionPending(w, entry.Handle) {
			continue
		}
		if deadline > now {
			schedule.Schedule(entry.Handle, id, deadline)
			continue
		}
		if err := s.beginCorpseDecay(w, entry.Handle); err != nil {
			schedule.Schedule(entry.Handle, id, now+1)
			if !errors.Is(err, ErrObjectDestructionQueueFull) && !errors.Is(err, gameworld.ErrChunkPersistenceBusy) && s.logger != nil {
				s.logger.Warn("Unable to begin corpse decay", zap.Uint64("entity_id", uint64(id)), zap.Error(err))
			}
		}
	}
}

// Existing burner restoration and corpse restoration share one activation hook.
type shardObjectRestoreReconciler struct {
	shard  *Shard
	burner contracts.RestoredObjectReconciler
}

func (r *shardObjectRestoreReconciler) ReconcileRestoredObject(w *ecs.World, handle types.Handle) bool {
	if r.burner != nil && !r.burner.ReconcileRestoredObject(w, handle) {
		return false
	}
	deadline, corpse := corpseDecayDeadline(w, handle)
	if corpse && deadline <= ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal {
		if id, alive := w.GetExternalID(handle); alive {
			// Admission belongs exclusively to the shared per-update budget.
			ecs.EnsureCorpseDecaySchedule(w).Schedule(handle, id, deadline)
		}
	}
	return !ecs.ObjectDestructionPending(w, handle)
}

func (s *Shard) completeCorpseDecay(handle types.Handle, destination *objectdefs.ObjectDef) bool {
	w := s.world
	id, alive := w.GetExternalID(handle)
	if !alive || !w.Alive(handle) {
		return false
	}
	ecs.RemoveComponent[components.InventoryOwner](w, handle)
	ecs.RemoveComponent[components.CorpseVisualState](w, handle)
	opts := gameworld.TransformObjectInPlaceOptions{
		DeleteBehaviorStateKeys: []string{"player_dead"}, ClearFlags: true,
		BehaviorRegistry: behaviors.MustDefaultRegistry(), EventBus: s.eventBus, Logger: s.logger,
	}
	if s.chunkManager != nil {
		opts.Chunks = s.chunkManager
	}
	if !gameworld.TransformObjectToDefInPlace(w, id, handle, destination, opts) {
		return false
	}
	if schedule, exists := ecs.TryGetResource[ecs.CorpseDecaySchedule](w); exists {
		schedule.Cancel(handle)
	}
	if s.chunkManager != nil {
		position, _ := ecs.GetComponent[components.Transform](w, handle)
		ref, _ := ecs.GetComponent[components.ChunkRef](w, handle)
		s.chunkManager.AddStaticToChunkSpatial(handle, ref.CurrentChunkX, ref.CurrentChunkY, int(position.X), int(position.Y))
	}
	return true
}
