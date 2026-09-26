package game

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/types"
)

type testPlowTerrain struct {
	chunk     *core.Chunk
	failWrite bool
}

func (terrain *testPlowTerrain) GetTileID(x, y int) (byte, bool) {
	if x < 0 || y < 0 || x >= 2 || y >= 2 {
		return 0, false
	}
	return terrain.chunk.SnapshotTiles().Tiles[y*2+x], true
}

func (terrain *testPlowTerrain) SetTile(x, y int, tile byte) bool {
	return !terrain.failWrite && terrain.chunk.SetTile(x, y, tile)
}

func newPlowTest(t *testing.T, tile byte, con int) (*ecs.World, types.Handle, *ActionService, *testPlowTerrain, *testActionSender) {
	t.Helper()
	registry, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := registry.Get("plow_tile")
	registry = actiondefs.NewRegistry([]actiondefs.Definition{*definition})
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 6, Y: 6})
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, Speed: constt.PlayerSpeed})
		ecs.AddComponent(w, h, components.CharacterProfile{Attributes: characterattrs.Values{characterattrs.CON: con}})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: entitystats.MaxStaminaFromCon(con)})
	})
	chunk := core.NewChunk(types.ChunkCoord{}, 0, 0, 2)
	if err := chunk.RestoreTiles([]byte{tile, types.TileGrass, types.TileGrass, types.TileGrass}, 0, 7); err != nil {
		t.Fatal(err)
	}
	terrain, sender := &testPlowTerrain{chunk: chunk}, &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"plow_tile": &plowTileActionHandler{terrain}}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "plow_tile")
	return world, player, service, terrain, sender
}

func finishPlow(world *ecs.World, player types.Handle, service *ActionService, sender *testActionSender) {
	for tick := 0; tick < 20; tick++ {
		if cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
			service.AdvanceCycle(world, 1, player, cycle, sender)
		}
	}
}

func TestPlowWhitelistExactCostAndRepeat(t *testing.T) {
	for _, tile := range []byte{types.TileConiferousForest, types.TileBroadleafForest, types.TileThicket, types.TileGrass} {
		for _, con := range []int{1, 20} {
			t.Run(fmt.Sprintf("tile=%d/CON=%d", tile, con), func(t *testing.T) {
				world, player, service, terrain, sender := newPlowTest(t, tile, con)
				// An object within the tile does not add an occupancy requirement.
				object := world.Spawn(2, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 1, Y: 1}) })
				service.HandleArmedClick(world, 1, player, 2, object, 1, 1)
				for tick := 0; tick < 19; tick++ {
					cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
					if !exists {
						t.Fatal("20-tick cycle ended early")
					}
					service.AdvanceCycle(world, 1, player, cycle, sender)
				}
				if terrain.chunk.SnapshotTiles().Version != 7 {
					t.Fatal("effect happened before tick 20")
				}
				finishPlow(world, player, service, sender)
				snapshot := terrain.chunk.SnapshotTiles()
				if snapshot.Version != 8 || snapshot.Tiles[0] != types.TilePlowed || snapshot.Tiles[1] != types.TileGrass || snapshot.Tiles[2] != types.TileGrass || snapshot.Tiles[3] != types.TileGrass {
					t.Fatalf("wrong effect: %#v", snapshot)
				}
				stats, _ := ecs.GetComponent[components.EntityStats](world, player)
				if stats.Stamina != entitystats.MaxStaminaFromCon(con)-250 {
					t.Fatalf("cost depends on max stamina: %v", stats.Stamina)
				}
				if service.State(world, player).Phase != "selecting" || service.State(world, player).Cursor != "dig" {
					t.Fatal("success disarmed plow")
				}
				service.HandleArmedClick(world, 1, player, 0, 0, 18, 6)
				if service.State(world, player).Phase != "approaching" {
					t.Fatal("next tile requires reactivation")
				}
			})
		}
	}
}

func TestPlowFailuresHaveNoPartialEffect(t *testing.T) {
	for _, failure := range []string{"plowed", "water", "missing", "changed on arrival", "changed on completion", "write failed", "lost affordability"} {
		t.Run(failure, func(t *testing.T) {
			world, player, service, terrain, sender := newPlowTest(t, types.TileGrass, 1)
			x := 6.0
			switch failure {
			case "plowed":
				terrain.chunk.SetTile(0, 0, types.TilePlowed)
			case "water":
				terrain.chunk.SetTile(0, 0, types.TileDeepWater)
			case "missing":
				x = 36
			case "changed on arrival":
				ecs.WithComponent(world, player, func(p *components.Transform) { p.X = 1 })
			case "write failed":
				terrain.failWrite = true
			}
			service.HandleArmedClick(world, 1, player, 0, 0, x, 6)
			if failure == "changed on arrival" || failure == "changed on completion" {
				terrain.chunk.SetTile(0, 0, types.TileSand)
			}
			if failure == "changed on arrival" {
				ecs.WithComponent(world, player, func(p *components.Transform) { p.X = 6 })
				if err := service.onPointMovementStopped(context.Background(), &ecs.PointMovementStoppedEvent{Layer: world.Layer, EntityID: 1, X: 6, Y: 6, TargetX: 6, TargetY: 6}); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "lost affordability" {
				ecs.WithComponent(world, player, func(s *components.EntityStats) { s.Stamina = 249 })
			}
			before := terrain.chunk.SnapshotTiles()
			statsBefore, _ := ecs.GetComponent[components.EntityStats](world, player)
			finishPlow(world, player, service, sender)
			after := terrain.chunk.SnapshotTiles()
			statsAfter, _ := ecs.GetComponent[components.EntityStats](world, player)
			if after.Version != before.Version || after.Tiles[0] != before.Tiles[0] || statsBefore.Stamina != statsAfter.Stamina {
				t.Fatal("failed plow had a partial effect")
			}
			if service.State(world, player).Phase != "selecting" || len(sender.alerts) == 0 {
				t.Fatal("failure did not alert and rearm")
			}
		})
	}
}
