package behaviors

import (
	"fmt"

	"origin/internal/game/behaviors/contracts"
)

type liftBehavior struct{}

func (liftBehavior) Key() string { return "lift" }

func (liftBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("lift def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "lift")
}
