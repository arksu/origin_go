package behaviors

import (
	"fmt"

	"origin/internal/game/behaviors/contracts"
)

type playerBehavior struct{}

func (playerBehavior) Key() string { return "player" }

func (playerBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("player def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "player")
}
