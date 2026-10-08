package world

import (
	"errors"
	"fmt"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newActivationBacklogFixture(t testing.TB, count int, expired bool) (*ChunkManager, *core.Chunk) {
	t.Helper()
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 0
	w := ecs.NewWorldWithCapacity(1, nil, 0)
	w.SpawnWithoutExternalID()
	runtime := int64(101)
	if expired {
		runtime = 110
	}
	ecs.SetResource(w, ecs.TimeState{Now: time.Unix(1000, 0), RuntimeSecondsTotal: runtime})
	cm := NewChunkManager(cfg, nil, w, nil, 0, 1, NewObjectFactory(nil), nil, nil, zap.NewNop())
	t.Cleanup(cm.Stop)
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStatePreloaded)
	objects := make([]*repository.Object, count)
	rows := make(map[types.EntityID][]repository.Inventory, count)
	for i := range objects {
		id := int64(i + 1000)
		objects[i] = &repository.Object{ID: id, TypeID: constt.DroppedItemTypeID, Region: 1,
			Data: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(fmt.Sprintf(`{"has_inventory":true,"contained_item_id":%d,"drop_time":100,"dropper_id":0,"time_basis":"%s"}`, id, constt.DroppedItemTimeBasisRuntimeSecondsV1))}}
		rows[types.EntityID(id)] = []repository.Inventory{{OwnerID: id}}
	}
	chunk.SetRawObjects(objects)
	chunk.SetRawInventoriesByOwner(rows)
	cm.chunks[chunk.Coord] = chunk
	return cm, chunk
}

func TestActivationBacklogAllocationsDoNotScaleWithUnvisitedTail(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint("expired=", expired), func(t *testing.T) {
			var baseline float64
			// Whole budget-sized spans keep allocation comparisons equivalent now
			// that retries advance through the backlog instead of revisiting its head.
			for _, count := range []int{128, 10240} {
				cm, chunk := newActivationBacklogFixture(t, count, expired)
				var err error
				allocs := testing.AllocsPerRun(100, func() {
					cm.remainingDroppedActivations = committedDroppedActivationBudget
					err = cm.activateChunkInternal(chunk.Coord, chunk)
				})
				require.NoError(t, err)
				if count == 128 {
					baseline = allocs
				} else {
					require.Equal(t, baseline, allocs)
				}
				require.Len(t, chunk.GetRawObjects(), count)
				require.Len(t, chunk.GetRawInventoriesByOwner(), count)
				require.Equal(t, 1, cm.world.EntityCount())
				if expired {
					require.Zero(t, cm.remainingDroppedActivations)
					require.Equal(t, min(count, chunkDropExpiryCapacity), cm.PendingExpiredDrops())
				} else {
					require.Equal(t, committedDroppedActivationBudget-1, cm.remainingDroppedActivations)
				}
			}
		})
	}
}

func liveBacklogDrop(cm *ChunkManager, id int64) (*repository.Object, []repository.Inventory) {
	raw := &repository.Object{ID: id, TypeID: constt.DroppedItemTypeID, Region: 1,
		Data: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(fmt.Sprintf(`{"has_inventory":true,"contained_item_id":%d,"drop_time":%d,"dropper_id":0,"time_basis":"%s"}`,
			id, ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal, constt.DroppedItemTimeBasisRuntimeSecondsV1))}}
	rows := []repository.Inventory{{OwnerID: id, Kind: int16(constt.InventoryDroppedItem), Version: 1,
		Data: []byte(fmt.Sprintf(`{"kind":3,"key":0,"width":1,"height":1,"version":1,"items":[{"item_id":%d,"type_id":9403,"quality":10,"quantity":1}]}`, id))}}
	return raw, rows
}

func installCapacityBacklogItemDefinitions(t *testing.T) {
	t.Helper()
	installCapacityObjectDefinitions(t)
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 9403, Key: "capacity-item", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 9404, Key: "capacity-bag", Size: itemdefs.Size{W: 1, H: 1},
			Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 1, H: 1}}},
	}))
}

