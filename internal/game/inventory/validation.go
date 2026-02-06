package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type ValidationError struct {
	Code    netproto.ErrorCode
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func NewValidationError(code netproto.ErrorCode, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}

// Validator provides validation logic for inventory operations
type Validator struct{}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{}
}

type ContainerInfo struct {
	Handle    types.Handle
	Container *components.InventoryContainer
	Owner     *components.InventoryOwner
}

func (v *Validator) ValidateExpectedVersions(
	w *ecs.World,
	expected []*netproto.InventoryExpected,
	containers map[string]*ContainerInfo,
) *ValidationError {
	for _, exp := range expected {
		key := makeContainerKey(exp.Ref)
		info, ok := containers[key]
		if !ok {
			return NewValidationError(
				netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
				"Container not found for version check",
			)
		}
		if info.Container.Version != exp.ExpectedRevision {
			return NewValidationError(
				netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
				"Inventory version mismatch",
			)
		}
	}
	return nil
}

func (v *Validator) ResolveContainer(
	w *ecs.World,
	ref *netproto.InventoryRef,
	playerID types.EntityID,
	playerHandle types.Handle,
) (*ContainerInfo, *ValidationError) {
	if ref == nil {
		return nil, NewValidationError(
			netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			"Container reference is nil",
		)
	}

	ownerID := types.EntityID(ref.OwnerId)

	// O(1) lookup via InventoryRefIndex
	refIndex := w.InventoryRefIndex()
	handle, found := refIndex.Lookup(uint8(ref.Kind), ownerID, ref.InventoryKey)
	if !found || !w.Alive(handle) {
		return nil, NewValidationError(
			netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			"Container not found",
		)
	}

	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, handle)
	if !hasContainer {
		return nil, NewValidationError(
			netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			"Container not found",
		)
	}

	// Authorization: player can access own inventories and nested containers of own items
	if ownerID != playerID {
		if !v.isNestedContainerOwnedByPlayer(w, playerHandle, ownerID) {
			return nil, NewValidationError(
				netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
				"Cannot access other entity's inventory",
			)
		}
	}

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return nil, NewValidationError(
			netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			"Player has no inventory owner component",
		)
	}

	return &ContainerInfo{
		Handle:    handle,
		Container: &container,
		Owner:     &owner,
	}, nil
}

// isNestedContainerOwnedByPlayer checks if ownerID (item_id) belongs to an item in player's inventories
func (v *Validator) isNestedContainerOwnedByPlayer(w *ecs.World, playerHandle types.Handle, itemOwnerID types.EntityID) bool {
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return false
	}

	for _, link := range owner.Inventories {
		// Skip nested containers themselves (they have OwnerID = itemID)
		if link.OwnerID != types.EntityID(0) && link.OwnerID == itemOwnerID {
			return true
		}
	}
	return false
}

func (v *Validator) FindItemInContainer(
	container *components.InventoryContainer,
	itemID types.EntityID,
) (int, *components.InvItem) {
	for i := range container.Items {
		if container.Items[i].ItemID == itemID {
			return i, &container.Items[i]
		}
	}
	return -1, nil
}

func (v *Validator) ValidateItemAllowedInContainer(
	item *components.InvItem,
	dstContainer *components.InventoryContainer,
	dstEquipSlot netproto.EquipSlot,
) *ValidationError {
	itemDef, ok := itemdefs.Global().GetByID(int(item.TypeID))
	if !ok {
		return NewValidationError(
			netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			"Item definition not found",
		)
	}

	switch dstContainer.Kind {
	case constt.InventoryHand:
		if itemDef.Allowed.Hand != nil && !*itemDef.Allowed.Hand {
			return NewValidationError(
				netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
				"Item cannot be held in hand",
			)
		}

	case constt.InventoryGrid:
		if itemDef.Allowed.Grid != nil && !*itemDef.Allowed.Grid {
			return NewValidationError(
				netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
				"Item cannot be placed in grid inventory",
			)
		}
		// Check container rules if this is a nested container
		if err := v.validateContainerRules(item, dstContainer); err != nil {
			return err
		}

	case constt.InventoryEquipment:
		if !v.isEquipSlotAllowed(itemDef, dstEquipSlot) {
			return NewValidationError(
				netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
				"Item cannot be equipped in this slot",
			)
		}
	}

	return nil
}

func (v *Validator) validateContainerRules(
	item *components.InvItem,
	dstContainer *components.InventoryContainer,
) *ValidationError {
	// Find the item that owns this container (if it's a nested container)
	// For now, we need to check if the container has rules via its owner item
	// This requires looking up the parent item's definition

	// Get the item definition for the item being placed
	_, ok := itemdefs.Global().GetByID(int(item.TypeID))
	if !ok {
		return nil // Already validated above
	}

	// Find the container's parent item definition to get rules
	// The container's OwnerID is the item_id of the parent item
	// We need to find that item's type_id to get its ContainerDef

	// For root containers (backpack, etc.), OwnerID is the character ID
	// For nested containers, OwnerID is the item_id
	// We can't easily get the parent item's definition here without more context
	// So we'll skip container rules validation for now and implement it when needed
	return nil
}

func (v *Validator) isEquipSlotAllowed(itemDef *itemdefs.ItemDef, slot netproto.EquipSlot) bool {
	if len(itemDef.Allowed.EquipmentSlots) == 0 {
		return false
	}

	slotName := equipSlotToString(slot)
	for _, allowed := range itemDef.Allowed.EquipmentSlots {
		if allowed == slotName {
			return true
		}
	}
	return false
}

func equipSlotToString(slot netproto.EquipSlot) string {
	switch slot {
	case netproto.EquipSlot_EQUIP_SLOT_HEAD:
		return "head"
	case netproto.EquipSlot_EQUIP_SLOT_CHEST:
		return "chest"
	case netproto.EquipSlot_EQUIP_SLOT_LEGS:
		return "legs"
	case netproto.EquipSlot_EQUIP_SLOT_FEET:
		return "feet"
	case netproto.EquipSlot_EQUIP_SLOT_HANDS:
		return "hands"
	case netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND:
		return "left_hand"
	case netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND:
		return "right_hand"
	case netproto.EquipSlot_EQUIP_SLOT_BACK:
		return "back"
	case netproto.EquipSlot_EQUIP_SLOT_NECK:
		return "neck"
	case netproto.EquipSlot_EQUIP_SLOT_RING_1:
		return "ring1"
	case netproto.EquipSlot_EQUIP_SLOT_RING_2:
		return "ring2"
	default:
		return ""
	}
}

func makeContainerKey(ref *netproto.InventoryRef) string {
	if ref == nil {
		return ""
	}
	// Create a unique key for the container based on owner, kind, and key
	return string(rune(ref.OwnerId)) + "_" + string(rune(ref.Kind)) + "_" + string(rune(ref.InventoryKey))
}

func MakeContainerKeyFromInfo(ownerID types.EntityID, kind constt.InventoryKind, key uint32) string {
	return string(rune(ownerID)) + "_" + string(rune(kind)) + "_" + string(rune(key))
}
