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
	refKey := ecs.InventoryRefKey{
		Kind:    constt.InventoryKind(ref.Kind),
		OwnerID: ownerID,
		Key:     ref.InventoryKey,
	}

	// O(1) lookup via InventoryRefIndex
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	handle, found := refIndex.Lookup(refKey.Kind, ownerID, ref.InventoryKey)
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
		// Personal nested (item belongs to player's own inventory tree) remains valid.
		if v.isNestedContainerOwnedByPlayer(w, playerHandle, ownerID) {
			goto authorized
		}

		openState, hasOpenState := ecs.TryGetResource[ecs.OpenContainerState](w)
		if !hasOpenState || !openState.IsRefOpened(playerID, refKey) {
			return nil, NewValidationError(
				netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
				"Cannot access other entity's inventory",
			)
		}

		// Nested container refs must still belong to the currently opened root object.
		if refKey.Kind == constt.InventoryGrid && refKey.Key == 0 {
			rootOwnerID, hasRoot := openState.GetOpenedRoot(playerID)
			if hasRoot && ownerID != rootOwnerID && !v.isNestedContainerUnderRoot(w, rootOwnerID, ownerID) {
				return nil, NewValidationError(
					netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
					"Nested inventory is no longer accessible",
				)
			}
		}
	}

authorized:
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

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, link := range owner.Inventories {
		// Skip the nested container itself; we need a parent container holding this item.
		if link.Kind == constt.InventoryGrid && link.Key == 0 && link.OwnerID == itemOwnerID {
			continue
		}
		if !w.Alive(link.Handle) {
			continue
		}

		container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !ok {
			continue
		}

		for _, item := range container.Items {
			if item.ItemID != itemOwnerID {
				continue
			}
			nestedHandle, nestedFound := refIndex.Lookup(constt.InventoryGrid, itemOwnerID, 0)
			return nestedFound && w.Alive(nestedHandle)
		}
	}
	return false
}

func (v *Validator) isNestedContainerUnderRoot(
	w *ecs.World,
	rootOwnerID types.EntityID,
	nestedOwnerID types.EntityID,
) bool {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, rootOwnerID, 0)
	if !found || !w.Alive(rootHandle) {
		return false
	}

	rootContainer, hasContainer := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if !hasContainer {
		return false
	}

	for _, item := range rootContainer.Items {
		if item.ItemID != nestedOwnerID {
			continue
		}
		nestedHandle, nestedFound := refIndex.Lookup(constt.InventoryGrid, nestedOwnerID, 0)
		return nestedFound && w.Alive(nestedHandle)
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

	// Nested container owner_id == parent item_id. Resolve parent item globally so
	// rules work regardless of where nested inventory is located (player, object, equip, etc.).
	parentTypeID := v.findItemTypeIDInOwner(w, dstInfo.Owner, containerOwnerID)
	if parentTypeID == 0 {
		parentTypeID = v.findItemTypeIDInWorld(w, containerOwnerID)
	}
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

// findItemTypeIDInOwner searches through owner's inventories for an item with the given ItemID
// and returns its TypeID. Returns 0 if not found.
func (v *Validator) findItemTypeIDInOwner(
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

// findItemTypeIDInWorld searches all inventory containers for an item with itemID.
// This covers nested inventories hosted outside player's InventoryOwner (e.g. world objects, equipment, stations).
func (v *Validator) findItemTypeIDInWorld(
	w *ecs.World,
	itemID types.EntityID,
) uint32 {
	if itemID == 0 {
		return 0
	}

	query := ecs.NewQuery(w).With(components.InventoryContainerComponentID)
	var foundTypeID uint32
	query.ForEach(func(h types.Handle) {
		if foundTypeID != 0 {
			return
		}
		container, ok := ecs.GetComponent[components.InventoryContainer](w, h)
		if !ok {
			return
		}
		for _, item := range container.Items {
			if item.ItemID == itemID {
				foundTypeID = item.TypeID
				return
			}
		}
	})

	return foundTypeID
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
