package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

type CyclicActionTargetKind uint8

const (
	CyclicActionTargetObject CyclicActionTargetKind = 1
	CyclicActionTargetSelf   CyclicActionTargetKind = 2
)

type ActiveCyclicAction struct {
	BehaviorKey      string
	ActionID         string
	FinishSoundKey   string
	CompleteSoundKey string

	TargetKind   CyclicActionTargetKind
	TargetID     types.EntityID
	TargetHandle types.Handle

	CycleDurationTicks uint32
	CycleElapsedTicks  uint32
	CycleIndex         uint32
	StartedTick        uint64
}

const ActiveCyclicActionComponentID ecs.ComponentID = 25

func init() {
	ecs.RegisterComponent[ActiveCyclicAction](ActiveCyclicActionComponentID)
}
