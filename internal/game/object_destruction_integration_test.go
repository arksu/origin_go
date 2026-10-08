package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type objectDestructionIntegrationFixture struct {
	shard   *Shard
	db      *persistence.Postgres
	cfg     *config.Config
	cm      *gameworld.ChunkManager
	target  types.Handle
	owned   []ecs.InventoryRefEntry
	logs    *observer.ObservedLogs
	stopped bool
}

func installObjectDestructionIntegrationDefs(t testing.TB) {
	t.Helper()
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems); objectdefs.SetGlobalForTesting(previousObjects) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "ore", Resource: "ore", Size: itemdefs.Size{W: 1, H: 1}, Stack: &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 1000}},
		{DefID: 2, Key: "bag", Resource: "bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 4, H: 4}}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 99, Key: "integration_station", HP: 100, Resource: "station", IsStatic: true,
			Behaviors: map[string]json.RawMessage{"container": json.RawMessage(`{}`)}, BehaviorOrder: []string{"container"},
			Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 6, H: 6}, Inventory: []objectdefs.InventoryDef{
				{Kind: "grid", Key: 0, W: 4, H: 4}, {Kind: "grid", Key: 3, W: 4, H: 4},
			}},
		},
	}))
}

func newObjectDestructionIntegrationFixture(t *testing.T, capacity uint32, x int) *objectDestructionIntegrationFixture {
	t.Helper()
	installObjectDestructionIntegrationDefs(t)
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", "../../migrations/schema.sql")
	ctx := context.Background()
	for chunkX := 0; chunkX < 2; chunkX++ {
		_, err := db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{Region: 1, X: chunkX, TilesData: make([]byte, constt.ChunkSize*constt.ChunkSize)})
		require.NoError(t, err)
	}
	require.NoError(t, db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
		ID: 1, TypeID: 99, Region: 1, X: x, Y: 50, Quality: 17, Hp: sql.NullFloat64{Float64: 100, Valid: true},
	}))
	nested := inventory.InventoryDataV1{Kind: uint8(constt.InventoryGrid), Width: 4, Height: 4, Version: 2,
		Items: []inventory.InventoryItemV1{{ItemID: 900, TypeID: 1, Quality: 23, Quantity: 2}},
	}
	for _, root := range []inventory.InventoryDataV1{
		{Kind: uint8(constt.InventoryGrid), Key: 0, Width: 4, Height: 4, Version: 1, Items: []inventory.InventoryItemV1{{ItemID: 30, TypeID: 2, Quality: 17, Quantity: 1, NestedInventory: &nested}}},
		{Kind: uint8(constt.InventoryGrid), Key: 3, Width: 4, Height: 4, Version: 1, Items: []inventory.InventoryItemV1{{ItemID: 20, TypeID: 1, Quality: 17, Quantity: 3}}},
	} {
		payload, err := json.Marshal(root)
		require.NoError(t, err)
		_, err = db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: 1, Kind: int16(root.Kind), InventoryKey: int16(root.Key), Data: payload, Version: 1})
		require.NoError(t, err)
	}
	w := ecs.NewWorldWithCapacity(capacity, nil, 0)
	logCore, logs := observer.New(zap.InfoLevel)
	shard := &Shard{world: w, layer: 0, logger: zap.New(logCore)}
	f := &objectDestructionIntegrationFixture{shard: shard, db: db, logs: logs}
	cfg := &config.Config{Game: config.GameConfig{ChunkLRUCapacity: 10, ChunkLRUTTL: 60, LoadWorkers: 1, SaveWorkers: 1,
		WorldWidthChunks: 2, WorldHeightChunks: 1, Region: 1,
	}}
	f.cfg = cfg
	f.cm = gameworld.NewChunkManager(cfg, db, w, shard, 0, 1, gameworld.NewObjectFactory(nil), nil, nil, shard.logger)
	shard.chunkManager = f.cm
	minimumX, minimumY, maximumX, maximumY := f.cm.WorldBounds()
	var err error
	shard.objectDestruction, err = NewObjectDestructionService(w, ObjectDestructionDependencies{
		Chunks: f.cm, Persister: gameworld.NewDroppedItemPersisterDB(db, shard.logger), IDs: &destructionIDsFixture{next: 100000},
		Items: itemdefs.Global(), WithWorldRead: shard.WithWorldRead, Quarantine: shard.quarantineDestroyedObject,
		MinX: minimumX, MinY: minimumY, MaxX: maximumX, MaxY: maximumY, Region: 1, Logger: shard.logger,
	})
	require.NoError(t, err)
	shard.objectDamage, err = NewObjectDamageService(w, shard.objectDestruction)
	require.NoError(t, err)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Now: time.Now(), UnixMs: 100000, RuntimeSecondsTotal: 10}
	t.Cleanup(func() { f.stop() })
	f.cm.RegisterEntity(9000, x, 50, false)
	loadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.cm.WaitPreloaded(loadCtx, types.ChunkCoord{}))
	f.eventually(t, func() bool { return w.Alive(w.GetHandleByEntityID(1)) })
	f.target = w.GetHandleByEntityID(1)
	require.NoError(t, shard.objectDamage.PrepareTarget(f.target))
	index := ecs.GetResource[ecs.InventoryRefIndex](w)
	f.owned = index.EntriesByOwnerInto(1, nil)
	child, found := index.Lookup(constt.InventoryGrid, 30, 0)
	require.True(t, found)
	f.owned = append(f.owned, ecs.InventoryRefEntry{InventoryRefKey: ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 30}, Handle: child})
	return f
}

