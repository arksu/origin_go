package inventory

import (
	"errors"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"
)

var ErrInvalidInventoryTree = errors.New("invalid inventory tree")

type inventorySnapshotFrame struct {
	items []components.InvItem
	data  *InventoryDataV1
}

// SerializeInventoryTree copies every nested inventory into the existing JSON
// representation. The caller holds the owning shard lock. The explicit stack
// bounds call-stack usage; duplicate item IDs and refs reject cycles instead of
// silently truncating the snapshot. No returned value points into ECS storage.
func SerializeInventoryTree(w *ecs.World, root components.InventoryContainer) (InventoryDataV1, error) {
	if w == nil || root.OwnerID == 0 || root.OwnerID > math.MaxInt64 || root.Version > uint64(math.MaxInt) {
		return InventoryDataV1{}, ErrInvalidInventoryTree
	}
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	registry := itemdefs.Global()
	data := inventoryTreeData(root)
	var frameBuffer [4]inventorySnapshotFrame
	frames := append(frameBuffer[:0], inventorySnapshotFrame{items: root.Items})
	var seenItems inventoryTreeVisited[types.EntityID]
	rootKey := ecs.InventoryRefKey{Kind: root.Kind, OwnerID: root.OwnerID, Key: root.Key}
	rootHandle, _ := refs.Lookup(root.Kind, root.OwnerID, root.Key)
	for len(frames) != 0 {
		frame := &frames[len(frames)-1]
		if len(frame.items) == 0 {
			frames = frames[:len(frames)-1]
			continue
		}
		item := frame.items[0]
		frame.items = frame.items[1:]
		if item.ItemID == 0 || item.ItemID > math.MaxInt64 {
			return InventoryDataV1{}, ErrInvalidInventoryTree
		}
		if !seenItems.add(item.ItemID) {
			return InventoryDataV1{}, ErrInvalidInventoryTree
		}
		dbItem := InventoryItemV1{
			ItemID: uint64(item.ItemID), TypeID: item.TypeID, Quality: item.Quality,
			Quantity: item.Quantity, X: item.X, Y: item.Y, EquipSlot: EquipSlotToString(item.EquipSlot),
		}
		childKey := ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: item.ItemID, Key: 0}
		childHandle, hasChild := refs.Lookup(childKey.Kind, childKey.OwnerID, childKey.Key)
		var definition *itemdefs.ItemDef
		knownDefinition := false
		if registry != nil {
			definition, knownDefinition = registry.GetByID(int(item.TypeID))
		}
		if !hasChild {
			if knownDefinition && definition.Container != nil {
				return InventoryDataV1{}, ErrInvalidInventoryTree
			}
			appendInventoryTreeItem(&data, frame.data, dbItem)
			continue
		}
		child, ok := ecs.GetComponent[components.InventoryContainer](w, childHandle)
		if !w.Alive(childHandle) || !ok || child.OwnerID != item.ItemID || child.Kind != constt.InventoryGrid ||
			child.Key != 0 || child.Version > uint64(math.MaxInt) || childKey == rootKey || childHandle == rootHandle {
			return InventoryDataV1{}, ErrInvalidInventoryTree
		}
		if knownDefinition && definition.Container != nil && (item.Quantity != 1 ||
			int(child.Width) != definition.Container.Size.W || int(child.Height) != definition.Container.Size.H) {
			return InventoryDataV1{}, ErrInvalidInventoryTree
		}
		childData := inventoryTreeData(child)
		dbItem.NestedInventory = &childData
		appendInventoryTreeItem(&data, frame.data, dbItem)
		frames = append(frames, inventorySnapshotFrame{items: child.Items, data: &childData})
	}
	return data, nil
}

func inventoryTreeData(container components.InventoryContainer) InventoryDataV1 {
	return InventoryDataV1{
		Kind: uint8(container.Kind), Key: container.Key, Width: container.Width,
		Height: container.Height, Version: int(container.Version),
		Items: make([]InventoryItemV1, 0, len(container.Items)),
	}
}

// Most inventories fit entirely in stack-backed traversal/identity buffers.
// Larger trees switch to hashed lookups, keeping traversal linear in their size.
type inventoryTreeVisited[T comparable] struct {
	values   [32]T
	count    int
	overflow map[T]struct{}
}

func (v *inventoryTreeVisited[T]) add(value T) bool {
	if v.overflow != nil {
		return v.addOverflow(value)
	}
	for _, existing := range v.values[:v.count] {
		if existing == value {
			return false
		}
	}
	if v.count < len(v.values) {
		v.values[v.count] = value
		v.count++
		return true
	}
	return v.addOverflow(value)
}

func (v *inventoryTreeVisited[T]) addOverflow(value T) bool {
	if v.overflow != nil {
		if _, exists := v.overflow[value]; exists {
			return false
		}
		v.overflow[value] = struct{}{}
		return true
	}
	v.overflow = make(map[T]struct{}, 2*len(v.values))
	for _, existing := range v.values {
		v.overflow[existing] = struct{}{}
	}
	v.overflow[value] = struct{}{}
	return true
}

func appendInventoryTreeItem(root, nested *InventoryDataV1, item InventoryItemV1) {
	if nested == nil {
		nested = root
	}
	nested.Items = append(nested.Items, item)
}
