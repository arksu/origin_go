package inventory

import (
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func skullMetadataFixture() *components.SkullMetadata {
	return &components.SkullMetadata{CharacterID: 42, Nickname: "Alice <b>literal</b>", DeathDate: "2026-01-10"}
}

func TestSkullMetadataSurvivesPlayerNestedSnapshotAndRestore(t *testing.T) {
	w, registry, _, roots := objectLootCaptureFixture()
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	deepest, _ := refs.Lookup(constt.InventoryGrid, 103, 0)
	metadata := skullMetadataFixture()
	ecs.WithComponent(w, deepest, func(c *components.InventoryContainer) { c.Items[0].Skull = metadata })
	root, _ := ecs.GetComponent[components.InventoryContainer](w, roots[0].Handle)
	snapshot, err := SerializeInventoryTree(w, root)
	require.NoError(t, err)
	savedSkull := snapshot.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].Skull
	require.Equal(t, metadata, savedSkull)
	require.NotSame(t, metadata, savedSkull)

	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var persisted InventoryDataV1
	require.NoError(t, json.Unmarshal(encoded, &persisted))
	restored := ecs.NewWorldForTesting()
	loaded, err := NewInventoryLoader(zap.NewNop()).LoadPlayerInventories(restored, 100, []InventoryDataV1{persisted})
	require.NoError(t, err)
	for _, handle := range loaded.ContainerHandles {
		container, ok := ecs.GetComponent[components.InventoryContainer](restored, handle)
		require.True(t, ok)
		if container.OwnerID == 103 {
			require.Equal(t, metadata, container.Items[0].Skull)
			require.NotSame(t, persisted.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].Skull, container.Items[0].Skull)
		}
	}
	metadata.Nickname = "Changed in live world"
	require.Equal(t, "Alice <b>literal</b>", savedSkull.Nickname)
}

func TestSkullMetadataObjectLootCaptureOwnsRootAndNestedMetadata(t *testing.T) {
	w, registry, _, roots := objectLootCaptureFixture()
	rootMetadata, nestedMetadata := skullMetadataFixture(), skullMetadataFixture()
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	root, _ := refs.Lookup(constt.InventoryGrid, 100, 5)
	deepest, _ := refs.Lookup(constt.InventoryGrid, 103, 0)
	ecs.WithComponent(w, root, func(c *components.InventoryContainer) { c.Items[0].Skull = rootMetadata })
	ecs.WithComponent(w, deepest, func(c *components.InventoryContainer) { c.Items[0].Skull = nestedMetadata })
	capture := NewObjectLootCapture(100, roots)
	finishObjectLootCapture(t, capture, w, registry, 1)
	items := capture.Items()
	require.Equal(t, rootMetadata, items[1].Skull)
	require.NotSame(t, rootMetadata, items[1].Skull)
	nestedSkull := items[0].NestedInventory.Items[0].NestedInventory.Items[0].Skull
	require.Equal(t, nestedMetadata, nestedSkull)
	require.NotSame(t, nestedMetadata, nestedSkull)
	rootMetadata.Nickname, nestedMetadata.Nickname = "Changed", "Changed"
	require.Equal(t, "Alice <b>literal</b>", items[1].Skull.Nickname)
	require.Equal(t, "Alice <b>literal</b>", nestedSkull.Nickname)
}

func TestSkullMetadataSurvivesMoveSwapDropAndPickup(t *testing.T) {
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createTestRegistry())
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w, playerID, player := setupTestWorld(t)
	grid, hand := setupPlayerWithInventories(w, playerID, player)
	ecs.AddComponent(w, player, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, player, components.EntityInfo{Region: 1})
	ecs.AddComponent(w, player, components.ChunkRef{})
	metadata := skullMetadataFixture()
	addItemToContainer(w, grid, components.InvItem{ItemID: 2001, TypeID: 1, Resource: "test", Quality: 7, Quantity: 1, W: 1, H: 1, Skull: metadata})
	addItemToContainer(w, grid, components.InvItem{ItemID: 2002, TypeID: 1, Resource: "test", Quality: 8, Quantity: 1, W: 1, H: 1, X: 1})
	persister := &failingDroppedPersister{}
	service := NewInventoryOperationService(zap.NewNop(), &dropTestIDAllocator{next: 9000}, persister)
	gridRef := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(playerID)}
	handRef := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID)}
	result := service.ExecuteMove(w, playerID, player, 1, &netproto.InventoryMoveSpec{Src: gridRef, Dst: handRef, ItemId: 2001}, nil)
	require.True(t, result.Success, "%+v", result)
	handState, _ := ecs.GetComponent[components.InventoryContainer](w, hand)
	require.Equal(t, metadata, handState.Items[0].Skull)
	result = service.ExecuteMove(w, playerID, player, 2, &netproto.InventoryMoveSpec{Src: handRef, Dst: gridRef, ItemId: 2001, DstPos: &netproto.GridPos{X: 1}, AllowSwapOrMerge: true}, nil)
	require.True(t, result.Success, "%+v", result)
	gridState, _ := ecs.GetComponent[components.InventoryContainer](w, grid)
	require.Equal(t, types.EntityID(2001), gridState.Items[0].ItemID)
	require.Equal(t, metadata, gridState.Items[0].Skull)
	handState, _ = ecs.GetComponent[components.InventoryContainer](w, hand)
	require.Equal(t, types.EntityID(2002), handState.Items[0].ItemID)
	require.Nil(t, handState.Items[0].Skull)

	result = service.ExecuteDropToWorld(w, playerID, player, 3, &netproto.InventoryMoveSpec{Src: gridRef, ItemId: 2001}, nil)
	require.True(t, result.Success, "%+v", result)
	require.Len(t, persister.records, 1)
	var dropped InventoryDataV1
	require.NoError(t, json.Unmarshal(persister.records[0].InventoryData, &dropped))
	require.Equal(t, metadata, dropped.Items[0].Skull)
	dropHandle, found := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryDroppedItem, 2001, 0)
	require.True(t, found)
	dropContainer, _ := ecs.GetComponent[components.InventoryContainer](w, dropHandle)
	require.Equal(t, metadata, dropContainer.Items[0].Skull)
	require.NotSame(t, metadata, dropContainer.Items[0].Skull)
	result = service.ExecutePickupFromWorld(w, playerID, player, 2001, gridRef)
	require.True(t, result.Success, "%+v", result)
	persisted := inventorySnapshotData(t, persister.inventories, constt.InventoryGrid)
	require.Equal(t, metadata, persisted.Items[0].Skull)
	gridState, _ = ecs.GetComponent[components.InventoryContainer](w, grid)
	require.Equal(t, metadata, gridState.Items[0].Skull)
}

