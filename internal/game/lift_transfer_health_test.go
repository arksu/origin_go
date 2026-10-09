package game

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	liftHealthPlayerID   types.EntityID = 11001
	liftHealthObjectID   types.EntityID = 11002
	liftHealthItemID     types.EntityID = 11003
	liftHealthTypeID                    = 11004
	liftHealthItemTypeID                = 11005
)

func installLiftTransferHealthDefinitions(t *testing.T) {
	t.Helper()
	oldObjects, oldItems := objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(oldObjects)
		itemdefs.SetGlobalForTesting(oldItems)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: liftHealthTypeID, Key: "lift-health-fixture", HP: 100, IsStatic: true, Indestructible: true,
		BehaviorOrder: []string{"container", "lift"},
		Behaviors:     map[string]json.RawMessage{"container": json.RawMessage(`{}`), "lift": json.RawMessage(`{}`)},
		Components: &objectdefs.Components{
			Collider:  &objectdefs.ColliderDef{W: 4, H: 4, Layer: 1, Mask: 1},
			Inventory: []objectdefs.InventoryDef{{Kind: "grid", W: 2, H: 2}},
		},
	}}))
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID: liftHealthItemTypeID, Key: "lift-health-item", Resource: "fixture", Size: itemdefs.Size{W: 1, H: 1},
	}}))
}

func newLiftHealthTransferShard(t *testing.T, layer int, factory *gameworld.ObjectFactory) (*Shard, types.Handle) {
	t.Helper()
	cfg := &config.Config{Game: config.GameConfig{
		ChunkLRUCapacity: 2, ChunkLRUTTL: 60, LoadWorkers: 1,
		WorldWidthChunks: 1, WorldHeightChunks: 1,
	}}
	w := ecs.NewWorldForTesting()
	w.Layer = layer
	ecs.GetResource[ecs.TimeState](w).UnixMs = 123456
	player := w.Spawn(liftHealthPlayerID, nil)
	ecs.AddComponent(w, player, components.Transform{X: float64(10 + layer*20), Y: 10})
	cm := gameworld.NewChunkManager(cfg, nil, w, nil, layer, 1, factory, behaviors.MustDefaultRegistry(), nil, zap.NewNop())
	t.Cleanup(cm.Stop)
	cm.RegisterEntity(liftHealthPlayerID, 10, 10, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, cm.WaitPreloaded(ctx, types.ChunkCoord{}))
	cm.Update(0)
	chunk := cm.GetChunkFast(types.ChunkCoord{})
	require.NotNil(t, chunk)
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	shard := &Shard{world: w, cfg: cfg, layer: layer, chunkManager: cm, logger: zap.NewNop()}
	shard.liftService = NewLiftService(w, cm, nil, nil, zap.NewNop())
	return shard, player
}

func newLiftHealthTransferFixture(t *testing.T, db *persistence.Postgres, hp float64) (*Game, *Shard, *Shard, types.Handle, types.Handle, types.Handle) {
	t.Helper()
	installLiftTransferHealthDefinitions(t)
	factory := gameworld.NewObjectFactory(nil)
	source, sourcePlayer := newLiftHealthTransferShard(t, 0, factory)
	target, targetPlayer := newLiftHealthTransferShard(t, 1, factory)
	g := &Game{db: db, objectFactory: factory, shardManager: &ShardManager{shards: map[int]*Shard{0: source, 1: target}}}
	data, err := json.Marshal(inventory.InventoryDataV1{
		Kind: uint8(constt.InventoryGrid), Width: 2, Height: 2, Version: 7,
		Items: []inventory.InventoryItemV1{{ItemID: uint64(liftHealthItemID), TypeID: liftHealthItemTypeID, Quality: 17, Quantity: 1}},
	})
	require.NoError(t, err)
	snapshot := gameworld.EmbeddedObjectSnapshotV1{
		Version: 1, EntityID: uint64(liftHealthObjectID), TypeID: liftHealthTypeID, Region: 1, Quality: 23, HP: &hp,
		ObjectData:      json.RawMessage(`{"v":1,"behaviors":{"fixture":{"counter":7}}}`),
		RootInventories: []gameworld.EmbeddedInventorySnapshotV1{{Kind: int16(constt.InventoryGrid), Version: 7, Data: data}},
	}
	object, err := factory.SpawnWorldObjectFromSnapshot(source.world, snapshot, gameworld.SnapshotSpawnOptions{
		X: 10, Y: 10, ChunkManager: source.chunkManager, BehaviorRegistry: behaviors.MustDefaultRegistry(), Logger: zap.NewNop(),
	})
	require.NoError(t, err)
	result := source.liftService.StartLift(source.world, liftHealthPlayerID, sourcePlayer, liftHealthObjectID, object)
	require.Equal(t, ActionSucceeded, result.Outcome)
	return g, source, target, sourcePlayer, targetPlayer, object
}

