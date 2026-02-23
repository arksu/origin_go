package components

import (
	"origin/internal/ecs"
)

// PendingBuildPlacement stores a server-side armed build placement.
// The player moves toward a phantom collider first; real object spawn happens on phantom collision.
type PendingBuildPlacement struct {
	BuildKey           string
	BuildDefID         int
	ResultObjectKey    string
	ResultObjectTypeID uint32

	TargetX int
	TargetY int

	PhantomHalfWidth  float64
	PhantomHalfHeight float64

	ExpireAtUnixMs int64
}

const PendingBuildPlacementComponentID ecs.ComponentID = 29

func init() {
	ecs.RegisterComponent[PendingBuildPlacement](PendingBuildPlacementComponentID)
}
