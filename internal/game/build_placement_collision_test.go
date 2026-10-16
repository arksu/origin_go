package game

import (
	"context"
	"strconv"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type buildPlacementCollisionFixture struct {
	*hearthBuildPlacementFixture
	ids *burnerOutcomeIDAllocator
}

func newBuildPlacementCollisionFixture(t *testing.T) *buildPlacementCollisionFixture {
	t.Helper()
	f := &buildPlacementCollisionFixture{hearthBuildPlacementFixture: newHearthBuildPlacementFixture(t), ids: &burnerOutcomeIDAllocator{next: 10000}}
	f.service.idAllocator = f.ids
	f.service.behaviorRegistry = behaviors.MustDefaultRegistry()
	ecs.AddComponent(f.shard.world, f.player, components.Transform{X: 184, Y: 200})
	ecs.AddComponent(f.shard.world, f.player, components.ChunkRef{})
	f.shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().AddDynamic(f.player, 184, 200)
	return f
}

func (f *buildPlacementCollisionFixture) request(t *testing.T, key string, x, y int) components.PendingBuildPlacement {
	t.Helper()
	f.service.HandleStartBuild(f.shard.world, 1, f.player, &netproto.C2S_BuildStart{
		BuildKey: key, Pos: &netproto.Vector2{X: int32(x), Y: int32(y)},
	})
	pending, _ := ecs.GetComponent[components.PendingBuildPlacement](f.shard.world, f.player)
	return pending
}

func (f *buildPlacementCollisionFixture) finalize() {
	w := f.shard.world
	ecs.AddComponent(w, f.player, components.CollisionResult{IsPhantom: true})
	systems.NewBuildPlacementSystem(w, f.service, nil).Update(w, 0)
}

func (f *buildPlacementCollisionFixture) assertRetired(t *testing.T) {
	t.Helper()
	w := f.shard.world
	require.False(t, ecs.HasComponent[components.PendingBuildPlacement](w, f.player))
	collider, found := ecs.GetComponent[components.Collider](w, f.player)
	require.True(t, found)
	require.Nil(t, collider.Phantom)
}

func (f *buildPlacementCollisionFixture) assertRejected(t *testing.T) {
	t.Helper()
	f.assertRetired(t)
	require.Len(t, f.sender.alerts, 1)
	require.Equal(t, "BUILD_PENDING_FAILED", f.sender.alerts[0].ReasonCode)
	require.Equal(t, types.EntityID(10000), f.ids.next, "refusal must precede ID allocation")
	require.Equal(t, types.InvalidHandle, f.shard.world.GetHandleByEntityID(10001))
}

func (f *buildPlacementCollisionFixture) addObject(t *testing.T, key string, id types.EntityID, x, y int) types.Handle {
	t.Helper()
	def, found := objectdefs.Global().GetByKey(key)
	require.True(t, found)
	handle := gameworld.SpawnEntityFromDef(f.shard.world, def, gameworld.DefSpawnParams{
		EntityID: id, X: float64(x), Y: float64(y), Region: 1, Layer: f.shard.world.Layer,
	})
	require.NotEqual(t, types.InvalidHandle, handle)
	coord := types.WorldToChunkCoord(x, y, constt.ChunkSize, constt.CoordPerTile)
	chunk := f.shard.chunkManager.GetChunk(coord)
	require.NotNil(t, chunk)
	chunk.Spatial().AddStatic(handle, x, y)
	return handle
}

func TestBuildPlacementRejectsHearthForOwnerAndForeign(t *testing.T) {
	for _, key := range []string{"box", "campfire", "hearth_fire"} {
		for _, owner := range []types.EntityID{1, 2} {
			for _, stage := range []string{"request", "final"} {
				t.Run(key+"/owner_"+strconv.Itoa(int(owner))+"/"+stage, func(t *testing.T) {
					f := newBuildPlacementCollisionFixture(t)
					w := f.shard.world
					if stage == "final" {
						require.Equal(t, key, f.request(t, key, 196, 200).BuildKey)
						require.Empty(t, f.sender.alerts)
					}
					f.addObject(t, "hearth_fire", 200, 200, 200)
					runtime := ecs.InitResource(w, hearth.NewRuntime(w, f.shard.chunkManager, nil, nil, nil, nil, nil, nil))
					seedHearthCollisionTestHome(t, runtime, owner, &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 200}, X: 200, Y: 200})
					before := w.EntityCount()
					if stage == "request" {
						f.request(t, key, 196, 200)
					} else {
						// The approach can hit its phantom before either player's body hits the hearth.
						ecs.AddComponent(w, f.player, components.CollisionResult{})
						collision := systems.NewCollisionSystem(w, f.shard.chunkManager, nil, 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0)
						collision.SetObjectCollisionFilter(runtime.IgnoreObjectCollision)
						ecs.GetResource[ecs.MovedEntities](w).Add(f.player, 188, 200)
						collision.Update(w, 0.1)
						result, found := ecs.GetComponent[components.CollisionResult](w, f.player)
						require.True(t, found)
						require.True(t, result.IsPhantom)
						systems.NewBuildPlacementSystem(w, f.service, nil).Update(w, 0)
					}
					f.assertRejected(t)
					require.Equal(t, before, w.EntityCount())
				})
			}
		}
	}
}

