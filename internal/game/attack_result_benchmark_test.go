package game

import (
	"fmt"
	"net"
	"testing"
	"time"

	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// This measures real encoding, critical enqueue, WebSocket writing and draining;
// allocations and time include transport, outside the gameplay's zero-alloc contract.
func BenchmarkAttackResultNetworkFanout(b *testing.B) {
	for _, fanout := range []int{1, 10, 100} {
		b.Run(fmt.Sprint(fanout), func(b *testing.B) {
			f := newAttackResultFixture(b, 32)
			actor := f.shard.world.Spawn(1, nil)
			visibility := ecs.GetResource[ecs.VisibilityState](f.shard.world)
			observers := make(map[types.Handle]struct{}, fanout)
			connections := make([]net.Conn, 0, fanout)
			for index := 0; index < fanout; index++ {
				id, handle := types.EntityID(index+1), actor
				if index > 0 {
					handle = f.shard.world.Spawn(id, nil)
				}
				_, conn := f.connect(b, id, 7)
				if err := conn.SetReadDeadline(time.Now().Add(time.Minute)); err != nil {
					b.Fatal(err)
				}
				connections = append(connections, conn)
				observers[handle] = struct{}{}
			}
			visibility.ObserversByVisibleTarget[actor] = observers
			hits := []netproto.AttackHit{{TargetId: 9007199254740993, Damage: 3.6}, {TargetId: 9007199254740995, Damage: 81.0 / 13}}
			sequence := &AttackEventSequence{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				eventID, err := sequence.Next()
				if err != nil {
					b.Fatal(err)
				}
				f.send(actor, 1, eventID, hits)
				for _, conn := range connections {
					_, op, err := wsutil.ReadServerData(conn)
					if err != nil || op != ws.OpBinary {
						b.Fatalf("attack-result drain: opcode=%v error=%v", op, err)
					}
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(fanout), "recipients/op")
		})
	}
}
