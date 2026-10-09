package components

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDryingStateRoundTripsRuntimeDeadlineAndItemIdentity(t *testing.T) {
	original := DryingBehaviorState{Entries: []DryingItemState{{InputItemID: 9007199254740993, InputTypeID: 10, CompletionRuntimeSeconds: 112233}}}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	var restored DryingBehaviorState
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, original, restored)
}

func TestDryingStateRejectsUnboundedOrAmbiguousRestoredWork(t *testing.T) {
	for _, raw := range []string{
		`{"entries":[{"input_item_id":0,"input_type_id":10,"completion_runtime_seconds":300}]}`,
		`{"entries":[{"input_item_id":1,"input_type_id":0,"completion_runtime_seconds":300}]}`,
		`{"entries":[{"input_item_id":1,"input_type_id":10,"completion_runtime_seconds":0}]}`,
		`{"entries":[{"input_item_id":1,"input_type_id":10,"completion_runtime_seconds":-1}]}`,
		`{"entries":[{"input_item_id":1,"input_type_id":10,"completion_runtime_seconds":300},{"input_item_id":1,"input_type_id":10,"completion_runtime_seconds":400}]}`,
		`{"entries":[{"input_item_id":1,"input_type_id":10,"completion_runtime_seconds":300},{"input_item_id":2,"input_type_id":10,"completion_runtime_seconds":300},{"input_item_id":3,"input_type_id":10,"completion_runtime_seconds":300},{"input_item_id":4,"input_type_id":10,"completion_runtime_seconds":300},{"input_item_id":5,"input_type_id":10,"completion_runtime_seconds":300}]}`,
	} {
		var state DryingBehaviorState
		require.Error(t, json.Unmarshal([]byte(raw), &state), raw)
	}
	var empty DryingBehaviorState
	require.NoError(t, json.Unmarshal([]byte(`{}`), &empty))
	require.Empty(t, empty.Entries)
}
