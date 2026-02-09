package systems

import (
	"reflect"
	"sort"

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
	eventBus  *eventbus.EventBus
	logger    *zap.Logger
	query     *ecs.PreparedQuery
	behaviors map[string]RuntimeBehavior
}

func NewObjectBehaviorSystem(eventBus *eventbus.EventBus, logger *zap.Logger) *ObjectBehaviorSystem {
	if logger == nil {
		logger = zap.NewNop()
	}

	handlers := map[string]RuntimeBehavior{
		"container": containerBehavior{},
	}

	return &ObjectBehaviorSystem{
		BaseSystem: ecs.NewBaseSystem("ObjectBehaviorSystem", ObjectBehaviorSystemPriority),
		eventBus:   eventBus,
		logger:     logger,
		behaviors:  handlers,
	}
}

func (s *ObjectBehaviorSystem) Update(w *ecs.World, dt float64) {
	if s.query == nil {
		s.query = ecs.NewPreparedQuery(
			w,
			(1<<ecs.ExternalIDComponentID)|
				(1<<components.EntityInfoComponentID)|
				(1<<components.ObjectInternalStateComponentID)|
				(1<<components.AppearanceComponentID),
			0,
		)
	}

	s.query.ForEach(func(h types.Handle) {
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
			behavior, found := s.behaviors[behaviorKey]
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
			s.publishAppearanceChanged(w, entityIDComp.ID, h)
		}
	})
}

func (s *ObjectBehaviorSystem) publishAppearanceChanged(w *ecs.World, targetID types.EntityID, targetHandle types.Handle) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.PublishAsync(
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
