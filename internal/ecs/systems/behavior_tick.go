package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"

	"go.uber.org/zap"
)

// Autonomous behavior transitions must precede cyclic action completion (315).
const BehaviorTickSystemPriority = 313

type BehaviorTickSystem struct {
	ecs.BaseSystem
	logger           *zap.Logger
	behaviorRegistry contracts.BehaviorRegistry
	budgetPerTick    int
	processBatch     []ecs.BehaviorTickKey
	executionDeps    *contracts.ExecutionDeps
}

type BehaviorTickSystemConfig struct {
	BudgetPerTick    int
	BehaviorRegistry contracts.BehaviorRegistry
	ExecutionDeps    *contracts.ExecutionDeps
}

func NewBehaviorTickSystem(logger *zap.Logger, cfg BehaviorTickSystemConfig) *BehaviorTickSystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BudgetPerTick <= 0 {
		cfg.BudgetPerTick = 200
	}

	return &BehaviorTickSystem{
		BaseSystem:       ecs.NewBaseSystem("BehaviorTickSystem", BehaviorTickSystemPriority),
		logger:           logger,
		behaviorRegistry: cfg.BehaviorRegistry,
		budgetPerTick:    cfg.BudgetPerTick,
		processBatch:     make([]ecs.BehaviorTickKey, 0, cfg.BudgetPerTick),
		executionDeps:    cfg.ExecutionDeps,
	}
}

func (s *BehaviorTickSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s.behaviorRegistry == nil {
		return
	}

	timeState := ecs.GetResource[ecs.TimeState](w)
	schedule := ecs.GetResource[ecs.BehaviorTickSchedule](w)
	s.processBatch = schedule.PopDue(timeState.Tick, s.budgetPerTick, s.processBatch[:0])
	for _, tickKey := range s.processBatch {
		s.processTickKey(w, timeState.Tick, tickKey)
	}

	// Tick deadlines retain precedence. Runtime work shares the same budget,
	// including entries rejected by execution-time identity checks.
	runtimeSchedule, exists := ecs.TryGetResource[ecs.BehaviorRuntimeSchedule](w)
	if !exists {
		return
	}
	for remaining := s.budgetPerTick - len(s.processBatch); remaining > 0; remaining-- {
		entry, due := runtimeSchedule.PopDue(timeState.RuntimeSecondsTotal)
		if !due {
			break
		}
		s.processRuntimeEntry(w, timeState.RuntimeSecondsTotal, entry)
	}
}

func (s *BehaviorTickSystem) processTickKey(w *ecs.World, currentTick uint64, tickKey ecs.BehaviorTickKey) {
	if tickKey.EntityID == 0 || tickKey.BehaviorKey == "" {
		return
	}

	handle := w.GetHandleByEntityID(tickKey.EntityID)
	if handle == types.InvalidHandle || !w.Alive(handle) {
		return
	}
	if ecs.ObjectDestructionPending(w, handle) {
		return
	}

	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasInfo || len(entityInfo.Behaviors) == 0 {
		return
	}
	if !containsBehaviorKey(entityInfo.Behaviors, tickKey.BehaviorKey) {
		return
	}

	behavior, found := s.behaviorRegistry.GetBehavior(tickKey.BehaviorKey)
	if !found || behavior == nil {
		return
	}
	tickBehavior, ok := behavior.(contracts.ScheduledTickBehavior)
	if !ok {
		return
	}

	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](w, handle)
	var runtimeState *components.RuntimeObjectState
	if hasInternalState {
		runtimeState, _ = components.GetRuntimeObjectState(internalState)
	}

	result, err := tickBehavior.OnScheduledTick(&contracts.BehaviorTickContext{
		World:        w,
		Handle:       handle,
		EntityID:     tickKey.EntityID,
		EntityType:   entityInfo.TypeID,
		BehaviorKey:  tickKey.BehaviorKey,
		CurrentTick:  currentTick,
		CurrentState: runtimeState,
		Deps:         s.executionDeps,
	})
	if err != nil {
		s.logger.Error("scheduled behavior tick failed",
			zap.Uint64("entity_id", uint64(tickKey.EntityID)),
			zap.String("behavior_key", tickKey.BehaviorKey),
			zap.Error(err),
		)
		return
	}
	if result.StateChanged {
		ecs.MarkObjectBehaviorDirty(w, handle)
	}
}

func (s *BehaviorTickSystem) processRuntimeEntry(w *ecs.World, currentRuntimeSeconds int64, entry ecs.BehaviorRuntimeEntry) {
	if entry.EntityID == 0 || entry.BehaviorKey == "" || !w.Alive(entry.Handle) {
		return
	}
	externalID, hasExternalID := ecs.GetComponent[ecs.ExternalID](w, entry.Handle)
	if !hasExternalID || externalID.ID != entry.EntityID || ecs.ObjectDestructionPending(w, entry.Handle) {
		return
	}
	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, entry.Handle)
	if !hasInfo || !containsBehaviorKey(entityInfo.Behaviors, entry.BehaviorKey) {
		return
	}
	behavior, found := s.behaviorRegistry.GetBehavior(entry.BehaviorKey)
	if !found || behavior == nil {
		return
	}
	runtimeBehavior, ok := behavior.(contracts.ScheduledRuntimeBehavior)
	if !ok {
		return
	}

	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](w, entry.Handle)
	var runtimeState *components.RuntimeObjectState
	if hasInternalState {
		runtimeState, _ = components.GetRuntimeObjectState(internalState)
	}
	result, err := runtimeBehavior.OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{
		World:                 w,
		Handle:                entry.Handle,
		EntityID:              entry.EntityID,
		EntityType:            entityInfo.TypeID,
		BehaviorKey:           entry.BehaviorKey,
		CurrentRuntimeSeconds: currentRuntimeSeconds,
		CurrentState:          runtimeState,
		Deps:                  s.executionDeps,
	})
	if err != nil {
		s.logger.Error("scheduled runtime behavior tick failed",
			zap.Uint64("entity_id", uint64(entry.EntityID)),
			zap.String("behavior_key", entry.BehaviorKey),
			zap.Error(err),
		)
		return
	}
	if result.StateChanged {
		ecs.MarkObjectBehaviorDirty(w, entry.Handle)
	}
}

func containsBehaviorKey(behaviorKeys []string, target string) bool {
	for _, behaviorKey := range behaviorKeys {
		if behaviorKey == target {
			return true
		}
	}
	return false
}
