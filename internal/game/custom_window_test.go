package game

import (
	"testing"

	netproto "origin/internal/network/proto"

	"github.com/stretchr/testify/require"
)

func TestCustomWindowTransitionsUseCriticalFIFOAndCurrentSession(t *testing.T) {
	f := newAttackResultFixture(t, 16)
	oldClient, oldConn := f.connect(t, 1, 4)
	client, conn := f.connect(t, 1, 9)
	window := &netproto.S2C_CustomWindow{EntityId: 20, StreamEpoch: 4, Title: "Hearth Fire",
		Fields: []*netproto.CustomWindowField{{Label: "Owner", Value: "Offline owner"}}}
	f.shard.mu.Lock()
	f.shard.SendCustomWindow(1, window)
	f.shard.SendCustomWindowClosed(1, 20)
	window.Fields[0].Value = "Another owner"
	f.shard.SendCustomWindow(1, window)
	f.shard.mu.Unlock()
	require.EqualValues(t, 4, window.StreamEpoch, "do not mutate the caller's snapshot")
	require.Empty(t, drainActionTransitions(t, oldClient, oldConn))
	messages := drainActionTransitions(t, client, conn)
	require.Len(t, messages, 3)
	require.Equal(t, "Offline owner", messages[0].GetCustomWindow().Fields[0].Value)
	require.EqualValues(t, 20, messages[0].GetCustomWindow().EntityId)
	require.EqualValues(t, 9, messages[0].GetCustomWindow().StreamEpoch)
	require.EqualValues(t, 20, messages[1].GetCustomWindowClosed().EntityId)
	require.EqualValues(t, 9, messages[1].GetCustomWindowClosed().StreamEpoch)
	require.Equal(t, "Another owner", messages[2].GetCustomWindow().Fields[0].Value)
	require.EqualValues(t, 9, messages[2].GetCustomWindow().StreamEpoch)
	client.InWorld.Store(false)
	f.shard.mu.Lock()
	f.shard.SendCustomWindow(1, window)
	f.shard.SendCustomWindowClosed(1, 20)
	f.shard.mu.Unlock()
	require.Empty(t, drainActionTransitions(t, client, conn))
}
