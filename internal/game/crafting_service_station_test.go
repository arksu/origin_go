package game

import (
	"context"
	"testing"

	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestCraftingServiceStartCraftRejectsUnsuitableStation(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 7001,
		Key:   "campfire",
		Name:  "Campfire",
		Station: &objectdefs.StationDef{
			Capabilities: []string{"cooking"},
			States:       []string{"unlit", "burning"},
			InitialState: "unlit",
			Resources:    []objectdefs.StationResourceDef{{Key: "thread", Amount: 1}},
		},
	}}))

	tests := []struct {
		name        string
		requirement craftdefs.StationRequirement
		station     components.StationState
	}{
		{
			name:        "missing capability",
			requirement: craftdefs.StationRequirement{Capability: "smithing"},
			station:     components.StationState{Capabilities: []string{"cooking"}, CurrentState: "burning", Resources: map[string]uint32{"thread": 1}},
		},
		{
			name:        "wrong state",
			requirement: craftdefs.StationRequirement{State: "burning"},
			station:     components.StationState{Capabilities: []string{"cooking"}, CurrentState: "unlit", Resources: map[string]uint32{"thread": 1}},
		},
		{
			name:        "missing station resource",
			requirement: craftdefs.StationRequirement{Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}},
			station:     components.StationState{Capabilities: []string{"cooking"}, CurrentState: "burning", Resources: map[string]uint32{"thread": 0}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
				DefID:                1,
				Key:                  "test_craft",
				RequiredLinkedObject: "campfire",
				TicksRequired:        1,
				StationRequirements:  []craftdefs.StationRequirement{test.requirement},
			}}))

			world := ecs.NewWorldForTesting()
			playerID := types.EntityID(101)
			playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.CharacterProfile{})
			})
			stationID := types.EntityID(102)
			stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7001})
				ecs.AddComponent(w, h, test.station)
			})
			ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
				PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
			})

			sender := &craftStationTestSender{}
			service := NewCraftingService(world, nil, nil, sender, zap.NewNop())
			service.HandleStartCraftOne(world, playerID, playerHandle, &netproto.C2S_StartCraftOne{CraftKey: "test_craft"})

			if _, active := ecs.GetComponent[components.ActiveCraft](world, playerHandle); active {
				t.Fatal("craft action started despite unsuitable station")
			}
			if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "CRAFT_STATION_REQUIREMENTS_NOT_MET" {
				t.Fatalf("unexpected alert: %#v", sender.alerts)
			}
		})
	}
}

func TestCraftingServiceCraftListReportsStationRequirementsAndFailure(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 7051, Key: "campfire", Name: "Campfire",
	}}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID:                1,
		Key:                  "cook_meat",
		RequiredLinkedObject: "campfire",
		StationRequirements: []craftdefs.StationRequirement{{
			Capability: "cooking",
			State:      "burning",
			Conditions: []craftdefs.StationCondition{{Source: "station", Kind: "value", Key: "temperature", Operator: "gte", Value: 600}},
			Consume:    []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}},
		}},
	}}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(151)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{})
	})
	stationID := types.EntityID(152)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7051})
		ecs.AddComponent(w, h, components.StationState{
			Capabilities: []string{"cooking"},
			CurrentState: "unlit",
			Values:       map[string]float64{"temperature": 20},
			Resources:    map[string]uint32{"thread": 1},
		})
	})
	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
		PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
	})

	entries := NewCraftingService(world, nil, nil, nil, zap.NewNop()).buildCraftList(world, playerID, playerHandle)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if !entry.Flags.HasStationRequirements || entry.Flags.StationRequirementsMet || entry.Flags.GetStationFailureCode() != "station_state_mismatch" {
		t.Fatalf("unexpected station flags: %#v", entry.Flags)
	}
	if len(entry.StationRequirements) != 1 {
		t.Fatalf("station requirements = %#v", entry.StationRequirements)
	}
	requirement := entry.StationRequirements[0]
	if requirement.Capability != "cooking" || requirement.GetState() != "burning" || len(requirement.Conditions) != 1 || len(requirement.Consume) != 1 {
		t.Fatalf("unexpected station requirement: %#v", requirement)
	}

	encoded, err := proto.Marshal(&netproto.S2C_CraftList{Recipes: entries})
	if err != nil {
		t.Fatalf("marshal craft list: %v", err)
	}
	var decoded netproto.S2C_CraftList
	if err := proto.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal craft list: %v", err)
	}
	decodedEntry := decoded.Recipes[0]
	if decodedEntry.Flags.GetStationFailureCode() != "station_state_mismatch" || len(decodedEntry.StationRequirements) != 1 || decodedEntry.StationRequirements[0].Consume[0].GetResourceKey() != "thread" {
		t.Fatalf("station requirements did not survive protobuf round trip: %#v", decodedEntry)
	}
}

