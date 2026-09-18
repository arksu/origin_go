package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"
)

const stationSystemPriority = 312

// StationSystem advances autonomous station rules independently of craft actions.
type StationSystem struct {
	ecs.BaseSystem
	eventBus *eventbus.EventBus
	query    *ecs.PreparedQuery
	handles  []types.Handle
}

func NewStationSystem(eventBus *eventbus.EventBus) *StationSystem {
	return &StationSystem{
		BaseSystem: ecs.NewBaseSystem("StationSystem", stationSystemPriority),
		eventBus:   eventBus,
		handles:    make([]types.Handle, 0, 64),
	}
}

func (s *StationSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s.query == nil {
		s.query = ecs.NewPreparedQuery(
			w,
			(1<<components.StationStateComponentID)|
				(1<<components.ObjectInternalStateComponentID),
			0,
		)
	}
	s.handles = s.handles[:0]
	s.query.ForEach(func(handle types.Handle) {
		s.handles = append(s.handles, handle)
	})

	for _, handle := range s.handles {
		changed := false
		ecs.MutateComponent[components.StationState](w, handle, func(station *components.StationState) bool {
			for _, rule := range station.AutonomousConsumption {
				if station.CurrentState != rule.RequiredState {
					continue
				}
				available := station.Resources[rule.ResourceKey]
				if available <= rule.AmountPerTick {
					if available > 0 {
						station.Resources[rule.ResourceKey] = 0
						changed = true
					}
					if station.CurrentState != rule.StateWhenDepleted {
						station.CurrentState = rule.StateWhenDepleted
						changed = true
					}
					continue
				}
				station.Resources[rule.ResourceKey] = available - rule.AmountPerTick
				changed = true
			}
			return changed
		})
		if changed {
			ecs.MutateComponent[components.ObjectInternalState](w, handle, func(state *components.ObjectInternalState) bool {
				state.IsDirty = true
				return true
			})
			stationID, hasStationID := w.GetExternalID(handle)
			if hasStationID && s.eventBus != nil {
				_ = s.eventBus.PublishSync(ecs.NewStationStateChangedEvent(w.Layer, stationID, handle))
			}
		}
	}
}
