package contracts

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// DryingBehaviorConfig describes immutable one-item drying conversions.
type DryingBehaviorConfig struct {
	Priority  int                   `json:"priority,omitempty"`
	Processes []DryingProcessConfig `json:"processes"`
}

type DryingProcessConfig struct {
	InputItemKey    string `json:"inputItemKey"`
	OutputItemKey   string `json:"outputItemKey"`
	DurationSeconds int64  `json:"durationSeconds"`

	InputTypeID  uint32 `json:"-"`
	OutputTypeID uint32 `json:"-"`
	InputWidth   uint8  `json:"-"`
	InputHeight  uint8  `json:"-"`
	OutputWidth  uint8  `json:"-"`
	OutputHeight uint8  `json:"-"`
}

// DryingBehaviorConfigTarget adds drying without expanding all config targets.
type DryingBehaviorConfigTarget interface {
	SetDryingBehaviorConfig(DryingBehaviorConfig)
}

// RootInventoryMutationContext is dispatched synchronously after a committed
// inventory operation, before another command may change the same root.
type RootInventoryMutationContext struct {
	World      *ecs.World
	Handle     types.Handle
	EntityID   types.EntityID
	EntityType uint32
	Deps       *ExecutionDeps
}

type RootInventoryMutationBehavior interface {
	OnRootInventoryMutation(*RootInventoryMutationContext) (BehaviorTickResult, error)
}