func TestCraftingServiceRefreshesOpenSnapshotWhenLinkedStationChanges(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 7061, Key: "campfire", Name: "Campfire",
	}}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID: 1, Key: "cook_meat", RequiredLinkedObject: "campfire",
		StationRequirements: []craftdefs.StationRequirement{{
			State: "burning", Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}},
		}},
	}}))

	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { _ = eventBus.Shutdown(context.Background()) })
	world := ecs.NewWorld(eventBus, 7)
	playerID := types.EntityID(161)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{})
	})
	stationID := types.EntityID(162)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7061})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.StationState{
			CurrentState: "burning",
			Resources:    map[string]uint32{"fuel": 1, "thread": 1},
			AutonomousConsumption: []components.StationAutonomousConsumption{{
				ResourceKey: "fuel", AmountPerTick: 1, RequiredState: "burning", StateWhenDepleted: "unlit",
			}},
		})
	})
	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
		PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
	})
	ecs.GetResource[ecs.OpenedWindowsState](world).Open(playerID, "craft")

	sender := &craftStationTestSender{}
	NewCraftingService(world, eventBus, nil, sender, zap.NewNop())
	if err := eventBus.PublishSync(ecs.NewLinkCreatedEvent(world.Layer, playerID, stationID)); err != nil {
		t.Fatalf("publish link created: %v", err)
	}
	if len(sender.craftLists) != 1 || !sender.craftLists[0].Recipes[0].Flags.StationRequirementsMet {
		t.Fatalf("link creation did not send available craft snapshot: %#v", sender.craftLists)
	}

	world.AddSystem(ecssystems.NewStationSystem(eventBus))
	world.Update(0)
	if len(sender.craftLists) != 2 || sender.craftLists[1].Recipes[0].Flags.GetStationFailureCode() != "station_state_mismatch" {
		t.Fatalf("fire extinguishing did not refresh craft snapshot: %#v", sender.craftLists)
	}

	ecs.MutateComponent[components.StationState](world, stationHandle, func(station *components.StationState) bool {
		station.CurrentState = "burning"
		station.Resources["thread"] = 0
		return true
	})
	if err := eventBus.PublishSync(ecs.NewStationStateChangedEvent(world.Layer, stationID, stationHandle)); err != nil {
		t.Fatalf("publish station state changed: %v", err)
	}
	if len(sender.craftLists) != 3 || sender.craftLists[2].Recipes[0].Flags.GetStationFailureCode() != "station_resource_missing" {
		t.Fatalf("station resource depletion did not refresh craft snapshot: %#v", sender.craftLists)
	}
}