func (f *objectDestructionIntegrationFixture) tick() {
	f.shard.mu.Lock()
	defer f.shard.mu.Unlock()
	state := ecs.GetResource[ecs.TimeState](f.shard.world)
	state.Now = state.Now.Add(time.Second)
	if f.shard.objectDestruction != nil {
		f.shard.objectDestruction.Update()
	}
	f.cm.Update(.05)
}

func (f *objectDestructionIntegrationFixture) eventually(t *testing.T, condition func() bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		f.tick()
		f.shard.mu.RLock()
		defer f.shard.mu.RUnlock()
		return condition()
	}, 5*time.Second, 5*time.Millisecond)
}

func (f *objectDestructionIntegrationFixture) hit(t *testing.T) {
	t.Helper()
	f.shard.mu.Lock()
	defer f.shard.mu.Unlock()
	result, err := f.shard.objectDamage.Apply(f.target, 100)
	require.NoError(t, err)
	require.True(t, result.EnteredDestruction)
	for _, ref := range f.owned {
		require.True(t, f.shard.world.Alive(ref.Handle), "capture retains all owned containers before commit")
	}
}

func (f *objectDestructionIntegrationFixture) stop() {
	if !f.stopped {
		f.cm.Stop()
		f.stopped = true
	}
}

func (f *objectDestructionIntegrationFixture) assertDurableReplacement(t *testing.T) []repository.Object {
	t.Helper()
	_, err := f.db.Queries().GetObjectByID(context.Background(), 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
	rows, err := f.db.Queries().GetInventoriesByOwner(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, rows)
	var objects []repository.Object
	for chunkX := 0; chunkX < 2; chunkX++ {
		chunkObjects, err := f.db.Queries().GetObjectsByChunk(context.Background(), repository.GetObjectsByChunkParams{Region: 1, ChunkX: chunkX})
		require.NoError(t, err)
		objects = append(objects, chunkObjects...)
	}
	require.Len(t, objects, 4)
	for _, object := range objects {
		require.Equal(t, constt.DroppedItemTypeID, object.TypeID)
		rows, err := f.db.Queries().GetInventoriesByOwner(context.Background(), object.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		var root inventory.InventoryDataV1
		require.NoError(t, json.Unmarshal(rows[0].Data, &root))
		require.Len(t, root.Items, 1)
		require.Equal(t, uint32(1), root.Items[0].Quantity)
		require.Equal(t, uint32(17), root.Items[0].Quality)
		if object.ID == 30 {
			require.NotNil(t, root.Items[0].NestedInventory)
			require.Equal(t, uint64(900), root.Items[0].NestedInventory.Items[0].ItemID)
			require.Equal(t, uint32(2), root.Items[0].NestedInventory.Items[0].Quantity)
		}
	}
	return objects
}

func (f *objectDestructionIntegrationFixture) assertMaterialized(t *testing.T, objects []repository.Object) {
	t.Helper()
	f.eventually(t, func() bool {
		for _, object := range objects {
			if !f.shard.world.Alive(f.shard.world.GetHandleByEntityID(types.EntityID(object.ID))) {
				return false
			}
		}
		return true
	})
	f.shard.mu.RLock()
	defer f.shard.mu.RUnlock()
	require.False(t, f.shard.world.Alive(f.target))
	for _, ref := range f.owned {
		require.False(t, f.shard.world.Alive(ref.Handle))
	}
	for _, object := range objects {
		root, found := ecs.GetResource[ecs.InventoryRefIndex](f.shard.world).Lookup(constt.InventoryDroppedItem, types.EntityID(object.ID), 0)
		require.True(t, found)
		container, ok := ecs.GetComponent[components.InventoryContainer](f.shard.world, root)
		require.True(t, ok)
		require.Len(t, container.Items, 1)
		require.Equal(t, uint32(1), container.Items[0].Quantity)
	}
	bag, found := ecs.GetResource[ecs.InventoryRefIndex](f.shard.world).Lookup(constt.InventoryGrid, 30, 0)
	require.True(t, found)
	container, ok := ecs.GetComponent[components.InventoryContainer](f.shard.world, bag)
	require.True(t, ok)
	require.Len(t, container.Items, 1)
	require.Equal(t, types.EntityID(900), container.Items[0].ItemID)
	require.Equal(t, uint32(2), container.Items[0].Quantity)
	require.Equal(t, uint32(23), container.Items[0].Quality)
}

func TestObjectDestructionPostgresRuntimeFactoryRoundtrip(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	f.hit(t)
	f.eventually(t, func() bool { return f.shard.objectDestruction.PendingCount() == 0 })
	f.assertMaterialized(t, f.assertDurableReplacement(t))
}

func TestObjectDestructionPostgresQuarantineClosesContainersLinksCarryAndVisibility(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	f.shard.mu.Lock()
	w := f.shard.world
	player := w.Spawn(2000, nil)
	ecs.AddComponent(w, player, components.Collider{Phantom: &components.PhantomCollider{WorldX: 50, WorldY: 50}})
	ecs.AddComponent(w, player, components.LiftCarryState{ObjectEntityID: 1, ObjectHandle: f.target})
	ecs.AddComponent(w, player, components.PendingLiftTransition{ObjectEntityID: 1, ObjectHandle: f.target, Mode: components.LiftTransitionModePutDown, TargetX: 50, TargetY: 50})
	ecs.AddComponent(w, f.target, components.LiftedObjectState{CarrierPlayerID: 2000, CarrierHandle: player})
	ecs.AddComponent(w, f.target, components.StationState{})
	f.shard.liftService = NewLiftService(w, f.cm, nil, nil, f.shard.logger)
	links := ecs.GetResource[ecs.LinkState](w)
	links.SetIntent(2000, 1, f.target, time.Now())
	links.SetLink(ecs.PlayerLink{PlayerID: 2000, PlayerHandle: player, TargetID: 1, TargetHandle: f.target})
	opened := ecs.GetResource[ecs.OpenContainerState](w)
	opened.SetRootOpened(2000, 1)
	rootRef := ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 1}
	nestedRef := ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 30}
	opened.OpenRef(2000, rootRef)
	opened.OpenRef(2000, nestedRef)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.VisibleByObserver[player] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{f.target: 1}}
	visibility.ObserversByVisibleTarget[f.target] = map[types.Handle]struct{}{player: {}}
	f.shard.mu.Unlock()

	f.hit(t)
	f.shard.WithWorldRead(func(w *ecs.World) {
		require.NotContains(t, links.LinkedByPlayer, types.EntityID(2000))
		require.NotContains(t, links.IntentByPlayer, types.EntityID(2000))
		require.False(t, opened.IsRefOpened(2000, rootRef))
		require.False(t, opened.IsRefOpened(2000, nestedRef))
		_, rootOpen := opened.GetOpenedRoot(2000)
		require.False(t, rootOpen)
		_, carrying := ecs.GetComponent[components.LiftCarryState](w, player)
		require.False(t, carrying)
		_, transitioning := ecs.GetComponent[components.PendingLiftTransition](w, player)
		require.False(t, transitioning)
		collider, _ := ecs.GetComponent[components.Collider](w, player)
		require.Nil(t, collider.Phantom)
		_, lifted := ecs.GetComponent[components.LiftedObjectState](w, f.target)
		require.False(t, lifted)
		_, station := ecs.GetComponent[components.StationState](w, f.target)
		require.False(t, station)
		_, solid := ecs.GetComponent[components.Collider](w, f.target)
		require.False(t, solid)
		require.NotContains(t, f.cm.GetChunkFast(types.ChunkCoord{}).GetHandles(), f.target)
		require.NotContains(t, visibility.ObserversByVisibleTarget, f.target)
		require.NotContains(t, visibility.VisibleByObserver[player].Known, f.target)
	})
	f.eventually(t, func() bool { return f.shard.objectDestruction.PendingCount() == 0 })
	f.assertMaterialized(t, f.assertDurableReplacement(t))
}

