package world

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newCapacityChunkManager(t *testing.T, capacity uint32) (*ChunkManager, *observer.ObservedLogs) {
	t.Helper()
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 0
	w := ecs.NewWorldWithCapacity(capacity, nil, 0)
	logCore, logs := observer.New(zap.DebugLevel)
	manager := NewChunkManager(cfg, nil, w, nil, 0, 1, NewObjectFactory(nil), nil, nil, zap.New(logCore))
	t.Cleanup(manager.Stop)
	return manager, logs
}

func installCapacityObjectDefinitions(t *testing.T) {
	t.Helper()
	previousObjects, previousItems := objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{HP: 100, DefID: 9401, Key: "capacity-tree", IsStatic: true},
		{HP: 100, DefID: 9402, Key: "capacity-container", IsStatic: true,
			Behaviors: map[string]json.RawMessage{"container": json.RawMessage(`{}`)}, BehaviorOrder: []string{"container"},
			Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{{Kind: "grid", W: 1, H: 1}}}},
	}))
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 9403, Key: "capacity-item", Size: itemdefs.Size{W: 1, H: 1}}}))
}

func TestChunkManagerCapacityDefersRawObjectsAndInventoriesUntilRetry(t *testing.T) {
	installCapacityObjectDefinitions(t)
	for _, full := range []bool{true, false} {
		t.Run(map[bool]string{true: "no_free_handle", false: "insufficient_inventory_handles"}[full], func(t *testing.T) {
			capacity, blockers := uint32(5), 4
			if !full {
				capacity, blockers = 4, 2
			}
			cm, logs := newCapacityChunkManager(t, capacity)
			blockerHandles := make([]types.Handle, blockers)
			for index := range blockerHandles {
				blockerHandles[index] = cm.world.SpawnWithoutExternalID()
			}
			coord := types.ChunkCoord{}
			chunk := core.NewChunk(coord, 1, 0, 128)
			chunk.SetState(types.ChunkStatePreloaded)
			first := &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 9501, TypeID: 9401, Region: 1, X: 10, Y: 10}
			box := &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 9502, TypeID: 9402, Region: 1, X: 20, Y: 10}
			pending := []*repository.Object{box}
			if full {
				pending = append(pending, &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 9503, TypeID: 9401, Region: 1, X: 30, Y: 10})
			}
			chunk.SetRawObjects(append([]*repository.Object{first}, pending...))
			inventoryData, err := json.Marshal(objectInventoryDataV1{
				Kind: uint8(constt.InventoryGrid), Width: 1, Height: 1, Version: 1,
				Items: []objectInventoryItem{{ItemID: 9601, TypeID: 9403, Quality: 10, Quantity: 1,
					NestedInventory: &objectInventoryDataV1{
						Kind: uint8(constt.InventoryGrid), Width: 1, Height: 1, Version: 1,
						Items: []objectInventoryItem{{ItemID: 9602, TypeID: 9403, Quality: 20, Quantity: 1}},
					}}},
			})
			require.NoError(t, err)
			rows := []repository.Inventory{{OwnerID: box.ID, Kind: int16(constt.InventoryGrid), Version: 1, Data: inventoryData}}
			chunk.SetRawInventoriesByOwner(map[types.EntityID][]repository.Inventory{9502: rows})
			dirtyIDs := map[types.EntityID]struct{}{9502: {}}
			chunk.SetRawDirtyObjectIDs(dirtyIDs)
			cm.chunks[coord] = chunk
			interest := newChunkInterest()
			interest.activeEntities[1] = struct{}{}
			cm.chunkInterests[coord] = interest

			require.NoError(t, cm.activateChunkInternal(coord, chunk))
			require.Equal(t, types.ChunkStateActive, chunk.GetState())
			firstHandle := cm.world.GetHandleByEntityID(9501)
			require.True(t, cm.world.Alive(firstHandle))
			require.Equal(t, blockers+1, cm.world.EntityCount(), "failed container must not leave partially allocated handles")
			require.Equal(t, types.InvalidHandle, cm.world.GetHandleByEntityID(9502))
			require.Equal(t, pending, chunk.GetRawObjects())
			require.Equal(t, rows, chunk.GetRawInventoriesByOwner()[9502])
			require.Equal(t, dirtyIDs, chunk.GetRawDirtyObjectIDs())
			require.Contains(t, cm.activationRetries, coord)
			require.Zero(t, logs.FilterMessage("failed to build object").Len())
			require.Equal(t, 1, logs.FilterMessage("Chunk activation deferred: insufficient ECS entity capacity").Len())

			// The untouched tail can retain an already-live transfer cache entry
			// until it is visited. Its ECS entity remains authoritative throughout.
			chunk.SetRawObjects(append(append([]*repository.Object(nil), pending...), first))
			chunk.SetRawDirtyObjectIDs(dirtyIDs)
			require.NoError(t, cm.activateChunkInternal(coord, chunk))
			require.Equal(t, append(append([]*repository.Object(nil), pending...), first), chunk.GetRawObjects())
			require.Equal(t, firstHandle, cm.world.GetHandleByEntityID(9501))
			require.Equal(t, rows, chunk.GetRawInventoriesByOwner()[9502])
			require.Equal(t, 1, logs.FilterMessage("Chunk activation deferred: insufficient ECS entity capacity").Len())

			for _, handle := range blockerHandles {
				require.True(t, cm.world.Despawn(handle))
			}
			retry := cm.activationRetries[coord]
			retry.due = time.Now().Add(-time.Second)
			cm.activationRetries[coord] = retry
			cm.Update(0)
			require.Empty(t, chunk.GetRawObjects())
			require.Empty(t, chunk.GetRawInventoriesByOwner())
			require.Equal(t, dirtyIDs, chunk.GetRawDirtyObjectIDs())
			require.NotContains(t, cm.activationRetries, coord)
			require.EqualValues(t, capacity, cm.world.EntityCount())
			require.Equal(t, firstHandle, cm.world.GetHandleByEntityID(9501))
			require.Len(t, chunk.GetHandles(), 1+len(pending))
			refIndex := ecs.GetResource[ecs.InventoryRefIndex](cm.world)
			root, exists := refIndex.Lookup(constt.InventoryGrid, 9502, 0)
			require.True(t, exists)
			rootContainer, exists := ecs.GetComponent[components.InventoryContainer](cm.world, root)
			require.True(t, exists)
			require.Len(t, rootContainer.Items, 1)
			require.Equal(t, types.EntityID(9601), rootContainer.Items[0].ItemID)
			nested, exists := refIndex.Lookup(constt.InventoryGrid, 9601, 0)
			require.True(t, exists)
			nestedContainer, exists := ecs.GetComponent[components.InventoryContainer](cm.world, nested)
			require.True(t, exists)
			require.Len(t, nestedContainer.Items, 1)
			require.Equal(t, types.EntityID(9602), nestedContainer.Items[0].ItemID)
			require.EqualValues(t, 20, nestedContainer.Items[0].Quality)
		})
	}
}

