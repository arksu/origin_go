package game

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
	"path/filepath"
	"testing"
)

type roastFixture struct {
	world                 *ecs.World
	playerID, stationID   types.EntityID
	player, station, root types.Handle
	recipe                *craftdefs.CraftDef
	service               *CraftingService
	sender                *craftStationTestSender
}

func setupRoastFixture(t *testing.T, sourceKey string) *roastFixture {
	t.Helper()
	oldItems, oldObjects, oldCrafts := itemdefs.Global(), objectdefs.Global(), craftdefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(oldItems)
		objectdefs.SetGlobalForTesting(oldObjects)
		craftdefs.SetGlobalForTesting(oldCrafts)
	})
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	crafts, err := craftdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "crafts"), zap.NewNop())
	require.NoError(t, err)
	recipe, ok := crafts.GetByKey("roasted_meat")
	require.True(t, ok)
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{*recipe}))
	recipe, _ = craftdefs.Global().GetByKey("roasted_meat")
	f := &roastFixture{world: ecs.NewWorldForTesting(), playerID: 801, stationID: 802, recipe: recipe, sender: &craftStationTestSender{}}
	playerDef, ok := objects.GetByKey("player")
	require.True(t, ok)
	f.player = f.world.Spawn(f.playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: uint32(playerDef.DefID)})
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 500})
	})
	source, ok := items.GetByKey(sourceKey)
	require.True(t, ok)
	f.root = f.world.Spawn(803, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{
			OwnerID: f.playerID, Kind: constt.InventoryGrid, Width: 4, Height: 4, Version: 1,
			Items: []components.InvItem{{ItemID: 900, TypeID: uint32(source.DefID), Quantity: 1, Quality: 37, W: 1, H: 1}},
		})
	})
	ecs.AddComponent(f.world, f.player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: f.playerID, Handle: f.root}}})
	campfire, ok := objects.GetByKey("campfire")
	require.True(t, ok)
	f.station = f.world.Spawn(f.stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: uint32(campfire.DefID)})
		ecs.AddComponent(w, h, components.StationState{Capabilities: []string{"cooking"}, CurrentState: "burning", Resources: map[string]uint32{"fuel": 2}})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	ecs.GetResource[ecs.LinkState](f.world).SetLink(ecs.PlayerLink{PlayerID: f.playerID, PlayerHandle: f.player, TargetID: f.stationID, TargetHandle: f.station})
	f.service = NewCraftingService(f.world, nil, inventory.NewInventoryExecutor(zap.NewNop(), &craftStationIDAllocator{next: 1000}, nil, nil, nil), f.sender, zap.NewNop())
	return f
}

func (f *roastFixture) start(t *testing.T, cycles uint32) components.ActiveCyclicAction {
	t.Helper()
	f.service.HandleStartCraftMany(f.world, f.playerID, f.player, &netproto.C2S_StartCraftMany{CraftKey: f.recipe.Key, Cycles: cycles})
	action, ok := ecs.GetComponent[components.ActiveCyclicAction](f.world, f.player)
	require.True(t, ok, "craft failed to start: %v", f.sender.alerts)
	require.Equal(t, components.CyclicActionTargetObject, action.TargetKind)
	require.Equal(t, f.stationID, action.TargetID)
	return action
}

func (f *roastFixture) items() []components.InvItem {
	c, _ := ecs.GetComponent[components.InventoryContainer](f.world, f.root)
	return c.Items
}
func (f *roastFixture) complete(action components.ActiveCyclicAction) contracts.BehaviorCycleDecision {
	return f.service.HandleCraftCycleComplete(f.world, f.playerID, f.player, action)
}