func TestStationSystemRunsBeforeCraftCompletionInSameTick(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 7071, Key: "campfire", Name: "Campfire",
	}}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID: 1, Key: "cook_meat", RequiredLinkedObject: "campfire", TicksRequired: 1,
		StationRequirements: []craftdefs.StationRequirement{{State: "burning"}},
	}}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(171)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.ActiveCraft{CraftKey: "cook_meat", RemainingCycles: 1})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID: craftSyntheticActionID, TargetKind: components.CyclicActionTargetObject,
			TargetID: 172, CycleDurationTicks: 1,
		})
	})
	stationID := types.EntityID(172)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7071})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.StationState{
			CurrentState: "burning", Resources: map[string]uint32{"fuel": 1},
			AutonomousConsumption: []components.StationAutonomousConsumption{{
				ResourceKey: "fuel", AmountPerTick: 1, RequiredState: "burning", StateWhenDepleted: "unlit",
			}},
		})
	})
	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
		PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
	})

	crafting := NewCraftingService(world, nil, nil, &craftStationTestSender{}, zap.NewNop())
	contextActions := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), zap.NewNop())
	contextActions.SetCraftingService(crafting)
	world.AddSystem(ecssystems.NewStationSystem(nil))
	world.AddSystem(NewCyclicActionSystem(contextActions, nil, zap.NewNop()))

	world.Update(0)
	station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
	if station.CurrentState != "unlit" {
		t.Fatalf("station state = %q, want unlit", station.CurrentState)
	}
	if _, active := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle); active {
		t.Fatal("craft completed despite station extinguishing in the same tick")
	}
}

func TestStationCraftEndToEndFinalizationAfterAutonomousUpdate(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	previousItems := itemdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 7081, Key: "branch", Name: "Branch", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 7082, Key: "bark", Name: "Bark", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 7083, Key: "player", Name: "Player"},
		{DefID: 7084, Key: "campfire", Name: "Campfire", BurnerConfig: &objectdefs.BurnerBehaviorConfig{SecondsPerFuel: 1}},
	}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID:                1,
		Key:                  "cook_branch",
		RequiredLinkedObject: "campfire",
		Inputs:               []craftdefs.CraftInput{{ItemKey: "branch", Count: 1, QualityWeight: 1}},
		Outputs:              []craftdefs.CraftOutput{{ItemKey: "bark", Count: 1}},
		StaminaCost:          10,
		TicksRequired:        1,
		StationRequirements: []craftdefs.StationRequirement{{
			State: "burning", Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}},
		}},
	}}))

	tests := []struct {
		name             string
		fuel             uint32
		wantOutput       bool
		wantThread       uint32
		wantStamina      float64
		wantFailureAlert bool
	}{
		{name: "fire extinguishes before finalization", fuel: 1, wantThread: 1, wantStamina: 500, wantFailureAlert: true},
		{name: "burning station finalizes atomically", fuel: 2, wantOutput: true, wantThread: 0, wantStamina: 490},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			playerID := types.EntityID(181)
			playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7083})
				ecs.AddComponent(w, h, components.CharacterProfile{})
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 500})
			})
			containerHandle := world.Spawn(types.EntityID(182), func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.InventoryContainer{
					OwnerID: playerID, Kind: constt.InventoryGrid, Width: 2, Height: 2,
					Items: []components.InvItem{{TypeID: 7081, Quantity: 1, W: 1, H: 1}},
				})
			})
			ecs.AddComponent(world, playerHandle, components.InventoryOwner{Inventories: []components.InventoryLink{{
				Kind: constt.InventoryGrid, OwnerID: playerID, Handle: containerHandle,
			}}})
			stationID := types.EntityID(183)
			stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7084})
				ecs.AddComponent(w, h, components.ObjectInternalState{})
				ecs.AddComponent(w, h, components.StationState{
					CurrentState: "burning", Resources: map[string]uint32{"fuel": test.fuel, "thread": 1},
					AutonomousConsumption: []components.StationAutonomousConsumption{{
						ResourceKey: "fuel", AmountPerTick: 1, RequiredState: "burning", StateWhenDepleted: "unlit",
					}},
				})
				ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
					components.SetBehaviorState(state, "burner", &components.BurnerBehaviorState{Fuel: test.fuel, NextFuelBurnAtRuntimeSecond: 1})
				})
			})
			ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
				PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
			})

			sender := &craftStationTestSender{}
			crafting := NewCraftingService(world, nil, inventory.NewInventoryExecutor(zap.NewNop(), &craftStationIDAllocator{next: 9800}, nil, nil, nil), sender, zap.NewNop())
			crafting.HandleStartCraftOne(world, playerID, playerHandle, &netproto.C2S_StartCraftOne{CraftKey: "cook_branch"})
			if _, active := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle); !active {
				reasonCode := ""
				if len(sender.alerts) > 0 {
					reasonCode = sender.alerts[0].ReasonCode
				}
				t.Fatalf("craft did not start: alert=%s", reasonCode)
			}

			contextActions := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), zap.NewNop())
			contextActions.SetCraftingService(crafting)
			ecs.SetResource(world, ecs.TimeState{RuntimeSecondsTotal: 1})
			world.AddSystem(ecssystems.NewBurnerSystem())
			world.AddSystem(NewCyclicActionSystem(contextActions, nil, zap.NewNop()))
			world.Update(0)

			container, _ := ecs.GetComponent[components.InventoryContainer](world, containerHandle)
			if test.wantOutput {
				if len(container.Items) != 1 || container.Items[0].TypeID != 7082 || container.Items[0].Quantity != 1 {
					t.Fatalf("output was not created atomically: %#v", container.Items)
				}
			} else if len(container.Items) != 1 || container.Items[0].TypeID != 7081 || container.Items[0].Quantity != 1 {
				t.Fatalf("input changed after failed finalization: %#v", container.Items)
			}
			station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
			if station.Resources["thread"] != test.wantThread {
				t.Fatalf("thread = %d, want %d", station.Resources["thread"], test.wantThread)
			}
			stats, _ := ecs.GetComponent[components.EntityStats](world, playerHandle)
			if stats.Stamina != test.wantStamina {
				t.Fatalf("stamina = %v, want %v", stats.Stamina, test.wantStamina)
			}
			if test.wantFailureAlert && (len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "CRAFT_STATION_REQUIREMENTS_NOT_MET") {
				t.Fatalf("missing finalization alert: %#v", sender.alerts)
			}
		})
	}
}

