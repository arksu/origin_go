package game

import (
	"context"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newHearthCollisionTestShard(t *testing.T) (*Shard, *hearth.Runtime, types.Handle) {
	t.Helper()
	shard, _ := newPlayerSpawnTestShard(t, 16)
	runtime := ecs.InitResource(shard.world, hearth.NewRuntime(shard.world, shard.chunkManager, nil, nil, nil, nil, nil, nil))
	shard.objectCollisionFilter = runtime.IgnoreObjectCollision
	target := addHearthCollisionTestObject(shard, 200, 200, 200, []string{"hearth"})
	return shard, runtime, target
}

func addHearthCollisionTestObject(shard *Shard, id types.EntityID, x, y int, behaviors []string) types.Handle {
	handle := shard.world.Spawn(id, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 36, Behaviors: behaviors, Region: 1, IsStatic: true})
		ecs.AddComponent(w, handle, components.Transform{X: float64(x), Y: float64(y)})
		ecs.AddComponent(w, handle, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: constt.PlayerLayer, Mask: constt.PlayerMask})
		ecs.AddComponent(w, handle, components.ObjectInternalState{})
	})
	shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().AddStatic(handle, x, y)
	return handle
}

func seedHearthCollisionTestHome(t *testing.T, runtime *hearth.Runtime, owner types.EntityID, home *hearth.Home) {
	t.Helper()
	version, started := runtime.Index.BeginLookup(owner)
	require.True(t, started)
	require.NoError(t, runtime.Index.CompleteLookup(owner, version, home))
}

func TestHearthCollisionPlayerSpawn(t *testing.T) {
	for _, scenario := range []string{
		"owner", "foreign", "unknown", "no_home", "pending_terminal", "other_region",
		"campfire", "other_obstacle", "terrain", "admin_objects", "admin_terrain", "no_filter",
	} {
		t.Run(scenario, func(t *testing.T) {
			shard, runtime, target := newHearthCollisionTestShard(t)
			home := &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 200}, X: 200, Y: 200}
			switch scenario {
			case "unknown":
			case "no_home":
				seedHearthCollisionTestHome(t, runtime, 10, nil)
			case "foreign", "admin_objects", "admin_terrain":
				seedHearthCollisionTestHome(t, runtime, 11, home)
			case "other_region":
				home.Object.Region = 2
				seedHearthCollisionTestHome(t, runtime, 10, home)
			default:
				seedHearthCollisionTestHome(t, runtime, 10, home)
			}
			policy := SpawnCollisionPolicy{}
			switch scenario {
			case "pending_terminal":
				require.NoError(t, runtime.Index.MarkTerminalPending(10))
			case "campfire":
				require.True(t, gameworld.TransformObjectToDefInPlace(shard.world, 200, target, &objectdefs.ObjectDef{
					DefID: 37, Key: "campfire", HP: 100, IsStatic: true, BehaviorOrder: []string{"burner"},
					Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 12, H: 12, Layer: constt.PlayerLayer, Mask: constt.PlayerMask}},
				}, gameworld.TransformObjectInPlaceOptions{}))
				require.Equal(t, target, shard.world.GetHandleByEntityID(200))
			case "other_obstacle", "admin_objects":
				addHearthCollisionTestObject(shard, 300, 200, 200, nil)
			case "terrain", "admin_terrain":
				require.True(t, shard.chunkManager.GetChunk(types.ChunkCoord{}).SetTile(200/constt.CoordPerTile, 200/constt.CoordPerTile, types.TileDeepWater))
			case "no_filter":
				shard.objectCollisionFilter = nil
			}
			if scenario == "admin_objects" || scenario == "admin_terrain" {
				policy.IgnoreObjectCollision = true
			}
			require.NoError(t, shard.PrepareEntityAOI(context.Background(), 10, 200, 200))
			handle, err := shard.trySpawnPlayerWithPolicy(200, 200, repository.Character{ID: 10}, nil, policy)
			require.NoError(t, err)
			if scenario == "owner" || scenario == "admin_objects" {
				require.NotEqual(t, types.InvalidHandle, handle)
				require.True(t, shard.world.Alive(handle))
			} else {
				require.Equal(t, types.InvalidHandle, handle)
				require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))
			}
		})
	}
}

func TestHearthCollisionMovement(t *testing.T) {
	for _, scenario := range []string{"owner", "foreign", "owner_overlap", "foreign_overlap", "other_obstacle", "terrain", "phantom"} {
		t.Run(scenario, func(t *testing.T) {
			shard, runtime, _ := newHearthCollisionTestShard(t)
			seedHearthCollisionTestHome(t, runtime, 10, &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 200}, X: 200, Y: 200})
			actorID := types.EntityID(10)
			startX, endX := 160.0, 240.0
			if scenario == "foreign" || scenario == "foreign_overlap" {
				actorID = 11
			}
			if scenario == "owner_overlap" || scenario == "foreign_overlap" {
				startX, endX = 197, 202
			}
			collider := components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: constt.PlayerLayer, Mask: constt.PlayerMask}
			switch scenario {
			case "other_obstacle":
				addHearthCollisionTestObject(shard, 300, 220, 200, nil)
			case "terrain":
				require.True(t, shard.chunkManager.GetChunk(types.ChunkCoord{}).SetTile(17, 16, types.TileDeepWater))
			case "phantom":
				collider.Phantom = &components.PhantomCollider{WorldX: 200, WorldY: 200, HalfWidth: 6, HalfHeight: 6}
			}
			actor := shard.world.Spawn(actorID, func(w *ecs.World, handle types.Handle) {
				ecs.AddComponent(w, handle, components.Transform{X: startX, Y: 200})
				ecs.AddComponent(w, handle, collider)
				ecs.AddComponent(w, handle, components.ChunkRef{})
				ecs.AddComponent(w, handle, components.CollisionResult{})
			})
			shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().AddDynamic(actor, int(startX), 200)
			system := systems.NewCollisionSystem(shard.world, shard.chunkManager, zap.NewNop(), 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0)
			system.SetObjectCollisionFilter(shard.objectCollisionFilter)
			ecs.GetResource[ecs.MovedEntities](shard.world).Add(actor, endX, 200)
			shard.mu.Lock()
			system.Update(shard.world, 0.1)
			shard.mu.Unlock()
			result, ok := ecs.GetComponent[components.CollisionResult](shard.world, actor)
			require.True(t, ok)
			if scenario == "owner" || scenario == "owner_overlap" {
				require.False(t, result.HasCollision)
				require.Equal(t, endX, result.FinalX)
			} else {
				require.True(t, result.HasCollision)
				require.Less(t, result.FinalX, endX)
				switch scenario {
				case "foreign", "foreign_overlap":
					require.Equal(t, types.EntityID(200), result.CollidedWith)
				case "other_obstacle":
					require.Equal(t, types.EntityID(300), result.CollidedWith)
				case "phantom":
					require.True(t, result.IsPhantom)
				}
			}
		})
	}
}
