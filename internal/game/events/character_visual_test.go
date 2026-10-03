package events

import (
	"origin/internal/combat"
	"sync"
	"testing"
	"time"

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

func TestCombatVisibilityEntryCapturesLatestState(t *testing.T) {
	w := ecs.NewWorldForTesting()
	target := spawnBatchTarget(w, 101, "boulder")
	ecs.AddComponent(w, target, components.CombatTestTarget{HP: 100, MaxHP: 100, Revision: 1, Receiver: true})
	actor := spawnBatchTarget(w, 102, "player")
	timing := ecs.GetResource[ecs.TimeState](w)
	timing.Now = time.Unix(100, 0)
	ecs.AddComponent(w, actor, components.CombatState{Revision: 1, Execution: &components.CombatExecution{ID: 1, ActionID: "axe_aoe", Direction: combat.Point{X: 1}, Weapon: combat.Weapon{Range: 18}, AngleDegrees: 90, StartedAt: timing.Now, StrikeAt: timing.Now.Add(600 * time.Millisecond), RecoveryEnd: timing.Now.Add(time.Second)}})
	// Vision queues handles, not snapshots. A later dispatcher captures current HP
	// and progress even if an impact occurred after the visibility event was queued.
	ecs.WithComponent(w, target, func(state *components.CombatTestTarget) { state.HP = 94; state.Revision = 2 })
	ecs.WithComponent(w, actor, func(state *components.CombatState) { state.Execution.StrikeResolved = true; state.Revision = 2 })
	timing.Now = timing.Now.Add(700 * time.Millisecond)
	timing.UnixMs = 700
	dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}
	snapshot := dispatcher.buildObjectSpawn(w, 101, target)
	require.Equal(t, float64(94), snapshot.CombatTarget.Hp)
	require.Equal(t, uint64(2), snapshot.CombatTarget.Revision)
	execution := dispatcher.buildObjectSpawn(w, 102, actor).CombatExecution
	require.Equal(t, "recovery", execution.Phase)
	require.Equal(t, float64(700), execution.ElapsedMs)
	require.Equal(t, float64(1), execution.LockedDirection.X)
	w.Despawn(target)
	replacement := spawnBatchTarget(w, 101, "boulder")
	require.NotEqual(t, target, replacement)
	require.Nil(t, dispatcher.buildObjectSpawn(w, 101, target), "late spawn cannot restore removed fixtures")
}
