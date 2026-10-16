package behaviors

import (
	"fmt"
	"math"

	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
)

type liftBehavior struct{}

func (liftBehavior) Key() string { return "lift" }

func (liftBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("lift config target def is nil")
	}
	def, ok := ctx.Def.(*objectdefs.ObjectDef)
	if !ok || def == nil {
		return 0, fmt.Errorf("lift requires an object definition")
	}
	var cfg objectdefs.LiftBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid lift config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	if size := cfg.PlacementSize; size != nil {
		if size.W <= 0 || size.H <= 0 || math.IsNaN(size.W) || math.IsNaN(size.H) || math.IsInf(size.W, 0) || math.IsInf(size.H, 0) {
			return 0, fmt.Errorf("lift.placementSize w and h must be finite and positive")
		}
	}
	def.LiftConfig = &cfg
	return cfg.Priority, nil
}
