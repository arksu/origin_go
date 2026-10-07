package game

import (
	"encoding/json"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type disconnectInventoryRecorder struct {
	aliveAtSave           bool
	containersAliveAtSave bool
	containerHandles      []types.Handle
	snapshots             []systems.InventorySnapshot
}

func (r *disconnectInventoryRecorder) SerializeInventories(world interface{}, id types.EntityID, handle types.Handle) []systems.InventorySnapshot {
	r.aliveAtSave = world.(*ecs.World).Alive(handle)
	r.containersAliveAtSave = true
	for _, container := range r.containerHandles {
		r.containersAliveAtSave = r.containersAliveAtSave && world.(*ecs.World).Alive(container)
	}
	r.snapshots = inventory.NewInventorySaver(zap.NewNop()).SerializeInventories(world, id, handle)
	return r.snapshots
}

func TestFinalPlayerDisconnectReleasesInventoriesAfterSnapshot(t *testing.T) {
	for _, mode := range []string{"immediate", "detached_expiry"} {
		t.Run(mode, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			playerID := types.EntityID(10)
			player := w.Spawn(playerID, nil)
			ecs.AddComponent(w, player, components.Transform{X: 100, Y: 200})
			ecs.AddComponent(w, player, components.EntityStats{Stamina: 100, Energy: 100})
			ecs.AddComponent(w, player, components.EntityHealth{SHP: 10.25, HHP: 19.6})
			index := ecs.GetResource[ecs.InventoryRefIndex](w)
			owned := make([]types.Handle, 0, 5)
			links := make([]components.InventoryLink, 0, 5)
			addContainer := func(ownerID types.EntityID, kind constt.InventoryKind, itemID types.EntityID) types.Handle {
				handle := w.SpawnWithoutExternalID()
				container := components.InventoryContainer{OwnerID: ownerID, Kind: kind, Width: 2, Height: 2, Version: 1}
				if itemID != 0 {
					container.Items = []components.InvItem{{ItemID: itemID, TypeID: 1, Quantity: 1, W: 1, H: 1}}
				}
				ecs.AddComponent(w, handle, container)
				index.Add(kind, ownerID, 0, handle)
				return handle
			}
			for _, container := range []struct {
				ownerID types.EntityID
				kind    constt.InventoryKind
				itemID  types.EntityID
			}{
				{playerID, constt.InventoryGrid, 101},
				{playerID, constt.InventoryHand, 0},
				{playerID, constt.InventoryEquipment, 0},
				{101, constt.InventoryGrid, 102},
				{102, constt.InventoryGrid, 103},
			} {
				handle := addContainer(container.ownerID, container.kind, container.itemID)
				owned = append(owned, handle)
				links = append(links, components.InventoryLink{OwnerID: container.ownerID, Kind: container.kind, Handle: handle})
			}
			unrelated := addContainer(20, constt.InventoryGrid, 0)
			ecs.AddComponent(w, player, components.InventoryOwner{Inventories: links})
			now := time.Unix(100, 0)
			ecs.GetResource[ecs.CharacterEntities](w).Add(playerID, player, now.Add(time.Hour))
			recorder := &disconnectInventoryRecorder{containerHandles: owned}
			// No save workers: the test observes synchronous snapshot capture without DB I/O.
			saver := systems.NewCharacterSaver(nil, 0, recorder, zap.NewNop())
			shard := &Shard{world: w, characterSaver: saver}
			if mode == "immediate" {
				shard.despawnDisconnectedPlayer(playerID, player)
			} else {
				ecs.GetResource[ecs.DetachedEntities](w).AddDetachedEntity(playerID, player, now.Add(time.Second), now)
				system := systems.NewExpireDetachedSystem(zap.NewNop(), saver, shard.onDetachedEntityExpired, nil)
				ecs.GetResource[ecs.TimeState](w).Now = now
				system.Update(w, 0)
				require.True(t, w.Alive(player), "reattachable player must remain alive before expiry")
				for _, handle := range owned {
					require.True(t, w.Alive(handle), "reattach needs all inventory entities")
				}
				require.Empty(t, recorder.snapshots)
				ecs.GetResource[ecs.TimeState](w).Now = now.Add(2 * time.Second)
				system.Update(w, 0)
			}
			require.True(t, recorder.aliveAtSave, "save must precede player despawn")
			require.True(t, recorder.containersAliveAtSave, "save must precede inventory cleanup at every depth")
			require.Len(t, recorder.snapshots, 3)
			for _, snapshot := range recorder.snapshots {
				if snapshot.Kind != int16(constt.InventoryGrid) {
					continue
				}
				var root inventory.InventoryDataV1
				require.NoError(t, json.Unmarshal(snapshot.Data, &root))
				require.Len(t, root.Items, 1)
				require.NotNil(t, root.Items[0].NestedInventory)
				require.EqualValues(t, 102, root.Items[0].NestedInventory.Items[0].ItemID)
			}
			require.False(t, w.Alive(player))
			for _, handle := range owned {
				require.False(t, w.Alive(handle))
			}
			for _, link := range links {
				_, found := index.Lookup(link.Kind, link.OwnerID, link.Key)
				require.False(t, found)
			}
			require.True(t, w.Alive(unrelated))
			require.Equal(t, 1, w.EntityCount())
			_, tracked := ecs.GetResource[ecs.CharacterEntities](w).Map[playerID]
			require.False(t, tracked)
		})
	}
}
