package components

import "origin/internal/ecs"

// StationState is the mutable, authoritative runtime state of a production station.
// Persistent fields are encoded through the world-object state codec.
type StationState struct {
	Capabilities          []string
	CurrentState          string
	Values                map[string]float64
	Resources             map[string]uint32
	AutonomousConsumption []StationAutonomousConsumption
}

type StationAutonomousConsumption struct {
	ResourceKey       string
	AmountPerTick     uint32
	RequiredState     string
	StateWhenDepleted string
}

func (s StationState) HasCapability(capability string) bool {
	for _, candidate := range s.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

// ConsumeResource applies one station-local consumption without allowing underflow.
func (s *StationState) ConsumeResource(resourceKey string, amount uint32) bool {
	if s == nil || resourceKey == "" || amount == 0 || s.Resources == nil {
		return false
	}
	available, exists := s.Resources[resourceKey]
	if !exists || available < amount {
		return false
	}
	s.Resources[resourceKey] = available - amount
	return true
}

func (s StationState) Snapshot() StationState {
	snapshot := StationState{
		Capabilities:          append([]string(nil), s.Capabilities...),
		CurrentState:          s.CurrentState,
		Values:                make(map[string]float64, len(s.Values)),
		Resources:             make(map[string]uint32, len(s.Resources)),
		AutonomousConsumption: append([]StationAutonomousConsumption(nil), s.AutonomousConsumption...),
	}
	for key, value := range s.Values {
		snapshot.Values[key] = value
	}
	for key, amount := range s.Resources {
		snapshot.Resources[key] = amount
	}
	return snapshot
}

const StationStateComponentID ecs.ComponentID = 34

func init() {
	ecs.RegisterComponent[StationState](StationStateComponentID)
}
