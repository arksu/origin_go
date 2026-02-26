package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

type LiftTransitionMode uint8

const (
	LiftTransitionModeNone LiftTransitionMode = iota
	LiftTransitionModePickupNoCollider
	LiftTransitionModePutDown
)

// PendingLiftTransition arms a phantom-collider finalize step for lift pickup/put-down.
type PendingLiftTransition struct {
	Mode LiftTransitionMode

	ObjectEntityID types.EntityID
	ObjectHandle   types.Handle

	TargetX float64
	TargetY float64

	UsesObjectCollider bool
	PhantomHalfW       float64
	PhantomHalfH       float64

	ExpireAtUnixMs int64
}

const PendingLiftTransitionComponentID ecs.ComponentID = 32

func init() {
	ecs.RegisterComponent[PendingLiftTransition](PendingLiftTransitionComponentID)
}

