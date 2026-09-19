package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

// BurnerSystem advances configured burners against the durable server-runtime clock.
type BurnerSystem struct {
	ecs.BaseSystem
	query             *ecs.PreparedQuery
	handles           []types.Handle
	exhaustionHandler BurnerExhaustionHandler
}

// BurnerExhaustionHandler performs a configured outcome after the final fuel
// unit has burned. Returning false leaves the burner intact for a later retry.
type BurnerExhaustionHandler func(w *ecs.World, handle types.Handle, config *objectdefs.BurnerBehaviorConfig) bool

func NewBurnerSystem(exhaustionHandler ...BurnerExhaustionHandler) *BurnerSystem {
	system := &BurnerSystem{BaseSystem: ecs.NewBaseSystem("BurnerSystem", 313)}
	if len(exhaustionHandler) > 0 {
		system.exhaustionHandler = exhaustionHandler[0]
	}
	return system
}

func (s *BurnerSystem) Update(w *ecs.World, dt float64) {
	if s.query == nil {
		s.query = ecs.NewPreparedQuery(w, (1<<components.ObjectInternalStateComponentID)|(1<<components.EntityInfoComponentID), 0)
	}
	s.handles = s.handles[:0]
	s.query.ForEach(func(h types.Handle) { s.handles = append(s.handles, h) })
	now := ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal
	for _, h := range s.handles {
		info, ok := ecs.GetComponent[components.EntityInfo](w, h)
		if !ok {
			continue
		}
		def, ok := objectdefs.Global().GetByID(int(info.TypeID))
		if !ok || def.BurnerConfig == nil {
			continue
		}
		exhausted := false
		ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
			burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner")
			if !ok || burner == nil {
				return
			}
			changed := false
			for burner.Fuel > 0 && now >= burner.NextFuelBurnAtRuntimeSecond {
				burner.Fuel--
				burner.NextFuelBurnAtRuntimeSecond += int64(def.BurnerConfig.SecondsPerFuel)
				changed = true
			}
			if changed {
				state.IsDirty = true
				if burner.Fuel == 0 {
					ecs.MutateComponent[components.StationState](w, h, func(station *components.StationState) bool {
						if station.CurrentState == "burning" {
							station.CurrentState = "unlit"
							return true
						}
						return false
					})
				}
			}
			if burner.Fuel == 0 && !burner.OutcomeCreated && def.BurnerConfig.DropItem != "" {
				exhausted = true
			}
		})
		if exhausted && s.exhaustionHandler != nil && s.exhaustionHandler(w, h, def.BurnerConfig) {
			ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
				burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner")
				if ok && burner != nil {
					burner.OutcomeCreated = true
					state.IsDirty = true
				}
			})
		}
	}
}