func TestBuildPlacementUsesFullFootprintAndExactContact(t *testing.T) {
	for _, tc := range []struct {
		name    string
		x, y    int
		blocked bool
	}{
		{"anchor outside", 209, 200, true},
		{"corner overlap", 209, 209, true},
		{"edge contact", 210, 200, false},
		{"corner contact", 210, 210, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuildPlacementCollisionFixture(t)
			f.addObject(t, "hearth_fire", 200, 200, 200)
			pending := f.request(t, "box", tc.x, tc.y)
			if tc.blocked {
				f.assertRejected(t)
				return
			}
			require.Equal(t, "box", pending.BuildKey)
			f.finalize()
			f.assertRetired(t)
			require.Empty(t, f.sender.alerts)
			site := f.shard.world.GetHandleByEntityID(10001)
			require.NotEqual(t, types.InvalidHandle, site)
			position, found := ecs.GetComponent[components.Transform](f.shard.world, site)
			require.True(t, found)
			require.Equal(t, float64(tc.x), position.X)
			require.Equal(t, float64(tc.y), position.Y)
		})
	}
}

func TestBuildPlacementRejectsOrdinaryCollidersOnEveryWorldLayer(t *testing.T) {
	for _, layer := range []int{-1, 0, 2} {
		for _, stage := range []string{"request", "final"} {
			t.Run(strconv.Itoa(layer)+"/"+stage, func(t *testing.T) {
				f := newBuildPlacementCollisionFixture(t)
				w := f.shard.world
				w.Layer = layer
				f.shard.chunkManager.GetChunk(types.ChunkCoord{}).Layer = layer
				if stage == "final" {
					require.Equal(t, "box", f.request(t, "box", 200, 200).BuildKey)
				}
				obstacle := spawnLiftPlacementCollider(w, 200, 200, 200, components.Collider{HalfWidth: 3, HalfHeight: 3, Layer: 1, Mask: 1})
				require.False(t, ecs.HasComponent[components.EntityInfo](w, obstacle))
				if stage == "request" {
					f.request(t, "box", 200, 200)
				} else {
					f.finalize()
				}
				f.assertRejected(t)
				require.Equal(t, 2, w.EntityCount())
			})
		}
	}
}

func TestBuildPlacementRechecksCurrentResultCollider(t *testing.T) {
	f := newBuildPlacementCollisionFixture(t)
	f.addObject(t, "hearth_fire", 200, 200, 200)
	pending := f.request(t, "box", 211, 200)
	require.Equal(t, "box", pending.BuildKey)
	require.Equal(t, float64(4), pending.PhantomHalfWidth)
	def, found := objectdefs.Global().GetByKey("box")
	require.True(t, found)
	def.Components.Collider.W = 20
	f.finalize()
	f.assertRejected(t)
}

func TestBuildPlacementUsesLiveColliderAfterConversionOrRemoval(t *testing.T) {
	for _, change := range []string{"campfire", "remove collider", "despawn"} {
		t.Run(change, func(t *testing.T) {
			f := newBuildPlacementCollisionFixture(t)
			require.Equal(t, "box", f.request(t, "box", 200, 200).BuildKey)
			handle := f.addObject(t, "hearth_fire", 200, 200, 200)
			switch change {
			case "campfire":
				def, found := objectdefs.Global().GetByKey("campfire")
				require.True(t, found)
				require.True(t, gameworld.TransformObjectToDefInPlace(f.shard.world, 200, handle, def, gameworld.TransformObjectInPlaceOptions{}))
			case "remove collider":
				ecs.RemoveComponent[components.Collider](f.shard.world, handle)
			case "despawn":
				require.True(t, f.shard.world.Despawn(handle))
			}
			f.finalize()
			if change == "campfire" {
				f.assertRejected(t)
				return
			}
			f.assertRetired(t)
			require.Empty(t, f.sender.alerts)
			require.Equal(t, types.EntityID(10001), f.ids.next)
			require.NotEqual(t, types.InvalidHandle, f.shard.world.GetHandleByEntityID(10001))
		})
	}
}