func TestRoastedMeatAllSpecies(t *testing.T) {
	for _, entry := range []struct{ source, target string }{
		{"beef", "roasted_beef"}, {"fox_meat", "roasted_fox_meat"}, {"rabbit_meat", "roasted_rabbit_meat"}, {"boar_meat", "roasted_boar_meat"}, {"bear_meat", "roasted_bear_meat"}, {"raw_deer_meat", "roasted_deer_meat"}, {"raw_mutton", "roasted_mutton"}, {"raw_pork", "roast_pork"}, {"raw_chicken_meat", "roasted_chicken_meat"},
	} {
		t.Run(entry.source, func(t *testing.T) {
			f := setupRoastFixture(t, entry.source)
			// Runtime must ignore both preview count and additional preview rows.
			f.recipe.Outputs = []craftdefs.CraftOutput{{ItemKey: "roasted_meat", Count: 5}, {ItemKey: "apple", Count: 2}}
			action := f.start(t, 1)
			require.Equal(t, contracts.BehaviorCycleDecisionComplete, f.complete(action))
			require.Len(t, f.items(), 1)
			result := f.items()[0]
			target, _ := itemdefs.Global().GetByKey(entry.target)
			require.Equal(t, uint32(target.DefID), result.TypeID)
			require.Equal(t, uint32(1), result.Quantity)
			require.Equal(t, uint32(37), result.Quality)
			require.NotEqual(t, types.EntityID(900), result.ItemID)
			profile, _ := ecs.GetComponent[components.CharacterProfile](f.world, f.player)
			require.Contains(t, profile.Discovery, entry.target)
			require.NotContains(t, profile.Discovery, "roasted_meat")
			stats, _ := ecs.GetComponent[components.EntityStats](f.world, f.player)
			require.Equal(t, float64(490), stats.Stamina)
			station, _ := ecs.GetComponent[components.StationState](f.world, f.station)
			require.Equal(t, uint32(2), station.Resources["fuel"])
		})
	}
}

func TestRoastFinalFailuresPreserveInput(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(*roastFixture)
	}{
		{"unlit", "CRAFT_STATION_REQUIREMENTS_NOT_MET", func(f *roastFixture) {
			ecs.MutateComponent[components.StationState](f.world, f.station, func(s *components.StationState) bool { s.CurrentState = "unlit"; return true })
		}},
		{"missing mapping", "CRAFT_ROAST_MAP_ENTRY_MISSING", func(f *roastFixture) { delete(f.recipe.OutputByInputKey, "rabbit_meat") }},
		{"invalid target", "CRAFT_OUTPUT_INVALID", func(f *roastFixture) { f.recipe.OutputByInputKey["rabbit_meat"] = "absent" }},
		{"unsupported quality", "CRAFT_QUALITY_FORMULA_UNSUPPORTED", func(f *roastFixture) { f.recipe.QualityFormula = "unsupported" }},
		{"stamina", "LOW_STAMINA", func(f *roastFixture) {
			ecs.MutateComponent[components.EntityStats](f.world, f.player, func(s *components.EntityStats) bool { s.Stamina = 0; return true })
		}},
		{"space", "CRAFT_NO_SPACE", func(f *roastFixture) {
			target, _ := itemdefs.Global().GetByKey("roasted_rabbit_meat")
			target.Size.W = 5
		}},
		{"give failure", "CRAFT_OUTPUT_CREATION_FAILED", func(f *roastFixture) {
			f.service.invExec = inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
		}},
		{"unlink", "", func(f *roastFixture) { ecs.GetResource[ecs.LinkState](f.world).RemoveLink(f.playerID) }},
		{"switch link", "", func(f *roastFixture) {
			other := f.world.Spawn(804, nil)
			ecs.GetResource[ecs.LinkState](f.world).SetLink(ecs.PlayerLink{PlayerID: f.playerID, PlayerHandle: f.player, TargetID: 804, TargetHandle: other})
		}},
		{"station removed", "", func(f *roastFixture) { f.world.Despawn(f.station) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupRoastFixture(t, "rabbit_meat")
			action := f.start(t, 1)
			tc.change(f)
			before := append([]components.InvItem(nil), f.items()...)
			statsBefore, _ := ecs.GetComponent[components.EntityStats](f.world, f.player)
			require.Equal(t, contracts.BehaviorCycleDecisionCanceled, f.complete(action))
			require.Equal(t, before, f.items())
			stats, _ := ecs.GetComponent[components.EntityStats](f.world, f.player)
			require.Equal(t, statsBefore, stats)
			profile, _ := ecs.GetComponent[components.CharacterProfile](f.world, f.player)
			require.Empty(t, profile.Discovery)
			if f.world.Alive(f.station) {
				station, _ := ecs.GetComponent[components.StationState](f.world, f.station)
				require.Equal(t, uint32(2), station.Resources["fuel"])
			}
			if tc.code != "" {
				require.NotEmpty(t, f.sender.alerts)
				require.Equal(t, tc.code, f.sender.alerts[len(f.sender.alerts)-1].ReasonCode)
			}
			if tc.name == "missing mapping" {
				alert := f.sender.alerts[len(f.sender.alerts)-1]
				require.Equal(t, "Roast can't be processed: no info rabbit_meat in roast map", alert.GetMessage())
				encoded, err := proto.Marshal(alert)
				require.NoError(t, err)
				var decoded netproto.S2C_MiniAlert
				require.NoError(t, proto.Unmarshal(encoded, &decoded))
				require.Equal(t, alert.GetMessage(), decoded.GetMessage())
			}
		})
	}
}

