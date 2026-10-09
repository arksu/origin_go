package world

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
)

func TestObjectDryingStateRoundTripAndBounds(t *testing.T) {
	internal := components.ObjectInternalState{HP: 42, HasHP: true}
	components.SetBehaviorState(&internal, "drying", &components.DryingBehaviorState{Entries: []components.DryingItemState{
		{InputItemID: 101, InputTypeID: 901, CompletionRuntimeSeconds: 300},
		{InputItemID: 102, InputTypeID: 901, CompletionRuntimeSeconds: 450},
	}})
	payload, exists, err := serializePersistentObjectState(internal)
	require.NoError(t, err)
	require.True(t, exists)
	factory := NewObjectFactory(nil)
	restored, err := factory.DeserializeObjectState(&repository.Object{TypeID: 19, Data: pqtype.NullRawMessage{Valid: true, RawMessage: payload}})
	require.NoError(t, err)
	runtime, ok := restored.(*components.RuntimeObjectState)
	require.True(t, ok)
	drying, ok := runtime.Behaviors["drying"].(*components.DryingBehaviorState)
	require.True(t, ok)
	require.Equal(t, int64(450), drying.Entries[1].CompletionRuntimeSeconds)
	for _, raw := range []string{
		`{"v":1,"behaviors":{"drying":{"entries":[{"input_item_id":0,"input_type_id":1,"completion_runtime_seconds":1}]}}}`,
		`{"v":1,"behaviors":{"drying":{"entries":[{"input_item_id":1,"input_type_id":1,"completion_runtime_seconds":1},{"input_item_id":1,"input_type_id":1,"completion_runtime_seconds":2}]}}}`,
		`{"v":1,"behaviors":{"drying":{"entries":[{"input_item_id":1,"input_type_id":1,"completion_runtime_seconds":1},{"input_item_id":2,"input_type_id":1,"completion_runtime_seconds":1},{"input_item_id":3,"input_type_id":1,"completion_runtime_seconds":1},{"input_item_id":4,"input_type_id":1,"completion_runtime_seconds":1},{"input_item_id":5,"input_type_id":1,"completion_runtime_seconds":1}]}}}`,
	} {
		_, err := factory.DeserializeObjectState(&repository.Object{TypeID: 19, Data: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(raw)}})
		require.Error(t, err)
	}
}

func TestObjectTransformCancelsDryingRuntimeSchedule(t *testing.T) {
	w := ecs.NewWorldForTesting()
	handle := w.Spawn(1, nil)
	ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 19, Behaviors: []string{"drying"}})
	ecs.AddComponent(w, handle, components.ObjectInternalState{HP: 100, HasHP: true})
	schedule := ecs.EnsureBehaviorRuntimeSchedule(w)
	require.True(t, schedule.Schedule(handle, 1, "drying", 300))
	require.True(t, TransformObjectToDefInPlace(w, 1, handle, &objectdefs.ObjectDef{DefID: 12, Key: "crate", HP: 100}, TransformObjectInPlaceOptions{}))
	require.Zero(t, schedule.PendingCount())
}
