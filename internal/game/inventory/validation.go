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
	handle, found := refIndex.Lookup(constt.InventoryKind(ref.Kind), ownerID, ref.InventoryKey)
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
	w *ecs.World,
	item *components.InvItem,
	dstInfo *ContainerInfo,
	dstEquipSlot netproto.EquipSlot,
) *ValidationError {
	itemDef, ok := itemdefs.Global().GetByID(int(item.TypeID))
	if !ok {
		return NewValidationError(
			netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			"Item definition not found",
		)
	}

	switch dstInfo.Container.Kind {
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
		if err := v.validateContainerRules(w, itemDef, dstInfo); err != nil {
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
	w *ecs.World,
	itemDef *itemdefs.ItemDef,
	dstInfo *ContainerInfo,
) *ValidationError {
	containerOwnerID := dstInfo.Container.OwnerID

	// Find the parent item's TypeID by searching through the owner's inventories
	parentTypeID := v.findItemTypeID(w, dstInfo.Owner, containerOwnerID)
	if parentTypeID == 0 {
		return nil // Root container (backpack) — no content rules
	}

	parentDef, ok := itemdefs.Global().GetByID(int(parentTypeID))
	if !ok || parentDef.Container == nil {
		return nil // No container definition — no rules
	}

	rules := &parentDef.Container.Rules
	return v.checkContentRules(rules, itemDef)
}

// findItemTypeID searches through the owner's inventories for an item with the given ItemID
// and returns its TypeID. Returns 0 if not found.
func (v *Validator) findItemTypeID(
	w *ecs.World,
	owner *components.InventoryOwner,
	itemID types.EntityID,
) uint32 {
	if owner == nil {
		return 0
	}
	for _, link := range owner.Inventories {
		container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !ok {
			continue
		}
		for _, item := range container.Items {
			if item.ItemID == itemID {
				return item.TypeID
			}
		}
	}
	return 0
}

// checkContentRules validates an item definition against container content rules.
func (v *Validator) checkContentRules(
	rules *itemdefs.ContentRules,
	itemDef *itemdefs.ItemDef,
) *ValidationError {
	// DenyItemKeys: reject if item key is blacklisted
	for _, denied := range rules.DenyItemKeys {
		if itemDef.Key == denied {
			return NewValidationError(
				netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
				"Item is not allowed in this container",
			)
		}
	}

	// AllowItemKeys: if non-empty, item key must be in the list
	if len(rules.AllowItemKeys) > 0 {
		for _, allowed := range rules.AllowItemKeys {
			if itemDef.Key == allowed {
				return nil
			}
		}
		// Key not in whitelist — reject (AllowItemKeys is strict)
		return NewValidationError(
			netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			"Item is not allowed in this container",
		)
	}

	// DenyTags: reject if item has any denied tag
	for _, denied := range rules.DenyTags {
		for _, tag := range itemDef.Tags {
			if tag == denied {
				return NewValidationError(
					netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
					"Item is not allowed in this container",
				)
			}
		}
	}

	// AllowTags: if non-empty, item must have at least one allowed tag
	if len(rules.AllowTags) > 0 {
		for _, allowed := range rules.AllowTags {
			for _, tag := range itemDef.Tags {
				if tag == allowed {
					return nil
				}
			}
		}
		return NewValidationError(
			netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			"Item is not allowed in this container",
		)
	}

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
