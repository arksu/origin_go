package game

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"go.uber.org/zap"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/playerstate"
	"origin/internal/types"
)

type digGiveCall struct {
	itemKey string
	count   uint32
	quality uint32
}

func TestDigCycleStartedBeforeKnockoutCannotGiveItemsOrChargeStamina(t *testing.T) {
	world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
	service.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 6, 6)
	statsBefore, _ := ecs.GetComponent[components.EntityStats](world, player)
	ecs.AddComponent(world, player, components.EntityHealth{HHP: 20, SHP: 3, KOUntilUnixMs: 61000, IsLying: true})
	cycle, active := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	if !active || !cycle.MutatesItems {
		t.Fatal("dig was not marked as an item cycle")
	}
	service.AdvanceCycle(world, 1, player, cycle, sender)
	statsAfter, _ := ecs.GetComponent[components.EntityStats](world, player)
	if len(recorder.calls) != 0 || statsAfter != statsBefore || len(sender.alerts) == 0 || sender.alerts[len(sender.alerts)-1].ReasonCode != playerstate.ItemsLockedReason {
		t.Fatalf("dig committed during KO: grants=%v stats=%+v alerts=%v", recorder.calls, statsAfter, sender.alerts)
	}
	ecs.WithComponent(world, player, func(health *components.EntityHealth) {
		health.SHP = 5
		health.KOUntilUnixMs = 0
		health.IsLying = false
	})
	system := NewCyclicActionSystem(nil, sender, nil)
	system.SetActionService(service)
	system.Update(world, .1)
	if len(recorder.calls) != 0 {
		t.Fatal("rejected dig resumed after standing")
	}
}

type digGiveRecorder struct {
	outcomes []contracts.GiveItemOutcome
	calls    []digGiveCall
}

func (recorder *digGiveRecorder) give(_ *ecs.World, _ types.EntityID, _ types.Handle, itemKey string, count, quality uint32) contracts.GiveItemOutcome {
	recorder.calls = append(recorder.calls, digGiveCall{itemKey: itemKey, count: count, quality: quality})
	if len(recorder.outcomes) == 0 {
		return contracts.GiveItemOutcome{Success: true, GrantedCount: 1}
	}
	outcome := recorder.outcomes[0]
	recorder.outcomes = recorder.outcomes[1:]
	return outcome
}

func newDigTest(t *testing.T, tile byte) (*ecs.World, types.Handle, *ActionService, *testPlowTerrain, *testActionSender, *digGiveRecorder) {
	t.Helper()
	registry, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	definition, found := registry.Get("dig")
	if !found {
		t.Fatal("dig action missing")
	}
	registry = actiondefs.NewRegistry([]actiondefs.Definition{*definition})
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: 6, Y: 6})
		ecs.AddComponent(world, handle, components.Movement{Mode: constt.Walk, Speed: constt.PlayerSpeed})
		ecs.AddComponent(world, handle, components.CharacterProfile{Attributes: characterattrs.Values{characterattrs.CON: 1}})
		ecs.AddComponent(world, handle, components.EntityStats{Stamina: entitystats.MaxStaminaFromCon(1)})
	})
	chunk := core.NewChunk(types.ChunkCoord{}, 0, 0, 2)
	if err := chunk.RestoreTiles([]byte{tile, types.TileGrass, types.TileGrass, types.TileGrass}, 0, 7); err != nil {
		t.Fatal(err)
	}
	terrain, sender, recorder := &testPlowTerrain{chunk: chunk}, &testActionSender{}, &digGiveRecorder{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{
		"dig": &digTileActionHandler{terrain: terrain, giveItem: recorder.give},
	}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "dig")
	return world, player, service, terrain, sender, recorder
}

func advanceDigCycle(t *testing.T, world *ecs.World, player types.Handle, service *ActionService, sender *testActionSender) {
	t.Helper()
	for tick := 0; tick < 20; tick++ {
		cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		if !exists {
			t.Fatalf("cycle ended before tick %d", tick+1)
		}
		service.AdvanceCycle(world, 1, player, cycle, sender)
	}
}