func TestObjectDestructionPostgresInactiveNeighborMaterializesAfterInterest(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, constt.ChunkWorldSize-1)
	f.hit(t)
	f.eventually(t, func() bool { return f.shard.objectDestruction.PendingCount() == 0 })
	objects := f.assertDurableReplacement(t)
	inactiveCount := 0
	f.shard.WithWorldRead(func(w *ecs.World) {
		for _, object := range objects {
			if object.ChunkX == 1 {
				inactiveCount++
				require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(types.EntityID(object.ID)))
			}
		}
	})
	require.Positive(t, inactiveCount, "fixed server timestamp produces boundary-crossing loot")
	f.shard.mu.Lock()
	f.cm.RegisterEntity(9001, constt.ChunkWorldSize+50, 50, false)
	f.shard.mu.Unlock()
	f.assertMaterialized(t, objects)
}

func TestObjectDestructionPostgresFullWorldRetainsCommittedRawUntilCapacityRetry(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 12, 50)
	var blockers []types.Handle
	f.shard.mu.Lock()
	for f.shard.world.EntityCount() < f.shard.world.EntityCapacity() {
		blockers = append(blockers, f.shard.world.SpawnWithoutExternalID())
	}
	f.shard.mu.Unlock()
	f.hit(t)
	f.eventually(t, func() bool { return f.shard.objectDestruction.PendingCount() == 0 })
	objects := f.assertDurableReplacement(t)
	chunk := f.cm.GetChunkFast(types.ChunkCoord{})
	require.NotEmpty(t, chunk.GetRawObjects(), "committed records survive insufficient ECS capacity")
	f.shard.mu.Lock()
	for _, handle := range blockers {
		f.shard.world.Despawn(handle)
	}
	f.shard.mu.Unlock()
	f.assertMaterialized(t, objects)
	require.Empty(t, chunk.GetRawObjects())
}