func liveBacklogBagDrop(cm *ChunkManager, id int64) (*repository.Object, []repository.Inventory) {
	raw, rows := liveBacklogDrop(cm, id)
	rows[0].Data = []byte(fmt.Sprintf(`{"kind":3,"key":0,"width":1,"height":1,"version":1,"items":[{"item_id":%d,"type_id":9404,"quality":10,"quantity":1,"nested_inventory":{"kind":0,"key":0,"width":1,"height":1,"version":1,"items":[{"item_id":%d,"type_id":9403,"quality":20,"quantity":1}]}}]}`, id, id+100000))
	return raw, rows
}

func TestActivationSkipsOversizedDropAndRetriesItsWholeInventoryTree(t *testing.T) {
	installCapacityBacklogItemDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 5)
	ecs.SetResource(cm.world, ecs.TimeState{Now: time.Unix(1000, 0), RuntimeSecondsTotal: 101})
	blockers := make([]types.Handle, 3)
	for i := range blockers {
		blockers[i] = cm.world.SpawnWithoutExternalID()
	}
	bag, bagRows := liveBacklogBagDrop(cm, 4000)
	simple, simpleRows := liveBacklogDrop(cm, 4001)
	chunk := expiryChunk(cm, bag, simple)
	chunk.SetRawInventoriesByOwner(map[types.EntityID][]repository.Inventory{4000: bagRows, 4001: simpleRows})

	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	simpleHandle := cm.world.GetHandleByEntityID(4001)
	require.True(t, cm.world.Alive(simpleHandle), "a three-handle bag must not block a two-handle drop")
	require.Equal(t, []*repository.Object{bag}, chunk.GetRawObjects())
	require.Equal(t, bagRows, chunk.GetRawInventoriesByOwner()[4000])
	require.NotContains(t, chunk.GetRawInventoriesByOwner(), types.EntityID(4001))
	require.Equal(t, committedDroppedActivationBudget-2, cm.remainingDroppedActivations)
	require.Equal(t, 5, cm.world.EntityCount(), "the deferred bag must allocate no partial entities")
	require.Len(t, chunk.GetHandles(), 1)
	index := ecs.GetResource[ecs.InventoryRefIndex](cm.world)
	for _, kind := range []constt.InventoryKind{constt.InventoryDroppedItem, constt.InventoryGrid} {
		_, exists := index.Lookup(kind, 4000, 0)
		require.False(t, exists, "the deferred bag must allocate no inventory refs")
	}
	require.Equal(t, types.InvalidHandle, cm.world.GetHandleByEntityID(4000))

	for _, blocker := range blockers {
		require.True(t, cm.world.Despawn(blocker))
	}
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	bagHandle := cm.world.GetHandleByEntityID(4000)
	require.True(t, cm.world.Alive(bagHandle))
	require.Equal(t, simpleHandle, cm.world.GetHandleByEntityID(4001))
	require.Equal(t, 5, cm.world.EntityCount())
	require.Len(t, chunk.GetHandles(), 2)
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.GetRawInventoriesByOwner())
	root, exists := index.Lookup(constt.InventoryDroppedItem, 4000, 0)
	require.True(t, exists)
	rootContainer, exists := ecs.GetComponent[components.InventoryContainer](cm.world, root)
	require.True(t, exists)
	require.Len(t, rootContainer.Items, 1)
	require.EqualValues(t, 9404, rootContainer.Items[0].TypeID)
	nested, exists := index.Lookup(constt.InventoryGrid, 4000, 0)
	require.True(t, exists)
	nestedContainer, exists := ecs.GetComponent[components.InventoryContainer](cm.world, nested)
	require.True(t, exists)
	require.Len(t, nestedContainer.Items, 1)
	require.Equal(t, types.EntityID(104000), nestedContainer.Items[0].ItemID)
	require.EqualValues(t, 20, nestedContainer.Items[0].Quality)
	// An additional activation cannot recreate either object or inventory tree.
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, bagHandle, cm.world.GetHandleByEntityID(4000))
	require.Equal(t, simpleHandle, cm.world.GetHandleByEntityID(4001))
	require.Equal(t, 5, cm.world.EntityCount())
	require.Len(t, chunk.GetHandles(), 2)
}

