package game

import (
	"fmt"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/types"
	"slices"
)

type combatWeapon struct {
	combat.Weapon
	ItemID types.EntityID
	Slot   string
}

// CombatStrengthProvider is sampled at impact so later attribute modifiers retain precision.
type CombatStrengthProvider func(*ecs.World, types.Handle) (float64, error)

func baseCombatStrength(world *ecs.World, actor types.Handle) (float64, error) {
	profile, exists := ecs.GetComponent[components.CharacterProfile](world, actor)
	if !exists {
		return 0, fmt.Errorf("combat actor has no strength profile")
	}
	return float64(characterattrs.Get(profile.Attributes, characterattrs.STR)), nil
}

func resolveCombatWeapon(world *ecs.World, actorID types.EntityID, definition *actiondefs.Definition) (combatWeapon, error) {
	items := itemdefs.Global()
	if world == nil || definition == nil || items == nil {
		return combatWeapon{}, fmt.Errorf("combat weapon definitions unavailable")
	}
	index := ecs.GetResource[ecs.InventoryRefIndex](world)
	handle, found := index.Lookup(constt.InventoryEquipment, actorID, 0)
	if !found {
		return combatWeapon{}, fmt.Errorf("combat requires equipped weapon")
	}
	container, found := ecs.GetComponent[components.InventoryContainer](world, handle)
	if !found {
		return combatWeapon{}, fmt.Errorf("combat equipment unavailable")
	}
	for _, slot := range []string{"right_hand", "left_hand"} {
		for _, item := range container.Items {
			if item.EquipSlot != inventory.StringToEquipSlot(slot) {
				continue
			}
			itemDef, exists := items.GetByID(int(item.TypeID))
			if !exists || itemDef.Weapon == nil {
				continue
			}
			compatible := false
			for _, requirement := range definition.Requirements.Equipment {
				if slices.Contains(requirement.Slots, slot) && ((requirement.ItemKey != "" && itemDef.Key == requirement.ItemKey) || (requirement.ItemTag != "" && slices.Contains(itemDef.Tags, requirement.ItemTag))) {
					compatible = true
					break
				}
			}
			if !compatible {
				continue
			}
			weapon := combatWeapon{Weapon: combat.Weapon{BaseDamage: itemDef.Weapon.BaseDamage, Range: itemDef.Weapon.Range, Quality: float64(item.Quality)}, ItemID: item.ItemID, Slot: slot}
			if item.ItemID == 0 {
				return combatWeapon{}, fmt.Errorf("equipped combat weapon has invalid identity")
			}
			if err := weapon.Validate(); err != nil {
				return combatWeapon{}, err
			}
			return weapon, nil
		}
	}
	return combatWeapon{}, fmt.Errorf("combat requires a compatible equipped weapon")
}
