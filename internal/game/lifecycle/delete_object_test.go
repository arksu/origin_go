package lifecycle

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestDeleteObjectRemovesOwnedRootAndNestedContainers(t *testing.T) {
	w := ecs.NewWorldForTesting()
	entityID := types.EntityID(501)
	objectHandle := w.Spawn(entityID, nil)
	rootHandle := w.SpawnWithoutExternalID()
	nestedHandle := w.SpawnWithoutExternalID()

	ecs.AddComponent(w, rootHandle, components.InventoryContainer{
		OwnerID: entityID,
		Kind:    constt.InventoryDroppedItem,
		Key:     0,
		Items:   []components.InvItem{{ItemID: entityID, TypeID: 1, Quantity: 1}},
	})
	ecs.AddComponent(w, nestedHandle, components.InventoryContainer{
		OwnerID: entityID,
		Kind:    constt.InventoryGrid,
		Key:     0,
	})

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	refIndex.Add(constt.InventoryDroppedItem, entityID, 0, rootHandle)
	refIndex.Add(constt.InventoryGrid, entityID, 0, nestedHandle)

	if !DeleteObject(w, entityID, objectHandle, DeleteObjectOptions{DeleteOwnedInventories: true}) {
		t.Fatal("DeleteObject returned false")
	}
	if w.Alive(objectHandle) || w.Alive(rootHandle) || w.Alive(nestedHandle) {
		t.Fatal("object and every owned inventory container must be despawned")
	}
	if _, found := refIndex.Lookup(constt.InventoryDroppedItem, entityID, 0); found {
		t.Fatal("dropped root inventory ref was not removed")
	}
	if _, found := refIndex.Lookup(constt.InventoryGrid, entityID, 0); found {
		t.Fatal("nested inventory ref was not removed")
	}
}