func requireLiftHealthTransferState(t *testing.T, shard *Shard, player types.Handle, hp float64, original components.LiftedObjectState) types.Handle {
	t.Helper()
	carry, ok := ecs.GetComponent[components.LiftCarryState](shard.world, player)
	require.True(t, ok)
	require.Equal(t, liftHealthObjectID, carry.ObjectEntityID)
	require.Equal(t, int64(123456), carry.StartedAtUnixMs)
	require.True(t, shard.world.Alive(carry.ObjectHandle))
	info, ok := ecs.GetComponent[components.EntityInfo](shard.world, carry.ObjectHandle)
	require.True(t, ok)
	require.True(t, info.Indestructible)
	state, ok := ecs.GetComponent[components.ObjectInternalState](shard.world, carry.ObjectHandle)
	require.True(t, ok)
	require.True(t, state.HasHP)
	require.Equal(t, hp, state.HP)
	runtimeState, ok := components.GetRuntimeObjectState(state)
	require.True(t, ok)
	require.Equal(t, json.RawMessage(`{"counter":7}`), runtimeState.Behaviors["fixture"])
	lifted, ok := ecs.GetComponent[components.LiftedObjectState](shard.world, carry.ObjectHandle)
	require.True(t, ok)
	original.CarrierHandle = player
	require.Equal(t, original, lifted)
	root, found := ecs.GetResource[ecs.InventoryRefIndex](shard.world).Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
	require.True(t, found)
	container, ok := ecs.GetComponent[components.InventoryContainer](shard.world, root)
	require.True(t, ok)
	require.Equal(t, uint64(7), container.Version)
	require.Len(t, container.Items, 1)
	require.Equal(t, liftHealthItemID, container.Items[0].ItemID)
	require.Equal(t, uint32(17), container.Items[0].Quality)
	require.Equal(t, uint32(1), container.Items[0].Quantity)
	return carry.ObjectHandle
}

func TestLiftCarryTransferHealthTargetAndRollback(t *testing.T) {
	for _, mode := range []string{"target", "rollback"} {
		for _, hp := range []float64{.6, 0} {
			t.Run(mode+"_"+map[bool]string{true: "zero", false: "fractional"}[hp == 0], func(t *testing.T) {
				db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
				g, source, target, sourcePlayer, targetPlayer, object := newLiftHealthTransferFixture(t, db, hp)
				originalMeta, _ := ecs.GetComponent[components.LiftedObjectState](source.world, object)
				participant := NewLiftCarryTransferParticipant(zap.NewNop())
				req := PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}
				stateAny, err := participant.CaptureSource(g, source, req, sourcePlayer)
				require.NoError(t, err)
				require.False(t, source.world.Alive(object))
				require.False(t, ecs.HasComponent[components.LiftCarryState](source.world, sourcePlayer))
				state := stateAny.(*liftCarryTransferState)
				require.Equal(t, hp, *state.ObjectSnapshot.HP)
				if mode == "target" {
					require.NoError(t, participant.RestoreTarget(g, target, req, targetPlayer, stateAny))
					restored := requireLiftHealthTransferState(t, target, targetPlayer, hp, originalMeta)
					require.Contains(t, target.chunkManager.GetChunkFast(types.ChunkCoord{}).GetHandles(), restored)
					loaded, err := db.Queries().GetObjectByID(context.Background(), int64(liftHealthObjectID))
					require.NoError(t, err)
					require.True(t, loaded.Hp.Valid)
					require.Equal(t, hp, loaded.Hp.Float64)
					require.Equal(t, 1, loaded.Layer)
					rows, err := db.Queries().GetInventoriesByOwner(context.Background(), int64(liftHealthObjectID))
					require.NoError(t, err)
					require.Len(t, rows, 1)
					var persisted inventory.InventoryDataV1
					require.NoError(t, json.Unmarshal(rows[0].Data, &persisted))
					require.Equal(t, uint64(liftHealthItemID), persisted.Items[0].ItemID)
				} else {
					require.NoError(t, participant.RestoreSourceRollback(g, source, req, sourcePlayer, stateAny))
					restored := requireLiftHealthTransferState(t, source, sourcePlayer, hp, originalMeta)
					require.Contains(t, source.chunkManager.GetChunkFast(types.ChunkCoord{}).GetHandles(), restored)
				}
			})
		}
	}
}

