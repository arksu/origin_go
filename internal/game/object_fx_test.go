package game

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

func drainObjectFx(t *testing.T, client *network.Client, conn net.Conn) []*netproto.S2C_Fx {
	t.Helper()
	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(t, err)
	require.True(t, client.SendCritical(barrier))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var effects []*netproto.S2C_Fx
	for {
		encoded, _, err := wsutil.ReadServerData(conn)
		require.NoError(t, err)
		message := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(encoded, message))
		if message.GetPong() != nil {
			return effects
		}
		require.NotNil(t, message.GetFx())
		effects = append(effects, message.GetFx())
	}
}

func TestObjectFxUsesObjectPositionAndOnlyCurrentObservers(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	w := f.shard.world
	source := w.Spawn(100, nil)
	ecs.AddComponent(w, source, components.Transform{X: 321, Y: -45})
	first, second := w.Spawn(1, nil), w.Spawn(2, nil)
	oldClient, oldConn := f.connect(t, 1, 4)
	clientA, connA := f.connect(t, 1, 7)
	clientB, connB := f.connect(t, 2, 9)
	outside, outsideConn := f.connect(t, 3, 7)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[source] = map[types.Handle]struct{}{first: {}, second: {}}
	send := func() {
		f.shard.mu.Lock()
		defer f.shard.mu.Unlock()
		f.shard.SendObjectFx(source, "skull_smoke")
	}
	send()
	for _, recipient := range []struct {
		client *network.Client
		conn   net.Conn
		epoch  uint32
	}{{clientA, connA, 7}, {clientB, connB, 9}} {
		effects := drainObjectFx(t, recipient.client, recipient.conn)
		require.Len(t, effects, 1)
		require.Equal(t, "skull_smoke", effects[0].FxKey)
		require.Equal(t, &netproto.Vector2{X: 321, Y: -45}, effects[0].Position)
		require.Equal(t, recipient.epoch, effects[0].StreamEpoch)
	}
	require.Empty(t, drainObjectFx(t, oldClient, oldConn))
	require.Empty(t, drainObjectFx(t, outside, outsideConn))
	for _, invalidate := range []func(){
		func() { clientB.InWorld.Store(false) },
		func() { clientB.StreamEpoch.Store(0) },
		func() { clientB.Layer++ },
		func() { clientB.CharacterID = 99 },
	} {
		invalidate()
		send()
		require.Len(t, drainObjectFx(t, clientA, connA), 1)
		require.Empty(t, drainObjectFx(t, clientB, connB))
		clientB.InWorld.Store(true)
		clientB.StreamEpoch.Store(9)
		clientB.Layer, clientB.CharacterID = f.shard.layer, 2
	}
	require.True(t, w.Despawn(second))
	w.Spawn(2, nil)
	send()
	require.Len(t, drainObjectFx(t, clientA, connA), 1)
	require.Empty(t, drainObjectFx(t, clientB, connB), "a stale observer handle cannot address its replacement")
	require.True(t, w.Despawn(source))
	w.Spawn(100, nil)
	send()
	require.Empty(t, drainObjectFx(t, clientA, connA), "a stale source cannot emit")
}

type objectFxTerminalSaver struct {
	*hearthActivationSaver
	onSave func()
	err    error
}

func (s objectFxTerminalSaver) SaveTerminal(context.Context, types.EntityID, hearth.ObjectKey, *repository.Object) error {
	if s.onSave != nil {
		s.onSave()
	}
	return s.err
}

func TestHearthDeathFxOnlyAfterSavedLiveConversion(t *testing.T) {
	for _, scenario := range []string{"surface", "underground", "deletion", "save_failure", "raw", "no_home"} {
		t.Run(scenario, func(t *testing.T) {
			f := newHearthActivationFixture(t)
			home := &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 1000}, X: 200, Y: 200}
			if scenario == "no_home" {
				home = nil
			} else {
				f.chunk.UpsertRawObject(f.rawHearth())
			}
			require.NoError(t, f.runtime.Index.LoadIfUnknown(1, home))
			if scenario != "raw" && scenario != "no_home" {
				f.activate(t)
				f.firstVisible(t)
			}
			networkFixture := newAttackResultFixture(t, 32)
			f.shard.Clients = make(map[types.EntityID]*network.Client)
			networkFixture.shard = f.shard
			client, conn := networkFixture.connect(t, 9000, 7)
			saver := objectFxTerminalSaver{hearthActivationSaver: f.saver, onSave: func() {
				require.Empty(t, drainObjectFx(t, client, conn), "no event before the save returns")
			}}
			if scenario == "save_failure" {
				saver.err = errors.New("commit failed")
			}
			index := f.runtime.Index
			*f.runtime = hearth.NewRuntime(f.shard.world, f.shard.chunkManager, f.reader,
				f.shard.chunkManager.ObjectFactory(), saver, behaviors.MustDefaultRegistry(), f.shard.eventBus,
				zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
			f.runtime.Index = index
			require.NoError(t, f.runtime.NotifyTerminal(1))
			switch scenario {
			case "deletion":
				f.shard.mu.Lock()
				require.NoError(t, f.runtime.CompleteDeletion(1))
				f.shard.mu.Unlock()
			case "save_failure":
				require.Panics(t, func() { _ = f.shard.completeHearthDeath(1) })
			case "underground":
				source := &Shard{layer: 1, permanentDeathIDs: []types.EntityID{1}, permanentDeathComplete: f.shard.completeHearthDeath}
				source.mu.Lock()
				source.unlockAndCompletePermanentDeaths()
				require.True(t, source.mu.TryLock())
				source.mu.Unlock()
			default:
				require.NoError(t, f.shard.completeHearthDeath(1))
			}
			effects := drainObjectFx(t, client, conn)
			if scenario == "surface" || scenario == "underground" {
				require.Len(t, effects, 1)
				require.Equal(t, "skull_smoke", effects[0].FxKey)
				require.EqualValues(t, 200, effects[0].Position.X)
				require.EqualValues(t, 200, effects[0].Position.Y)
			} else {
				require.Empty(t, effects)
			}
			require.NoError(t, f.shard.completeHearthDeath(1))
			require.Empty(t, drainObjectFx(t, client, conn), "terminal completion never replays an event")
		})
	}
}
