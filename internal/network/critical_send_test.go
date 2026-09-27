package network

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net"
	"sync"
	"testing"
	"time"
)

func TestCriticalOverflowIsLocalAndDoesNotWaitForDisconnectLocks(t *testing.T) {
	server := &Server{logger: zap.NewNop(), clients: make(map[uint64]*Client)}
	newClient := func(id uint64) *Client {
		connection, peer := net.Pipe()
		t.Cleanup(func() { peer.Close() })
		client := &Client{ID: id, conn: connection, server: server, sendCh: make(chan []byte, 1), closeCh: make(chan struct{})}
		server.clients[id] = client
		t.Cleanup(client.Close)
		return client
	}
	slow, healthy := newClient(1), newClient(2)
	var callbackLock sync.Mutex
	disconnected := make(chan struct{})
	server.onDisconnect = func(client *Client) {
		if client == slow {
			callbackLock.Lock()
			callbackLock.Unlock()
			close(disconnected)
		}
	}
	slow.Send([]byte{1})
	slow.Send([]byte{2})
	select {
	case <-slow.Done():
		t.Fatal("ordinary send changed behavior")
	default:
	}
	callbackLock.Lock()
	returned := make(chan bool, 1)
	go func() { returned <- slow.SendCritical([]byte{3}) }()
	select {
	case accepted := <-returned:
		require.False(t, accepted)
	case <-time.After(time.Second):
		callbackLock.Unlock()
		t.Fatal("send waited for disconnect callback")
	}
	callbackLock.Unlock()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("connection not closed")
	}
	require.True(t, healthy.SendCritical([]byte{4}))
	select {
	case <-healthy.Done():
		t.Fatal("healthy connection closed")
	default:
	}
	require.False(t, slow.SendCritical([]byte{5}))
}