func TestActivationOversizedDropsRespectBudgetAndAdvancePastPrefix(t *testing.T) {
	installCapacityBacklogItemDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 5)
	ecs.SetResource(cm.world, ecs.TimeState{Now: time.Unix(1000, 0), RuntimeSecondsTotal: 101})
	for i := 0; i < 3; i++ {
		cm.world.SpawnWithoutExternalID()
	}
	const bagCount = committedDroppedActivationBudget*2 + 1
	objects := make([]*repository.Object, 0, bagCount+1)
	rows := make(map[types.EntityID][]repository.Inventory, bagCount+1)
	for i := 0; i < bagCount; i++ {
		raw, rootRows := liveBacklogBagDrop(cm, int64(5000+i))
		objects = append(objects, raw)
		rows[types.EntityID(raw.ID)] = rootRows
	}
	simple, simpleRows := liveBacklogDrop(cm, 6000)
	objects = append(objects, simple)
	rows[6000] = simpleRows
	chunk := expiryChunk(cm, objects...)
	chunk.SetRawInventoriesByOwner(rows)
	for pass := 0; pass < 2; pass++ {
		cm.remainingDroppedActivations = committedDroppedActivationBudget
		require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
		require.Zero(t, cm.remainingDroppedActivations, "pass %d must visit exactly the bounded budget", pass)
		require.Equal(t, 3, cm.world.EntityCount())
		require.Empty(t, chunk.GetHandles())
		require.Equal(t, objects, chunk.GetRawObjects())
		require.Len(t, chunk.GetRawInventoriesByOwner(), bagCount+1)
	}
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, committedDroppedActivationBudget-2, cm.remainingDroppedActivations)
	require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(6000)), "untouched suffix must be visited before retained oversized bags")
	require.Equal(t, objects[:bagCount], chunk.GetRawObjects())
	require.Len(t, chunk.GetRawInventoriesByOwner(), bagCount)
	require.Equal(t, 5, cm.world.EntityCount())
	index := ecs.GetResource[ecs.InventoryRefIndex](cm.world)
	for i := 0; i < bagCount; i++ {
		id := types.EntityID(5000 + i)
		require.Equal(t, types.InvalidHandle, cm.world.GetHandleByEntityID(id))
		for _, kind := range []constt.InventoryKind{constt.InventoryDroppedItem, constt.InventoryGrid} {
			_, exists := index.Lookup(kind, id, 0)
			require.False(t, exists)
		}
	}
}

func TestActivationAdvancesPastExpiredDeletesThatKeepFailing(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm := newDropExpiryTestManager(t)
	deleter := &dropExpiryTestDeleter{err: errors.New("database unavailable")}
	cm.objectFactory.SetObjectDeleter(deleter)
	const expiredCount, liveCount = committedDroppedActivationBudget * 2, committedDroppedActivationBudget + 3
	objects := make([]*repository.Object, 0, expiredCount+liveCount)
	rows := make(map[types.EntityID][]repository.Inventory, expiredCount+liveCount)
	for i := 0; i < expiredCount; i++ {
		id := types.EntityID(1000 + i)
		objects = append(objects, expiredDropRaw(t, id))
		rows[id] = []repository.Inventory{{OwnerID: int64(id), Kind: int16(constt.InventoryDroppedItem)}}
	}
	for i := 0; i < liveCount; i++ {
		raw, rootRows := liveBacklogDrop(cm, int64(2000+i))
		objects = append(objects, raw)
		rows[types.EntityID(raw.ID)] = rootRows
	}
	chunk := expiryChunk(cm, objects...)
	chunk.SetRawInventoriesByOwner(rows)
	for pass := 0; pass < 2; pass++ {
		cm.remainingDroppedActivations = committedDroppedActivationBudget
		require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
		require.Zero(t, cm.remainingDroppedActivations)
		for len(cm.saveQueue) > 0 {
			runExpiryTestJob(t, cm)
		}
		cm.updateExpiredDrops()
	}
	require.Equal(t, expiredCount, deleter.calls)
	require.Equal(t, expiredCount, cm.PendingExpiredDrops())
	require.Empty(t, chunk.GetHandles())
	for pass, wantLive := range []int{committedDroppedActivationBudget, liveCount} {
		cm.remainingDroppedActivations = committedDroppedActivationBudget
		require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
		require.Len(t, chunk.GetHandles(), wantLive, "pass %d must progress beyond retained failures", pass)
		require.Equal(t, expiredCount, deleter.calls, "activation performs no deletion I/O")
	}
	for i := 0; i < liveCount; i++ {
		require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(types.EntityID(2000+i))))
		require.NotContains(t, chunk.GetRawInventoriesByOwner(), types.EntityID(2000+i))
	}
	// Repairing persistence eventually deletes every retained record exactly once.
	deleter.err = nil
	state := ecs.GetResource[ecs.TimeState](cm.world)
	state.Now = state.Now.Add(time.Second)
	cm.updateExpiredDrops()
	for len(cm.saveQueue) > 0 {
		runExpiryTestJob(t, cm)
	}
	for cm.PendingExpiredDrops() > 0 {
		cm.updateExpiredDrops()
	}
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.GetRawInventoriesByOwner())
	require.Zero(t, cm.persistenceFor(chunk.Coord).pins.Load())
	require.Equal(t, expiredCount*2, deleter.calls)
}

