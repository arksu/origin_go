package events

import (
	"sync"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConcurrentObjectSpawnSnapshotsWithMissingOptionalComponents(t *testing.T) {
	w := ecs.NewWorldWithCapacity(16, nil, 0)
	character := w.Spawn(101, nil)
	ecs.AddComponent(w, character, components.Appearance{Resource: "player"})
	ecs.AddComponent(w, character, components.EntityInfo{TypeID: 1})
	ecs.AddComponent(w, character, components.Transform{X: 20, Y: 30})
	dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}
	var worldMu sync.RWMutex
	var readers sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			// Async visibility callbacks share the shard's read lock.
			worldMu.RLock()
			defer worldMu.RUnlock()
			spawn := dispatcher.buildObjectSpawn(w, 101, character)
			if spawn == nil || spawn.CharacterVisual == nil || spawn.EntityId != 101 || spawn.CarriedByEntityId != 0 {
				t.Error("expected a character snapshot with no carried-object state")
			}
		}()
	}
	close(start)
	readers.Wait()
	if w.GetStorage(ecs.GetComponentID[components.Collider]()) != nil ||
		w.GetStorage(ecs.GetComponentID[components.LiftedObjectState]()) != nil {
		t.Fatal("spawn snapshots must not create optional component storage")
	}
}

func TestObjectSpawnCapturesLatestEquipmentForLateObserver(t *testing.T) {
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1002, Key: "stone_axe"}}))
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	character := w.Spawn(101, nil)
	ecs.AddComponent(w, character, components.Appearance{Resource: "player"})
	ecs.AddComponent(w, character, components.EntityInfo{TypeID: 1})
	ecs.AddComponent(w, character, components.Transform{X: 20, Y: 30})
	equipment := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, equipment, components.InventoryContainer{OwnerID: 101, Kind: constt.InventoryEquipment, Version: 4,
		Items: []components.InvItem{{ItemID: 200, TypeID: 1002, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND}},
	})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryEquipment, 101, 0, equipment)
	dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}
	first := dispatcher.buildObjectSpawn(w, 101, character)
	require.Equal(t, "stone_axe", first.CharacterVisual.Equipment[0].VisualKey)
	ecs.MutateComponent[components.InventoryContainer](w, equipment, func(c *components.InventoryContainer) bool {
		c.Items = nil
		c.Version++
		return true
	})
	late := dispatcher.buildObjectSpawn(w, 101, character)
	require.Equal(t, uint64(5), late.CharacterVisual.Revision)
	require.Empty(t, late.CharacterVisual.Equipment)
	require.Len(t, first.CharacterVisual.Equipment, 1)
	require.Nil(t, dispatcher.buildObjectSpawn(w, 999, character))
	w.Despawn(character)
	require.Nil(t, dispatcher.buildObjectSpawn(w, 101, character))
}

func TestDelayedDespawnChecksCurrentVisibilityAndIncarnation(t *testing.T) {
	w := ecs.NewWorldForTesting()
	observer := w.Spawn(101, nil)
	target := w.Spawn(102, nil)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	require.False(t, targetVisibleToObserver(w, 101, 102))
	visibility.ObserversByVisibleTarget[target] = map[types.Handle]struct{}{observer: {}}
	require.True(t, targetVisibleToObserver(w, 101, 102))
	w.Despawn(target)
	require.False(t, targetVisibleToObserver(w, 101, 102))
	replacement := w.Spawn(102, nil)
	visibility.ObserversByVisibleTarget[replacement] = map[types.Handle]struct{}{observer: {}}
	require.True(t, targetVisibleToObserver(w, 101, 102), "old despawn must not erase the replacement")
}
