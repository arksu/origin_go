package game

import (
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func addHearthTransferNestedInventory(t *testing.T, f *hearthReturnTransferFixture, ownerID, bagID types.EntityID) {
	t.Helper()
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	w := f.source.world
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	root, found := refs.Lookup(constt.InventoryGrid, ownerID, 0)
	require.True(t, found)
	bag, found := itemdefs.Global().GetByKey("seed_bag")
	require.True(t, found)
	seed, found := itemdefs.Global().GetByKey("seed_wheat")
	require.True(t, found)
	nested, err := f.game.inventoryLoader.LoadPlayerInventories(w, bagID, []inventory.InventoryDataV1{{
		Kind: uint8(constt.InventoryGrid), Width: uint8(bag.Container.Size.W), Height: uint8(bag.Container.Size.H), Version: 19,
		Items: []inventory.InventoryItemV1{{ItemID: uint64(bagID + 1), TypeID: uint32(seed.DefID), Quality: 23, Quantity: 7, X: 2, Y: 3}},
	}})
	require.NoError(t, err)
	require.Empty(t, nested.Warnings)
	require.Len(t, nested.ContainerHandles, 1)
	refs.Add(constt.InventoryGrid, bagID, 0, nested.ContainerHandles[0])
	ecs.WithComponent(w, root, func(container *components.InventoryContainer) {
		container.Version = 11
		container.Items = []components.InvItem{{
			ItemID: bagID, TypeID: uint32(bag.DefID), Resource: bag.ResolveResource(true),
			Quality: 17, Quantity: 1, W: uint8(bag.Size.W), H: uint8(bag.Size.H), X: 1, Y: 1,
		}}
	})
	if ownerID == 10 {
		ecs.WithComponent(w, f.player, func(owner *components.InventoryOwner) {
			owner.Inventories = append(owner.Inventories, components.InventoryLink{
				Kind: constt.InventoryGrid, OwnerID: bagID, Handle: nested.ContainerHandles[0],
			})
		})
	}
}

func hearthTransferInventoryData(t *testing.T, shard *Shard) map[ecs.InventoryRefKey]inventory.InventoryDataV1 {
	t.Helper()
	shard.mu.Lock()
	defer shard.mu.Unlock()
	refs := ecs.GetResource[ecs.InventoryRefIndex](shard.world)
	data := make(map[ecs.InventoryRefKey]inventory.InventoryDataV1)
	for _, ownerID := range []types.EntityID{10, 3000, 7001, 7101} {
		entries := refs.EntriesByOwnerInto(ownerID, nil)
		require.NotEmpty(t, entries, "owner %d lost its inventories", ownerID)
		for _, ref := range entries {
			require.True(t, shard.world.Alive(ref.Handle))
			container, exists := ecs.GetComponent[components.InventoryContainer](shard.world, ref.Handle)
			require.True(t, exists)
			snapshot, err := inventory.SerializeInventoryTree(shard.world, container)
			require.NoError(t, err)
			data[ref.InventoryRefKey] = snapshot
		}
	}
	return data
}

func hearthTransferEntityCount(f *hearthReturnTransferFixture) int {
	count := 0
	for _, shard := range f.shards {
		shard.mu.Lock()
		count += shard.world.EntityCount()
		shard.mu.Unlock()
	}
	return count
}

func runHearthInventoryTransfer(
	t *testing.T,
	f *hearthReturnTransferFixture,
	req PlayerTransferRequest,
	rollback bool,
	wantData map[ecs.InventoryRefKey]inventory.InventoryDataV1,
) {
	t.Helper()
	source := f.shards[req.SourceLayer]
	oldRefs := make(map[ecs.InventoryRefKey]types.Handle, len(wantData))
	var refs *ecs.InventoryRefIndex
	func() {
		source.mu.Lock()
		defer source.mu.Unlock()
		refs = ecs.GetResource[ecs.InventoryRefIndex](source.world)
		for key := range wantData {
			handle, found := refs.Lookup(key.Kind, key.OwnerID, key.Key)
			require.True(t, found)
			oldRefs[key] = handle
		}
	}()

	// Check the source before target/rollback restoration can overwrite its
	// index entries. Same-layer restoration must also retire every old handle.
	spawnTarget, checkedDetach := req.SpawnTarget, false
	req.SpawnTarget = func(target *Shard, character repository.Character, runtime playerRuntimeState, finalize playerSpawnFinalizer) (types.Handle, repository.Character, error) {
		source.mu.Lock()
		for key, old := range oldRefs {
			assert.False(t, source.world.Alive(old), "old inventory remains alive: %+v", key)
			_, found := refs.Lookup(key.Kind, key.OwnerID, key.Key)
			assert.False(t, found, "source inventory reference remains after detach: %+v", key)
		}
		chunk := source.chunkManager.GetChunkFast(types.ChunkCoord{})
		assert.Nil(t, chunk.GetRawObjectByID(3000), "source raw cache must not reactivate the captured carried object")
		assert.Empty(t, chunk.GetRawInventoriesByOwner()[3000])
		source.mu.Unlock()
		checkedDetach = true
		if spawnTarget != nil {
			return spawnTarget(target, character, runtime, finalize)
		}
		handle, err := f.game.spawnTeleportedPlayerWithRuntime(f.client, target, character, req.TargetX, req.TargetY, req.IgnoreObjectCollision, &runtime, finalize)
		return handle, character, err
	}
	var resultErr error
	onResult := req.OnResult
	req.OnResult = func(shard *Shard, handle types.Handle, err error) {
		resultErr = err
		if onResult != nil {
			onResult(shard, handle, err)
		}
	}
	f.run(t, req)
	require.True(t, checkedDetach)
	resultLayer := req.TargetLayer
	if rollback {
		require.Error(t, resultErr)
		resultLayer = req.SourceLayer
	} else {
		require.NoError(t, resultErr)
	}
	require.Equal(t, resultLayer, f.client.Layer)
	assert.Equal(t, wantData, hearthTransferInventoryData(t, f.shards[resultLayer]), "all roots and nested contents must survive restoration")
	// The fixture has no game loop. Run ordinary chunk activation between
	// transfers, including any pending activation/retry work.
	for _, shard := range f.shards {
		shard.mu.Lock()
		shard.chunkManager.Update(ecs.GetResource[ecs.TimeState](shard.world).Delta)
		shard.mu.Unlock()
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	for key, old := range oldRefs {
		assert.False(t, source.world.Alive(old), "old inventory handle survived restoration: %+v", key)
		if resultLayer != req.SourceLayer {
			_, found := refs.Lookup(key.Kind, key.OwnerID, key.Key)
			assert.False(t, found, "cross-layer source still owns inventory reference: %+v", key)
		}
	}
}

func TestHearthReturnPostgresRepeatedTransfersRetireInventoryTrees(t *testing.T) {
	for _, sourceLayer := range []int{0, 1} {
		for _, rollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("layer_%d_rollback_%t", sourceLayer, rollback), func(t *testing.T) {
				f := newHearthReturnTransferFixture(t, sourceLayer, true, false)
				other := f.shards[1-sourceLayer]
				other.characterSaver = systems.NewCharacterSaver(f.db, 1, inventory.NewInventorySaver(f.game.logger), f.game.logger)
				t.Cleanup(other.characterSaver.Stop)
				addHearthTransferNestedInventory(t, f, 10, 7001)
				addHearthTransferNestedInventory(t, f, 3000, 7101)
				f.source.mu.Lock()
				carry, _ := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
				// Model the cache left by an earlier completed carry transfer,
				// including when the next transfer must roll back.
				NewLiftCarryTransferParticipant(f.game.logger).patchChunkCachesAfterTransfer(f.game, f.source,
					&liftCarryTransferState{ObjectEntityID: 3000}, carry.ObjectHandle)
				f.source.mu.Unlock()
				if rollback {
					home := f.runtime.Index.Snapshot(10).Home
					addHearthCollisionTestObject(f.shards[0], 4000, home.X, home.Y, nil)
				}
				wantData := hearthTransferInventoryData(t, f.source)
				var wantEntities int
				for attempt := 0; attempt < 3; attempt++ {
					for _, shard := range f.shards {
						shard.mu.Lock()
						// Expire the previous successful return's ordinary cooldown.
						ecs.GetResource[ecs.TimeState](shard.world).UnixMs = 10000 + int64(attempt)*360001
						shard.mu.Unlock()
					}
					f.source.mu.Lock()
					f.player = f.source.world.GetHandleByEntityID(10)
					f.source.mu.Unlock()
					runHearthInventoryTransfer(t, f, f.request(t), rollback, wantData)
					if !rollback {
						// Leave the Hearth through the ordinary transfer path so every
						// iteration repeats a return from the original position/layer.
						runHearthInventoryTransfer(t, f, PlayerTransferRequest{
							PlayerID: 10, SourceLayer: 0, TargetLayer: sourceLayer, TargetX: 200, TargetY: 200,
							Cause: PlayerTransferCauseAdminTeleport,
						}, false, wantData)
					}
					if attempt == 0 {
						// The first round trip also activates the destination's world
						// objects. Compare later rounds at the same lifecycle boundary.
						wantEntities = hearthTransferEntityCount(f)
					} else {
						assert.Equal(t, wantEntities, hearthTransferEntityCount(f), "repeated transfers must not accumulate orphan inventory entities")
					}
				}
			})
		}
	}
}