func TestCraftingServiceFinalValidationCancelsBeforeInputConsumption(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	previousItems := itemdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 7101, Key: "raw_meat", Name: "Raw Meat", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 7102, Key: "cooked_meat", Name: "Cooked Meat", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 7103, Key: "campfire", Name: "Campfire",
	}}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID:                1,
		Key:                  "cook_meat",
		RequiredLinkedObject: "campfire",
		Inputs:               []craftdefs.CraftInput{{ItemKey: "raw_meat", Count: 1, QualityWeight: 1}},
		Outputs:              []craftdefs.CraftOutput{{ItemKey: "cooked_meat", Count: 1}},
		TicksRequired:        1,
		StationRequirements:  []craftdefs.StationRequirement{{State: "burning", Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}}},
	}}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(201)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.ActiveCraft{CraftKey: "cook_meat", RemainingCycles: 1})
	})
	containerHandle := world.Spawn(types.EntityID(202), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{
			OwnerID: playerID, Kind: constt.InventoryGrid, Width: 2, Height: 2,
			Items: []components.InvItem{{TypeID: 7101, Quantity: 1, W: 1, H: 1}},
		})
	})
	ecs.AddComponent(world, playerHandle, components.InventoryOwner{Inventories: []components.InventoryLink{{
		Kind: constt.InventoryGrid, OwnerID: playerID, Handle: containerHandle,
	}}})

	stationID := types.EntityID(203)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7103})
		ecs.AddComponent(w, h, components.StationState{CurrentState: "unlit", Resources: map[string]uint32{"thread": 1}})
	})

	service := NewCraftingService(world, nil, inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil), &craftStationTestSender{}, zap.NewNop())
	decision := service.HandleCraftCycleComplete(world, playerID, playerHandle, components.ActiveCyclicAction{
		TargetKind:   components.CyclicActionTargetObject,
		TargetID:     stationID,
		TargetHandle: stationHandle,
	})
	if decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("decision = %v, want canceled", decision)
	}
	container, _ := ecs.GetComponent[components.InventoryContainer](world, containerHandle)
	if len(container.Items) != 1 || container.Items[0].Quantity != 1 {
		t.Fatalf("craft inputs changed after failed final validation: %#v", container.Items)
	}
	station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
	if station.Resources["thread"] != 1 {
		t.Fatalf("station resources changed after failed final validation: %#v", station.Resources)
	}
}