func TestBuildPlacementRejectedRequestPreservesPreviousPreview(t *testing.T) {
	for _, cause := range []string{"obstacle", "missing index"} {
		t.Run(cause, func(t *testing.T) {
			f := newBuildPlacementCollisionFixture(t)
			w := f.shard.world
			pending := f.request(t, "box", 230, 200)
			require.Equal(t, "box", pending.BuildKey)
			collider, _ := ecs.GetComponent[components.Collider](w, f.player)
			phantom := *collider.Phantom
			movement, _ := ecs.GetComponent[components.Movement](w, f.player)
			if cause == "obstacle" {
				f.addObject(t, "hearth_fire", 200, 200, 200)
			} else {
				ecs.SetResource(w, (*core.WorldColliderSpatial)(nil))
			}
			f.request(t, "campfire", 200, 200)
			current, found := ecs.GetComponent[components.PendingBuildPlacement](w, f.player)
			require.True(t, found)
			require.Equal(t, pending, current)
			collider, _ = ecs.GetComponent[components.Collider](w, f.player)
			require.Equal(t, phantom, *collider.Phantom)
			currentMovement, _ := ecs.GetComponent[components.Movement](w, f.player)
			require.Equal(t, movement, currentMovement)
			require.Len(t, f.sender.alerts, 1)
			require.Equal(t, "BUILD_PENDING_FAILED", f.sender.alerts[0].ReasonCode)
			require.Equal(t, types.EntityID(10000), f.ids.next)
		})
	}
}

func TestBuildPlacementRejectsMissingIndexAtFinalization(t *testing.T) {
	f := newBuildPlacementCollisionFixture(t)
	require.Equal(t, "box", f.request(t, "box", 200, 200).BuildKey)
	ecs.SetResource(f.shard.world, (*core.WorldColliderSpatial)(nil))
	f.finalize()
	f.assertRejected(t)
	require.Equal(t, 1, f.shard.world.EntityCount())
}

func TestBuildPlacementRejectsIncompleteSpatialQuery(t *testing.T) {
	loadHearthBuildRecipeForTest(t)
	shard, _ := newPlayerSpawnTestShard(t, 5000)
	w := shard.world
	shard.chunkManager.GetChunk(types.ChunkCoord{}).SetState(types.ChunkStateActive)
	spatial := core.AttachColliderSpatial(w)
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
	})
	ids, sender := &burnerOutcomeIDAllocator{next: 10000}, &lockedBuildSender{}
	f := &buildPlacementCollisionFixture{hearthBuildPlacementFixture: &hearthBuildPlacementFixture{
		shard: shard, player: player, sender: sender,
		service: &BuildService{world: w, chunkManager: shard.chunkManager, alerts: sender, idAllocator: ids, behaviorRegistry: behaviors.MustDefaultRegistry()},
	}, ids: ids}
	pending := f.request(t, "box", 230, 200)
	require.Equal(t, "box", pending.BuildKey)
	collider, _ := ecs.GetComponent[components.Collider](w, player)
	phantom := *collider.Phantom
	for i := 0; i <= placementMaxVisits; i++ {
		spawnLiftPlacementCollider(w, types.EntityID(i+100), 200, 200, components.Collider{HalfWidth: 1, HalfHeight: 1})
	}
	f.request(t, "box", 200, 200)
	current, found := ecs.GetComponent[components.PendingBuildPlacement](w, player)
	require.True(t, found)
	require.Equal(t, pending, current)
	collider, _ = ecs.GetComponent[components.Collider](w, player)
	require.Equal(t, phantom, *collider.Phantom)
	require.Len(t, sender.alerts, 1)
	require.Equal(t, "BUILD_PENDING_FAILED", sender.alerts[0].ReasonCode)
	require.Equal(t, placementMaxVisits, spatial.Index.LastQueryStats().Visits)
	// The overloaded region moves around the still-current accepted target.
	sender.alerts = nil
	for i := 0; i <= placementMaxVisits; i++ {
		ecs.WithComponent(w, w.GetHandleByEntityID(types.EntityID(i+100)), func(position *components.Transform) { position.X = 230 })
	}
	before := w.EntityCount()
	f.finalize()
	f.assertRejected(t)
	require.Equal(t, before, w.EntityCount())
	require.Equal(t, placementMaxVisits, spatial.Index.LastQueryStats().Visits)
}

