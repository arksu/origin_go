package game

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type attackResultFixture struct {
	shard     *Shard
	server    *network.Server
	url       string
	connected chan *network.Client
}

func newAttackResultFixture(tb testing.TB, queueCapacity int) *attackResultFixture {
	tb.Helper()
	logger := zap.NewNop()
	server := network.NewServer(&config.NetworkConfig{ReadTimeout: time.Hour, WriteTimeout: time.Second},
		&config.GameConfig{SendChannelBuffer: queueCapacity, Audio: config.DefaultAudioConfig()}, logger)
	f := &attackResultFixture{
		shard:  &Shard{layer: 3, world: ecs.NewWorldWithCapacity(2048, nil, 3), logger: logger, Clients: make(map[types.EntityID]*network.Client)},
		server: server, connected: make(chan *network.Client, 1),
	}
	server.SetOnConnect(func(client *network.Client) { f.connected <- client })
	mux := http.NewServeMux()
	require.NoError(tb, server.Start("127.0.0.1:0", mux))
	tb.Cleanup(server.Stop)
	httpServer := httptest.NewServer(mux)
	tb.Cleanup(httpServer.Close)
	f.url = "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	return f
}

func (f *attackResultFixture) connect(tb testing.TB, id types.EntityID, epoch uint32) (*network.Client, net.Conn) {
	tb.Helper()
	conn, _, _, err := ws.Dial(context.Background(), f.url)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	var client *network.Client
	select {
	case client = <-f.connected:
	case <-time.After(time.Second):
		tb.Fatal("missing attack-result client")
	}
	client.CharacterID, client.Layer = id, f.shard.layer
	client.StreamEpoch.Store(epoch)
	client.InWorld.Store(true)
	f.shard.Clients[id] = client
	return client, conn
}

func (f *attackResultFixture) send(actor types.Handle, actorID types.EntityID, eventID uint64, hits []netproto.AttackHit) {
	f.shard.mu.Lock()
	f.shard.SendAttackResult(actor, actorID, eventID, hits)
	f.shard.mu.Unlock()
}

// The Pong barrier shares the gameplay queue and proves zero/extra delivery
// without relying on a timeout to guess whether the writer has run.
func drainAttackResults(tb testing.TB, client *network.Client, conn net.Conn) []*netproto.S2C_AttackResult {
	tb.Helper()
	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(tb, err)
	require.True(tb, client.SendCritical(barrier))
	require.NoError(tb, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var results []*netproto.S2C_AttackResult
	for {
		payload, op, err := wsutil.ReadServerData(conn)
		require.NoError(tb, err)
		require.Equal(tb, ws.OpBinary, op)
		message := &netproto.ServerMessage{}
		require.NoError(tb, proto.Unmarshal(payload, message))
		if message.GetPong() != nil {
			return results
		}
		require.NotNil(tb, message.GetAttackResult())
		results = append(results, message.GetAttackResult())
	}
}

func TestAttackEventSequenceDoesNotWrap(t *testing.T) {
	sequence := &AttackEventSequence{}
	first, err := sequence.Next()
	require.NoError(t, err)
	require.EqualValues(t, 1, first)
	sequence.next.Store(math.MaxUint64 - 1)
	last, err := sequence.Next()
	require.NoError(t, err)
	require.EqualValues(t, uint64(math.MaxUint64), last)
	for i := 0; i < 3; i++ {
		value, err := sequence.Next()
		require.Zero(t, value)
		require.ErrorIs(t, err, ErrAttackEventSequenceExhausted)
	}
	require.EqualValues(t, uint64(math.MaxUint64), sequence.next.Load())
	require.Zero(t, testing.AllocsPerRun(100, func() { _, _ = sequence.Next() }))
	sequence.next.Store(0)
	require.Zero(t, testing.AllocsPerRun(100, func() { _, _ = sequence.Next() }))
}

func TestAttackEventSequenceConcurrentUniquenessAndExhaustion(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		t.Run(map[bool]string{false: "shared sequence", true: "saturation"}[exhausted], func(t *testing.T) {
			sequence := &AttackEventSequence{}
			if exhausted {
				sequence.next.Store(math.MaxUint64 - 2)
			}
			const workers, perWorker = 16, 32
			type nextResult struct {
				value uint64
				err   error
			}
			results := make(chan nextResult, workers*perWorker)
			var wait sync.WaitGroup
			for worker := 0; worker < workers; worker++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					for i := 0; i < perWorker; i++ {
						value, err := sequence.Next()
						results <- nextResult{value, err}
					}
				}()
			}
			wait.Wait()
			close(results)
			seen := make(map[uint64]bool)
			for result := range results {
				if result.err != nil {
					require.True(t, exhausted)
					require.ErrorIs(t, result.err, ErrAttackEventSequenceExhausted)
					require.Zero(t, result.value)
					continue
				}
				require.NotZero(t, result.value)
				require.False(t, seen[result.value], "event ID reused")
				seen[result.value] = true
			}
			if exhausted {
				require.Len(t, seen, 2)
				require.True(t, seen[math.MaxUint64-1])
				require.True(t, seen[math.MaxUint64])
			} else {
				require.Len(t, seen, workers*perWorker)
				for id := uint64(1); id <= workers*perWorker; id++ {
					require.True(t, seen[id])
				}
			}
		})
	}
}

