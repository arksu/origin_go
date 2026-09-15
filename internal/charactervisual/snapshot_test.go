package charactervisual

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func visualWorld(t *testing.T) (*ecs.World, types.Handle, types.Handle) {
	t.Helper()
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1002, Key: "stone_axe"}}))
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	character := w.Spawn(101, nil)
	ecs.AddComponent(w, character, components.Appearance{Resource: "player"})
	equipment := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, equipment, components.InventoryContainer{OwnerID: 101, Kind: constt.InventoryEquipment, Version: 9007199254740993})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryEquipment, 101, 0, equipment)
	return w, character, equipment
}

func TestSnapshotPublicSlotsAreSortedCopiedAndVersioned(t *testing.T) {
	w, character, equipment := visualWorld(t)
	ecs.MutateComponent[components.InventoryContainer](w, equipment, func(c *components.InventoryContainer) bool {
		c.Items = []components.InvItem{
			{ItemID: 201, TypeID: 1002, Quality: 87, Resource: "private_override", EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND},
			{ItemID: 202, TypeID: 1002, Quality: 12, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND},
		}
		return true
	})
	state, err := Snapshot(w, character)
	require.NoError(t, err)
	require.Equal(t, uint64(9007199254740993), state.Revision)
	require.Equal(t, []*netproto.CharacterEquipmentVisual{
		{Slot: netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND, VisualKey: "stone_axe"},
		{Slot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND, VisualKey: "stone_axe"},
	}, state.Equipment)
	encoded, err := proto.Marshal(state)
	require.NoError(t, err)
	decoded := &netproto.CharacterVisualState{}
	require.NoError(t, proto.Unmarshal(encoded, decoded))
	require.True(t, proto.Equal(state, decoded))
	ecs.MutateComponent[components.InventoryContainer](w, equipment, func(c *components.InventoryContainer) bool {
		c.Items[0].EquipSlot = netproto.EquipSlot_EQUIP_SLOT_HEAD
		c.Items = nil
		c.Version++
		return true
	})
	empty, err := Snapshot(w, character)
	require.NoError(t, err)
	require.Empty(t, empty.Equipment)
	require.Equal(t, state.Revision+1, empty.Revision)
	require.Equal(t, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND, state.Equipment[1].Slot)
}

func TestSnapshotGenerationSeparatesRecreatedEntitiesAndLayers(t *testing.T) {
	w, character, _ := visualWorld(t)
	first, err := Snapshot(w, character)
	require.NoError(t, err)
	w.Layer = -1
	otherLayer, err := Snapshot(w, character)
	require.NoError(t, err)
	require.NotEqual(t, first.Generation, otherLayer.Generation)
	w.Layer = 0
	w.Despawn(character)
	stale, err := Snapshot(w, character)
	require.NoError(t, err)
	require.Nil(t, stale)
	newCharacter := w.Spawn(101, nil)
	ecs.AddComponent(w, newCharacter, components.Appearance{Resource: "player"})
	respawn, err := Snapshot(w, newCharacter)
	require.NoError(t, err)
	require.NotEqual(t, first.Generation, respawn.Generation)
}

func TestSnapshotRejectsInvalidEquipment(t *testing.T) {
	for _, items := range [][]components.InvItem{
		{{TypeID: 1002, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_NONE}},
		{{TypeID: 1002, EquipSlot: 123}},
		{{TypeID: 9999, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND}},
		{{TypeID: 1002, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND}, {TypeID: 1002, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND}},
	} {
		w, character, equipment := visualWorld(t)
		ecs.MutateComponent[components.InventoryContainer](w, equipment, func(c *components.InventoryContainer) bool { c.Items = items; return true })
		state, err := Snapshot(w, character)
		require.Error(t, err)
		require.Nil(t, state)
	}
}

func TestSnapshotWithoutEquipmentAndNonCharacter(t *testing.T) {
	w := ecs.NewWorldForTesting()
	handle := w.Spawn(101, nil)
	state, err := Snapshot(w, handle)
	require.NoError(t, err)
	require.Nil(t, state)
	ecs.AddComponent(w, handle, components.Appearance{Resource: "player"})
	state, err = Snapshot(w, handle)
	require.NoError(t, err)
	require.NotEmpty(t, state.Generation)
	require.Zero(t, state.Revision)
	require.Empty(t, state.Equipment)
}
