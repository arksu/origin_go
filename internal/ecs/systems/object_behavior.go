package systems

import (
	"reflect"
	"sort"
	"time"

	constt "origin/internal/const"
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

type BehaviorResult struct {
	Flags    []string
	State    any
	HasState bool
}

type BehaviorContext struct {
	World      *ecs.World
	Handle     types.Handle
	EntityID   types.EntityID
	EntityInfo components.EntityInfo
	Def        *objectdefs.ObjectDef
	PrevState  any
	PrevFlags  []string
}

type RuntimeBehavior interface {
	Key() string
	Apply(ctx *BehaviorContext) BehaviorResult
}

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
		runner:              newObjectBehaviorRunner(eventBus, logger),
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

func RecomputeObjectBehaviorsNow(w *ecs.World, eventBus *eventbus.EventBus, logger *zap.Logger, handles []types.Handle) {
	if len(handles) == 0 {
		return
	}

	runner := newObjectBehaviorRunner(eventBus, logger)
	for _, h := range handles {
		runner.processHandle(w, h)
	}
}

type objectBehaviorRunner struct {
	eventBus  *eventbus.EventBus
	logger    *zap.Logger
	behaviors map[string]RuntimeBehavior
}

func newObjectBehaviorRunner(eventBus *eventbus.EventBus, logger *zap.Logger) *objectBehaviorRunner {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &objectBehaviorRunner{
		eventBus: eventBus,
		logger:   logger,
		behaviors: map[string]RuntimeBehavior{
			"container": containerBehavior{},
		},
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

	nextFlagsSet := make(map[string]struct{}, 8)
	nextState := currentState.State
	hasNextState := false

	ctx := &BehaviorContext{
		World:      w,
		Handle:     h,
		EntityID:   entityIDComp.ID,
		EntityInfo: entityInfo,
		Def:        def,
		PrevState:  currentState.State,
		PrevFlags:  currentState.Flags,
	}

	for _, behaviorKey := range entityInfo.Behaviors {
		behavior, found := r.behaviors[behaviorKey]
		if !found {
			continue
		}
		result := behavior.Apply(ctx)
		for _, flag := range result.Flags {
			if flag == "" {
				continue
			}
			nextFlagsSet[flag] = struct{}{}
		}
		if result.HasState {
			nextState = result.State
			hasNextState = true
		}
	}

	nextFlags := setToSortedSlice(nextFlagsSet)
	flagsChanged := !equalStringSlices(currentState.Flags, nextFlags)

	stateChanged := false
	if hasNextState {
		stateChanged = !reflect.DeepEqual(currentState.State, nextState)
	}

	nextResource := objectdefs.ResolveAppearanceResource(def, nextFlags)
	if nextResource == "" {
		nextResource = def.Resource
	}
	appearanceChanged := nextResource != currentAppearance.Resource

	if !flagsChanged && !stateChanged && !appearanceChanged {
		return
	}

	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
		if flagsChanged {
			state.Flags = nextFlags
		}
		if hasNextState && stateChanged {
			state.State = nextState
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

func setToSortedSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type containerBehavior struct{}

func (containerBehavior) Key() string { return "container" }

func (containerBehavior) Apply(ctx *BehaviorContext) BehaviorResult {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](ctx.World)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, ctx.EntityID, 0)
	if !found || !ctx.World.Alive(rootHandle) {
		return BehaviorResult{}
	}

	rootContainer, hasContainer := ecs.GetComponent[components.InventoryContainer](ctx.World, rootHandle)
	if !hasContainer || len(rootContainer.Items) == 0 {
		return BehaviorResult{}
	}

	return BehaviorResult{
		Flags: []string{"container.has_items"},
	}
}