func TestSendAttackResultFullPacketsAndRecipientEpochs(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	const actorID types.EntityID = 9007199254740993
	actor := f.shard.world.Spawn(actorID, nil)
	observerA := f.shard.world.Spawn(2, nil)
	observerB := f.shard.world.Spawn(3, nil)
	actorClient, actorConn := f.connect(t, actorID, 17)
	clientA, connA := f.connect(t, 2, 19)
	clientB, connB := f.connect(t, 3, 17)
	visibility := ecs.GetResource[ecs.VisibilityState](f.shard.world)
	visibility.ObserversByVisibleTarget[actor] = map[types.Handle]struct{}{actor: {}, observerA: {}, observerB: {}}
	for i, hits := range [][]netproto.AttackHit{
		nil,
		{{TargetId: 9007199254740995, Damage: 0}},
		{{TargetId: 9007199254740995, Damage: 3.6}, {TargetId: math.MaxUint64, Damage: 81.0 / 13}},
	} {
		eventID := uint64(math.MaxUint64) - uint64(2-i)
		f.send(actor, actorID, eventID, hits)
		for _, recipient := range []struct {
			client *network.Client
			conn   net.Conn
			epoch  uint32
		}{{actorClient, actorConn, 17}, {clientA, connA, 19}, {clientB, connB, 17}} {
			results := drainAttackResults(t, recipient.client, recipient.conn)
			require.Len(t, results, 1, "attacker present in reverse index must not be sent twice")
			result := results[0]
			require.Equal(t, eventID, result.EventId)
			require.EqualValues(t, actorID, result.AttackerId)
			require.Equal(t, recipient.epoch, result.StreamEpoch)
			require.Len(t, result.Hits, len(hits), "unknown targets must remain in every recipient's list")
			for hit := range hits {
				require.Equal(t, hits[hit].TargetId, result.Hits[hit].TargetId)
				require.Equal(t, hits[hit].Damage, result.Hits[hit].Damage)
			}
		}
	}
}

func TestSendAttackResultOwnsEncodedHitBytes(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	actor := f.shard.world.Spawn(1, nil)
	client, conn := f.connect(t, 1, 7)
	scratch := []netproto.AttackHit{{TargetId: 42, Damage: 0.49}}
	f.send(actor, 1, 101, scratch)
	scratch[0].TargetId, scratch[0].Damage = 43, 6.23
	f.send(actor, 1, 102, scratch)
	scratch[0].TargetId, scratch[0].Damage = 44, math.NaN()
	results := drainAttackResults(t, client, conn)
	require.Len(t, results, 2)
	require.EqualValues(t, 101, results[0].EventId)
	require.EqualValues(t, 42, results[0].Hits[0].TargetId)
	require.Equal(t, 0.49, results[0].Hits[0].Damage)
	require.EqualValues(t, 102, results[1].EventId)
	require.EqualValues(t, 43, results[1].Hits[0].TargetId)
	require.Equal(t, 6.23, results[1].Hits[0].Damage)
}

