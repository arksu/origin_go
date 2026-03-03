package behaviors

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func TestPlayerDeathBehavior_UnequipSuccess(t *testing.T) {
	setupPlayerDeathItemRegistry(t)

	world, playerID, playerHandle, corpseID, corpseHandle, equipmentHandle := setupPlayerDeathBehaviorWorld(
		t,
		[]components.InvItem{
			{
				ItemID:    9101,
				TypeID:    9001,
				Quality:   77,
				Quantity:  1,
				W:         1,
				H:         1,
				EquipSlot: netproto.EquipSlot_EQUIP_SLOT_HEAD,
			},
		},
	)

	called := false
	result := playerDeathBehavior{}.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     corpseID,
		TargetHandle: corpseHandle,
		ActionID:     actionUnequip,
		Deps: &contracts.ExecutionDeps{
			GiveItem: func(_ *ecs.World, gotPlayerID types.EntityID, gotPlayerHandle types.Handle, itemKey string, count uint32, quality uint32) contracts.GiveItemOutcome {
				called = true
				if gotPlayerID != playerID {
					t.Fatalf("unexpected player id: got %d, want %d", gotPlayerID, playerID)
				}
				if gotPlayerHandle != playerHandle {
					t.Fatalf("unexpected player handle: got %d, want %d", gotPlayerHandle, playerHandle)
				}
				if itemKey != "iron_helmet_test" {
					t.Fatalf("unexpected item key: %s", itemKey)
				}
				if count != 1 {
					t.Fatalf("unexpected count: %d", count)
				}
				if quality != 77 {
					t.Fatalf("unexpected quality: %d", quality)
				}
				return contracts.GiveItemOutcome{Success: true}
			},
		},
	})

	if !result.OK {
		t.Fatalf("expected success, got %+v", result)
	}
	if !called {
		t.Fatalf("expected GiveItem call")
	}

	equipment, ok := ecs.GetComponent[components.InventoryContainer](world, equipmentHandle)
	if !ok {
		t.Fatalf("equipment container missing")
	}
	if len(equipment.Items) != 0 {
		t.Fatalf("expected item removed from corpse equipment, got %d items", len(equipment.Items))
	}
	if equipment.Version != 2 {
		t.Fatalf("expected equipment revision increment, got %d", equipment.Version)
	}

	state, hasState := ecs.GetComponent[components.ObjectInternalState](world, corpseHandle)
	if !hasState || !state.IsDirty {
		t.Fatalf("expected corpse state marked dirty after unequip")
	}
}

func TestPlayerDeathBehavior_UnequipGiveFailureKeepsEquipment(t *testing.T) {
	setupPlayerDeathItemRegistry(t)

	world, playerID, playerHandle, corpseID, corpseHandle, equipmentHandle := setupPlayerDeathBehaviorWorld(
		t,
		[]components.InvItem{
			{
				ItemID:    9102,
				TypeID:    9001,
				Quality:   55,
				Quantity:  1,
				W:         1,
				H:         1,
				EquipSlot: netproto.EquipSlot_EQUIP_SLOT_CHEST,
			},
		},
	)

	result := playerDeathBehavior{}.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     corpseID,
		TargetHandle: corpseHandle,
		ActionID:     actionUnequip,
		Deps: &contracts.ExecutionDeps{
			GiveItem: func(_ *ecs.World, _ types.EntityID, _ types.Handle, _ string, _ uint32, _ uint32) contracts.GiveItemOutcome {
				return contracts.GiveItemOutcome{Success: false}
			},
		},
	})

	if result.OK {
		t.Fatalf("expected failure when GiveItem fails")
	}
	if !result.UserVisible {
		t.Fatalf("expected user-visible failure")
	}

	equipment, ok := ecs.GetComponent[components.InventoryContainer](world, equipmentHandle)
	if !ok {
		t.Fatalf("equipment container missing")
	}
	if len(equipment.Items) != 1 {
		t.Fatalf("expected corpse equipment unchanged, got %d items", len(equipment.Items))
	}
	if equipment.Version != 1 {
		t.Fatalf("expected version unchanged, got %d", equipment.Version)
	}

	state, hasState := ecs.GetComponent[components.ObjectInternalState](world, corpseHandle)
	if !hasState {
		t.Fatalf("corpse state missing")
	}
	if state.IsDirty {
		t.Fatalf("did not expect corpse dirty flag on failed give")
	}
}

