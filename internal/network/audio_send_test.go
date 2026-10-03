package network

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"

	"origin/internal/config"
)

func newAudioTestClient(t *testing.T, gameplayCapacity, audioCapacity int) (*Client, net.Conn) {
	t.Helper()
	connection, peer := net.Pipe()
	server := NewServer(&config.NetworkConfig{WriteTimeout: time.Second}, nil, zap.NewNop())
	client := &Client{
		ID: 1, conn: connection, server: server, logger: zap.NewNop(),
		sendCh: make(chan []byte, gameplayCapacity), audioCh: make(chan []byte, audioCapacity),
		closeCh: make(chan struct{}), writeBuf: bufio.NewWriterSize(connection, 4096),
	}
	client.InWorld.Store(true)
	server.clients[client.ID] = client
	t.Cleanup(func() {
		client.Close()
		peer.Close()
		server.cancel()
	})
	return client, peer
}

func readAudioTestFrame(t *testing.T, peer net.Conn) string {
	t.Helper()
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	payload, operation, err := wsutil.ReadServerData(peer)
	if err != nil {
		t.Fatal(err)
	}
	if operation != ws.OpBinary {
		t.Fatalf("unexpected frame operation: %v", operation)
	}
	return string(payload)
}

func TestAudioBurstDoesNotOccupyGameplayQueue(t *testing.T) {
	client, _ := newAudioTestClient(t, 1, 1)
	if got := client.SendAudio([]byte("first audio")); got != AudioSendAccepted {
		t.Fatalf("empty audio queue rejected payload: %v", got)
	}
	for attempt := 0; attempt < 1000; attempt++ {
		if got := client.SendAudio([]byte("excess audio")); got != AudioSendFull {
			t.Fatalf("full audio queue result: %v", got)
		}
	}
	if len(client.sendCh) != 0 || len(client.audioCh) != 1 {
		t.Fatal("audio consumed gameplay capacity or exceeded its own capacity")
	}
	if !client.SendCritical([]byte("critical state")) {
		t.Fatal("audio burst prevented critical admission")
	}
	select {
	case <-client.Done():
		t.Fatal("audio overflow closed a healthy gameplay connection")
	default:
	}
	if string(<-client.audioCh) != "first audio" {
		t.Fatal("audio overflow replaced an already accepted batch")
	}
}

func TestAudioAdmissionRejectsDetachedClosedAndStoppingClients(t *testing.T) {
	t.Run("detached", func(t *testing.T) {
		client, _ := newAudioTestClient(t, 1, 1)
		client.InWorld.Store(false)
		if client.SendAudio([]byte("audio")) != AudioSendClosed || len(client.audioCh) != 0 {
			t.Fatal("detached client accepted audio")
		}
	})
	t.Run("closed", func(t *testing.T) {
		client, _ := newAudioTestClient(t, 1, 1)
		client.Close()
		if client.SendAudio([]byte("audio")) != AudioSendClosed || len(client.audioCh) != 0 {
			t.Fatal("closed client accepted audio")
		}
	})
	t.Run("server stopping", func(t *testing.T) {
		client, _ := newAudioTestClient(t, 1, 1)
		client.server.cancel()
		if client.SendAudio([]byte("audio")) != AudioSendClosed || len(client.audioCh) != 0 {
			t.Fatal("stopping server accepted audio")
		}
	})
}

func TestWriterPrioritizesGameplayWhenBothQueuesAreReady(t *testing.T) {
	client, peer := newAudioTestClient(t, 2, 1)
	client.SendAudio([]byte("audio"))
	client.SendCritical([]byte("first gameplay"))
	client.SendCritical([]byte("second gameplay"))
	client.server.wg.Add(1)
	finished := make(chan struct{})
	go func() {
		client.writeLoop()
		close(finished)
	}()
	for _, want := range []string{"first gameplay", "second gameplay", "audio"} {
		if got := readAudioTestFrame(t, peer); got != want {
			t.Fatalf("frame order: wanted %q, got %q", want, got)
		}
	}
	client.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("writer did not exit after close")
	}
}

func TestAudioWakeupRechecksGameplayBeforeWriting(t *testing.T) {
	client, peer := newAudioTestClient(t, 1, 1)
	client.SendAudio([]byte("audio"))
	selectedAudio := <-client.audioCh
	client.SendCritical([]byte("new gameplay"))
	finished := make(chan error, 1)
	go func() { finished <- client.writePendingMessages(nil, selectedAudio) }()
	for _, want := range []string{"new gameplay", "audio"} {
		if got := readAudioTestFrame(t, peer); got != want {
			t.Fatalf("audio wakeup bypassed pending gameplay: wanted %q, got %q", want, got)
		}
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestWriterTakesAtMostOneAudioBatchPerGameplayDrain(t *testing.T) {
	client, peer := newAudioTestClient(t, 1, 2)
	client.SendAudio([]byte("first audio"))
	client.SendAudio([]byte("second audio"))
	finished := make(chan error, 1)
	go func() { finished <- client.writePendingMessages([]byte("gameplay"), nil) }()
	for _, want := range []string{"gameplay", "first audio"} {
		if got := readAudioTestFrame(t, peer); got != want {
			t.Fatalf("unexpected frame: wanted %q, got %q", want, got)
		}
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if len(client.audioCh) != 1 {
		t.Fatal("writer drained more than one audio batch")
	}
}

func TestWriterShutdownWinsOverPendingAudio(t *testing.T) {
	client, _ := newAudioTestClient(t, 1, 1)
	client.SendAudio([]byte("audio"))
	client.server.cancel()
	if err := client.writePendingMessages(nil, []byte("selected audio")); err != context.Canceled {
		t.Fatalf("writer did not honor shutdown: %v", err)
	}
	if client.writeBuf.Buffered() != 0 {
		t.Fatal("shutdown buffered audio")
	}
}