func TestBuildPlacementFindsLoadedCrossChunkCollider(t *testing.T) {
	f := newBuildPlacementCollisionFixture(t)
	f.shard.cfg.Game.WorldWidthChunks = 2
	coord := types.ChunkCoord{X: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, f.shard.chunkManager.WaitPreloaded(ctx, coord))
	f.shard.chunkManager.GetChunk(coord).SetState(types.ChunkStateActive)
	handle := f.addObject(t, "box", 200, constt.ChunkWorldSize+10, 200)
	ecs.WithComponent(f.shard.world, handle, func(c *components.Collider) { c.HalfWidth = 18 })
	f.request(t, "box", constt.ChunkWorldSize-6, 200)
	f.assertRejected(t)
}

func TestBuildPlacementOnlyFirstOverlappingPreviewCreatesSite(t *testing.T) {
	f := newBuildPlacementCollisionFixture(t)
	w := f.shard.world
	second := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 220, Y: 200})
		ecs.AddComponent(w, h, components.Movement{})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
	})
	require.Equal(t, "box", f.request(t, "box", 200, 200).BuildKey)
	f.service.HandleStartBuild(w, 2, second, &netproto.C2S_BuildStart{BuildKey: "campfire", Pos: &netproto.Vector2{X: 200, Y: 200}})
	require.True(t, ecs.HasComponent[components.PendingBuildPlacement](w, second))
	require.Empty(t, f.sender.alerts)
	f.finalize()
	site := w.GetHandleByEntityID(10001)
	require.NotEqual(t, types.InvalidHandle, site)
	info, found := ecs.GetComponent[components.EntityInfo](w, site)
	require.True(t, found)
	require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
	ecs.AddComponent(w, second, components.CollisionResult{IsPhantom: true})
	systems.NewBuildPlacementSystem(w, f.service, nil).Update(w, 0)
	require.False(t, ecs.HasComponent[components.PendingBuildPlacement](w, second))
	collider, found := ecs.GetComponent[components.Collider](w, second)
	require.True(t, found)
	require.Nil(t, collider.Phantom)
	require.Len(t, f.sender.alerts, 1)
	require.Equal(t, "BUILD_PENDING_FAILED", f.sender.alerts[0].ReasonCode)
	require.Equal(t, types.EntityID(10001), f.ids.next)
	require.Equal(t, 3, w.EntityCount())
	f.sender.alerts = nil
	f.request(t, "box", 200, 200)
	f.assertRetired(t)
	require.Len(t, f.sender.alerts, 1)
	require.Equal(t, "BUILD_PENDING_FAILED", f.sender.alerts[0].ReasonCode)
	require.Equal(t, types.EntityID(10001), f.ids.next)
}

func TestBuildPlacementStaleFinalizationPreservesNewPreview(t *testing.T) {
	f := newBuildPlacementCollisionFixture(t)
	w := f.shard.world
	previous := f.request(t, "box", 200, 200)
	require.Equal(t, "box", previous.BuildKey)
	current := f.request(t, "campfire", 230, 200)
	require.Equal(t, "campfire", current.BuildKey)
	collider, _ := ecs.GetComponent[components.Collider](w, f.player)
	phantom := *collider.Phantom
	f.service.FinalizePendingBuildPlacement(w, 1, f.player, previous)
	after, found := ecs.GetComponent[components.PendingBuildPlacement](w, f.player)
	require.True(t, found)
	require.Equal(t, current, after)
	collider, _ = ecs.GetComponent[components.Collider](w, f.player)
	require.Equal(t, phantom, *collider.Phantom)
	require.Empty(t, f.sender.alerts)
	require.Equal(t, types.EntityID(10000), f.ids.next)
	require.Equal(t, 1, w.EntityCount())
}
