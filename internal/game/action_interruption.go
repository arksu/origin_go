package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// actionExecutionAdmission prepares an accepted action before its timed progress
// starts. Start remains the completion hook for timed actions.
type actionExecutionAdmission interface {
	BeforeExecution(*ecs.World, types.EntityID, types.Handle) string
}

type actionGameplayInterruption interface {
	InterruptOnGameplayAttempt() bool
}

// Interrupt applies the active handler's stricter inactivity policy. Ordinary
// actions keep their existing cancellation rules. Call under the owning shard lock.
func (service *ActionService) Interrupt(world *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	if service == nil || world != service.world || !world.Alive(playerHandle) {
		return
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists {
		return
	}
	policy, ok := service.handlers[active.ActionID].(actionGameplayInterruption)
	if ok && policy.InterruptOnGameplayAttempt() {
		service.Cancel(world, playerID, playerHandle)
	}
}
