package hearth

import (
	"math"
	"testing"

	"origin/internal/actiondefs"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReturnActionDefinitionUsesConfiguredTickRate(t *testing.T) {
	for _, tickRate := range []int{1, 10, 30, 60} {
		registry, err := actiondefs.LoadFromDirectory("../../../data/actions", zap.NewNop())
		require.NoError(t, err)
		require.NoError(t, PrepareReturnActionDefinition(registry, tickRate))
		definition, found := registry.Get(ReturnActionID)
		require.True(t, found)
		require.Equal(t, 5*tickRate, definition.Execution.Ticks)
		require.Zero(t, definition.Execution.Stamina)
		require.Equal(t, uint32(360000), definition.Cooldown)
	}
}

func TestReturnActionDefinitionRejectsInvalidStartup(t *testing.T) {
	require.Error(t, PrepareReturnActionDefinition(nil, 10))
	for _, tickRate := range []int{0, -1, math.MaxUint32/5 + 1} {
		registry := actiondefs.NewRegistry([]actiondefs.Definition{{
			ID: ReturnActionID, Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Cooldown: 360000,
		}})
		require.Error(t, PrepareReturnActionDefinition(registry, tickRate))
	}
}
