package components

import "origin/internal/ecs"

// CombatTestTarget marks every transient range fixture, including non-receivers.
// Persistence must skip this component even if another system marks it dirty.
type CombatTestTarget struct {
	HP, MaxHP float64
	Revision  uint64
	Receiver  bool
}

const CombatTestTargetComponentID ecs.ComponentID = 38

func init() { ecs.RegisterComponent[CombatTestTarget](CombatTestTargetComponentID) }
