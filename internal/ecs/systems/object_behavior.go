package systems

import (
	"reflect"
	"sort"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	// ObjectBehaviorSystemPriority runs after vision and interaction systems.
	ObjectBehaviorSystemPriority = 360
	debugFallbackInterval        = 5 * time.Second
)

type ObjectBehaviorSystem struct {
	ecs.BaseSystem
	logger              *zap.Logger
	runner              *objectBehaviorRunner
	budgetPerTick       int
	enableDebugFallback bool
	debugQuery          *ecs.PreparedQuery
	debugScratch        []types.Handle
	debugCursor         int
	processBatch        []types.Handle
	lastDebugFallbackAt time.Time
}

type ObjectBehaviorConfig struct {
	BudgetPerTick       int
	EnableDebugFallback bool
	BehaviorRegistry    types.BehaviorRegistry
}

func NewObjectBehaviorSystem(eventBus *eventbus.EventBus, logger *zap.Logger, cfg ObjectBehaviorConfig) *ObjectBehaviorSystem {
	if logger == nil {
		logger = zap.NewNop()
	}

	if cfg.BudgetPerTick <= 0 {
		cfg.BudgetPerTick = 512
	}

	return &ObjectBehaviorSystem{
		BaseSystem:          ecs.NewBaseSystem("ObjectBehaviorSystem", ObjectBehaviorSystemPriority),
		logger:              logger,
		runner:              newObjectBehaviorRunner(eventBus, logger, cfg.BehaviorRegistry),
		budgetPerTick:       cfg.BudgetPerTick,
		enableDebugFallback: cfg.EnableDebugFallback,
		processBatch:        make([]types.Handle, 0, cfg.BudgetPerTick),
		debugScratch:        make([]types.Handle, 0, 512),
	}
}

func (s *ObjectBehaviorSystem) Update(w *ecs.World, dt float64) {
	queue := ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](w)
	s.processBatch = queue.Drain(s.budgetPerTick, s.processBatch[:0])
	for _, h := range s.processBatch {
		s.runner.processHandle(w, h)
	}

	if s.enableDebugFallback {
		now := ecs.GetResource[ecs.TimeState](w).Now
		if s.lastDebugFallbackAt.IsZero() || now.Sub(s.lastDebugFallbackAt) >= debugFallbackInterval {
			s.runDebugFallback(w)
			s.lastDebugFallbackAt = now
		}
	}
}

func (s *ObjectBehaviorSystem) runDebugFallback(w *ecs.World) {
	if s.debugQuery == nil {
		s.debugQuery = ecs.NewPreparedQuery(
			w,
			(1<<ecs.ExternalIDComponentID)|
				(1<<components.EntityInfoComponentID)|
				(1<<components.ObjectInternalStateComponentID)|
				(1<<components.AppearanceComponentID),
			0,
		)
	}

	s.debugScratch = s.debugScratch[:0]
	s.debugQuery.ForEach(func(h types.Handle) {
		s.debugScratch = append(s.debugScratch, h)
	})
	if len(s.debugScratch) == 0 {
		s.debugCursor = 0
		return
	}
	if s.debugCursor >= len(s.debugScratch) {
		s.debugCursor = 0
	}

	end := s.debugCursor + s.budgetPerTick
	if end > len(s.debugScratch) {
		end = len(s.debugScratch)
	}

	for _, h := range s.debugScratch[s.debugCursor:end] {
		s.runner.processHandle(w, h)
	}

	s.debugCursor = end
	if s.debugCursor >= len(s.debugScratch) {
		s.debugCursor = 0
	}
}

func RecomputeObjectBehaviorsNow(
	w *ecs.World,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
	behaviorRegistry types.BehaviorRegistry,
	handles []types.Handle,
) {
	if len(handles) == 0 {
		return
	}

	runner := newObjectBehaviorRunner(eventBus, logger, behaviorRegistry)
	for _, h := range handles {
		runner.processHandle(w, h)
	}
}

type objectBehaviorRunner struct {
	eventBus         *eventbus.EventBus
	logger           *zap.Logger
	behaviorRegistry types.BehaviorRegistry
}

func newObjectBehaviorRunner(
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
	behaviorRegistry types.BehaviorRegistry,
) *objectBehaviorRunner {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &objectBehaviorRunner{
		eventBus:         eventBus,
		logger:           logger,
		behaviorRegistry: behaviorRegistry,
	}
}

func (r *objectBehaviorRunner) processHandle(w *ecs.World, h types.Handle) {
	if h == types.InvalidHandle || !w.Alive(h) {
		return
	}

	entityIDComp, hasExternalID := ecs.GetComponent[ecs.ExternalID](w, h)
	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, h)
	currentState, hasState := ecs.GetComponent[components.ObjectInternalState](w, h)
	currentAppearance, hasAppearance := ecs.GetComponent[components.Appearance](w, h)
	if !hasExternalID || !hasInfo || !hasState || !hasAppearance {
		return
	}
	if len(entityInfo.Behaviors) == 0 {
		return
	}

	def, ok := objectdefs.Global().GetByID(int(entityInfo.TypeID))
	if !ok {
		return
	}

	nextState := currentState.State
	hasNextState := false

	ctx := &types.BehaviorRuntimeContext{
		World:      w,
		Handle:     h,
		EntityID:   entityIDComp.ID,
		EntityType: entityInfo.TypeID,
		PrevState:  currentState.State,
		PrevFlags:  append([]string(nil), currentState.Flags...),
	}
	nextFlags := make([]string, 0, len(entityInfo.Behaviors))

	if r.behaviorRegistry == nil {
		return
	}

	for _, behaviorKey := range entityInfo.Behaviors {
		behavior, found := r.behaviorRegistry.GetBehavior(behaviorKey)
		if !found {
			continue
		}
		runtimeBehavior, ok := behavior.(types.RuntimeBehavior)
		if !ok {
			continue
		}
		result := runtimeBehavior.ApplyRuntime(ctx)
		if result.HasState {
			nextState = result.State
			hasNextState = true
		}
		if len(result.Flags) > 0 {
			nextFlags = append(nextFlags, result.Flags...)
		}
	}
	nextFlags = uniqueSortedStrings(nextFlags)

	stateChanged := false
	if hasNextState {
		if currentState.State == nil && nextState == nil {
			stateChanged = false
		} else {
			stateChanged = !reflect.DeepEqual(currentState.State, nextState)
		}
	}
	flagsChanged := !reflect.DeepEqual(currentState.Flags, nextFlags)

	nextResource := objectdefs.ResolveAppearanceResource(def, nextFlags)
	if nextResource == "" {
		nextResource = def.Resource
	}
	appearanceChanged := nextResource != currentAppearance.Resource

	if !stateChanged && !flagsChanged && !appearanceChanged {
		return
	}

	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
		if hasNextState && stateChanged {
			state.State = nextState
		}
		if flagsChanged {
			state.Flags = append(state.Flags[:0], nextFlags...)
		}
		state.IsDirty = true
	})

	if appearanceChanged {
		ecs.WithComponent(w, h, func(appearance *components.Appearance) {
			appearance.Resource = nextResource
		})
		r.publishAppearanceChanged(w, entityIDComp.ID, h)
	}
}

func (r *objectBehaviorRunner) publishAppearanceChanged(w *ecs.World, targetID types.EntityID, targetHandle types.Handle) {
	if r.eventBus == nil {
		return
	}
	r.eventBus.PublishAsync(
		ecs.NewEntityAppearanceChangedEvent(w.Layer, targetID, targetHandle),
		eventbus.PriorityMedium,
	)
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		unique[value] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}

	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
