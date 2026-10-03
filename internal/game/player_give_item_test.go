package game

import (
	"path/filepath"
	"slices"
	"testing"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type testPlayerGiveItemSender struct {
	inventory []*netproto.S2C_InventoryOpResult
	exp       []*netproto.S2C_ExpGained
	fx        []*netproto.S2C_Fx
	sound     []*netproto.S2C_Sound
}

type digTestIDAllocator struct {
	next types.EntityID
}

func (allocator *digTestIDAllocator) GetFreeID() types.EntityID {
	allocator.next++
	return allocator.next
}

func (sender *testPlayerGiveItemSender) SendInventoryOpResult(_ types.EntityID, update *netproto.S2C_InventoryOpResult) {
	sender.inventory = append(sender.inventory, update)
}

func (sender *testPlayerGiveItemSender) SendExpGained(_ types.EntityID, update *netproto.S2C_ExpGained) {
	sender.exp = append(sender.exp, update)
}

func (sender *testPlayerGiveItemSender) SendFx(_ types.EntityID, effect *netproto.S2C_Fx) {
	sender.fx = append(sender.fx, effect)
}

func (sender *testPlayerGiveItemSender) SendSound(_ types.EntityID, sound *netproto.S2C_Sound) {
	sender.sound = append(sender.sound, sound)
}

func installDigInventory(t *testing.T, world *ecs.World, player types.Handle) (types.Handle, types.Handle) {
	t.Helper()
	previousObjects := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, Key: "player", Name: "Player"}}))
	ecs.AddComponent(world, player, components.EntityInfo{TypeID: 1})
	root := world.SpawnWithoutExternalID()
	hand := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, root, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryGrid, Version: 1, Width: 1, Height: 1})
	ecs.AddComponent(world, hand, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryHand, Version: 1})
	ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{
		{Kind: constt.InventoryGrid, OwnerID: 1, Handle: root},
		{Kind: constt.InventoryHand, OwnerID: 1, Handle: hand},
	}})
	index := ecs.GetResource[ecs.InventoryRefIndex](world)
	index.Add(constt.InventoryGrid, 1, 0, root)
	index.Add(constt.InventoryHand, 1, 0, hand)
	return root, hand
}

func loadProductionItemsForDigTest(t *testing.T) {
	t.Helper()
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	definitions, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	itemdefs.SetGlobalForTesting(definitions)
}

func TestDigUsesRealGiveItemOrderAndStopsAtCapacity(t *testing.T) {
	loadProductionItemsForDigTest(t)
	world, player, service, terrain, actionSender, _ := newDigTest(t, types.TileGrass)
	root, hand := installDigInventory(t, world, player)
	notifications := &testPlayerGiveItemSender{}
	executor := inventory.NewInventoryExecutor(zap.NewNop(), &digTestIDAllocator{next: 10000}, nil, nil, nil)
	service.handlers["dig"] = &digTileActionHandler{terrain: terrain, giveItem: newPlayerGiveItemAdapter(executor, notifications)}

	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, actionSender)
	rootInventory, _ := ecs.GetComponent[components.InventoryContainer](world, root)
	handInventory, _ := ecs.GetComponent[components.InventoryContainer](world, hand)
	soil, _ := itemdefs.Global().GetByKey("soil")
	if len(rootInventory.Items) != 1 || rootInventory.Items[0].TypeID != uint32(soil.DefID) || rootInventory.Items[0].Quality != 10 || len(handInventory.Items) != 0 {
		t.Fatalf("first dig did not use root grid: root=%#v hand=%#v", rootInventory.Items, handInventory.Items)
	}
	if len(notifications.inventory) != 1 || len(notifications.inventory[0].Updated) == 0 {
		t.Fatalf("first grant did not notify inventory: %#v", notifications.inventory)
	}
	profile, _ := ecs.GetComponent[components.CharacterProfile](world, player)
	if !slices.Contains(profile.Discovery, "soil") {
		t.Fatalf("first grant did not record discovery: %#v", profile.Discovery)
	}

	advanceDigCycle(t, world, player, service, actionSender)
	handInventory, _ = ecs.GetComponent[components.InventoryContainer](world, hand)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if len(handInventory.Items) != 1 || handInventory.Items[0].TypeID != uint32(soil.DefID) || handInventory.Items[0].Quality != 10 ||
		stats.Stamina != 400 || service.State(world, player).Phase != "idle" || len(notifications.inventory) != 2 {
		t.Fatalf("hand fallback did not finish charged cycle: hand=%#v stamina=%v state=%#v updates=%d", handInventory.Items, stats.Stamina, service.State(world, player), len(notifications.inventory))
	}
	if terrain.chunk.SnapshotTiles().Version != 7 {
		t.Fatal("dig altered tile while granting real items")
	}

	service.Activate(world, 1, player, "dig")
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, actionSender)
	stats, _ = ecs.GetComponent[components.EntityStats](world, player)
	if stats.Stamina != 400 || len(notifications.inventory) != 2 || len(actionSender.alerts) == 0 || service.State(world, player).Phase != "idle" {
		t.Fatalf("full inventory and hand granted or charged: stamina=%v updates=%d alerts=%#v", stats.Stamina, len(notifications.inventory), actionSender.alerts)
	}
}

func TestPlayerGiveItemAdapterForwardsDiscoveryNotifications(t *testing.T) {
	loadProductionItemsForDigTest(t)
	world, player, _, _, _, _ := newDigTest(t, types.TileGrass)
	installDigInventory(t, world, player)
	notifications := &testPlayerGiveItemSender{}
	executor := inventory.NewInventoryExecutor(zap.NewNop(), &digTestIDAllocator{next: 11000}, nil, nil, nil)
	give := newPlayerGiveItemAdapter(executor, notifications)
	outcome := give(world, 1, player, "branch", 1, 10)
	if !outcome.Success || outcome.GrantedCount != 1 || outcome.PlacedInHand ||
		len(notifications.inventory) != 1 || len(notifications.exp) != 1 || notifications.exp[0].GetLp() != 10 ||
		len(notifications.fx) != 1 || len(notifications.sound) != 0 {
		t.Fatalf("adapter lost inventory or discovery updates: outcome=%#v notifications=%#v", outcome, notifications)
	}
}
