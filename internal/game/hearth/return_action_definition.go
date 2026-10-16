package hearth

import (
	"fmt"
	"math"

	"origin/internal/actiondefs"
)

const ReturnActionID = "travel_to_hearth_fire"

// PrepareReturnActionDefinition runs before the registry is published. Both the
// action catalog and the ordinary cycle then use the configured five seconds.
func PrepareReturnActionDefinition(registry *actiondefs.Registry, tickRate int) error {
	definition, found := registry.Get(ReturnActionID)
	if !found || definition.Target.Kind != actiondefs.TargetNone || definition.Execution.Stamina != 0 ||
		definition.Execution.Repeat || definition.Cooldown != 360000 {
		return fmt.Errorf("invalid Hearth return action definition")
	}
	if tickRate <= 0 || tickRate > math.MaxUint32/5 {
		return fmt.Errorf("invalid Hearth return tick rate: %d", tickRate)
	}
	definition.Execution.Ticks = 5 * tickRate
	return nil
}