func TestRoastAvailabilityAndStartUseMappedFit(t *testing.T) {
	for _, sourceKey := range []string{"beef", "raw_pork"} {
		for _, targetFits := range []bool{true, false} {
			t.Run(sourceKey+map[bool]string{true: " mapped fits", false: " mapped too large"}[targetFits], func(t *testing.T) {
				f := setupRoastFixture(t, sourceKey)
				target, _ := itemdefs.Global().GetByKey(f.recipe.OutputByInputKey[sourceKey])
				preview, _ := itemdefs.Global().GetByKey("roasted_meat")
				if targetFits {
					preview.Size.W = 5
				} else {
					target.Size.W = 5
				}
				entries := f.service.buildCraftList(f.world, f.playerID, f.player)
				require.Len(t, entries, 1)
				require.Equal(t, targetFits, entries[0].Flags.HasOutputSpace)
				require.Equal(t, targetFits, entries[0].Flags.CanStartNow)
				require.Equal(t, "raw_meat", entries[0].Inputs[0].GetItemTag())
				require.Equal(t, "roasted_meat", entries[0].Outputs[0].ItemKey)
				f.service.HandleStartCraftOne(f.world, f.playerID, f.player, &netproto.C2S_StartCraftOne{CraftKey: f.recipe.Key})
				_, active := ecs.GetComponent[components.ActiveCraft](f.world, f.player)
				require.Equal(t, targetFits, active)
				require.Len(t, f.items(), 1)
				require.Equal(t, types.EntityID(900), f.items()[0].ItemID)
			})
		}
	}
	t.Run("missing map has distinct start error", func(t *testing.T) {
		f := setupRoastFixture(t, "rabbit_meat")
		delete(f.recipe.OutputByInputKey, "rabbit_meat")
		entries := f.service.buildCraftList(f.world, f.playerID, f.player)
		require.True(t, entries[0].Flags.HasInputs)
		require.False(t, entries[0].Flags.CanStartNow)
		require.Empty(t, f.sender.alerts)
		f.service.HandleStartCraftOne(f.world, f.playerID, f.player, &netproto.C2S_StartCraftOne{CraftKey: f.recipe.Key})
		require.Len(t, f.sender.alerts, 1)
		require.Equal(t, "CRAFT_ROAST_MAP_ENTRY_MISSING", f.sender.alerts[0].ReasonCode)
		require.Equal(t, "Roast can't be processed: no info rabbit_meat in roast map", f.sender.alerts[0].GetMessage())
		require.Len(t, f.items(), 1)
	})
}

func TestRoastUsesCurrentSourceAcrossCycles(t *testing.T) {
	f := setupRoastFixture(t, "beef")
	action := f.start(t, 2)
	pork, _ := itemdefs.Global().GetByKey("raw_pork")
	ecs.MutateComponent[components.InventoryContainer](f.world, f.root, func(c *components.InventoryContainer) bool {
		c.Items = append([]components.InvItem{{ItemID: 901, TypeID: uint32(pork.DefID), Quantity: 1, Quality: 61, W: 1, H: 1, X: 1}}, c.Items...)
		c.Version++
		return true
	})
	require.Equal(t, contracts.BehaviorCycleDecisionContinue, f.complete(action))
	roastPork, _ := itemdefs.Global().GetByKey("roast_pork")
	require.Len(t, f.items(), 2)
	require.Equal(t, uint32(roastPork.DefID), f.items()[1].TypeID)
	require.Equal(t, uint32(61), f.items()[1].Quality)
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, f.complete(action))
	roastBeef, _ := itemdefs.Global().GetByKey("roasted_beef")
	require.Len(t, f.items(), 2)
	require.Equal(t, uint32(roastBeef.DefID), f.items()[1].TypeID)
	require.Equal(t, uint32(37), f.items()[1].Quality)
}

