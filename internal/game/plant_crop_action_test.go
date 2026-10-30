package game

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type plantingTerrain struct{ *testPlowTerrain }

func (p plantingTerrain) GetTileID(x, y int) (byte, bool) {
	return p.chunk.TileID(x, y, 2)
}

func (p plantingTerrain) GetChunkFast(coord types.ChunkCoord) *core.Chunk {
	if coord != p.chunk.Coord {
		return nil
	}
	return p.chunk
}

type plantingIDs struct{ last types.EntityID }

func (ids *plantingIDs) GetFreeID() types.EntityID { ids.last++; return ids.last }

type plantingFixture struct {
	w       *ecs.World
	player  types.Handle
	hand    types.Handle
	handler *plantCropActionHandler
	actions *ActionService
	sender  *testActionSender
	terrain plantingTerrain
	def     *objectdefs.ObjectDef
	notices int
}

func newPlantingFixture(t testing.TB, capacity uint32) *plantingFixture {
	t.Helper()
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems); objectdefs.SetGlobalForTesting(previousObjects) })
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	registry := behaviors.MustDefaultRegistry()
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), registry, zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	w := ecs.NewWorldWithCapacity(capacity, nil, 0)
	core.AttachColliderSpatial(w)
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 100
	f := &plantingFixture{w: w, sender: &testActionSender{}}
	f.def, _ = objects.GetByKey("crop_carrot")
	f.player = w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 6, Y: 6})
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, Speed: constt.PlayerSpeed})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 30})
	})
	f.hand = w.SpawnWithoutExternalID()
	seed, _ := items.GetByKey("seed_carrot")
	ecs.AddComponent(w, f.hand, components.InventoryContainer{
		OwnerID: 1, Kind: constt.InventoryHand, Version: 1, HandMouseOffsetX: 4, HandMouseOffsetY: 5,
		Items: []components.InvItem{{ItemID: 50, TypeID: uint32(seed.DefID), Quantity: 1, Quality: 10, W: 1, H: 1}},
	})
	ecs.AddComponent(w, f.player, components.InventoryOwner{Inventories: []components.InventoryLink{{OwnerID: 1, Kind: constt.InventoryHand, Handle: f.hand}}})
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 2)
	chunk.SetState(types.ChunkStateActive)
	require.NoError(t, chunk.RestoreTiles([]byte{types.TilePlowed, types.TilePlowed, types.TileGrass, types.TileGrass}, 0, 1))
	f.terrain = plantingTerrain{&testPlowTerrain{chunk: chunk}}
	f.handler, err = newPlantCropActionHandler(w, f.terrain, &plantingIDs{last: 1000}, registry, objects, items, 2,
		func(*ecs.World, types.EntityID, types.Handle) { f.notices++ }, f.sender)
	require.NoError(t, err)
	f.handler.randomInt64 = func(int64) int64 { return 0 }
	defs, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	require.NoError(t, err)
	plant, _ := defs.Get("plant")
	f.actions, err = NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{*plant}), map[string]ActionHandler{"plant": f.handler}, f.sender)
	require.NoError(t, err)
	f.handler.actions = f.actions
	return f
}

func (f *plantingFixture) finish() {
	for i := 0; i < 10; i++ {
		if cycle, found := ecs.GetComponent[components.ActiveCyclicAction](f.w, f.player); found {
			f.actions.AdvanceCycle(f.w, 1, f.player, cycle, f.sender)
		}
	}
}