func TestSendAttackResultSkipsStaleHandlesAndClientSessions(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	w := f.shard.world
	actor := w.Spawn(1, nil)
	observer := w.Spawn(2, nil)
	actorClient, actorConn := f.connect(t, 1, 7)
	observerClient, observerConn := f.connect(t, 2, 8)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[actor] = map[types.Handle]struct{}{observer: {}}
	for _, invalidate := range []func(){
		func() { observerClient.InWorld.Store(false) },
		func() { observerClient.StreamEpoch.Store(0) },
		func() { observerClient.CharacterID = 99 },
		func() { observerClient.Layer = f.shard.layer + 1 },
	} {
		invalidate()
		f.send(actor, 1, 100, nil)
		require.Len(t, drainAttackResults(t, actorClient, actorConn), 1)
		require.Empty(t, drainAttackResults(t, observerClient, observerConn))
		observerClient.InWorld.Store(true)
		observerClient.StreamEpoch.Store(8)
		observerClient.CharacterID, observerClient.Layer = 2, f.shard.layer
	}
	require.True(t, w.Despawn(observer))
	replacement := w.Spawn(2, nil)
	visibility.ObserversByVisibleTarget[actor][replacement] = struct{}{}
	f.send(actor, 1, 101, nil)
	require.Len(t, drainAttackResults(t, actorClient, actorConn), 1)
	require.Len(t, drainAttackResults(t, observerClient, observerConn), 1, "old observer generation must not duplicate the replacement")
	f.send(actor, 1, 0, nil)
	f.send(actor, 2, 102, nil)
	require.True(t, w.Despawn(actor))
	w.Spawn(1, nil)
	f.send(actor, 1, 103, nil)
	require.Empty(t, drainAttackResults(t, actorClient, actorConn))
	require.Empty(t, drainAttackResults(t, observerClient, observerConn))
}

func TestSendAttackResultUsesCurrentClientOnly(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	actor := f.shard.world.Spawn(1, nil)
	oldClient, oldConn := f.connect(t, 1, 7)
	currentClient, currentConn := f.connect(t, 1, 9)
	f.send(actor, 1, 100, nil)
	require.Empty(t, drainAttackResults(t, oldClient, oldConn))
	results := drainAttackResults(t, currentClient, currentConn)
	require.Len(t, results, 1)
	require.EqualValues(t, 9, results[0].StreamEpoch)
}

func TestSendAttackResultCriticalOverflowDoesNotBlockHealthyRecipients(t *testing.T) {
	f := newAttackResultFixture(t, 1)
	actor := f.shard.world.Spawn(1, nil)
	observer := f.shard.world.Spawn(2, nil)
	actorClient, actorConn := f.connect(t, 1, 7)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	f.server.SetOnConnect(func(client *network.Client) { f.connected <- client; <-release })
	slow, _ := f.connect(t, 2, 7)
	visibility := ecs.GetResource[ecs.VisibilityState](f.shard.world)
	visibility.ObserversByVisibleTarget[actor] = map[types.Handle]struct{}{observer: {}}
	disconnected := make(chan struct{})
	f.server.SetOnDisconnect(func(client *network.Client) {
		if client == slow {
			f.shard.ClientsMu.Lock()
			delete(f.shard.Clients, 2)
			f.shard.ClientsMu.Unlock()
			close(disconnected)
		}
	})
	require.True(t, slow.SendCritical([]byte{1}))
	returned := make(chan struct{})
	go func() { f.send(actor, 1, 100, nil); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("attack delivery waited for disconnect callback")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("full critical queue did not disconnect its client")
	}
	require.False(t, slow.SendCritical([]byte{2}))
	require.NoError(t, actorConn.SetReadDeadline(time.Now().Add(3*time.Second)))
	payload, op, err := wsutil.ReadServerData(actorConn)
	require.NoError(t, err)
	require.Equal(t, ws.OpBinary, op)
	message := &netproto.ServerMessage{}
	require.NoError(t, proto.Unmarshal(payload, message))
	require.EqualValues(t, 100, message.GetAttackResult().GetEventId())
	require.Empty(t, drainAttackResults(t, actorClient, actorConn), "delivery failure must not enqueue a replay")
	releaseOnce.Do(func() { close(release) })
}
