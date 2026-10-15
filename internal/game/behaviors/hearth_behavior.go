package behaviors

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/hearth"
)

type hearthBehavior struct{}

func (hearthBehavior) Key() string { return "hearth" }

func (hearthBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("hearth def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "hearth")
}

func (hearthBehavior) CompleteConstruction(ctx *contracts.ConstructionContext, complete func() bool) contracts.BehaviorResult {
	if ctx != nil && ctx.World != nil && ctx.World.Layer == hearth.SurfaceLayer {
		if runtime, ok := ecs.TryGetResource[hearth.Runtime](ctx.World); ok {
			return runtime.CompleteConstruction(ctx, complete)
		}
	}
	return contracts.BehaviorResult{UserVisible: true, ReasonCode: "Cannot complete this hearth here.", Severity: contracts.BehaviorAlertSeverityWarning}
}
