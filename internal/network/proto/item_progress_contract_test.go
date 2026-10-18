package proto

import (
	"testing"

	"github.com/stretchr/testify/require"
	goproto "google.golang.org/protobuf/proto"
)

func TestItemProgressWirePresenceAndTiming(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		progress *ItemProgress
	}{
		{"ordinary", nil},
		{"running", &ItemProgress{DurationSeconds: 120, Timing: &ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: 420}}},
		{"paused", &ItemProgress{DurationSeconds: 120, Timing: &ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: 30}}},
		{"paused zero retains presence", &ItemProgress{DurationSeconds: 120, Timing: &ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: 0}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			original := &ItemInstance{ItemId: 9, TypeId: 3, Progress: scenario.progress}
			wire, err := goproto.Marshal(original)
			require.NoError(t, err)
			var restored ItemInstance
			require.NoError(t, goproto.Unmarshal(wire, &restored))
			require.True(t, goproto.Equal(original, &restored))
			if scenario.progress == nil {
				require.Nil(t, restored.Progress)
			} else {
				require.IsType(t, scenario.progress.Timing, restored.Progress.Timing)
			}
		})
	}
	field := (&ItemInstance{}).ProtoReflect().Descriptor().Fields().ByName("progress")
	require.EqualValues(t, 13, field.Number())
	require.True(t, field.HasPresence())
	timing := (&ItemProgress{}).ProtoReflect().Descriptor().Oneofs().ByName("timing")
	require.Equal(t, 2, timing.Fields().Len())
	require.EqualValues(t, 2, timing.Fields().ByName("completion_runtime_seconds").Number())
	require.EqualValues(t, 3, timing.Fields().ByName("paused_remaining_seconds").Number())
}