func TestPlayerDeathBehavior_NoEligibleItemsNoAction(t *testing.T) {
	setupPlayerDeathItemRegistry(t)

	world, playerID, playerHandle, corpseID, corpseHandle, _ := setupPlayerDeathBehaviorWorld(
		t,
		[]components.InvItem{
			{
				ItemID:    9103,
				TypeID:    9001,
				Quality:   1,
				Quantity:  1,
				W:         1,
				H:         1,
				EquipSlot: netproto.EquipSlot_EQUIP_SLOT_NONE,
			},
		},
	)

	actions := playerDeathBehavior{}.ProvideActions(&contracts.BehaviorActionListContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     corpseID,
		TargetHandle: corpseHandle,
	})
	if len(actions) != 0 {
		t.Fatalf("expected no context actions, got %+v", actions)
	}

	validate := playerDeathBehavior{}.ValidateAction(&contracts.BehaviorActionValidateContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     corpseID,
		TargetHandle: corpseHandle,
		ActionID:     actionUnequip,
		Phase:        contracts.BehaviorValidationPhasePreview,
	})
	if validate.OK {
		t.Fatalf("expected validation failure when no eligible equipped items")
	}
}

func TestPlayerDeathBehavior_SkipsContainerItems(t *testing.T) {
	setupPlayerDeathItemRegistry(t)

	world, playerID, playerHandle, corpseID, corpseHandle, _ := setupPlayerDeathBehaviorWorld(
		t,
		[]components.InvItem{
			{
				ItemID:    9104,
				TypeID:    9002,
				Quality:   1,
				Quantity:  1,
				W:         1,
				H:         1,
				EquipSlot: netproto.EquipSlot_EQUIP_SLOT_BACK,
			},
		},
	)

	actions := playerDeathBehavior{}.ProvideActions(&contracts.BehaviorActionListContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     corpseID,
		TargetHandle: corpseHandle,
	})
	if len(actions) != 0 {
		t.Fatalf("container item must be excluded from unequip candidates")
	}

	validate := playerDeathBehavior{}.ValidateAction(&contracts.BehaviorActionValidateContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     corpseID,
		TargetHandle: corpseHandle,
		ActionID:     actionUnequip,
		Phase:        contracts.BehaviorValidationPhasePreview,
	})
	if validate.OK {
		t.Fatalf("expected validation failure for container-only equipment")
	}
}

func setupPlayerDeathBehaviorWorld(
	t *testing.T,
	equipmentItems []components.InvItem,
) (
	*ecs.World,
	types.EntityID,
	types.Handle,
	types.EntityID,
	types.Handle,
	types.Handle,
) {
	t.Helper()

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(7001)
	playerHandle := world.Spawn(playerID, nil)

	corpseID := types.EntityID(8001)
	corpseHandle := world.Spawn(corpseID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	equipmentHandle := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, equipmentHandle, components.InventoryContainer{
		OwnerID: corpseID,
		Kind:    constt.InventoryEquipment,
		Key:     0,
		Version: 1,
		Items:   append([]components.InvItem(nil), equipmentItems...),
	})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryEquipment, corpseID, 0, equipmentHandle)

	return world, playerID, playerHandle, corpseID, corpseHandle, equipmentHandle
}

func newPlayerDeathBehaviorItemRegistry() *itemdefs.Registry {
	return itemdefs.NewRegistry([]itemdefs.ItemDef{
		{
			DefID:    9001,
			Key:      "iron_helmet_test",
			Name:     "Iron Helmet Test",
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "iron_helmet_test",
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(true),
				Grid: boolPtr(true),
			},
		},
		{
			DefID:    9002,
			Key:      "seed_bag_test",
			Name:     "Seed Bag Test",
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "seed_bag_test",
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(true),
				Grid: boolPtr(true),
			},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 2, H: 2},
			},
		},
	})
}

func setupPlayerDeathItemRegistry(t *testing.T) {
	t.Helper()
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})
	itemdefs.SetGlobalForTesting(newPlayerDeathBehaviorItemRegistry())
}

func boolPtr(value bool) *bool {
	v := value
	return &v
}
