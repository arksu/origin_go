package game

import (
	"math"
	"net"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/combatpresentation"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func drainCombatTelegraphs(t *testing.T, client *network.Client, conn net.Conn) []*netproto.S2C_CombatTelegraph {
	t.Helper()
	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(t, err)
	require.True(t, client.SendCritical(barrier))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var result []*netproto.S2C_CombatTelegraph
	for {
		data, _, err := wsutil.ReadServerData(conn)
		require.NoError(t, err)
		var message netproto.ServerMessage
		require.NoError(t, proto.Unmarshal(data, &message))
		if message.GetPong() != nil {
			return result
		}
		require.NotNil(t, message.GetCombatTelegraph())
		result = append(result, message.GetCombatTelegraph())
	}
}

func TestCombatTelegraphUsesCurrentObserverEpochAndClears(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	w := f.shard.world
	combatpresentation.Attach(w, f.shard)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 1, TickPeriod: 100 * time.Millisecond, UnixMs: 100000}
	fox := w.Spawn(100, nil)
	ecs.AddComponent(w, fox, components.NPCBrain{})
	observer := w.Spawn(200, nil)
	unrelated := w.Spawn(201, nil)
	client, conn := f.connect(t, 200, 71)
	other, otherConn := f.connect(t, 201, 72)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[fox] = map[types.Handle]struct{}{observer: {}}
	f.shard.mu.Lock()
	require.True(t, combatpresentation.Start(w, fox, 1, 0, actiondefs.Sector{Range: 12, Angle: math.Pi / 3}, 4))
	f.shard.mu.Unlock()
	packets := drainCombatTelegraphs(t, client, conn)
	require.Len(t, packets, 1)
	require.EqualValues(t, 71, packets[0].StreamEpoch)
	require.EqualValues(t, 100, packets[0].EntityId)
	require.True(t, packets[0].State.Active)
	require.Empty(t, drainCombatTelegraphs(t, other, otherConn))
	client.StreamEpoch.Store(73)
	f.shard.mu.Lock()
	combatpresentation.Stop(w, fox, 1)
	f.shard.mu.Unlock()
	packets = drainCombatTelegraphs(t, client, conn)
	require.Len(t, packets, 1)
	require.EqualValues(t, 73, packets[0].StreamEpoch)
	require.False(t, packets[0].State.Active)
	require.EqualValues(t, 2, packets[0].State.Revision)
	w.Despawn(unrelated)
}
