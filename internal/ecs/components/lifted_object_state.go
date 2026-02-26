package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// LiftedObjectState stores runtime restore metadata while an object is carried.
type LiftedObjectState struct {
	CarrierPlayerID types.EntityID
	CarrierHandle   types.Handle

	OriginalIsStatic bool
	HadCollider      bool
	OriginalCollider Collider

	LiftedAtUnixMs int64
}

const LiftedObjectStateComponentID ecs.ComponentID = 31

func init() {
	ecs.RegisterComponent[LiftedObjectState](LiftedObjectStateComponentID)
}