func TestChunkManagerCapacityWarningsAreBoundedAcrossChunks(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm, logs := newCapacityChunkManager(t, 1)
	cm.world.SpawnWithoutExternalID()
	var lastChunk *core.Chunk
	for index := 0; index < 20; index++ {
		coord := types.ChunkCoord{X: index}
		chunk := core.NewChunk(coord, 1, 0, 128)
		chunk.SetState(types.ChunkStatePreloaded)
		objects := make([]*repository.Object, 300)
		for objectIndex := range objects {
			objects[objectIndex] = &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: int64(index*300 + objectIndex + 1), TypeID: 9401}
		}
		chunk.SetRawObjects(objects)
		require.NoError(t, cm.activateChunkInternal(coord, chunk))
		require.Equal(t, objects, chunk.GetRawObjects())
		require.Empty(t, chunk.GetHandles())
		lastChunk = chunk
	}
	require.Zero(t, logs.FilterMessage("failed to build object").Len())
	warnings := logs.FilterMessage("Chunk activation deferred: insufficient ECS entity capacity")
	require.Equal(t, 1, warnings.Len(), "many full chunks must share one warning interval")
	fields := warnings.All()[0].ContextMap()
	require.EqualValues(t, 1, fields["active_entities"])
	require.EqualValues(t, 1, fields["entity_capacity"])
	require.EqualValues(t, 300, fields["deferred_objects"])
	cm.lastActivationCapacityWarning = time.Now().Add(-activationCapacityLogInterval)
	require.NoError(t, cm.activateChunkInternal(lastChunk.Coord, lastChunk))
	require.Equal(t, 2, logs.FilterMessage("Chunk activation deferred: insufficient ECS entity capacity").Len())
}

func TestChunkManagerCapacityHandlingKeepsUnrelatedBuildErrors(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm, logs := newCapacityChunkManager(t, 1)
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStatePreloaded)
	invalid := &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 9701, TypeID: 9499}
	deferred := &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 9703, TypeID: 9401}
	chunk.SetRawObjects([]*repository.Object{invalid, {Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 9702, TypeID: 9401}, deferred})
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, []*repository.Object{invalid, deferred}, chunk.GetRawObjects())
	require.Equal(t, 1, logs.FilterMessage("failed to build object").Len())
	require.EqualValues(t, 9701, logs.FilterMessage("failed to build object").All()[0].ContextMap()["object_id"])
	require.Equal(t, 1, logs.FilterMessage("Chunk activation deferred: insufficient ECS entity capacity").Len())
}
