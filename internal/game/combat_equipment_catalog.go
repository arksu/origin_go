package game

import (
	"fmt"
	"math"
	"strings"

	"origin/internal/actiondefs"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
)

type combatSlotMask uint16

// Equipment slot 5 is reserved. Keep iteration in protocol slot order so armor
// sums have the same floating-point result regardless of inventory slice order.
var combatEquipmentSlots = [...]netproto.EquipSlot{
	netproto.EquipSlot_EQUIP_SLOT_HEAD, netproto.EquipSlot_EQUIP_SLOT_CHEST,
	netproto.EquipSlot_EQUIP_SLOT_LEGS, netproto.EquipSlot_EQUIP_SLOT_FEET,
	netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND,
	netproto.EquipSlot_EQUIP_SLOT_BACK, netproto.EquipSlot_EQUIP_SLOT_NECK,
	netproto.EquipSlot_EQUIP_SLOT_RING_1, netproto.EquipSlot_EQUIP_SLOT_RING_2,
}

const combatSlotArraySize = int(netproto.EquipSlot_EQUIP_SLOT_RING_2) + 1

func combatSlotBit(slot netproto.EquipSlot) combatSlotMask {
	if slot <= netproto.EquipSlot_EQUIP_SLOT_NONE || int(slot) >= combatSlotArraySize || slot == 5 {
		return 0
	}
	return 1 << uint(slot)
}

type combatItemParameters struct {
	baseDamage, baseArmor float64
	allowedSlots          combatSlotMask
}

// CombatEquipmentCatalog owns an immutable snapshot of item definitions. It
// may be shared by resolvers in different shards. Rebuild it when defs change.
type CombatEquipmentCatalog struct {
	items map[uint32]combatItemParameters
	keys  map[string]uint32
	tags  map[string][]uint32
}

func NewCombatEquipmentCatalog(items *itemdefs.Registry) (*CombatEquipmentCatalog, error) {
	if items == nil {
		return nil, fmt.Errorf("combat equipment catalog requires item definitions")
	}
	catalog := &CombatEquipmentCatalog{
		items: make(map[uint32]combatItemParameters, items.Count()),
		keys:  make(map[string]uint32, items.Count()),
		tags:  make(map[string][]uint32),
	}
	for _, definition := range items.All() {
		if definition.DefID <= 0 || uint64(definition.DefID) > math.MaxUint32 {
			return nil, fmt.Errorf("combat equipment: item %q has an invalid type ID", definition.Key)
		}
		if _, exists := catalog.keys[definition.Key]; exists {
			return nil, fmt.Errorf("combat equipment: duplicate item key %q", definition.Key)
		}
		parameters := combatItemParameters{}
		for _, name := range definition.Allowed.EquipmentSlots {
			bit := combatSlotBit(inventory.StringToEquipSlot(name))
			if bit == 0 || parameters.allowedSlots&bit != 0 {
				return nil, fmt.Errorf("combat equipment: item %q has an invalid or duplicate slot %q", definition.Key, name)
			}
			parameters.allowedSlots |= bit
		}
		if definition.Melee != nil {
			if !positiveCombatParameter(definition.Melee.BaseDamage) {
				return nil, fmt.Errorf("combat equipment: item %q has invalid base damage", definition.Key)
			}
			parameters.baseDamage = definition.Melee.BaseDamage
		}
		if definition.Armor != nil {
			if !positiveCombatParameter(definition.Armor.BaseArmor) {
				return nil, fmt.Errorf("combat equipment: item %q has invalid base armor", definition.Key)
			}
			parameters.baseArmor = definition.Armor.BaseArmor
		}
		typeID := uint32(definition.DefID)
		catalog.items[typeID] = parameters
		catalog.keys[definition.Key] = typeID
		for _, tag := range definition.Tags {
			catalog.tags[tag] = append(catalog.tags[tag], typeID)
		}
	}
	return catalog, nil
}

// PreparedMeleeAction contains only static, compiled selectors. Other equipment
// requirements remain the responsibility of the action availability checks.
type PreparedMeleeAction struct {
	catalog    *CombatEquipmentCatalog
	multiplier float64
	slots      combatSlotMask
	types      map[uint32]struct{}
}

func (catalog *CombatEquipmentCatalog) PrepareMeleeAction(definition *actiondefs.Definition) (*PreparedMeleeAction, error) {
	if catalog == nil || catalog.items == nil || definition == nil || definition.Target.Kind != actiondefs.TargetDirection ||
		definition.Combat == nil || !positiveCombatParameter(definition.Combat.DamageMultiplier) {
		return nil, fmt.Errorf("melee preparation requires a catalog and a valid combat action")
	}
	var source *actiondefs.EquipmentRequirement
	for index := range definition.Requirements.Equipment {
		requirement := &definition.Requirements.Equipment[index]
		if requirement.DamageSource {
			if source != nil {
				return nil, fmt.Errorf("melee action %q has multiple damage sources", definition.ID)
			}
			source = requirement
		}
	}
	if source == nil {
		return nil, fmt.Errorf("melee action %q requires exactly one damage source", definition.ID)
	}
	key, tag := strings.TrimSpace(source.ItemKey), strings.TrimSpace(source.ItemTag)
	if (key == "") == (tag == "") || len(source.Slots) == 0 {
		return nil, fmt.Errorf("melee action %q has an invalid damage source selector", definition.ID)
	}
	action := &PreparedMeleeAction{catalog: catalog, multiplier: definition.Combat.DamageMultiplier, types: make(map[uint32]struct{})}
	for _, name := range source.Slots {
		bit := combatSlotBit(inventory.StringToEquipSlot(strings.TrimSpace(name)))
		if bit == 0 || action.slots&bit != 0 {
			return nil, fmt.Errorf("melee action %q has an invalid or duplicate source slot %q", definition.ID, name)
		}
		action.slots |= bit
	}
	if key != "" {
		if typeID, exists := catalog.keys[key]; exists {
			action.includeType(typeID)
		}
	} else {
		for _, typeID := range catalog.tags[tag] {
			action.includeType(typeID)
		}
	}
	if len(action.types) == 0 {
		return nil, fmt.Errorf("melee action %q selects no equipable melee item", definition.ID)
	}
	return action, nil
}

func (action *PreparedMeleeAction) includeType(typeID uint32) {
	parameters := action.catalog.items[typeID]
	if parameters.baseDamage > 0 && parameters.allowedSlots&action.slots != 0 {
		action.types[typeID] = struct{}{}
	}
}

func positiveCombatParameter(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
