package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestInventoryReservationGuardsExternalRootAndOpenedNested(t *testing.T) {
	w, playerID, player := setupTestWorld(t)
	ecs.AddComponent(w, player, components.InventoryOwner{})
	ownerID := types.EntityID(2000)
	owner := w.Spawn(ownerID, nil)
	root := createGridContainer(w, ownerID, 0, 4, 4)
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, ownerID, 0, root)
	addItemToContainer(w, root, components.InvItem{ItemID: 3000, TypeID: 1, Quantity: 1, W: 1, H: 1})
	nested := createGridContainer(w, 3000, 0, 2, 2)
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, 3000, 0, nested)
	open := ecs.GetResource[ecs.OpenContainerState](w)
	open.SetRootOpened(playerID, ownerID)
	for _, id := range []types.EntityID{ownerID, 3000} {
		open.OpenRef(playerID, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: id})
	}
	validator := NewValidator()
	for _, id := range []types.EntityID{ownerID, 3000} {
		_, err := validator.ResolveContainer(w, &netproto.InventoryRef{OwnerId: uint64(id)}, playerID, player)
		require.Nil(t, err)
	}
	require.True(t, ecs.ReserveInventoryOwner(w, ownerID, owner))
	for _, id := range []types.EntityID{ownerID, 3000} {
		info, err := validator.ResolveContainer(w, &netproto.InventoryRef{OwnerId: uint64(id)}, playerID, player)
		require.Nil(t, info)
		require.NotNil(t, err)
		require.Equal(t, netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, err.Code)
	}
	require.True(t, ecs.ReleaseInventoryOwner(w, ownerID, owner))
	for _, id := range []types.EntityID{ownerID, 3000} {
		_, err := validator.ResolveContainer(w, &netproto.InventoryRef{OwnerId: uint64(id)}, playerID, player)
		require.Nil(t, err)
	}
}
