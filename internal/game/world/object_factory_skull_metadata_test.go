package world

import (
	"encoding/json"
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"

	"github.com/stretchr/testify/require"
)

func TestObjectFactoryRestoresSkullMetadataInEveryRootKind(t *testing.T) {
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 3014, Key: "skull", Resource: "items/skull", Name: "Skull", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	for _, kind := range []constt.InventoryKind{constt.InventoryGrid, constt.InventoryHand, constt.InventoryEquipment, constt.InventoryDroppedItem} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			metadata := &components.SkullMetadata{CharacterID: 42, Nickname: "Alice", DeathDate: "2026-01-10"}
			data := inventory.InventoryDataV1{Kind: uint8(kind), Version: 1, Width: 4, Height: 4,
				Items: []inventory.InventoryItemV1{{ItemID: 100, TypeID: 3014, Quality: 7, Quantity: 1, Skull: metadata}},
			}
			encoded, err := json.Marshal(data)
			require.NoError(t, err)
			var persisted objectInventoryDataV1
			require.NoError(t, json.Unmarshal(encoded, &persisted))
			factory := NewObjectFactory(nil)
			handle := factory.spawnContainerTreeFromData(w, 100, persisted)
			require.True(t, w.Alive(handle))
			container, ok := ecs.GetComponent[components.InventoryContainer](w, handle)
			require.True(t, ok)
			require.Len(t, container.Items, 1)
			require.Equal(t, metadata, container.Items[0].Skull)
			require.NotSame(t, persisted.Items[0].Skull, container.Items[0].Skull)
			require.Equal(t, "Alice died on January 10, 2026", container.Items[0].Skull.HintExt())
		})
	}
}
