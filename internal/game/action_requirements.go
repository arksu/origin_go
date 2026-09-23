package game

import (
	"slices"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/types"
)

func (service *ActionService) UnavailableReason(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, definition *actiondefs.Definition) string {
	if service == nil || world == nil || definition == nil || !world.Alive(playerHandle) {
		return "ACTION_UNAVAILABLE"
	}
	if reason := requirementsReason(world, playerID, playerHandle, definition); reason != "" {
		return reason
	}
	handler := service.handlers[definition.ID]
	if handler == nil {
		return "ACTION_UNAVAILABLE"
	}
	return handler.UnavailableReason(world, playerID, playerHandle)
}

func requirementsReason(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, definition *actiondefs.Definition) string {
	if len(definition.Requirements.Skills) > 0 {
		profile, exists := ecs.GetComponent[components.CharacterProfile](world, playerHandle)
		if !exists {
			return "ACTION_REQUIRES_SKILL"
		}
		for _, skill := range definition.Requirements.Skills {
			if !slices.Contains(profile.Skills, skill) {
				return "ACTION_REQUIRES_SKILL"
			}
		}
	}
	if len(definition.Requirements.Equipment) > 0 {
		if !hasRequiredEquipment(world, playerID, definition.Requirements.Equipment) {
			return "ACTION_REQUIRES_EQUIPMENT"
		}
	}
	if definition.Execution.Stamina > 0 {
		stats, exists := ecs.GetComponent[components.EntityStats](world, playerHandle)
		if !exists || stats.Stamina < definition.Execution.Stamina {
			return "LOW_STAMINA"
		}
	}
	return ""
}

func hasRequiredEquipment(world *ecs.World, playerID types.EntityID, requirements []actiondefs.EquipmentRequirement) bool {
	index := ecs.GetResource[ecs.InventoryRefIndex](world)
	containerHandle, exists := index.Lookup(constt.InventoryEquipment, playerID, 0)
	if !exists || !world.Alive(containerHandle) {
		return false
	}
	container, exists := ecs.GetComponent[components.InventoryContainer](world, containerHandle)
	if !exists {
		return false
	}
	items := itemdefs.Global()
	if items == nil {
		return false
	}
	for _, requirement := range requirements {
		matched := false
		for _, item := range container.Items {
			if !matchesEquipmentSlot(item, requirement.Slots) {
				continue
			}
			definition, found := items.GetByID(int(item.TypeID))
			if !found {
				continue
			}
			if requirement.ItemKey != "" && definition.Key == requirement.ItemKey ||
				requirement.ItemTag != "" && slices.Contains(definition.Tags, requirement.ItemTag) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func matchesEquipmentSlot(item components.InvItem, slots []string) bool {
	for _, slot := range slots {
		if item.EquipSlot == inventory.StringToEquipSlot(slot) {
			return true
		}
	}
	return false
}

func (service *ActionService) chargeStamina(world *ecs.World, playerHandle types.Handle, cost float64) bool {
	if cost == 0 {
		return true
	}
	return behaviors.ConsumePlayerActionStamina(world, playerHandle, cost)
}