func TestPlantingCommitsSeedCostQualityAndPersistedGrowth(t *testing.T) {
	f := newPlantingFixture(t, 16)
	f.handler.StartPlanting(f.w, 1, f.player, 1, 1)
	require.Equal(t, "executing", f.actions.State(f.w, f.player).Phase)
	f.finish()
	require.Equal(t, "idle", f.actions.State(f.w, f.player).Phase)
	hand, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
	require.Empty(t, hand.Items)
	require.Zero(t, hand.HandMouseOffsetX)
	require.Zero(t, hand.HandMouseOffsetY)
	require.Equal(t, uint64(2), hand.Version)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, 20.0, stats.Stamina)
	require.Equal(t, 1, f.notices)
	require.Empty(t, f.handler.seeds)
	crop := f.w.GetHandleByEntityID(1001)
	require.True(t, f.w.Alive(crop))
	info, _ := ecs.GetComponent[components.EntityInfo](f.w, crop)
	require.Equal(t, uint32(8), info.Quality)
	require.Equal(t, &components.CropBehaviorState{Stage: 0, NextGrowthRuntimeSeconds: 14500}, behaviors.CropState(f.w, crop))
	appearance, _ := ecs.GetComponent[components.Appearance](f.w, crop)
	require.Equal(t, "wild_windsown_weed/0", appearance.Resource)
	factory := gameworld.NewObjectFactory(nil)
	raw, err := factory.Serialize(f.w, crop)
	require.NoError(t, err)
	restored := ecs.NewWorldForTesting()
	copy, err := factory.Build(restored, raw, nil)
	require.NoError(t, err)
	state, err := factory.DeserializeObjectState(raw)
	require.NoError(t, err)
	ecs.WithComponent(restored, copy, func(internal *components.ObjectInternalState) { internal.State = state })
	require.NoError(t, f.handler.registry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
		World: restored, Handle: copy, EntityID: 1001, EntityType: uint32(f.def.DefID), Reason: contracts.ObjectBehaviorInitReasonRestore,
	}, f.def.BehaviorOrder))
	require.Equal(t, behaviors.CropState(f.w, crop), behaviors.CropState(restored, copy), "typed growth state survives save/reload")
	f.finish()
	stats, _ = ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, 20.0, stats.Stamina, "late completion cannot pay twice")
}

func TestPlantingRandomMissIsCompletedPaidAttempt(t *testing.T) {
	f := newPlantingFixture(t, 16)
	chance := 0.0
	f.def.CropConfig.PlantingSuccessChance = &chance
	f.handler.randomInt64 = func(int64) int64 { t.Fatal("a failed planting must not draw quality"); return 0 }
	f.handler.StartPlanting(f.w, 1, f.player, 6, 6)
	f.finish()
	hand, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
	require.Empty(t, hand.Items)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, 20.0, stats.Stamina)
	require.Equal(t, 2, f.w.EntityCount())
	require.Equal(t, "idle", f.actions.State(f.w, f.player).Phase)
	require.Len(t, f.sender.finished, 1)
	require.Equal(t, "PLANTING_MISSED", f.sender.alerts[len(f.sender.alerts)-1].ReasonCode)
}

func TestPlantingChanceBoundaryRequiresOnlyExactCost(t *testing.T) {
	for _, draw := range []float64{.499, .5} {
		f := newPlantingFixture(t, 16)
		chance := .5
		f.def.CropConfig.PlantingSuccessChance = &chance
		f.handler.random = func() float64 { return draw }
		ecs.WithComponent(f.w, f.player, func(stats *components.EntityStats) { stats.Stamina = 10 })
		f.handler.StartPlanting(f.w, 1, f.player, 6, 6)
		f.finish()
		stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
		require.Zero(t, stats.Stamina, "exact action cost needs no long-action percentage reserve")
		hand, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
		require.Empty(t, hand.Items)
		require.Equal(t, draw < chance, f.w.Alive(f.w.GetHandleByEntityID(1001)))
	}
}

func TestPlantingFailureAndCancellationKeepSeedAndStamina(t *testing.T) {
	for _, scenario := range []string{"cancel", "replacement", "terrain", "occupied", "stamina", "capacity", "unloaded"} {
		t.Run(scenario, func(t *testing.T) {
			capacity := uint32(16)
			if scenario == "capacity" {
				capacity = 2
			}
			f := newPlantingFixture(t, capacity)
			f.handler.StartPlanting(f.w, 1, f.player, 6, 6)
			switch scenario {
			case "cancel":
				f.actions.Cancel(f.w, 1, f.player)
			case "replacement":
				ecs.WithComponent(f.w, f.hand, func(hand *components.InventoryContainer) { hand.Items[0].ItemID = 51; hand.Version++ })
			case "terrain":
				require.True(t, f.terrain.SetTile(0, 0, types.TileGrass))
			case "occupied":
				f.w.Spawn(2000, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: 6, Y: 6})
					ecs.AddComponent(w, h, components.EntityInfo{IsStatic: true})
					ecs.AddComponent(w, h, components.Collider{HalfWidth: 2, HalfHeight: 2})
				})
			case "stamina":
				ecs.WithComponent(f.w, f.player, func(stats *components.EntityStats) { stats.Stamina = 9 })
			case "unloaded":
				f.terrain.chunk.SetState(types.ChunkStateInactive)
			}
			f.finish()
			hand, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
			require.Len(t, hand.Items, 1)
			stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
			if scenario == "stamina" {
				require.Equal(t, 9.0, stats.Stamina)
			} else {
				require.Equal(t, 30.0, stats.Stamina)
			}
			require.False(t, f.w.Alive(f.w.GetHandleByEntityID(1001)))
			require.Zero(t, f.notices)
		})
	}
}