func TestCraftingServiceCycleCommitRollsBackWhenOutputCreationFails(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	previousItems := itemdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 7201, Key: "raw_meat", Name: "Raw Meat", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 7202, Key: "cooked_meat", Name: "Cooked Meat", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 7203, Key: "campfire", Name: "Campfire"}}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID:                1,
		Key:                  "cook_meat",
		RequiredLinkedObject: "campfire",
		Inputs:               []craftdefs.CraftInput{{ItemKey: "raw_meat", Count: 1, QualityWeight: 1}},
		Outputs:              []craftdefs.CraftOutput{{ItemKey: "cooked_meat", Count: 1}},
		TicksRequired:        1,
		StationRequirements:  []craftdefs.StationRequirement{{State: "burning", Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}}},
	}}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(301)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.ActiveCraft{CraftKey: "cook_meat", RemainingCycles: 1})
	})
	containerHandle := world.Spawn(types.EntityID(302), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{
			OwnerID: playerID, Kind: constt.InventoryGrid, Width: 2, Height: 2,
			Items: []components.InvItem{{TypeID: 7201, Quantity: 1, W: 1, H: 1}},
		})
	})
	ecs.AddComponent(world, playerHandle, components.InventoryOwner{Inventories: []components.InventoryLink{{
		Kind: constt.InventoryGrid, OwnerID: playerID, Handle: containerHandle,
	}}})

	stationID := types.EntityID(303)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7203})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.StationState{
			CurrentState: "burning",
			Resources:    map[string]uint32{"thread": 1, "fuel": 2},
			AutonomousConsumption: []components.StationAutonomousConsumption{{
				ResourceKey:       "fuel",
				AmountPerTick:     1,
				RequiredState:     "burning",
				StateWhenDepleted: "unlit",
			}},
		})
	})

	service := NewCraftingService(world, nil, inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil), &craftStationTestSender{}, zap.NewNop())
	decision := service.HandleCraftCycleComplete(world, playerID, playerHandle, components.ActiveCyclicAction{
		TargetKind:   components.CyclicActionTargetObject,
		TargetID:     stationID,
		TargetHandle: stationHandle,
	})
	if decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("decision = %v, want canceled", decision)
	}
	container, _ := ecs.GetComponent[components.InventoryContainer](world, containerHandle)
	if len(container.Items) != 1 || container.Items[0].Quantity != 1 {
		t.Fatalf("craft inputs were not rolled back: %#v", container.Items)
	}
	station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
	if station.Resources["thread"] != 1 {
		t.Fatalf("station resources were not rolled back: %#v", station.Resources)
	}
}

