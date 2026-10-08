package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDestroyedRootAndStalePersonalBagCannotMutateItems(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())
	w, playerID, player, root, nested, hand, bagID := setupGiveItemWorld(t)
	// Move the bag to a world object but deliberately preserve a stale player link.
	ecs.WithComponent(w, root, func(c *components.InventoryContainer) { c.Items = nil })
	sourceID := types.EntityID(9000)
	source := w.Spawn(sourceID, nil)
	sourceRoot := createGridContainer(w, sourceID, 0, 2, 2)
	addItemToContainer(w, sourceRoot, components.InvItem{ItemID: bagID, TypeID: 200, Quantity: 1, W: 1, H: 1})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, sourceID, 0, sourceRoot)
	ecs.InitResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{source: true}})
	open := ecs.GetResource[ecs.OpenContainerState](w)
	open.SetRootOpened(playerID, sourceID)
	nestedRef := ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: bagID}
	open.OpenRef(playerID, nestedRef)
	validator := NewValidator()
	_, err := validator.ResolveContainer(w, &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(sourceID)}, playerID, player)
	require.NotNil(t, err)
	_, err = validator.ResolveContainer(w, &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagID)}, playerID, player)
	require.NotNil(t, err)
	open.CloseAllForPlayer(playerID)
	_, err = validator.ResolveContainer(w, &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagID)}, playerID, player)
	require.NotNil(t, err)

	// No player capacity remains. GiveItem must not fall back to the stale bag.
	for i := uint8(0); i < 2; i++ {
		addItemToContainer(w, root, components.InvItem{ItemID: 9100 + types.EntityID(i), TypeID: 202, Quantity: 1, W: 1, H: 1, X: i})
	}
	addItemToContainer(w, hand, components.InvItem{ItemID: 9200, TypeID: 202, Quantity: 1, W: 1, H: 1})
	executor := NewInventoryExecutor(zap.NewNop(), &sequentialIDAllocator{next: 9300}, nil, nil, nil)
	result := executor.GiveItem(w, playerID, player, "wheat_seed_mini", 1, 10)
	require.False(t, result.Success)
	bag, _ := ecs.GetComponent[components.InventoryContainer](w, nested)
	require.Empty(t, bag.Items)

	addItemToContainer(w, nested, components.InvItem{ItemID: 9400, TypeID: 201, Quantity: 1, Quality: 10, W: 1, H: 1})
	preview := executor.PreviewCraftInputs(w, playerID, player, &craftdefs.CraftDef{Inputs: []craftdefs.CraftInput{{ItemKey: "wheat_seed_mini", Count: 1, QualityWeight: 1}}})
	require.False(t, preview.Success)
	bag, _ = ecs.GetComponent[components.InventoryContainer](w, nested)
	require.Len(t, bag.Items, 1)
	require.Equal(t, uint32(1), bag.Items[0].Quantity)
}

func TestStaleBagLinkCannotAuthorizeItsNestedItems(t *testing.T) {
	w, playerID, player, root, nested, _, _ := setupGiveItemWorld(t)
	ecs.WithComponent(w, root, func(c *components.InventoryContainer) { c.Items = nil })
	childBagID := types.EntityID(9500)
	child := createGridContainer(w, childBagID, 0, 1, 1)
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, childBagID, 0, child)
	addItemToContainer(w, nested, components.InvItem{ItemID: childBagID, TypeID: 200, Quantity: 1, W: 1, H: 1})
	// The old player's link to the removed outer bag is deliberately present.
	// Its contents cannot establish ownership of further nested containers.
	require.False(t, NestedContainerOwnedByPlayer(w, player, childBagID))
	_, err := NewValidator().ResolveContainer(w,
		&netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(childBagID)}, playerID, player)
	require.NotNil(t, err)
}