func TestObjectDestructionPostgresMaterializesAtMost32DropsPerTick(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 256, 50)
	func() {
		f.shard.mu.Lock()
		defer f.shard.mu.Unlock()
		root, found := ecs.GetResource[ecs.InventoryRefIndex](f.shard.world).Lookup(constt.InventoryGrid, 1, 3)
		require.True(t, found)
		ecs.MutateComponent[components.InventoryContainer](f.shard.world, root, func(c *components.InventoryContainer) bool {
			c.Items[0].Quantity = 70
			c.Version++
			return true
		})
	}()
	f.hit(t)
	f.shard.mu.Lock()
	f.shard.objectDestruction.Update() // Admit the worker, without installing a page yet.
	f.shard.mu.Unlock()
	operation := &f.shard.objectDestruction.operations[0]
	require.Eventually(t, func() bool {
		operation.mu.Lock()
		defer operation.mu.Unlock()
		return operation.phase == destructionReady && operation.committed && operation.err == nil
	}, 5*time.Second, 5*time.Millisecond)
	objects, err := f.db.Queries().GetObjectsByChunk(context.Background(), repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	require.Len(t, objects, 71, "one transaction commits all units before publication")
	f.tick()
	f.shard.mu.RLock()
	live := 0
	for _, object := range objects {
		if f.shard.world.Alive(f.shard.world.GetHandleByEntityID(types.EntityID(object.ID))) {
			live++
		}
	}
	f.shard.mu.RUnlock()
	require.Equal(t, ObjectDestructionDropBudget, live)
	f.eventually(t, func() bool {
		if f.shard.objectDestruction.PendingCount() != 0 {
			return false
		}
		for _, object := range objects {
			if !f.shard.world.Alive(f.shard.world.GetHandleByEntityID(types.EntityID(object.ID))) {
				return false
			}
		}
		return true
	})
}

