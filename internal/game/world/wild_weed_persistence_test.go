package world

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSpotManagerApplyCommittedStateIsOwnedAndMonotonic(t *testing.T) {
	m, _, _ := newSpotTestManager(t, spotTestRow(1, "www", 6, 6, 20))
	newest := SpotStateCommit{SpotID: 1, Snapshot: SpotStateSnapshot{
		State: json.RawMessage(`{"www":{"objects":{}}}`), Revision: 4, LastRuntimeSeconds: 200,
	}}
	require.NoError(t, m.ApplyCommittedState(newest))
	newest.Snapshot.State[1] = 'x'
	for _, revision := range []int64{3, 4} {
		require.NoError(t, m.ApplyCommittedState(SpotStateCommit{SpotID: 1, Snapshot: SpotStateSnapshot{
			State: json.RawMessage(`{"stale":true}`), Revision: revision, LastRuntimeSeconds: 100,
		}}))
	}
	snapshot, ok := m.GetState(1)
	require.True(t, ok)
	require.EqualValues(t, 4, snapshot.Revision)
	require.EqualValues(t, 200, snapshot.LastRuntimeSeconds)
	require.JSONEq(t, `{"www":{"objects":{}}}`, string(snapshot.State))
	require.ErrorIs(t, m.ApplyCommittedState(SpotStateCommit{SpotID: 2, Snapshot: snapshot}), ErrSpotNotFound)
	for _, invalid := range []SpotStateSnapshot{
		{State: json.RawMessage(`{}`)},
		{State: json.RawMessage(`{}`), Revision: 5, LastRuntimeSeconds: -1},
		{State: json.RawMessage(`[]`), Revision: 5},
	} {
		require.ErrorIs(t, m.ApplyCommittedState(SpotStateCommit{SpotID: 1, Snapshot: invalid}), ErrInvalidSpotState)
	}
}

func TestSpotManagerOlderStandaloneAcknowledgementDoesNotReplaceCommittedDeletion(t *testing.T) {
	m, _, queue := newSpotTestManager(t, spotTestRow(1, "www", 6, 6, 20))
	require.NoError(t, m.SaveState(1, 1, json.RawMessage(`{"before_deletion":true}`), 100))
	require.Len(t, queue.jobs, 1)
	// The standalone save commits first, but an object deletion commits and is
	// applied before its delayed worker acknowledgement reaches the owner.
	require.NoError(t, m.ApplyCommittedState(SpotStateCommit{SpotID: 1, Snapshot: SpotStateSnapshot{
		State: json.RawMessage(`{"www":{"objects":{}}}`), Revision: 3, LastRuntimeSeconds: 200,
	}}))
	queue.jobs[0].Complete(nil)
	m.Update(time.Now())
	require.Zero(t, m.PendingCount())
	snapshot, _ := m.GetState(1)
	require.EqualValues(t, 3, snapshot.Revision)
	require.EqualValues(t, 200, snapshot.LastRuntimeSeconds)
	require.JSONEq(t, `{"www":{"objects":{}}}`, string(snapshot.State))
}
