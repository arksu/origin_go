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

func TestDeleteObjectRemovesNestedContainersOwnedByDeletedItems(t *testing.T) {
	w := ecs.NewWorldForTesting()
	objectID := types.EntityID(501)
	itemID := types.EntityID(502)
	objectHandle := w.Spawn(objectID, nil)
	rootHandle := w.SpawnWithoutExternalID()
	nestedHandle := w.SpawnWithoutExternalID()

	ecs.AddComponent(w, rootHandle, components.InventoryContainer{
		OwnerID: objectID,
		Kind:    constt.InventoryGrid,
		Items:   []components.InvItem{{ItemID: itemID, TypeID: 1, Quantity: 1}},
	})
	ecs.AddComponent(w, nestedHandle, components.InventoryContainer{OwnerID: itemID, Kind: constt.InventoryGrid})

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	refIndex.Add(constt.InventoryGrid, objectID, 0, rootHandle)
	refIndex.Add(constt.InventoryGrid, itemID, 0, nestedHandle)

	if !DeleteObject(w, objectID, objectHandle, DeleteObjectOptions{DeleteOwnedInventories: true}) {
		t.Fatal("DeleteObject returned false")
	}
	if w.Alive(nestedHandle) {
		t.Fatal("nested inventory owned by a deleted item must be despawned")
	}
	if _, found := refIndex.Lookup(constt.InventoryGrid, itemID, 0); found {
		t.Fatal("nested inventory ref must be removed")
	}
}

func TestDeleteOwnedInventoryContainersReleasesDeepTreeAndPreservesForeignEntity(t *testing.T) {
	w := ecs.NewWorldForTesting()
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	var tree []types.Handle
	for ownerID := types.EntityID(501); ownerID < 505; ownerID++ {
		h := w.SpawnWithoutExternalID()
		container := components.InventoryContainer{OwnerID: ownerID, Kind: constt.InventoryGrid}
		if ownerID < 504 {
			container.Items = []components.InvItem{{ItemID: ownerID + 1}}
		}
		ecs.AddComponent(w, h, container)
		refs.Add(container.Kind, ownerID, 0, h)
		tree = append(tree, h)
	}
	foreign := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, foreign, components.InventoryContainer{OwnerID: 999, Kind: constt.InventoryGrid, Key: 1})
	refs.Add(constt.InventoryGrid, 503, 1, foreign)
	DeleteOwnedInventoryContainers(w, 501)
	for index, h := range tree {
		if w.Alive(h) {
			t.Fatalf("tree container %d remains alive", index)
		}
		if _, found := refs.Lookup(constt.InventoryGrid, types.EntityID(501+index), 0); found {
			t.Fatalf("tree ref %d remains indexed", index)
		}
	}
	if !w.Alive(foreign) {
		t.Fatal("mismatched owner ref deleted a foreign inventory entity")
	}
}