func TestObjectDestructionPostgresShutdownDrainCommitsPendingReplacement(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	f.hit(t)
	f.shard.objectDestruction.StopAdmission()
	f.shard.drainObjectDestruction()
	require.Zero(t, f.shard.objectDestruction.PendingCount())
	require.False(t, f.shard.world.Alive(f.target))
	f.assertDurableReplacement(t)
	f.stop()
	require.Empty(t, f.logs.FilterMessage("Shutdown object destructions incomplete").All())
	require.Empty(t, f.logs.FilterMessage("chunk still has pending persistence during shutdown").All())
}

func TestObjectDestructionPostgresRestartAfterCommitBeforeRuntimeCompletion(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	f.hit(t)
	f.shard.mu.Lock()
	f.shard.objectDestruction.Update()
	f.shard.mu.Unlock()
	operation := &f.shard.objectDestruction.operations[0]
	require.Eventually(t, func() bool {
		operation.mu.Lock()
		defer operation.mu.Unlock()
		return operation.phase == destructionReady && operation.committed && operation.err == nil
	}, 5*time.Second, 5*time.Millisecond)
	// Discard the runtime before its completion page is applied. Stop cannot save
	// the pinned source over the already committed replacement transaction.
	f.stop()
	require.True(t, f.shard.world.Alive(f.target))
	objects := f.assertDurableReplacement(t)
	require.Equal(t, 1, f.shard.objectDestruction.PendingCount())

	w := ecs.NewWorldWithCapacity(64, nil, 0)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Now: time.Now(), RuntimeSecondsTotal: 10}
	shard := &Shard{world: w, layer: 0, logger: f.shard.logger}
	cm := gameworld.NewChunkManager(f.cfg, f.db, w, shard, 0, 1, gameworld.NewObjectFactory(nil), nil, nil, shard.logger)
	shard.chunkManager = cm
	restarted := &objectDestructionIntegrationFixture{shard: shard, db: f.db, cm: cm, cfg: f.cfg}
	t.Cleanup(restarted.stop)
	cm.RegisterEntity(9000, 50, 50, false)
	loadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, cm.WaitPreloaded(loadCtx, types.ChunkCoord{}))
	restarted.assertMaterialized(t, objects)
	require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(1), "destroyed source must not return after restart")
	require.Equal(t, 9, w.EntityCount(), "four drops, four roots and one retained bag inventory")
	for i := 0; i < 3; i++ {
		restarted.tick()
	}
	require.Equal(t, 9, w.EntityCount(), "activation retries must not duplicate committed loot")
}

func TestObjectDestructionPostgresShutdownReportsRejectedCaptureAndKeepsSource(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	f.shard.mu.Lock()
	ecs.MutateComponent[components.InventoryContainer](f.shard.world, f.owned[0].Handle, func(c *components.InventoryContainer) bool { c.Items[0].Quality = 0; c.Version++; return true })
	f.shard.mu.Unlock()
	f.hit(t)
	f.eventually(t, func() bool { return len(f.logs.FilterMessage("Object destruction persistence failed").All()) > 0 })
	f.shard.objectDestruction.StopAdmission()
	f.stop()
	require.NotEmpty(t, f.logs.FilterMessage("chunk still has pending persistence during shutdown").All())
	require.True(t, f.shard.world.Alive(f.target))
	for _, ref := range f.owned {
		require.True(t, f.shard.world.Alive(ref.Handle))
	}
	object, err := f.db.Queries().GetObjectByID(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 100.0, object.Hp.Float64)
	rows, err := f.db.Queries().GetInventoriesByOwner(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	objects, err := f.db.Queries().GetObjectsByChunk(context.Background(), repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	require.Len(t, objects, 1)
}
