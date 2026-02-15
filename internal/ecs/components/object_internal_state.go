package components

import (
	"encoding/json"

	"origin/internal/ecs"
)

// ObjectInternalState tracks runtime state and dirty flag for world objects.
// Added to all non-player entities on chunk activation; used to skip
// unchanged objects during persistence.
type ObjectInternalState struct {
	State   any
	Flags   []string
	IsDirty bool
}

type ObjectStateEnvelope struct {
	Version   int                        `json:"v"`
	Behaviors map[string]json.RawMessage `json:"behaviors,omitempty"`
}

type RuntimeObjectState struct {
	Behaviors map[string]any
}

type TreeBehaviorState struct {
	ChopPoints     int    `json:"chop_points,omitempty"`
	Stage          int    `json:"stage,omitempty"`
	NextGrowthTick uint64 `json:"next_growth_tick,omitempty"`
}

const ObjectInternalStateComponentID ecs.ComponentID = 23

func init() {
	ecs.RegisterComponent[ObjectInternalState](ObjectInternalStateComponentID)
}
