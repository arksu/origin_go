package components

import "origin/internal/ecs"

// ActionAnimation retains the last public revision after the gameplay cycle ends.
// It is transient and is discarded with the entity incarnation.
type ActionAnimation struct {
	Key         string
	Revision    uint64
	CycleIndex  uint32
	StartedTick uint64
	TotalTicks  uint32
	ExecutionID uint64
}

const ActionAnimationComponentID ecs.ComponentID = 36

func init() { ecs.RegisterComponent[ActionAnimation](ActionAnimationComponentID) }
