package network

import (
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestChunkVisibilityOverflowEndsConnection(t *testing.T) {
	connection, peer := net.Pipe()
	defer peer.Close()
	server := &Server{logger: zap.NewNop(), clients: make(map[uint64]*Client)}
	client := &Client{ID: 1, conn: connection, server: server, sendCh: make(chan []byte, 1), closeCh: make(chan struct{})}
	defer client.Close()
	server.clients[1] = client
	if !client.SendChunkVisibility([]byte{1}) {
		t.Fatal("empty queue rejected packet")
	}
	if client.SendChunkVisibility([]byte{2}) {
		t.Fatal("full queue accepted packet")
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("overflow left connection open")
	}
}