func TestCraftingServiceCycleCommitConsumesAndCreatesOutputTogether(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	previousItems := itemdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 7301, Key: "raw_meat", Name: "Raw Meat", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 7302, Key: "cooked_meat", Name: "Cooked Meat", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 7303, Key: "campfire", Name: "Campfire"},
		{DefID: 7304, Key: "player", Name: "Player"},
	}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID:                1,
		Key:                  "cook_meat",
		RequiredLinkedObject: "campfire",
		Inputs:               []craftdefs.CraftInput{{ItemKey: "raw_meat", Count: 1, QualityWeight: 1}},
		Outputs:              []craftdefs.CraftOutput{{ItemKey: "cooked_meat", Count: 1}},
		StaminaCost:          10,
		TicksRequired:        1,
		StationRequirements:  []craftdefs.StationRequirement{{State: "burning", Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}}},
	}}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(401)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7304})
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 500})
		ecs.AddComponent(w, h, components.ActiveCraft{CraftKey: "cook_meat", RemainingCycles: 1})
	})
	containerHandle := world.Spawn(types.EntityID(402), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{
			OwnerID: playerID, Kind: constt.InventoryGrid, Width: 2, Height: 2,
			Items: []components.InvItem{{TypeID: 7301, Quantity: 1, W: 1, H: 1}},
		})
	})
	ecs.AddComponent(world, playerHandle, components.InventoryOwner{Inventories: []components.InventoryLink{{
		Kind: constt.InventoryGrid, OwnerID: playerID, Handle: containerHandle,
	}}})

	stationID := types.EntityID(403)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7303})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.StationState{
			CurrentState: "burning",
			Resources:    map[string]uint32{"thread": 1, "fuel": 2},
			AutonomousConsumption: []components.StationAutonomousConsumption{{
				ResourceKey:       "fuel",
				AmountPerTick:     1,
				RequiredState:     "burning",
				StateWhenDepleted: "unlit",
			}},
		})
	})

	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
		PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
	})
	ecs.GetResource[ecs.OpenedWindowsState](world).Open(playerID, "craft")
	sender := &craftStationTestSender{}
	service := NewCraftingService(world, nil, inventory.NewInventoryExecutor(zap.NewNop(), &craftStationIDAllocator{next: 8000}, nil, nil, nil), sender, zap.NewNop())
	decision := service.HandleCraftCycleComplete(world, playerID, playerHandle, components.ActiveCyclicAction{
		TargetKind:   components.CyclicActionTargetObject,
		TargetID:     stationID,
		TargetHandle: stationHandle,
	})
	if decision != contracts.BehaviorCycleDecisionComplete {
		reasonCode := ""
		if len(sender.alerts) > 0 {
			reasonCode = sender.alerts[0].ReasonCode
		}
		t.Fatalf("decision = %v, want complete; alert=%s", decision, reasonCode)
	}
	container, _ := ecs.GetComponent[components.InventoryContainer](world, containerHandle)
	if len(container.Items) != 1 || container.Items[0].TypeID != 7302 || container.Items[0].Quantity != 1 {
		t.Fatalf("unexpected crafted inventory: %#v", container.Items)
	}
	station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
	if station.Resources["thread"] != 0 {
		t.Fatalf("station resource not consumed: %#v", station.Resources)
	}
	if station.Resources["fuel"] != 2 {
		t.Fatalf("craft consumed autonomous fuel: %#v", station.Resources)
	}
	objectState, _ := ecs.GetComponent[components.ObjectInternalState](world, stationHandle)
	if !objectState.IsDirty {
		t.Fatal("station craft consumption did not mark object dirty")
	}
	stats, _ := ecs.GetComponent[components.EntityStats](world, playerHandle)
	if stats.Stamina != 490 {
		t.Fatalf("stamina = %v, want 490", stats.Stamina)
	}
	if len(sender.craftLists) != 1 || sender.craftLists[0].Recipes[0].Flags.GetStationFailureCode() != "station_resource_missing" {
		t.Fatalf("craft completion did not refresh snapshot after station resource consumption: %#v", sender.craftLists)
	}
	world.AddSystem(ecssystems.NewStationSystem(nil))
	world.Update(0)
	station, _ = ecs.GetComponent[components.StationState](world, stationHandle)
	if station.Resources["fuel"] != 1 {
		t.Fatalf("station runtime did not consume fuel independently: %#v", station.Resources)
	}
}

