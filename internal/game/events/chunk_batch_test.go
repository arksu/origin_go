package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestChunkActivationSpawnBatchHasFinalAppearanceWithoutUpserts(t *testing.T) {
	for _, laterUpdate := range []string{"normal", "forced"} {
		t.Run(laterUpdate, func(t *testing.T) {
			f := newBatchFixture(t)
			installChunkBatchDefinitions(t)
			w := f.shard.World()
			cm := f.shard.ChunkManager()
			f.dispatcher.Subscribe(f.bus)
			var appearanceEvents atomic.Int32
			f.bus.SubscribeAsync(ecs.TopicGameplayEntityAppearance, eventbus.PriorityMedium, func(_ context.Context, _ eventbus.Event) error {
				appearanceEvents.Add(1)
				return nil
			})
			observer := w.Spawn(1, nil)
			ecs.AddComponent(w, observer, components.Transform{X: constt.ChunkWorldSize - 6, Y: 200})
			ecs.AddComponent(w, observer, components.Vision{Radius: 100, Power: 100})
			ecs.AddComponent(w, observer, components.ChunkRef{})
			visibility := ecs.GetResource[ecs.VisibilityState](w)
			visibility.VisibleByObserver[observer] = ecs.ObserverVisibility{Known: make(map[types.Handle]types.EntityID)}
			client, conn := f.connect(t, 1)
			vision := ecssystems.NewVisionSystem(w, cm, f.bus, false, zap.NewNop())

			for phase, coord := range []types.ChunkCoord{{}, {X: 1}} {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				require.NoError(t, cm.WaitPreloaded(ctx, coord))
				cancel()
				chunk := cm.GetChunkFast(coord)
				require.NotNil(t, chunk)
				firstID := 1000 + phase*400
				positionX := constt.ChunkWorldSize - 36 + phase*50
				objects, inventories, expected := restoredChunkBatchObjects(firstID, coord, positionX, 300)
				chunk.SetRawObjects(objects)
				chunk.SetRawInventoriesByOwner(inventories)

				// Keep restoration notifications queued until Vision has installed
				// observers: dispatch-time-only suppression would duplicate every spawn.
				release := blockChunkBatchEvents(t, f.bus)
				anchorID := types.EntityID(9000 + phase)
				cm.RegisterEntity(anchorID, positionX, 200, false)
				require.Equal(t, types.ChunkStateActive, chunk.GetState())
				require.Empty(t, chunk.GetRawObjects())
				for id, resource := range expected {
					handle := w.GetHandleByEntityID(types.EntityID(id))
					require.True(t, w.Alive(handle))
					appearance, ok := ecs.GetComponent[components.Appearance](w, handle)
					require.True(t, ok)
					require.Equal(t, resource, appearance.Resource)
					state, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
					require.True(t, ok)
					require.False(t, state.IsDirty, "appearance recomputation must preserve restored persistence state")
				}
				ecs.GetResource[ecs.TimeState](w).Now = time.Unix(100+int64(phase)*10, 0)
				if phase > 0 {
					// Model moving into view of a newly loaded neighbor; movement
					// invalidates the existing stationary-observer visibility cache.
					ecs.WithComponent(w, observer, func(transform *components.Transform) { transform.X -= 10 })
				}
				if phase > 0 && laterUpdate == "forced" {
					vision.ForceUpdateForObserver(w, observer)
				} else {
					vision.Update(w, 0)
				}
				release()
				flushChunkBatchEvents(t, f.bus)
				messages := f.drain(t, client, conn)
				assertChunkSpawnBatch(t, messages, expected, client.StreamEpoch.Load())
				require.Zero(t, appearanceEvents.Load(), "restoration must not enqueue delayed appearance upserts")
				t.Logf("chunk phase %d: %d restored objects -> %d WebSocket message, all final appearances present", phase, len(expected), len(messages))
			}

			vision.ForceUpdateForObserver(w, observer)
			flushChunkBatchEvents(t, f.bus)
			require.Empty(t, f.drain(t, client, conn), "unchanged visibility must not emit an empty batch or delayed restoration upserts")
		})
	}
}

