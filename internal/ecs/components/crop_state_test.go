package components

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCropStatePersistenceValidation(t *testing.T) {
	for _, valid := range []CropBehaviorState{
		{Stage: 0, NextGrowthRuntimeSeconds: 10},
		{Stage: 2, NextGrowthRuntimeSeconds: 99},
		{Stage: 3},
	} {
		data, err := json.Marshal(valid)
		require.NoError(t, err)
		var restored CropBehaviorState
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, valid, restored)
	}
	for _, invalid := range []string{
		`{"stage":-1,"next_growth_runtime_seconds":10}`,
		`{"stage":4}`,
		`{"stage":0}`,
		`{"stage":2,"next_growth_runtime_seconds":-1}`,
		`{"stage":3,"next_growth_runtime_seconds":10}`,
	} {
		var state CropBehaviorState
		require.Error(t, json.Unmarshal([]byte(invalid), &state), invalid)
	}
}
