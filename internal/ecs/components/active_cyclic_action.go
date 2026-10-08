package components

import (
	"origin/internal/actionanimationdefs"
	"origin/internal/ecs"
	"origin/internal/types"
)

type CyclicActionTargetKind uint8

const (
	CyclicActionTargetObject CyclicActionTargetKind = 1
	CyclicActionTargetSelf   CyclicActionTargetKind = 2
)

type ActiveCyclicAction struct {
	MutatesItems     bool
	BehaviorKey      string
	ActionID         string
	ActionGeneration uint64
	CompleteSoundKey string
	SoundBinding     *actionanimationdefs.Definition
	NextSoundCue     int

	TargetKind        CyclicActionTargetKind
	TargetID          types.EntityID
	TargetHandle      types.Handle
	HasTargetPosition bool
	HasFacingAngle    bool
	TargetX           float64
	TargetY           float64
	// FacingAngle is the fixed world-space direction accepted at cycle start.
	FacingAngle float64

	CycleDurationTicks      uint32
	CycleElapsedTicks       uint32
	CycleIndex              uint32
	StartedTick             uint64
	ActionCompletionStarted bool
}

const ActiveCyclicActionComponentID ecs.ComponentID = 25

func init() {
	ecs.RegisterComponent[ActiveCyclicAction](ActiveCyclicActionComponentID)
}
