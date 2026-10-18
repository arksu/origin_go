package objectdefs

import (
	"testing"

	"origin/internal/game/behaviors/contracts"

	"github.com/stretchr/testify/require"
)

func TestDryingConfigOwnsOutputSlicesAndAllowsEveryAlternative(t *testing.T) {
	cfg := contracts.DryingBehaviorConfig{Priority: 17, Processes: []contracts.DryingProcessConfig{{
		InputItemKey: "wet", OutputItemKeys: []string{"dry_a", "dry_b"},
		DurationSeconds: 300, VisualCategory: "herbs", InputTypeID: 10, InputWidth: 2, InputHeight: 1,
		Outputs: []contracts.DryingOutputConfig{{TypeID: 11, Width: 1, Height: 1}, {TypeID: 12, Width: 2, Height: 1}},
	}}}
	var def ObjectDef
	def.SetDryingBehaviorConfig(cfg)
	cfg.Processes[0].OutputItemKeys[0] = "changed"
	cfg.Processes[0].Outputs[0].TypeID = 99
	cfg.Processes[0].VisualCategory = "hide"
	require.Equal(t, 17, def.DryingConfig.Priority)
	process := def.DryingConfig.Processes[0]
	require.Equal(t, []string{"dry_a", "dry_b"}, process.OutputItemKeys)
	require.Equal(t, []DryingOutputConfig{{TypeID: 11, Width: 1, Height: 1}, {TypeID: 12, Width: 2, Height: 1}}, process.Outputs)
	require.Equal(t, "herbs", process.VisualCategory)
	require.EqualValues(t, 300, process.DurationSeconds)
	for _, typeID := range []uint32{10, 11, 12} {
		require.True(t, def.DryingConfig.AllowsItemTypeID(typeID))
	}
	for _, typeID := range []uint32{0, 13, 99} {
		require.False(t, def.DryingConfig.AllowsItemTypeID(typeID))
	}
	var missing *DryingBehaviorConfig
	require.False(t, missing.AllowsItemTypeID(10))
}