func TestCraftingServiceStartCraftManyStopsWhenStationChangesBetweenCycles(t *testing.T) {
	previousCrafts := craftdefs.Global()
	previousObjects := objectdefs.Global()
	previousItems := itemdefs.Global()
	t.Cleanup(func() {
		craftdefs.SetGlobalForTesting(previousCrafts)
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 7401, Key: "raw_meat", Name: "Raw Meat", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 7402, Key: "cooked_meat", Name: "Cooked Meat", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 7403, Key: "campfire", Name: "Campfire"},
		{DefID: 7404, Key: "player", Name: "Player"},
	}))
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID:                1,
		Key:                  "cook_meat",
		RequiredLinkedObject: "campfire",
		Inputs:               []craftdefs.CraftInput{{ItemKey: "raw_meat", Count: 1, QualityWeight: 1}},
		Outputs:              []craftdefs.CraftOutput{{ItemKey: "cooked_meat", Count: 1}},
		TicksRequired:        1,
		StationRequirements:  []craftdefs.StationRequirement{{State: "burning", Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}}},
	}}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(501)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7404})
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 500})
	})
	containerHandle := world.Spawn(types.EntityID(502), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{
			OwnerID: playerID, Kind: constt.InventoryGrid, Width: 3, Height: 3,
			Items: []components.InvItem{{TypeID: 7401, Quantity: 2, W: 1, H: 1}},
		})
	})
	ecs.AddComponent(world, playerHandle, components.InventoryOwner{Inventories: []components.InventoryLink{{
		Kind: constt.InventoryGrid, OwnerID: playerID, Handle: containerHandle,
	}}})

	stationID := types.EntityID(503)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 7403})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.StationState{CurrentState: "burning", Resources: map[string]uint32{"thread": 2}})
	})
	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{
		PlayerID: playerID, PlayerHandle: playerHandle, TargetID: stationID, TargetHandle: stationHandle,
	})

	service := NewCraftingService(world, nil, inventory.NewInventoryExecutor(zap.NewNop(), &craftStationIDAllocator{next: 9000}, nil, nil, nil), &craftStationTestSender{}, zap.NewNop())
	service.HandleStartCraftMany(world, playerID, playerHandle, &netproto.C2S_StartCraftMany{CraftKey: "cook_meat", Cycles: 2})
	action, active := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle)
	if !active {
		t.Fatal("multi-cycle craft did not start")
	}
	if decision := service.HandleCraftCycleComplete(world, playerID, playerHandle, action); decision != contracts.BehaviorCycleDecisionContinue {
		t.Fatalf("first decision = %v, want continue", decision)
	}
	ecs.MutateComponent[components.StationState](world, stationHandle, func(station *components.StationState) bool {
		station.CurrentState = "unlit"
		return true
	})
	if decision := service.HandleCraftCycleComplete(world, playerID, playerHandle, action); decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("second decision = %v, want canceled", decision)
	}

	container, _ := ecs.GetComponent[components.InventoryContainer](world, containerHandle)
	if len(container.Items) != 2 || container.Items[0].TypeID != 7401 || container.Items[0].Quantity != 1 || container.Items[1].TypeID != 7402 {
		t.Fatalf("unexpected inventory after canceled second cycle: %#v", container.Items)
	}
	station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
	if station.Resources["thread"] != 1 {
		t.Fatalf("second cycle consumed station resource: %#v", station.Resources)
	}
}

type craftStationIDAllocator struct {
	next types.EntityID
}

func (a *craftStationIDAllocator) GetFreeID() types.EntityID {
	a.next++
	return a.next
}

type craftStationTestSender struct {
	alerts     []*netproto.S2C_MiniAlert
	craftLists []*netproto.S2C_CraftList
}

func (s *craftStationTestSender) SendMiniAlert(_ types.EntityID, alert *netproto.S2C_MiniAlert) {
	s.alerts = append(s.alerts, alert)
}

func (*craftStationTestSender) SendInventoryUpdate(types.EntityID, []*netproto.InventoryState) {}
func (*craftStationTestSender) SendExpGained(types.EntityID, *netproto.S2C_ExpGained)          {}
func (s *craftStationTestSender) SendCraftList(_ types.EntityID, list *netproto.S2C_CraftList) {
	s.craftLists = append(s.craftLists, list)
}
