package game

import (
	"errors"
	"math"

	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

var (
	ErrInvalidCombatEquipment = errors.New("combat equipment: invalid equipment state")
	ErrInvalidCombatOwner     = errors.New("combat equipment: invalid owner")
	ErrInvalidMeleeAction     = errors.New("combat equipment: action belongs to another or invalid catalog")
	ErrMeleeWeaponUnavailable = errors.New("combat equipment: melee weapon unavailable")
	ErrInvalidCombatResolver  = errors.New("combat equipment: resolver requires a world, inventory index and catalog")
)

type EquippedMelee struct {
	ItemID     types.EntityID
	TypeID     uint32
	BaseDamage float64
	Quality    float64
	RawDamage  float64
}

// CombatEquipmentResolver is bound to one World and its inventory index and
// component storages. Construct outside the tick path; call only while holding
// the owning shard's lock. Recreate after replacing the World or definitions.
// It retains no dynamic equipment state between calls.
type CombatEquipmentResolver struct {
	world      *ecs.World
	catalog    *CombatEquipmentCatalog
	index      *ecs.InventoryRefIndex
	identities *ecs.ComponentStorage[ecs.ExternalID]
	containers *ecs.ComponentStorage[components.InventoryContainer]
}

func NewCombatEquipmentResolver(world *ecs.World, catalog *CombatEquipmentCatalog) (*CombatEquipmentResolver, error) {
	if world == nil || catalog == nil || catalog.items == nil {
		return nil, ErrInvalidCombatResolver
	}
	index, exists := ecs.TryGetResource[ecs.InventoryRefIndex](world)
	if !exists {
		return nil, ErrInvalidCombatResolver
	}
	return &CombatEquipmentResolver{
		world: world, catalog: catalog, index: index,
		identities: ecs.GetOrCreateStorage[ecs.ExternalID](world),
		containers: ecs.GetOrCreateStorage[components.InventoryContainer](world),
	}, nil
}

type combatEquippedItem struct {
	itemID     types.EntityID
	typeID     uint32
	quality    uint32
	parameters combatItemParameters
}

// readEquipment makes one index lookup and one bounded pass over the container.
// IDs are inventory instance IDs, not handles of independent ECS entities.
func (resolver *CombatEquipmentResolver) readEquipment(owner types.Handle, slots *[combatSlotArraySize]combatEquippedItem) error {
	if !resolver.world.Alive(owner) {
		return ErrInvalidCombatOwner
	}
	identity, exists := resolver.identities.Get(owner)
	if !exists || identity.ID == 0 {
		return ErrInvalidCombatOwner
	}
	handle, exists := resolver.index.Lookup(constt.InventoryEquipment, identity.ID, 0)
	if !exists {
		return nil
	}
	if !resolver.world.Alive(handle) {
		return ErrInvalidCombatEquipment
	}
	container, exists := resolver.containers.Get(handle)
	if !exists || container.OwnerID != identity.ID || container.Kind != constt.InventoryEquipment || container.Key != 0 ||
		len(container.Items) > len(combatEquipmentSlots) {
		return ErrInvalidCombatEquipment
	}
	var occupied combatSlotMask
	var seen [len(combatEquipmentSlots)]types.EntityID
	for index, item := range container.Items {
		bit := combatSlotBit(item.EquipSlot)
		parameters, known := resolver.catalog.items[item.TypeID]
		if item.ItemID == 0 || bit == 0 || occupied&bit != 0 || !known || parameters.allowedSlots&bit == 0 {
			return ErrInvalidCombatEquipment
		}
		for _, id := range seen[:index] {
			if id == item.ItemID {
				return ErrInvalidCombatEquipment
			}
		}
		seen[index] = item.ItemID
		occupied |= bit
		slots[item.EquipSlot] = combatEquippedItem{item.ItemID, item.TypeID, item.Quality, parameters}
	}
	return nil
}

func (resolver *CombatEquipmentResolver) ResolveMeleeWeapon(owner types.Handle, action *PreparedMeleeAction, effectiveSTR float64) (EquippedMelee, error) {
	if action == nil || action.catalog != resolver.catalog || action.types == nil {
		return EquippedMelee{}, ErrInvalidMeleeAction
	}
	if !positiveCombatParameter(effectiveSTR) {
		return EquippedMelee{}, combat.ErrInvalidInput
	}
	var slots [combatSlotArraySize]combatEquippedItem
	if err := resolver.readEquipment(owner, &slots); err != nil {
		return EquippedMelee{}, err
	}
	var selected EquippedMelee
	for _, slot := range combatEquipmentSlots {
		item := slots[slot]
		if item.itemID == 0 || action.slots&combatSlotBit(slot) == 0 {
			continue
		}
		if _, eligible := action.types[item.typeID]; !eligible {
			continue
		}
		quality := float64(item.quality)
		raw, err := combat.MeleeRawDamage(item.parameters.baseDamage, effectiveSTR, quality, action.multiplier)
		if err != nil {
			return EquippedMelee{}, err
		}
		if selected.ItemID == 0 || raw > selected.RawDamage || (raw == selected.RawDamage && item.itemID < selected.ItemID) {
			selected = EquippedMelee{ItemID: item.itemID, TypeID: item.typeID, BaseDamage: item.parameters.baseDamage, Quality: quality, RawDamage: raw}
		}
	}
	if selected.ItemID == 0 {
		return EquippedMelee{}, ErrMeleeWeaponUnavailable
	}
	return selected, nil
}

func (resolver *CombatEquipmentResolver) ResolveArmor(owner types.Handle) (float64, error) {
	var slots [combatSlotArraySize]combatEquippedItem
	if err := resolver.readEquipment(owner, &slots); err != nil {
		return 0, err
	}
	armor := 0.0
	for _, slot := range combatEquipmentSlots {
		item := slots[slot]
		if item.itemID == 0 || item.parameters.baseArmor == 0 {
			continue
		}
		contribution, err := combat.ArmorContribution(item.parameters.baseArmor, float64(item.quality))
		if err != nil {
			return 0, err
		}
		armor += contribution
		if math.IsInf(armor, 0) || math.IsNaN(armor) {
			return 0, combat.ErrNonFiniteResult
		}
	}
	return armor, nil
}