func TestPlantingApproachRetainsOriginalSeed(t *testing.T) {
	f := newPlantingFixture(t, 16)
	ecs.WithComponent(f.w, f.player, func(position *components.Transform) { position.X = 1; position.Y = 1 })
	f.handler.StartPlanting(f.w, 1, f.player, 6, 6)
	require.Equal(t, "approaching", f.actions.State(f.w, f.player).Phase)
	ecs.WithComponent(f.w, f.hand, func(hand *components.InventoryContainer) { hand.Items[0].ItemID = 51; hand.Version++ })
	ecs.WithComponent(f.w, f.player, func(position *components.Transform) { position.X = 6; position.Y = 6 })
	require.NoError(t, f.actions.onPointMovementStopped(context.Background(), &ecs.PointMovementStoppedEvent{
		EntityID: 1, Layer: f.w.Layer, TargetX: 6, TargetY: 6, X: 6, Y: 6,
	}))
	f.finish()
	require.Equal(t, "idle", f.actions.State(f.w, f.player).Phase)
	require.Equal(t, "PLANT_SEED_CHANGED", f.sender.alerts[len(f.sender.alerts)-1].ReasonCode)
	require.Equal(t, 2, f.w.EntityCount())
}

func TestPlantingRejectsDuplicatePassableCrop(t *testing.T) {
	f := newPlantingFixture(t, 16)
	f.handler.StartPlanting(f.w, 1, f.player, 6, 6)
	f.finish()
	seed, _ := itemdefs.Global().GetByKey("seed_carrot")
	ecs.WithComponent(f.w, f.hand, func(hand *components.InventoryContainer) {
		hand.Items = []components.InvItem{{ItemID: 51, TypeID: uint32(seed.DefID), Quantity: 1, Quality: 10, W: 1, H: 1}}
		hand.Version++
	})
	f.handler.StartPlanting(f.w, 1, f.player, 6, 6)
	f.finish()
	require.Equal(t, "PLANT_TILE_OCCUPIED", f.sender.alerts[len(f.sender.alerts)-1].ReasonCode)
	require.Equal(t, 3, f.w.EntityCount())
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, 20.0, stats.Stamina)
}

func TestCropQualityDrawBoundaries(t *testing.T) {
	for _, test := range []struct {
		seed    uint32
		n, draw int64
		want    uint32
	}{
		{10, 2, 0, 8}, {10, 2, 4, 12}, {0, 2, 0, 0}, {math.MaxUint32, 2, 4, math.MaxUint32},
		{10, 0, 0, 10}, {0, math.MaxUint32, 2 * math.MaxUint32, math.MaxUint32},
	} {
		require.Equal(t, test.want, rollCropQuality(test.seed, test.n, func(limit int64) int64 {
			require.Equal(t, test.n*2+1, limit)
			return test.draw
		}))
	}
}

func TestPlantingRejectsAmbiguousSeedCatalog(t *testing.T) {
	f := newPlantingFixture(t, 16)
	duplicate := *f.def
	duplicate.DefID, duplicate.Key = 9991, "duplicate_carrot"
	_, err := newPlantCropActionHandler(f.w, f.terrain, &plantingIDs{}, f.handler.registry,
		objectdefs.NewRegistry([]objectdefs.ObjectDef{*f.def, duplicate}), itemdefs.Global(), 2, nil, nil)
	require.ErrorContains(t, err, "share seed")
}

func BenchmarkPlantingSiteValidation(b *testing.B) {
	f := newPlantingFixture(b, 16)
	target := ActionTarget{X: 6, Y: 6}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if reason := f.handler.ValidateTarget(f.w, 1, f.player, target); reason != "" {
			b.Fatal(reason)
		}
	}
}

var _ contracts.EntityIDAllocator = (*plantingIDs)(nil)