func TestLiftCarryTransferIndestructibleRuntimeRestore(t *testing.T) {
	for _, mode := range []string{"target", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			g, source, target, sourcePlayer, targetPlayer, object := newLiftHealthTransferFixture(t, nil, .49)
			originalMeta, exists := ecs.GetComponent[components.LiftedObjectState](source.world, object)
			require.True(t, exists)
			participant := NewLiftCarryTransferParticipant(zap.NewNop())
			req := PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}
			stateAny, err := participant.CaptureSource(g, source, req, sourcePlayer)
			require.NoError(t, err)
			require.False(t, source.world.Alive(object))
			if mode == "target" {
				// Exercise the real target runtime restore; database durability is covered separately.
				state := stateAny.(*liftCarryTransferState)
				require.NoError(t, participant.restoreCarryToShard(g, target, req, targetPlayer, state, false))
				restored := requireLiftHealthTransferState(t, target, targetPlayer, .49, originalMeta)
				require.Contains(t, target.chunkManager.GetChunkFast(types.ChunkCoord{}).GetHandles(), restored)
			} else {
				require.NoError(t, participant.RestoreSourceRollback(g, source, req, sourcePlayer, stateAny))
				restored := requireLiftHealthTransferState(t, source, sourcePlayer, .49, originalMeta)
				require.Contains(t, source.chunkManager.GetChunkFast(types.ChunkCoord{}).GetHandles(), restored)
			}
		})
	}
}

func TestLiftCarryTransferInvalidHealthPreservesSourceUntilRepair(t *testing.T) {
	for _, failure := range []string{"missing", "negative", "nan", "positive_infinity", "negative_infinity"} {
		t.Run(failure, func(t *testing.T) {
			g, source, _, player, _, object := newLiftHealthTransferFixture(t, nil, .6)
			originalCarry, _ := ecs.GetComponent[components.LiftCarryState](source.world, player)
			originalMeta, _ := ecs.GetComponent[components.LiftedObjectState](source.world, object)
			refIndex := ecs.GetResource[ecs.InventoryRefIndex](source.world)
			root, found := refIndex.Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
			require.True(t, found)
			chunk := source.chunkManager.GetChunkFast(types.ChunkCoord{})
			beforeHandles := chunk.GetHandles()
			originalState, found := ecs.GetComponent[components.ObjectInternalState](source.world, object)
			require.True(t, found)
			if failure == "missing" {
				ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) { state.HasHP = false })
			} else {
				invalid := map[string]float64{"negative": -1, "nan": math.NaN(), "positive_infinity": math.Inf(1), "negative_infinity": math.Inf(-1)}[failure]
				ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) { state.HP = invalid })
			}
			participant := NewLiftCarryTransferParticipant(zap.NewNop())
			req := PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}
			stateAny, err := participant.CaptureSource(g, source, req, player)
			if failure == "missing" {
				require.ErrorIs(t, err, gameworld.ErrObjectHealthMissing)
			} else {
				require.ErrorIs(t, err, gameworld.ErrInvalidObjectHP)
			}
			require.Nil(t, stateAny)
			require.True(t, source.world.Alive(object))
			require.True(t, source.world.Alive(root))
			require.Equal(t, beforeHandles, chunk.GetHandles())
			carry, _ := ecs.GetComponent[components.LiftCarryState](source.world, player)
			require.Equal(t, originalCarry, carry)
			indexed, found := refIndex.Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
			require.True(t, found)
			require.Equal(t, root, indexed)
			afterFailure, found := ecs.GetComponent[components.ObjectInternalState](source.world, object)
			require.True(t, found)
			require.Equal(t, originalState.State, afterFailure.State)
			require.Equal(t, originalState.Flags, afterFailure.Flags)
			require.Equal(t, originalState.IsDirty, afterFailure.IsDirty)
			ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) {
				state.HP = .49
				state.HasHP = true
			})
			repairedState, found := ecs.GetComponent[components.ObjectInternalState](source.world, object)
			require.True(t, found)
			require.Equal(t, originalState.State, repairedState.State)
			require.Equal(t, originalState.Flags, repairedState.Flags)
			stateAny, err = participant.CaptureSource(g, source, req, player)
			require.NoError(t, err)
			require.False(t, source.world.Alive(object))
			require.NoError(t, participant.RestoreSourceRollback(g, source, req, player, stateAny))
			requireLiftHealthTransferState(t, source, player, .49, originalMeta)
		})
	}
}
