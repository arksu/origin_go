package network

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"origin/internal/config"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newAuthenticationLifecycleClient(t *testing.T, onDisconnect func(*Client)) *Client {
	t.Helper()
	connection, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	server := NewServer(&config.NetworkConfig{}, &config.GameConfig{}, zap.NewNop())
	server.SetOnDisconnect(onDisconnect)
	client := &Client{ID: 1, conn: connection, server: server, closeCh: make(chan struct{}), sendCh: make(chan []byte, 2)}
	t.Cleanup(client.Close)
	return client
}

func TestAuthenticationCloseDefersDisconnectUntilAssociation(t *testing.T) {
	var notifications int
	client := newAuthenticationLifecycleClient(t, func(c *Client) {
		notifications++
		require.Equal(t, types.EntityID(17), c.CharacterID)
	})
	require.True(t, client.BeginAuthentication())
	client.Close() // The database transaction is still in flight.
	require.Zero(t, notifications)
	client.CharacterID = 17
	client.EndAuthentication()
	require.Equal(t, 1, notifications)
	client.EndAuthentication()
	client.Close()
	require.Equal(t, 1, notifications)
	require.False(t, client.BeginAuthentication())
}

func TestAuthenticationAssociationAndCloseConcurrent(t *testing.T) {
	for i := 0; i < 100; i++ {
		var notifications atomic.Int32
		var observed atomic.Uint64
		client := newAuthenticationLifecycleClient(t, func(c *Client) {
			observed.Store(uint64(c.CharacterID))
			notifications.Add(1)
		})
		require.True(t, client.BeginAuthentication())
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); client.Close() }()
		client.CharacterID = 17
		client.EndAuthentication()
		wg.Wait()
		require.Equal(t, int32(1), notifications.Load())
		require.Equal(t, uint64(17), observed.Load())
	}
}

func TestAuthenticationRejectsRepeatedAssociation(t *testing.T) {
	client := newAuthenticationLifecycleClient(t, nil)
	require.True(t, client.BeginAuthentication())
	require.False(t, client.BeginAuthentication())
	client.CharacterID = 17
	client.EndAuthentication()
	require.False(t, client.BeginAuthentication())
}