func TestSkullHintExtMatchesBothSnapshotPathsAndProtoRoundtrip(t *testing.T) {
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createTestRegistry())
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	for _, metadata := range []*components.SkullMetadata{skullMetadataFixture(), nil} {
		item := components.InvItem{ItemID: 2001, TypeID: 1, Resource: "test", Quality: 7, Quantity: 1, W: 1, H: 1, Skull: metadata}
		container := components.InventoryContainer{OwnerID: 100, Kind: constt.InventoryHand, Version: 1, Items: []components.InvItem{item}}
		executor := &InventoryExecutor{}
		state := executor.convertContainerToState(w, &ContainerInfo{Container: &container})
		fromOperation := systems.BuildItemInstanceProto(state.Items[0])
		fromLogin := NewSnapshotSender(zap.NewNop()).buildItemInstance(w, item)
		require.True(t, proto.Equal(fromLogin, fromOperation))
		require.Equal(t, metadata.HintExt(), fromOperation.GetHintExt())
		if metadata == nil {
			require.Nil(t, fromOperation.HintExt, "ordinary skull does not emit the optional hint")
		}
		encoded, err := proto.Marshal(fromOperation)
		require.NoError(t, err)
		var decoded netproto.ItemInstance
		require.NoError(t, proto.Unmarshal(encoded, &decoded))
		require.Equal(t, metadata.HintExt(), decoded.GetHintExt())
	}
}

func TestSkullMetadataZeroQualitySurvivesDestructionLootAndRestore(t *testing.T) {
	w, _, _, roots := objectLootCaptureFixture()
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "ore", Resource: "ore", Size: itemdefs.Size{W: 1, H: 1}, Stack: &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 1000}},
		{DefID: 2, Key: "bag", Resource: "bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 4, H: 4}}},
		{DefID: 3014, Key: "skull", Resource: "items/skull.png", Size: itemdefs.Size{W: 1, H: 1}},
	})
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	root, _ := refs.Lookup(constt.InventoryGrid, 100, 5)
	deepest, _ := refs.Lookup(constt.InventoryGrid, 103, 0)
	metadata := skullMetadataFixture()
	for _, handle := range []types.Handle{root, deepest} {
		ecs.WithComponent(w, handle, func(c *components.InventoryContainer) {
			c.Items[0].TypeID, c.Items[0].Quality, c.Items[0].Quantity = 3014, 0, 1
			c.Items[0].Skull = metadata.Clone()
		})
	}
	capture := NewObjectLootCapture(100, roots)
	finishObjectLootCapture(t, capture, w, registry, 1)
	items := capture.Items()
	require.Len(t, items, 2)
	skull := items[1]
	require.Equal(t, uint32(0), skull.Quality)
	require.Equal(t, metadata, skull.Skull)
	require.Equal(t, uint32(0), items[0].NestedInventory.Items[0].NestedInventory.Items[0].Quality)
	require.Equal(t, metadata, items[0].NestedInventory.Items[0].NestedInventory.Items[0].Skull)

	params := SpawnDroppedEntityParams{DroppedEntityID: skull.ItemID, ItemID: skull.ItemID, TypeID: skull.TypeID,
		Resource: skull.Resource, Quality: skull.Quality, Quantity: 1, W: skull.W, H: skull.H, Skull: skull.Skull, NowRuntimeSeconds: 12}
	record, err := BuildDroppedItemPersistenceRecord(params, nil)
	require.NoError(t, err)
	var persisted InventoryDataV1
	require.NoError(t, json.Unmarshal(record.InventoryData, &persisted))
	require.Equal(t, uint32(0), persisted.Items[0].Quality)
	require.Equal(t, metadata, persisted.Items[0].Skull)
	drop, err := SpawnDroppedEntity(w, params)
	require.NoError(t, err)
	container, ok := ecs.GetComponent[components.InventoryContainer](w, drop.ContainerHandle)
	require.True(t, ok)
	require.Equal(t, uint32(0), container.Items[0].Quality)
	require.Equal(t, metadata, container.Items[0].Skull)

	restored := ecs.NewWorldForTesting()
	loaded, err := NewInventoryLoader(zap.NewNop()).LoadPlayerInventories(restored, skull.ItemID, []InventoryDataV1{persisted})
	require.NoError(t, err)
	require.Len(t, loaded.ContainerHandles, 1)
	restoredDrop, ok := ecs.GetComponent[components.InventoryContainer](restored, loaded.ContainerHandles[0])
	require.True(t, ok)
	require.Equal(t, uint32(0), restoredDrop.Items[0].Quality)
	require.Equal(t, metadata, restoredDrop.Items[0].Skull)
	require.Equal(t, "Alice <b>literal</b> died on January 10, 2026", restoredDrop.Items[0].Skull.HintExt())
}
