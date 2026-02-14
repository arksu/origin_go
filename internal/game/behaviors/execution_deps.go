package behaviors

import "origin/internal/game/behaviors/contracts"

func resolveExecutionDeps(deps *contracts.ExecutionDeps) contracts.ExecutionDeps {
	if deps == nil {
		return contracts.ExecutionDeps{}
	}
	return *deps
}
