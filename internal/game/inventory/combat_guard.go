package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
)

func combatEquipmentLocked(world *ecs.World, container *components.InventoryContainer, slot netproto.EquipSlot) bool {
	if container == nil || container.Kind != constt.InventoryEquipment || (slot != netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND && slot != netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND) {
		return false
	}
	return components.CombatCommitted(world, world.GetHandleByEntityID(container.OwnerID))
}

func combatInventoryRejected() *OperationResult {
	return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Combat equipment is locked until recovery ends"}
}
