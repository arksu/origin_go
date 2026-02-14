package behaviors

import (
	"fmt"

	"origin/internal/types"
)

type playerBehavior struct{}

func (playerBehavior) Key() string { return "player" }

func (playerBehavior) ValidateAndApplyDefConfig(ctx *types.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("player def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "player")
}
