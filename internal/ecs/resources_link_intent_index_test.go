package ecs

import (
	"testing"
	"time"

	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestLinkIntentReverseIndexTracksRetargetAndRemoval(t *testing.T) {
	var links LinkState
	now := time.Now()
	oldTarget, newTarget := types.MakeHandle(1, 1), types.MakeHandle(1, 2)
	links.SetIntent(3, 1, oldTarget, now)
	require.Contains(t, links.IntentPlayersByTarget[oldTarget], types.EntityID(3))
	links.SetIntent(3, 1, newTarget, now)
	require.Empty(t, links.IntentPlayersByTarget[oldTarget])
	require.Contains(t, links.IntentPlayersByTarget[newTarget], types.EntityID(3))
	links.SetIntent(3, 1, newTarget, now)
	require.Len(t, links.IntentPlayersByTarget[newTarget], 1)
	links.ClearIntent(3)
	require.Empty(t, links.IntentPlayersByTarget)
	require.Empty(t, links.IntentByPlayer)
	links.SetIntent(3, 1, types.InvalidHandle, now)
	require.Empty(t, links.IntentPlayersByTarget)
	links.ClearIntent(3)
	require.Empty(t, links.IntentByPlayer)
}