func TestRoastManyStopsOnExtinguishedStation(t *testing.T) {
	f := setupRoastFixture(t, "beef")
	ecs.MutateComponent[components.InventoryContainer](f.world, f.root, func(c *components.InventoryContainer) bool { c.Items[0].Quantity = 2; return true })
	action := f.start(t, 2)
	require.Equal(t, contracts.BehaviorCycleDecisionContinue, f.complete(action))
	before := append([]components.InvItem(nil), f.items()...)
	require.Len(t, before, 2)
	require.Equal(t, uint32(1), before[0].Quantity)
	ecs.MutateComponent[components.StationState](f.world, f.station, func(s *components.StationState) bool { s.CurrentState = "unlit"; return true })
	require.Equal(t, contracts.BehaviorCycleDecisionCanceled, f.complete(action))
	require.Equal(t, before, f.items())
}

func TestRoastStationLinksAndPortableCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed bool
		change  func(*roastFixture)
	}{
		{"campfire", true, func(*roastFixture) {}},
		{"different cooking station", true, func(f *roastFixture) {
			objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 9900, Key: "stove"}}))
			ecs.AddComponent(f.world, f.station, components.EntityInfo{TypeID: 9900})
		}},
		{"exact key retained", false, func(f *roastFixture) { f.recipe.RequiredLinkedObject = "another_station" }},
		{"missing link", false, func(f *roastFixture) { ecs.GetResource[ecs.LinkState](f.world).RemoveLink(f.playerID) }},
		{"wrong capability", false, func(f *roastFixture) {
			ecs.MutateComponent[components.StationState](f.world, f.station, func(s *components.StationState) bool { s.Capabilities = []string{"smithing"}; return true })
		}},
		{"unlit", false, func(f *roastFixture) {
			ecs.MutateComponent[components.StationState](f.world, f.station, func(s *components.StationState) bool { s.CurrentState = "unlit"; return true })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupRoastFixture(t, "beef")
			tc.change(f)
			entries := f.service.buildCraftList(f.world, f.playerID, f.player)
			require.Equal(t, tc.allowed, entries[0].Flags.CanStartNow)
			f.service.HandleStartCraftOne(f.world, f.playerID, f.player, &netproto.C2S_StartCraftOne{CraftKey: f.recipe.Key})
			_, active := ecs.GetComponent[components.ActiveCraft](f.world, f.player)
			require.Equal(t, tc.allowed, active)
		})
	}
	t.Run("portable fixed craft", func(t *testing.T) {
		f := setupRoastFixture(t, "beef")
		f.recipe.StationRequirements = nil
		f.recipe.OutputByInputKey = nil
		ecs.GetResource[ecs.LinkState](f.world).RemoveLink(f.playerID)
		f.service.HandleStartCraftOne(f.world, f.playerID, f.player, &netproto.C2S_StartCraftOne{CraftKey: f.recipe.Key})
		action, active := ecs.GetComponent[components.ActiveCyclicAction](f.world, f.player)
		require.True(t, active)
		require.Equal(t, components.CyclicActionTargetSelf, action.TargetKind)
		require.Equal(t, contracts.BehaviorCycleDecisionComplete, f.complete(action))
		preview, _ := itemdefs.Global().GetByKey("roasted_meat")
		require.Equal(t, uint32(preview.DefID), f.items()[0].TypeID)
	})
}

func TestRoastFuelExhaustionBeforeCompletionInSameTick(t *testing.T) {
	f := setupRoastFixture(t, "beef")
	f.recipe.TicksRequired = 1
	f.start(t, 1)
	ecs.MutateComponent[components.StationState](f.world, f.station, func(s *components.StationState) bool {
		s.Resources["fuel"] = 1
		s.AutonomousConsumption = []components.StationAutonomousConsumption{{ResourceKey: "fuel", AmountPerTick: 1, RequiredState: "burning", StateWhenDepleted: "unlit"}}
		return true
	})
	actions := NewContextActionService(f.world, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), zap.NewNop())
	actions.SetCraftingService(f.service)
	f.world.AddSystem(ecssystems.NewStationSystem(nil))
	f.world.AddSystem(NewCyclicActionSystem(actions, nil, zap.NewNop()))
	f.world.Update(0)
	require.Len(t, f.items(), 1)
	require.Equal(t, types.EntityID(900), f.items()[0].ItemID)
	stats, _ := ecs.GetComponent[components.EntityStats](f.world, f.player)
	require.Equal(t, float64(500), stats.Stamina)
	require.NotEmpty(t, f.sender.alerts)
	require.Equal(t, "CRAFT_STATION_REQUIREMENTS_NOT_MET", f.sender.alerts[len(f.sender.alerts)-1].ReasonCode)
}