func TestDigEligibleTerrainGivesMappedQ10ItemWithoutTileChange(t *testing.T) {
	for _, test := range []struct {
		tile byte
		item string
	}{
		{types.TileGrass, "soil"},
		{types.TileShallowWater, "clay"},
		{types.TileMountain, "stone"},
		{types.TileSand, "sand"},
	} {
		t.Run(test.item, func(t *testing.T) {
			world, player, service, terrain, sender, recorder := newDigTest(t, test.tile)
			object := world.Spawn(2, func(world *ecs.World, handle types.Handle) {
				ecs.AddComponent(world, handle, components.Transform{X: 1, Y: 1})
			})
			before := terrain.chunk.SnapshotTiles()
			service.HandleArmedClick(world, 1, player, 2, object, 1, 1)
			cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
			if !exists || cycle.TargetX != 6 || cycle.TargetY != 6 || cycle.CycleDurationTicks != 20 {
				t.Fatalf("click did not target tile center: %#v", cycle)
			}
			advanceDigCycle(t, world, player, service, sender)
			after := terrain.chunk.SnapshotTiles()
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			cycle, exists = ecs.GetComponent[components.ActiveCyclicAction](world, player)
			if len(recorder.calls) != 1 || recorder.calls[0] != (digGiveCall{test.item, 1, 10}) ||
				stats.Stamina != 700 || !exists || cycle.CycleIndex != 2 || service.State(world, player).Phase != "executing" {
				t.Fatalf("wrong dig result: calls=%#v stamina=%v cycle=%#v", recorder.calls, stats.Stamina, cycle)
			}
			if after.Version != before.Version || !slices.Equal(after.Tiles, before.Tiles) {
				t.Fatalf("dig changed terrain: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestDigRejectsIneligibleAndUnloadedTiles(t *testing.T) {
	for _, test := range []struct {
		name string
		tile byte
		x    float64
		want string
	}{
		{"deep water", types.TileDeepWater, 1, "BAD_TERRAIN"},
		{"plowed", types.TilePlowed, 1, "BAD_TERRAIN"},
		{"unloaded", types.TileGrass, 36, "ACTION_INVALID_TARGET"},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, player, service, _, sender, recorder := newDigTest(t, test.tile)
			service.HandleArmedClick(world, 1, player, 0, 0, test.x, 1)
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != test.want ||
				len(recorder.calls) != 0 || stats.Stamina != 1000 || service.State(world, player).Phase != "selecting" {
				t.Fatalf("invalid tile changed action: alerts=%#v calls=%#v state=%#v", sender.alerts, recorder.calls, service.State(world, player))
			}
			if _, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
				t.Fatal("rejected tile began a cycle")
			}
			movement, _ := ecs.GetComponent[components.Movement](world, player)
			if movement.TargetType != constt.TargetNone {
				t.Fatal("rejected tile began an approach")
			}
		})
	}
}

func TestDigReadsCurrentTerrainOnEveryCycle(t *testing.T) {
	world, player, service, terrain, sender, recorder := newDigTest(t, types.TileGrass)
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, sender)
	if !terrain.chunk.SetTile(0, 0, types.TileShallowWater) {
		t.Fatal("failed to change tile externally")
	}
	versionAfterExternalChange := terrain.chunk.SnapshotTiles().Version
	advanceDigCycle(t, world, player, service, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	if !slices.Equal(recorder.calls, []digGiveCall{{"soil", 1, 10}, {"clay", 1, 10}}) ||
		stats.Stamina != 400 || !exists || cycle.CycleIndex != 3 || terrain.chunk.SnapshotTiles().Version != versionAfterExternalChange {
		t.Fatalf("dig did not use current terrain: calls=%#v stamina=%v cycle=%#v", recorder.calls, stats.Stamina, cycle)
	}
}

func TestDigStopsAfterHandGrantAndRequiresNewClick(t *testing.T) {
	world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
	recorder.outcomes = []contracts.GiveItemOutcome{
		{Success: true, GrantedCount: 1},
		{Success: true, GrantedCount: 1, PlacedInHand: true},
	}
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, sender)
	advanceDigCycle(t, world, player, service, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if len(recorder.calls) != 2 || stats.Stamina != 400 || service.State(world, player).Phase != "idle" || len(sender.finished) != 1 {
		t.Fatalf("hand grant did not stop charged cycle: calls=%#v stamina=%v state=%#v", recorder.calls, stats.Stamina, service.State(world, player))
	}
	if _, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
		t.Fatal("hand grant began a successor cycle")
	}
	service.Activate(world, 1, player, "dig")
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, sender)
	stats, _ = ecs.GetComponent[components.EntityStats](world, player)
	if len(recorder.calls) != 3 || stats.Stamina != 100 {
		t.Fatalf("new click did not start new dig: calls=%#v stamina=%v", recorder.calls, stats.Stamina)
	}
}

