package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"

	"go.uber.org/zap"
)

const BehaviorTickSystemPriority = 355

type BehaviorTickSystem struct {
	ecs.BaseSystem
	logger           *zap.Logger
	behaviorRegistry contracts.BehaviorRegistry
	budgetPerTick    int
	processBatch     []ecs.BehaviorTickKey
}

type BehaviorTickSystemConfig struct {
	BudgetPerTick    int
	BehaviorRegistry contracts.BehaviorRegistry
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
}

func (s *BehaviorTickSystem) processTickKey(w *ecs.World, currentTick uint64, tickKey ecs.BehaviorTickKey) {
	if tickKey.EntityID == 0 || tickKey.BehaviorKey == "" {
		return
	}

	handle := w.GetHandleByEntityID(tickKey.EntityID)
	if handle == types.InvalidHandle || !w.Alive(handle) {
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

func containsBehaviorKey(behaviorKeys []string, target string) bool {
	for _, behaviorKey := range behaviorKeys {
		if behaviorKey == target {
			return true
		}
	}
	return false
}
