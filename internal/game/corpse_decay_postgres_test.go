package game

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type corpseAmbiguousCommitPersister struct {
	*gameworld.DroppedItemPersisterDB
	calls atomic.Int32
}

func (p *corpseAmbiguousCommitPersister) TransformObjectWithDroppedItems(ctx context.Context, raw *repository.Object, maxID types.EntityID, next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) error {
	err := p.DroppedItemPersisterDB.TransformObjectWithDroppedItems(ctx, raw, maxID, next)
	if err == nil && p.calls.Add(1) == 1 {
		return errors.New("simulate lost commit acknowledgement")
	}
	return err
}

func TestCorpseDecayPostgresLootAndRestartAfterAmbiguousCommit(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 256, 1279)
	destination := installCorpseLootDefinitions(t)
	w := f.shard.world
	f.shard.objectDestruction.deps.TransformCommitted = f.shard.completeCorpseDecay
	persister := &corpseAmbiguousCommitPersister{DroppedItemPersisterDB: gameworld.NewDroppedItemPersisterDB(f.db, f.shard.logger)}
	f.shard.objectDestruction.deps.Persister = persister
	f.shard.mu.Lock()
	ecs.WithComponent(w, f.target, func(info *components.EntityInfo) {
		info.Indestructible = true
		info.Behaviors = []string{"container", "player_dead"}
	})
	ecs.WithComponent(w, f.target, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "player_dead", &components.CorpseDecayBehaviorState{DecayAtRuntimeSeconds: 10})
	})
	ecs.AddComponent(w, f.target, components.CorpseVisualState{})
	for _, kind := range []constt.InventoryKind{constt.InventoryEquipment, constt.InventoryHand} {
		container := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, container, components.InventoryContainer{
			OwnerID: 1, Kind: kind, Version: 1, Width: 1, Height: 1,
			Items: []components.InvItem{lootItem(types.EntityID(1000+int(kind)), 1, 1)},
		})
		ecs.GetResource[ecs.InventoryRefIndex](w).Add(kind, 1, 0, container)
		f.owned = append(f.owned, ecs.InventoryRefEntry{InventoryRefKey: ecs.InventoryRefKey{Kind: kind, OwnerID: 1}, Handle: container})
	}
	require.NoError(t, f.shard.objectDestruction.TransformWithLoot(f.target, destination))
	f.shard.objectDestruction.Update()
	f.shard.mu.Unlock()
	op := &f.shard.objectDestruction.operations[0]
	require.Eventually(t, func() bool {
		op.mu.Lock()
		defer op.mu.Unlock()
		return op.phase == destructionReady
	}, 5*time.Second, 5*time.Millisecond)

	// A new process sees the complete skeleton and all drops even before the
	// old shard acknowledges the transaction or changes its ECS corpse.
	ctx := context.Background()
	skeleton, err := f.db.Queries().GetObjectByID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 17, skeleton.TypeID)
	require.Equal(t, int16(17), skeleton.Quality)
	require.Equal(t, 1279, skeleton.X)
	oldRoots, err := f.db.Queries().GetInventoriesByOwner(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, oldRoots)
	var durable []repository.Object
	for chunkX := 0; chunkX < 2; chunkX++ {
		rows, err := f.db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1, ChunkX: chunkX})
		require.NoError(t, err)
		durable = append(durable, rows...)
	}
	require.Len(t, durable, 7, "one same-ID skeleton and six ground drops")
	restarted := ecs.NewWorldWithCapacity(256, nil, 0)
	*ecs.GetResource[ecs.TimeState](restarted) = *ecs.GetResource[ecs.TimeState](w)
	factory := gameworld.NewObjectFactory(nil)
	for i := range durable {
		roots, err := f.db.Queries().GetInventoriesByOwner(ctx, durable[i].ID)
		require.NoError(t, err)
		handle, err := factory.Build(restarted, &durable[i], roots)
		require.NoError(t, err)
		require.True(t, restarted.Alive(handle))
	}
	restored := restarted.GetHandleByEntityID(1)
	info, exists := ecs.GetComponent[components.EntityInfo](restarted, restored)
	require.True(t, exists)
	require.Equal(t, uint32(17), info.TypeID)
	require.True(t, info.Indestructible)
	require.False(t, ecs.HasComponent[components.CorpseVisualState](restarted, restored))
	require.False(t, ecs.HasComponent[components.InventoryOwner](restarted, restored))
	bag, exists := ecs.GetResource[ecs.InventoryRefIndex](restarted).Lookup(constt.InventoryGrid, 30, 0)
	require.True(t, exists)
	contents, exists := ecs.GetComponent[components.InventoryContainer](restarted, bag)
	require.True(t, exists)
	require.Len(t, contents.Items, 1)
	require.Equal(t, types.EntityID(900), contents.Items[0].ItemID)
	require.Equal(t, uint32(2), contents.Items[0].Quantity)

	f.eventually(t, func() bool { return f.shard.objectDestruction.PendingCount() == 0 })
	require.Equal(t, int32(2), persister.calls.Load())
	f.shard.mu.RLock()
	require.True(t, w.Alive(f.target))
	require.Equal(t, f.target, w.GetHandleByEntityID(1))
	info, _ = ecs.GetComponent[components.EntityInfo](w, f.target)
	require.Equal(t, uint32(17), info.TypeID)
	require.False(t, ecs.ObjectDestructionPending(w, f.target))
	for _, ref := range f.owned {
		require.False(t, w.Alive(ref.Handle))
	}
	f.shard.mu.RUnlock()
	for chunkX := 0; chunkX < 2; chunkX++ {
		rows, err := f.db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1, ChunkX: chunkX})
		require.NoError(t, err)
		for _, row := range rows {
			var original *repository.Object
			for i := range durable {
				if durable[i].ID == row.ID {
					original = &durable[i]
					break
				}
			}
			require.NotNil(t, original, "retry must not add another durable identity")
			require.Equal(t, original.TypeID, row.TypeID)
			require.Equal(t, original.X, row.X)
			require.Equal(t, original.Y, row.Y)
			require.Equal(t, original.Data, row.Data)
		}
	}
}