func TestDigFailureAndLostStaminaHaveNoRewardOrActionCost(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome contracts.GiveItemOutcome
		stamina float64
	}{
		{"full inventory and hand", contracts.GiveItemOutcome{Message: "no free space"}, 1000},
		{"failed grant", contracts.GiveItemOutcome{Success: true, GrantedCount: 0}, 1000},
		{"stamina spent during cycle", contracts.GiveItemOutcome{Success: true, GrantedCount: 1}, 299},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
			recorder.outcomes = []contracts.GiveItemOutcome{test.outcome}
			service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
			ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = test.stamina })
			if test.stamina < 300 {
				cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
				service.AdvanceCycle(world, 1, player, cycle, sender)
			} else {
				advanceDigCycle(t, world, player, service, sender)
			}
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			wantCalls := 1
			if test.stamina < 300 {
				wantCalls = 0
			}
			if len(recorder.calls) != wantCalls || stats.Stamina != test.stamina || service.State(world, player).Phase != "idle" || len(sender.alerts) == 0 {
				t.Fatalf("failed dig had effect or cost: calls=%#v stamina=%v state=%#v alerts=%#v", recorder.calls, stats.Stamina, service.State(world, player), sender.alerts)
			}
		})
	}
}

func TestDigStopsWhenTerrainOrPositionChanges(t *testing.T) {
	for _, failure := range []string{"terrain", "position"} {
		t.Run(failure, func(t *testing.T) {
			world, player, service, terrain, sender, recorder := newDigTest(t, types.TileGrass)
			service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
			if failure == "terrain" {
				terrain.chunk.SetTile(0, 0, types.TileDeepWater)
			} else {
				ecs.WithComponent(world, player, func(transform *components.Transform) { transform.X = 30 })
			}
			advanceDigCycle(t, world, player, service, sender)
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if len(recorder.calls) != 0 || stats.Stamina != 1000 || service.State(world, player).Phase != "idle" || len(sender.alerts) == 0 {
				t.Fatalf("changed target had effect: calls=%#v stamina=%v state=%#v", recorder.calls, stats.Stamina, service.State(world, player))
			}
		})
	}
}

func TestDigStopsUnfinishedRepeatWhenTargetChanges(t *testing.T) {
	for _, failure := range []string{"terrain", "position"} {
		t.Run(failure, func(t *testing.T) {
			world, player, service, terrain, sender, recorder := newDigTest(t, types.TileGrass)
			service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
			advanceDigCycle(t, world, player, service, sender)
			if failure == "terrain" {
				terrain.chunk.SetTile(0, 0, types.TileDeepWater)
			} else {
				ecs.WithComponent(world, player, func(transform *components.Transform) { transform.X = 30 })
			}
			advanceDigCycle(t, world, player, service, sender)
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if len(recorder.calls) != 1 || stats.Stamina != 700 || service.State(world, player).Phase != "idle" {
				t.Fatalf("unfinished repeat had an effect: calls=%#v stamina=%v state=%#v", recorder.calls, stats.Stamina, service.State(world, player))
			}
		})
	}
}

