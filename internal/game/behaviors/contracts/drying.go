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
	InputItemKey    string   `json:"inputItemKey"`
	OutputItemKeys  []string `json:"outputItemKeys"`
	DurationSeconds int64    `json:"durationSeconds"`
	VisualCategory  string   `json:"visualCategory,omitempty"`

	InputTypeID uint32               `json:"-"`
	InputWidth  uint8                `json:"-"`
	InputHeight uint8                `json:"-"`
	Outputs     []DryingOutputConfig `json:"-"`
}

type DryingOutputConfig struct {
	TypeID uint32
	Width  uint8
	Height uint8
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