func installChunkBatchDefinitions(t *testing.T) {
	t.Helper()
	previousObjects, previousItems := objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{HP: 100,
			DefID: 700, Key: "batch_tree", IsStatic: true, Resource: "tree/young", BehaviorOrder: []string{"tree"},
			Behaviors:  map[string]json.RawMessage{"tree": json.RawMessage(`{}`)},
			Appearance: []objectdefs.Appearance{{ID: "mature", When: &objectdefs.AppearanceWhen{Flags: []string{"tree.stage2"}}, Resource: "tree/mature"}},
			TreeConfig: &objectdefs.TreeBehaviorConfig{Stages: []objectdefs.TreeStageConfig{{StageDuration: 10}, {}}},
		},
		{HP: 100,
			DefID: 701, Key: "batch_box", IsStatic: true, Resource: "box/empty", BehaviorOrder: []string{"container"},
			Behaviors:  map[string]json.RawMessage{"container": json.RawMessage(`{}`)},
			Appearance: []objectdefs.Appearance{{ID: "filled", When: &objectdefs.AppearanceWhen{Flags: []string{"container.has_items"}}, Resource: "box/filled"}},
			Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{{Kind: "grid", W: 1, H: 1}}},
		},
	}))
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 900, Key: "batch_stone", Resource: "stone", Size: itemdefs.Size{W: 1, H: 1}}}))
}

func restoredChunkBatchObjects(firstID int, coord types.ChunkCoord, positionX, count int) ([]*repository.Object, map[types.EntityID][]repository.Inventory, map[uint64]string) {
	objects := make([]*repository.Object, 0, count)
	inventories := make(map[types.EntityID][]repository.Inventory)
	expected := make(map[uint64]string, count)
	for index := range count {
		id := int64(firstID + index)
		object := &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: id, TypeID: 700, X: positionX + index%20, Y: 180 + index/20, ChunkX: coord.X, ChunkY: coord.Y, Quality: 10}
		if index%2 == 0 {
			object.Data = pqtype.NullRawMessage{Valid: true, RawMessage: json.RawMessage(`{"v":1,"behaviors":{"tree":{"stage":2}}}`)}
			expected[uint64(id)] = "tree/mature"
		} else {
			object.TypeID = 701
			inventories[types.EntityID(id)] = []repository.Inventory{{
				OwnerID: id, Kind: int16(constt.InventoryGrid), Version: 1,
				Data: json.RawMessage(`{"width":1,"height":1,"items":[{"item_id":10000,"type_id":900,"quality":10,"quantity":1}]}`),
			}}
			expected[uint64(id)] = "box/filled"
		}
		objects = append(objects, object)
	}
	return objects, inventories, expected
}

func assertChunkSpawnBatch(t *testing.T, messages []*netproto.ServerMessage, expected map[uint64]string, epoch uint32) {
	t.Helper()
	require.Len(t, messages, 1)
	batch := messages[0].GetObjectSpawnBatch()
	require.NotNil(t, batch)
	require.Len(t, batch.Spawns, len(expected))
	seen := make(map[uint64]bool, len(expected))
	for _, entry := range batch.Spawns {
		require.False(t, seen[entry.EntityId], "duplicate spawn for %d", entry.EntityId)
		seen[entry.EntityId] = true
		require.Equal(t, expected[entry.EntityId], entry.ResourcePath)
		require.Equal(t, epoch, entry.StreamEpoch)
	}
}

func blockChunkBatchEvents(t *testing.T, bus *eventbus.EventBus) func() {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var token eventbus.SubscriptionToken
	token = bus.SubscribeAsync("test.chunk.block", eventbus.PriorityMedium, func(ctx context.Context, _ eventbus.Event) error {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		bus.Unsubscribe(token)
		return nil
	})
	bus.PublishAsync(eventbus.NewEvent("test.chunk.block", nil), eventbus.PriorityMedium)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("eventbus worker did not reach blocker")
	}
	return unblock
}

func flushChunkBatchEvents(t *testing.T, bus *eventbus.EventBus) {
	t.Helper()
	flushed := make(chan struct{})
	token := bus.SubscribeAsync("test.chunk.flush", eventbus.PriorityMedium, func(_ context.Context, _ eventbus.Event) error {
		close(flushed)
		return nil
	})
	defer bus.Unsubscribe(token)
	bus.PublishAsync(eventbus.NewEvent("test.chunk.flush", nil), eventbus.PriorityMedium)
	select {
	case <-flushed:
	case <-time.After(3 * time.Second):
		t.Fatal("chunk event queue did not drain")
	}
}
