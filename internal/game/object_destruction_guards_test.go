package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestPendingObjectCannotTransformLiftOrOfferContextActions(t *testing.T) {
	w := ecs.NewWorldForTesting()
	target := w.Spawn(10, nil)
	ecs.AddComponent(w, target, components.EntityInfo{TypeID: 100, Behaviors: []string{"lift", "container"}})
	ecs.AddComponent(w, target, components.ObjectInternalState{HP: 0, HasHP: true, IsDirty: true})
	ecs.InitResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{target: true}})
	before, _ := ecs.GetComponent[components.EntityInfo](w, target)
	require.False(t, gameworld.TransformObjectToDefInPlace(w, 10, target, &objectdefs.ObjectDef{DefID: 101, Key: "new", HP: 50}, gameworld.TransformObjectInPlaceOptions{}))
	after, _ := ecs.GetComponent[components.EntityInfo](w, target)
	require.Equal(t, before, after)
	require.False(t, (&LiftService{}).isLiftableTarget(w, target))
	ctx := &ContextActionService{world: w}
	require.Nil(t, ctx.ComputeActions(w, 1, types.InvalidHandle, 10, target))
	require.False(t, ctx.ExecuteAction(w, 1, types.InvalidHandle, 10, target, "open"))
	require.Empty(t, ctx.behaviorOrder(target, w))
}

func TestDestroyedWorldBagCannotReopenThroughStalePersonalLink(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	bag := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, bag, components.InventoryContainer{OwnerID: 50, Kind: constt.InventoryGrid, Items: []components.InvItem{{ItemID: 51, TypeID: 2, Quantity: 1}}})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, 50, 0, bag)
	ecs.AddComponent(w, player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: 50, Handle: bag}}})
	service := NewOpenContainerService(w, nil, &testContainerSender{}, nil)
	err := service.HandleOpenRequest(w, 1, player, &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 50})
	require.NotNil(t, err)
	require.Equal(t, netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, err.Code)
	require.False(t, ecs.GetResource[ecs.OpenContainerState](w).IsRefOpened(1, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 50}))
}
