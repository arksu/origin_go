package ecs

import (
	"github.com/stretchr/testify/require"
	"origin/internal/types"
	"testing"
)

func TestSpawnBatchOwnsTargets(t *testing.T) {
	entries := []SpawnBatchEntry{{EntityID: 12, Handle: types.Handle(15)}}
	event := NewEntitySpawnBatchEvent(7, entries, 2)
	entries[0] = SpawnBatchEntry{EntityID: 99}
	require.Equal(t, []SpawnBatchEntry{{EntityID: 12, Handle: types.Handle(15)}}, event.Entries)
	require.Equal(t, TopicGameplayEntitySpawnBatch, event.Topic())
}

func TestMoveBatchOwnsEntriesAndCoordinates(t *testing.T) {
	targetX, targetY := 10, 20
	entries := []MoveBatchEntry{{EntityID: 12, X: 1, TargetX: &targetX, TargetY: &targetY}}
	event := NewObjectMoveBatchEvent(2, entries)
	targetX, targetY = 99, 100
	entries[0].X = 42
	require.Equal(t, 1, event.Entries[0].X)
	require.Equal(t, 10, *event.Entries[0].TargetX)
	require.Equal(t, 20, *event.Entries[0].TargetY)
}
