package events

import (
	"context"
	"testing"
	"time"

	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestPeriodicMovementBatchesDeliverEveryEntry(t *testing.T) {
	for _, carries := range []bool{false, true} {
		t.Run(map[bool]string{false: "100_movers", true: "100_movers_and_100_carries"}[carries], func(t *testing.T) {
			f := newBatchFixture(t)
			w := f.shard.World()
			observer := w.Spawn(1, nil)
			client, conn := f.connect(t, 1)
			visibility := ecs.GetResource[ecs.VisibilityState](w)
			moved := ecs.GetResource[ecs.MovedEntities](w)
			ecs.GetResource[ecs.TimeState](w).UnixMs = 123456
			players := make([]types.Handle, 100)
			for index := 0; index < 100; index++ {
				playerID, objectID := types.EntityID(100+index), types.EntityID(1000+index)
				position := float64(100 + index*2)
				player := w.Spawn(playerID, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: position, Y: 100})
					ecs.AddComponent(w, h, components.CollisionResult{FinalX: position + 1, FinalY: 100})
					ecs.AddComponent(w, h, components.Movement{State: _const.StateMoving, Mode: _const.Walk, VelocityX: 10, MoveSeq: 3})
				})
				players[index] = player
				moved.Add(player, position, 100)
				visibility.ObserversByVisibleTarget[player] = map[types.Handle]struct{}{observer: {}}
				if carries {
					object := w.Spawn(objectID, func(w *ecs.World, h types.Handle) {
						ecs.AddComponent(w, h, components.Transform{X: position, Y: 100})
						ecs.AddComponent(w, h, components.EntityInfo{})
						ecs.AddComponent(w, h, components.ChunkRef{})
						ecs.AddComponent(w, h, components.ObjectInternalState{})
						ecs.AddComponent(w, h, components.LiftedObjectState{CarrierPlayerID: playerID, CarrierHandle: player})
					})
					ecs.AddComponent(w, player, components.LiftCarryState{ObjectEntityID: objectID, ObjectHandle: object})
					visibility.ObserversByVisibleTarget[object] = map[types.Handle]struct{}{observer: {}}
				}
			}
			barriers := make(chan struct{}, 1)
			f.bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(ctx context.Context, event eventbus.Event) error {
				if event.(*ecs.ObjectMoveBatchEvent).Layer == -1 {
					barriers <- struct{}{}
					return nil
				}
				return f.dispatcher.handleObjectMoveBatch(ctx, event)
			})
			flushEvents := func() {
				f.bus.PublishAsync(ecs.NewObjectMoveBatchEvent(-1, nil), eventbus.PriorityMedium)
				select {
				case <-barriers:
				case <-time.After(3 * time.Second):
					t.Fatal("movement dispatch barrier timed out")
				}
			}
			transform := systems.NewTransformUpdateSystem(w, f.shard.ChunkManager(), f.bus, zap.NewNop())
			lift := game.NewLiftService(w, f.shard.ChunkManager(), f.bus, nil, nil)
			follow := systems.NewLiftCarryFollowSystem(w, lift, f.bus, nil)
			transform.Update(w, 0.1)
			follow.Update(w, 0.1)
			flushEvents()
			messages := f.drain(t, client, conn)
			wantMessages, wantEntries := 1, 100
			if carries {
				wantMessages, wantEntries = 2, 200
			}
			require.Len(t, messages, wantMessages)
			seen := make(map[uint64]bool, wantEntries)
			payloadBytes := 0
			baselinePayloadBytes := 0
			for _, message := range messages {
				payloadBytes += proto.Size(message)
				batch := message.GetObjectMoveBatch()
				require.NotNil(t, batch)
				require.Len(t, batch.Moves, 100)
				for _, entry := range batch.Moves {
					baselinePayloadBytes += proto.Size(&netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMove{ObjectMove: entry}})
					require.False(t, seen[entry.EntityId], "duplicate entity %d", entry.EntityId)
					seen[entry.EntityId] = true
					isCarry := entry.EntityId >= 1000
					index := int(entry.EntityId) - 100
					if isCarry {
						index = int(entry.EntityId) - 1000
						require.True(t, carries)
						require.EqualValues(t, 100+index, entry.CarriedByEntityId)
						require.Zero(t, entry.MoveSeq)
					} else {
						require.EqualValues(t, 3, entry.MoveSeq)
					}
					require.GreaterOrEqual(t, index, 0)
					require.Less(t, index, 100)
					require.EqualValues(t, 101+index*2, entry.Movement.Position.X)
					require.EqualValues(t, 100, entry.Movement.Position.Y)
					require.EqualValues(t, 123456, entry.ServerTimeMs)
				}
			}
			require.Len(t, seen, wantEntries)
			t.Logf("per pass: %d -> %d messages, %d -> %d payload bytes, %d complete distinct entries; equivalent at 10 TPS: %d -> %d messages/s", wantEntries, len(messages), baselinePayloadBytes, payloadBytes, len(seen), wantEntries*10, len(messages)*10)
			follow.Update(w, 0.1)
			flushEvents()
			require.Empty(t, f.drain(t, client, conn), "stationary carries emitted a message")
			if carries {
				require.True(t, lift.ForceDropCarryAtPlayerPosition(w, 100, players[0], false))
				flushEvents()
				transitions := f.drain(t, client, conn)
				require.Len(t, transitions, 1)
				require.IsType(t, &netproto.ServerMessage_ObjectMove{}, transitions[0].Payload)
				require.EqualValues(t, 1000, transitions[0].GetObjectMove().EntityId)
				require.Zero(t, transitions[0].GetObjectMove().CarriedByEntityId)
			}
		})
	}
}

func TestObjectMoveEncoderReusesCompleteSubsetWithoutChangingPartialSubsets(t *testing.T) {
	entries := []ecs.MoveBatchEntry{{EntityID: 10, X: 100}, {EntityID: 11, X: 200}, {EntityID: 12, X: 300}}
	encoder := newObjectMoveEncoder(entries)
	full, err := encoder.marshalVisible([]int{0, 1, 2})
	require.NoError(t, err)
	partial, err := encoder.marshalVisible([]int{0, 2})
	require.NoError(t, err)
	single, err := encoder.marshalVisible([]int{1})
	require.NoError(t, err)
	repeatedFull, err := encoder.marshalVisible([]int{0, 1, 2})
	require.NoError(t, err)
	require.True(t, &full[0] == &repeatedFull[0], "complete subsets should share encoded bytes")
	for index, encoded := range [][]byte{full, partial, single} {
		message := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(encoded, message))
		moves := message.GetObjectMoveBatch().GetMoves()
		if move := message.GetObjectMove(); move != nil {
			moves = []*netproto.S2C_ObjectMove{move}
		}
		want := [][]uint64{{10, 11, 12}, {10, 12}, {11}}[index]
		require.Len(t, moves, len(want))
		for entryIndex, move := range moves {
			require.Equal(t, want[entryIndex], move.EntityId)
		}
	}
}