func TestDigApproachAndCancellation(t *testing.T) {
	for _, behavior := range []string{"blocked", "arrived", "cancel", "timeout"} {
		t.Run(behavior, func(t *testing.T) {
			world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
			service.HandleArmedClick(world, 1, player, 0, 0, 13, 6)
			if service.State(world, player).Phase != "approaching" {
				t.Fatal("dig did not approach target tile")
			}
			if _, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
				t.Fatal("cycle began before arrival")
			}
			if behavior == "cancel" {
				service.Cancel(world, 1, player)
			} else if behavior == "timeout" {
				active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
				ecs.GetResource[ecs.TimeState](world).UnixMs = active.ExpireAtUnixMs
				service.Recheck(world, 1, player)
			} else {
				position := float64(6)
				if behavior == "arrived" {
					position = 18
					ecs.WithComponent(world, player, func(transform *components.Transform) { transform.X = 18 })
				}
				if err := service.onPointMovementStopped(context.Background(), &ecs.PointMovementStoppedEvent{
					Layer: world.Layer, EntityID: 1, X: position, Y: 6, TargetX: 18, TargetY: 6,
				}); err != nil {
					t.Fatal(err)
				}
			}
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if behavior == "arrived" {
				if service.State(world, player).Phase != "executing" {
					t.Fatal("arrival did not start cycle")
				}
				advanceDigCycle(t, world, player, service, sender)
				if len(recorder.calls) != 1 {
					t.Fatal("arrival did not grant item")
				}
			} else if len(recorder.calls) != 0 || stats.Stamina != 1000 || service.State(world, player).Phase != "idle" {
				t.Fatalf("blocked or canceled approach had effect: calls=%#v stamina=%v state=%#v", recorder.calls, stats.Stamina, service.State(world, player))
			}
		})
	}
}

func TestDigLowStaminaBeforeApproachAndCancelDuringCycle(t *testing.T) {
	world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
	ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = 299 })
	service.HandleArmedClick(world, 1, player, 0, 0, 13, 6)
	if service.State(world, player).Phase != "idle" || len(sender.alerts) == 0 || sender.alerts[0].ReasonCode != "LOW_STAMINA" {
		t.Fatalf("low stamina began approach: state=%#v alerts=%#v", service.State(world, player), sender.alerts)
	}
	ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = 1000 })
	service.Activate(world, 1, player, "dig")
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	service.AdvanceCycle(world, 1, player, cycle, sender)
	service.Cancel(world, 1, player)
	service.AdvanceCycle(world, 1, player, cycle, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if len(recorder.calls) != 0 || stats.Stamina != 1000 || service.State(world, player).Phase != "idle" {
		t.Fatalf("canceled cycle granted or charged: calls=%#v stamina=%v state=%#v", recorder.calls, stats.Stamina, service.State(world, player))
	}
}

func TestDigStopsWhenNextCycleIsUnaffordable(t *testing.T) {
	world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
	ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = 500 })
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if len(recorder.calls) != 1 || stats.Stamina != 200 || service.State(world, player).Phase != "idle" {
		t.Fatalf("unaffordable repeat did not preserve first item/cost: calls=%#v stamina=%v state=%#v", recorder.calls, stats.Stamina, service.State(world, player))
	}
	if _, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
		t.Fatal("unaffordable repeat began another cycle")
	}
}

func TestDigCostIsIndependentOfMaximumStamina(t *testing.T) {
	world, player, service, _, sender, recorder := newDigTest(t, types.TileGrass)
	ecs.WithComponent(world, player, func(profile *components.CharacterProfile) {
		profile.Attributes[characterattrs.CON] = 20
	})
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	advanceDigCycle(t, world, player, service, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if len(recorder.calls) != 1 || stats.Stamina != 700 {
		t.Fatalf("dig cost changed with CON: calls=%#v stamina=%v", recorder.calls, stats.Stamina)
	}
}