func TestActivationAdvancesPastMalformedDropPrefixAndRetriesAfterRepair(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm := newDropExpiryTestManager(t)
	objects := make([]*repository.Object, 0, committedDroppedActivationBudget+1)
	rows := make(map[types.EntityID][]repository.Inventory, committedDroppedActivationBudget+1)
	validPayloads := make([][]byte, committedDroppedActivationBudget)
	for i := 0; i <= committedDroppedActivationBudget; i++ {
		raw, rootRows := liveBacklogDrop(cm, int64(3000+i))
		if i < committedDroppedActivationBudget {
			validPayloads[i] = raw.Data.RawMessage
			raw.Data.RawMessage = []byte(`{`)
		}
		objects = append(objects, raw)
		rows[types.EntityID(raw.ID)] = rootRows
	}
	chunk := expiryChunk(cm, objects...)
	chunk.SetRawInventoriesByOwner(rows)
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Empty(t, chunk.GetHandles())
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(types.EntityID(3000+committedDroppedActivationBudget))))
	require.Len(t, chunk.GetRawObjects(), committedDroppedActivationBudget)
	for i, payload := range validPayloads {
		objects[i].Data.RawMessage = payload
	}
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Len(t, chunk.GetHandles(), committedDroppedActivationBudget+1)
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.GetRawInventoriesByOwner())
}

func TestActivationConsumesRemovalHolesWithinDropBudget(t *testing.T) {
	cm, chunk := newActivationBacklogFixture(t, 10000, false)
	// Create interior holes; the first record remains a real row. Removing it
	// subsequently discards only that edge slot without scanning the suffix.
	for id := types.EntityID(1001); id < 1101; id++ {
		chunk.RemoveCommittedObject(id)
	}
	chunk.RemoveCommittedObject(1000)
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Zero(t, cm.remainingDroppedActivations)
	require.Len(t, chunk.GetRawObjects(), 10000-1-committedDroppedActivationBudget)
	require.Nil(t, chunk.GetRawObjects()[0], "holes beyond the visit budget remain untouched")
}

func BenchmarkActivationRawBacklog(b *testing.B) {
	for _, expired := range []bool{false, true} {
		for _, count := range []int{100, 10000} {
			b.Run(fmt.Sprintf("expired=%v/rows=%d", expired, count), func(b *testing.B) {
				cm, chunk := newActivationBacklogFixture(b, count, expired)
				require.NoError(b, cm.activateChunkInternal(chunk.Coord, chunk))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cm.remainingDroppedActivations = committedDroppedActivationBudget
					if err := cm.activateChunkInternal(chunk.Coord, chunk); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
