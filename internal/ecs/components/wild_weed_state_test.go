package components

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWildWeedStatePreservesSpotIDAndCutStage(t *testing.T) {
	original := WildWeedBehaviorState{SpotID: 9_007_199_254_740_993, Stage: 0, NextGrowthRuntimeSeconds: 28_800}
	encoded, err := json.Marshal(original)
	require.NoError(t, err)
	var restored WildWeedBehaviorState
	require.NoError(t, json.Unmarshal(encoded, &restored))
	require.Equal(t, original, restored)
}

func TestWildWeedStateRejectsInvalidGrowth(t *testing.T) {
	for _, payload := range []string{
		`{"spot_id":-1,"stage":3}`,
		`{"spot_id":1,"stage":4}`,
		`{"spot_id":1,"stage":-1}`,
		`{"spot_id":1,"stage":0}`,
		`{"spot_id":1,"stage":1,"next_growth_runtime_seconds":-1}`,
	} {
		var state WildWeedBehaviorState
		require.Error(t, json.Unmarshal([]byte(payload), &state), payload)
	}
	var mature WildWeedBehaviorState
	require.NoError(t, json.Unmarshal([]byte(`{"spot_id":0,"stage":3}`), &mature))
}
