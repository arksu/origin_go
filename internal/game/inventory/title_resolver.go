package inventory

import (
	"fmt"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"
	"strings"
)

// MustResolveGridInventoryTitle resolves a non-empty title for GRID inventory containers.
// Fail-fast by design: invalid links/defs must surface as hard errors.
func MustResolveGridInventoryTitle(w *ecs.World, ownerID types.EntityID) string {
	if w == nil {
		panic("resolve grid inventory title: world is nil")
	}
	if ownerID == 0 {
		panic("resolve grid inventory title: ownerID is 0")
	}

	// Nested container owner is item_id. Resolve item def name first.
	if nestedTypeID, found := findItemTypeIDByItemID(w, ownerID); found {
		itemDef, ok := itemdefs.Global().GetByID(int(nestedTypeID))
		if !ok || itemDef == nil {
			panic(fmt.Sprintf("resolve grid inventory title: item def not found for typeId=%d ownerID=%d", nestedTypeID, ownerID))
		}
		name := strings.TrimSpace(itemDef.Name)
		if name == "" {
			panic(fmt.Sprintf("resolve grid inventory title: empty item def name for typeId=%d ownerID=%d", nestedTypeID, ownerID))
		}
		return name
	}

	// Root container owner is entity_id. Resolve object def name via entity type.
	handle := w.GetHandleByEntityID(ownerID)
	if handle == types.InvalidHandle || !w.Alive(handle) {
		panic(fmt.Sprintf("resolve grid inventory title: owner entity not found ownerID=%d", ownerID))
	}

	entityInfo, ok := ecs.GetComponent[components.EntityInfo](w, handle)
	if !ok {
		panic(fmt.Sprintf("resolve grid inventory title: owner entity has no EntityInfo ownerID=%d", ownerID))
	}
	objectDef, found := objectdefs.Global().GetByID(int(entityInfo.TypeID))
	if !found || objectDef == nil {
		panic(fmt.Sprintf("resolve grid inventory title: object def not found typeId=%d ownerID=%d", entityInfo.TypeID, ownerID))
	}
	name := strings.TrimSpace(objectDef.Name)
	if name == "" {
		panic(fmt.Sprintf("resolve grid inventory title: empty object def name typeId=%d ownerID=%d", entityInfo.TypeID, ownerID))
	}
	return name
}

func findItemTypeIDByItemID(w *ecs.World, itemID types.EntityID) (uint32, bool) {
	if itemID == 0 {
		return 0, false
	}

	var (
		found  bool
		typeID uint32
	)

	ecs.NewQuery(w).
		With(components.InventoryContainerComponentID).
		ForEach(func(h types.Handle) {
			container, ok := ecs.GetComponent[components.InventoryContainer](w, h)
			if !ok {
				return
			}
			if container.Kind == constt.InventoryDroppedItem {
				return
			}
			for _, item := range container.Items {
				if item.ItemID != itemID {
					continue
				}
				if !found {
					found = true
					typeID = item.TypeID
					return
				}
				if typeID != item.TypeID {
					panic(fmt.Sprintf("resolve grid inventory title: ambiguous item type for itemID=%d (%d vs %d)", itemID, typeID, item.TypeID))
				}
			}
		})

	return typeID, found
}
