package game

import (
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/actiondefs"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func combatTestDefinitions() []itemdefs.ItemDef {
	hands := itemdefs.Allowed{EquipmentSlots: []string{"left_hand", "right_hand"}}
	allSlots := make([]string, 0, len(combatEquipmentSlots))
	for _, slot := range combatEquipmentSlots {
		allSlots = append(allSlots, inventory.EquipSlotToString(slot))
	}
	return []itemdefs.ItemDef{
		{DefID: 1, Key: "axe", Tags: []string{"weapon", "axe"}, Allowed: hands, Melee: &itemdefs.MeleeDef{BaseDamage: 6}},
		{DefID: 2, Key: "sword", Tags: []string{"weapon", "sword"}, Allowed: hands, Melee: &itemdefs.MeleeDef{BaseDamage: 4}},
		{DefID: 3, Key: "knife", Tags: []string{"weapon", "knife"}, Allowed: hands, Melee: &itemdefs.MeleeDef{BaseDamage: 3}},
		{DefID: 4, Key: "pike", Tags: []string{"weapon", "pike"}, Allowed: hands, Melee: &itemdefs.MeleeDef{BaseDamage: 7}},
		{DefID: 5, Key: "bow", Tags: []string{"weapon", "bow"}, Allowed: hands},
		{DefID: 6, Key: "shield_blade", Tags: []string{"weapon"}, Allowed: hands, Melee: &itemdefs.MeleeDef{BaseDamage: 5}, Armor: &itemdefs.ArmorDef{BaseArmor: 4}},
		{DefID: 7, Key: "shirt", Allowed: itemdefs.Allowed{EquipmentSlots: []string{"chest"}}, Armor: &itemdefs.ArmorDef{BaseArmor: 4}},
		{DefID: 8, Key: "helmet", Allowed: itemdefs.Allowed{EquipmentSlots: []string{"head"}}, Armor: &itemdefs.ArmorDef{BaseArmor: 8}},
		{DefID: 9, Key: "universal", Tags: []string{"weapon"}, Allowed: itemdefs.Allowed{EquipmentSlots: allSlots}, Melee: &itemdefs.MeleeDef{BaseDamage: 5}, Armor: &itemdefs.ArmorDef{BaseArmor: 1}},
		{DefID: 10, Key: "unequipable", Tags: []string{"weapon"}, Melee: &itemdefs.MeleeDef{BaseDamage: 20}},
	}
}

func combatTestAction() *actiondefs.Definition {
	return &actiondefs.Definition{
		ID: "melee_test", Target: actiondefs.Target{Kind: actiondefs.TargetDirection},
		Combat: &actiondefs.Combat{HitMode: actiondefs.HitAll, DamageMultiplier: 1},
		Requirements: actiondefs.Requirements{Equipment: []actiondefs.EquipmentRequirement{
			{Slots: []string{"left_hand", "right_hand"}, ItemTag: "weapon", DamageSource: true},
		}},
	}
}

type combatEquipmentFixture struct {
	world     *ecs.World
	owner     types.Handle
	container types.Handle
	catalog   *CombatEquipmentCatalog
	resolver  *CombatEquipmentResolver
	action    *PreparedMeleeAction
}

func newCombatEquipmentFixture(t testing.TB, definitions []itemdefs.ItemDef) *combatEquipmentFixture {
	t.Helper()
	if definitions == nil {
		definitions = combatTestDefinitions()
	}
	world := ecs.NewWorldForTesting()
	return combatFixtureInWorld(t, world, definitions)
}

func combatFixtureInWorld(t testing.TB, world *ecs.World, definitions []itemdefs.ItemDef) *combatEquipmentFixture {
	t.Helper()
	catalog, err := NewCombatEquipmentCatalog(itemdefs.NewRegistry(definitions))
	require.NoError(t, err)
	action, err := catalog.PrepareMeleeAction(combatTestAction())
	require.NoError(t, err)
	owner := world.Spawn(1, nil)
	container := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, container, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryEquipment})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryEquipment, 1, 0, container)
	resolver, err := NewCombatEquipmentResolver(world, catalog)
	require.NoError(t, err)
	return &combatEquipmentFixture{world, owner, container, catalog, resolver, action}
}

func (fixture *combatEquipmentFixture) equip(items ...components.InvItem) {
	ecs.WithComponent(fixture.world, fixture.container, func(container *components.InventoryContainer) { container.Items = items })
}

func combatTestItem(id types.EntityID, typeID, quality uint32, slot netproto.EquipSlot) components.InvItem {
	return components.InvItem{ItemID: id, TypeID: typeID, Quality: quality, Quantity: 1, EquipSlot: slot}
}

func TestCombatEquipmentPreparation(t *testing.T) {
	catalog, err := NewCombatEquipmentCatalog(itemdefs.NewRegistry(combatTestDefinitions()))
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		change func(*actiondefs.Definition)
	}{
		{"no source", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].DamageSource = false }},
		{"two sources", func(def *actiondefs.Definition) {
			def.Requirements.Equipment = append(def.Requirements.Equipment, def.Requirements.Equipment[0])
		}},
		{"ordinary", func(def *actiondefs.Definition) { def.Target.Kind = actiondefs.TargetObject }},
		{"no combat", func(def *actiondefs.Definition) { def.Combat = nil }},
		{"zero multiplier", func(def *actiondefs.Definition) { def.Combat.DamageMultiplier = 0 }},
		{"infinite multiplier", func(def *actiondefs.Definition) { def.Combat.DamageMultiplier = math.Inf(1) }},
		{"nan multiplier", func(def *actiondefs.Definition) { def.Combat.DamageMultiplier = math.NaN() }},
		{"no selector", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].ItemTag = "" }},
		{"two selectors", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].ItemKey = "axe" }},
		{"unknown tag", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].ItemTag = "unknown" }},
		{"unknown key", func(def *actiondefs.Definition) {
			def.Requirements.Equipment[0].ItemTag = ""
			def.Requirements.Equipment[0].ItemKey = "unknown"
		}},
		{"bow", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].ItemTag = "bow" }},
		{"unequipable", func(def *actiondefs.Definition) {
			def.Requirements.Equipment[0].ItemTag = ""
			def.Requirements.Equipment[0].ItemKey = "unequipable"
		}},
		{"no slots", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].Slots = nil }},
		{"unknown slot", func(def *actiondefs.Definition) { def.Requirements.Equipment[0].Slots = []string{"unknown"} }},
		{"duplicate slot", func(def *actiondefs.Definition) {
			def.Requirements.Equipment[0].Slots = []string{"left_hand", "left_hand"}
		}},
		{"incompatible slots", func(def *actiondefs.Definition) {
			def.Requirements.Equipment[0].ItemTag = "axe"
			def.Requirements.Equipment[0].Slots = []string{"head"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := combatTestAction()
			test.change(definition)
			action, err := catalog.PrepareMeleeAction(definition)
			require.Error(t, err)
			require.Nil(t, action)
		})
	}
	_, err = catalog.PrepareMeleeAction(nil)
	require.Error(t, err)
	for _, key := range []string{"axe", "sword", "knife", "pike"} {
		definition := combatTestAction()
		definition.Requirements.Equipment[0].ItemKey = key
		definition.Requirements.Equipment[0].ItemTag = ""
		definition.Requirements.Equipment = append(definition.Requirements.Equipment, actiondefs.EquipmentRequirement{ItemTag: "helmet", Slots: []string{"head"}})
		action, err := catalog.PrepareMeleeAction(definition)
		require.NoError(t, err)
		require.Len(t, action.types, 1)
	}
}

func TestCombatEquipmentCatalogValidation(t *testing.T) {
	_, err := NewCombatEquipmentCatalog(nil)
	require.Error(t, err)
	for _, test := range []struct {
		name   string
		change func(*itemdefs.ItemDef)
	}{
		{"zero ID", func(def *itemdefs.ItemDef) { def.DefID = 0 }},
		{"overflow ID", func(def *itemdefs.ItemDef) { def.DefID = int(uint64(math.MaxUint32) + 1) }},
		{"unknown slot", func(def *itemdefs.ItemDef) { def.Allowed.EquipmentSlots = []string{"unknown"} }},
		{"duplicate slot", func(def *itemdefs.ItemDef) { def.Allowed.EquipmentSlots = []string{"head", "head"} }},
		{"zero damage", func(def *itemdefs.ItemDef) { def.Melee.BaseDamage = 0 }},
		{"infinite damage", func(def *itemdefs.ItemDef) { def.Melee.BaseDamage = math.Inf(1) }},
		{"nan armor", func(def *itemdefs.ItemDef) { def.Armor = &itemdefs.ArmorDef{BaseArmor: math.NaN()} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			definitions := combatTestDefinitions()[:1]
			test.change(&definitions[0])
			catalog, err := NewCombatEquipmentCatalog(itemdefs.NewRegistry(definitions))
			require.Error(t, err)
			require.Nil(t, catalog)
		})
	}
	world := ecs.NewWorldForTesting()
	_, err = NewCombatEquipmentResolver(world, nil)
	require.Error(t, err)
	_, err = NewCombatEquipmentResolver(world, &CombatEquipmentCatalog{})
	require.Error(t, err)
	catalog, err := NewCombatEquipmentCatalog(itemdefs.NewRegistry(nil))
	require.NoError(t, err)
	_, err = NewCombatEquipmentResolver(nil, catalog)
	require.Error(t, err)
	_, err = NewCombatEquipmentResolver(&ecs.World{}, catalog)
	require.ErrorIs(t, err, ErrInvalidCombatResolver)
	var emptyCatalog *CombatEquipmentCatalog
	_, err = emptyCatalog.PrepareMeleeAction(combatTestAction())
	require.Error(t, err)
}

func TestCombatEquipmentSnapshotsAndCatalogBinding(t *testing.T) {
	definitions := combatTestDefinitions()
	fixture := newCombatEquipmentFixture(t, definitions)
	fixture.equip(combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	definitions[0].Melee.BaseDamage = 200
	definitions[0].Tags[0] = "changed"
	definitions[0].Allowed.EquipmentSlots[0] = "head"
	definition := combatTestAction()
	action, err := fixture.catalog.PrepareMeleeAction(definition)
	require.NoError(t, err)
	definition.Combat.DamageMultiplier = 500
	definition.Requirements.Equipment[0].Slots[1] = "head"
	weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, action, 1)
	require.NoError(t, err)
	require.Equal(t, 6.0, weapon.RawDamage)
	otherCatalog, err := NewCombatEquipmentCatalog(itemdefs.NewRegistry(combatTestDefinitions()))
	require.NoError(t, err)
	otherAction, err := otherCatalog.PrepareMeleeAction(combatTestAction())
	require.NoError(t, err)
	for _, invalid := range []*PreparedMeleeAction{nil, {}, otherAction} {
		weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, invalid, 1)
		require.ErrorIs(t, err, ErrInvalidMeleeAction)
		require.Zero(t, weapon)
	}
	// The same immutable catalog and prepared action can serve a different World.
	world := ecs.NewWorldForTesting()
	owner := world.Spawn(1, nil)
	resolver, err := NewCombatEquipmentResolver(world, fixture.catalog)
	require.NoError(t, err)
	_, err = resolver.ResolveMeleeWeapon(owner, action, 1)
	require.ErrorIs(t, err, ErrMeleeWeaponUnavailable)
}

func TestCombatEquipmentSourceRestrictsSelectorAndSlots(t *testing.T) {
	fixture := newCombatEquipmentFixture(t, nil)
	definition := combatTestAction()
	definition.Requirements.Equipment[0].ItemTag = ""
	definition.Requirements.Equipment[0].ItemKey = "axe"
	definition.Requirements.Equipment[0].Slots = []string{"left_hand"}
	action, err := fixture.catalog.PrepareMeleeAction(definition)
	require.NoError(t, err)
	fixture.equip(combatTestItem(100, 1, 1000, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND), combatTestItem(101, 4, 1000, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, action, 1)
	require.ErrorIs(t, err, ErrMeleeWeaponUnavailable)
	require.Zero(t, weapon)
	fixture.equip(combatTestItem(100, 4, 1000, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND), combatTestItem(101, 1, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	weapon, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, action, 1)
	require.NoError(t, err)
	require.Equal(t, types.EntityID(101), weapon.ItemID)
}

func TestCombatEquipmentArmorAndIgnoredInventories(t *testing.T) {
	fixture := newCombatEquipmentFixture(t, nil)
	for _, kind := range []constt.InventoryKind{constt.InventoryGrid, constt.InventoryHand} {
		handle := fixture.world.SpawnWithoutExternalID()
		ecs.AddComponent(fixture.world, handle, components.InventoryContainer{
			OwnerID: 1, Kind: kind, Items: []components.InvItem{combatTestItem(200, 6, 100, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND)},
		})
		ecs.GetResource[ecs.InventoryRefIndex](fixture.world).Add(kind, 1, 0, handle)
	}
	for _, expected := range []float64{0, 4, 12} {
		armor, err := fixture.resolver.ResolveArmor(fixture.owner)
		require.NoError(t, err)
		require.Equal(t, expected, armor)
		if expected == 0 {
			_, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
			require.ErrorIs(t, err, ErrMeleeWeaponUnavailable)
			fixture.equip(combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST))
		} else {
			fixture.equip(combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST), combatTestItem(101, 8, 10, netproto.EquipSlot_EQUIP_SLOT_HEAD))
		}
	}
	fixture.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	armor, err := fixture.resolver.ResolveArmor(fixture.owner)
	require.NoError(t, err)
	require.Equal(t, 4.0, armor)
	weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.NoError(t, err)
	require.Equal(t, 5.0, weapon.RawDamage)
	// Separate instances of one TypeID both contribute armor.
	fixture.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND), combatTestItem(101, 6, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	armor, err = fixture.resolver.ResolveArmor(fixture.owner)
	require.NoError(t, err)
	require.Equal(t, 8.0, armor)
	ecs.GetResource[ecs.InventoryRefIndex](fixture.world).Remove(constt.InventoryEquipment, 1, 0)
	armor, err = fixture.resolver.ResolveArmor(fixture.owner)
	require.NoError(t, err)
	require.Zero(t, armor)
	_, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.ErrorIs(t, err, ErrMeleeWeaponUnavailable)
}

func TestCombatEquipmentMeleeSelectionAndChanges(t *testing.T) {
	fixture := newCombatEquipmentFixture(t, nil)
	for typeID, expected := range map[uint32]float64{1: 6, 2: 4, 3: 3, 4: 7} {
		fixture.equip(combatTestItem(100, typeID, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
		weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
		require.NoError(t, err)
		require.Equal(t, expected, weapon.RawDamage)
		require.Equal(t, typeID, weapon.TypeID)
	}
	fixture.equip(combatTestItem(100, 5, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	_, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.ErrorIs(t, err, ErrMeleeWeaponUnavailable)
	items := []components.InvItem{
		combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND),
		combatTestItem(101, 2, 160, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND),
	}
	fixture.equip(items...)
	weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.NoError(t, err)
	require.Equal(t, types.EntityID(101), weapon.ItemID)
	require.Equal(t, 8.0, weapon.RawDamage) // Smaller base damage, higher quality.
	slices.Reverse(items)
	reordered, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.NoError(t, err)
	require.Equal(t, weapon, reordered)
	items[0].Quality = 10
	weapon, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 2.25)
	require.NoError(t, err)
	require.Equal(t, types.EntityID(100), weapon.ItemID)
	expected, err := combat.MeleeRawDamage(6, 2.25, 10, 1)
	require.NoError(t, err)
	require.Equal(t, expected, weapon.RawDamage)
	// Same TypeID with distinct instance IDs; tied damage chooses smaller ID.
	items[0] = combatTestItem(99, 1, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND)
	for iteration := 0; iteration < 2; iteration++ {
		weapon, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
		require.NoError(t, err)
		require.Equal(t, types.EntityID(99), weapon.ItemID)
		slices.Reverse(items)
	}
	// Swap the indexed container: a resolver must not retain its old handle.
	old := fixture.container
	fixture.container = fixture.world.SpawnWithoutExternalID()
	ecs.AddComponent(fixture.world, fixture.container, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryEquipment})
	ecs.GetResource[ecs.InventoryRefIndex](fixture.world).Add(constt.InventoryEquipment, 1, 0, fixture.container)
	fixture.equip(combatTestItem(102, 3, 40, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	fixture.world.Despawn(old)
	weapon, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.NoError(t, err)
	require.Equal(t, types.EntityID(102), weapon.ItemID)
	fixture.equip(combatTestItem(103, 7, 40, netproto.EquipSlot_EQUIP_SLOT_CHEST))
	armor, err := fixture.resolver.ResolveArmor(fixture.owner)
	require.NoError(t, err)
	require.Equal(t, 8.0, armor)
	_, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.ErrorIs(t, err, ErrMeleeWeaponUnavailable)
}

func TestCombatEquipmentArmorDeterministicOrder(t *testing.T) {
	definitions := combatTestDefinitions()
	for index, base := range []float64{1e16, 1, 1} {
		definitions[index].Armor = &itemdefs.ArmorDef{BaseArmor: base}
		definitions[index].Allowed.EquipmentSlots = []string{"head", "chest", "legs", "left_hand", "right_hand"}
	}
	fixture := newCombatEquipmentFixture(t, definitions)
	items := []components.InvItem{
		combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_HEAD),
		combatTestItem(101, 2, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST),
		combatTestItem(102, 3, 10, netproto.EquipSlot_EQUIP_SLOT_LEGS),
	}
	var expected uint64
	for iteration := 0; iteration < 6; iteration++ {
		fixture.equip(items...)
		armor, err := fixture.resolver.ResolveArmor(fixture.owner)
		require.NoError(t, err)
		if iteration == 0 {
			expected = math.Float64bits(armor)
		}
		require.Equal(t, expected, math.Float64bits(armor))
		if iteration%2 == 0 {
			items[0], items[1] = items[1], items[0]
		} else {
			items[1], items[2] = items[2], items[1]
		}
	}
}

func TestCombatEquipmentInvalidState(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*combatEquipmentFixture)
		err    error
	}{
		{"stale owner", func(f *combatEquipmentFixture) { f.world.Despawn(f.owner); f.world.Spawn(1, nil) }, ErrInvalidCombatOwner},
		{"no owner identity", func(f *combatEquipmentFixture) { f.resolver.identities.Remove(f.owner) }, ErrInvalidCombatOwner},
		{"zero owner identity", func(f *combatEquipmentFixture) {
			ecs.WithComponent(f.world, f.owner, func(id *ecs.ExternalID) { id.ID = 0 })
		}, ErrInvalidCombatOwner},
		{"stale container", func(f *combatEquipmentFixture) { f.world.Despawn(f.container); f.world.SpawnWithoutExternalID() }, ErrInvalidCombatEquipment},
		{"no container component", func(f *combatEquipmentFixture) { f.resolver.containers.Remove(f.container) }, ErrInvalidCombatEquipment},
		{"foreign container", func(f *combatEquipmentFixture) {
			ecs.WithComponent(f.world, f.container, func(c *components.InventoryContainer) { c.OwnerID = 2 })
		}, ErrInvalidCombatEquipment},
		{"wrong kind", func(f *combatEquipmentFixture) {
			ecs.WithComponent(f.world, f.container, func(c *components.InventoryContainer) { c.Kind = constt.InventoryHand })
		}, ErrInvalidCombatEquipment},
		{"wrong key", func(f *combatEquipmentFixture) {
			ecs.WithComponent(f.world, f.container, func(c *components.InventoryContainer) { c.Key = 1 })
		}, ErrInvalidCombatEquipment},
		{"too many items", func(f *combatEquipmentFixture) { f.equip(make([]components.InvItem, 11)...) }, ErrInvalidCombatEquipment},
		{"unknown type", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(101, 999, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
		}, ErrInvalidCombatEquipment},
		{"zero item ID", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(0, 1, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
		}, ErrInvalidCombatEquipment},
		{"duplicate item ID", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND), combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
		}, ErrInvalidCombatEquipment},
		{"duplicate slot", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND), combatTestItem(101, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
		}, ErrInvalidCombatEquipment},
		{"none slot", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_NONE))
		}, ErrInvalidCombatEquipment},
		{"reserved slot", func(f *combatEquipmentFixture) { f.equip(combatTestItem(100, 1, 10, 5)) }, ErrInvalidCombatEquipment},
		{"negative slot", func(f *combatEquipmentFixture) { f.equip(combatTestItem(100, 1, 10, -1)) }, ErrInvalidCombatEquipment},
		{"slot out of range", func(f *combatEquipmentFixture) { f.equip(combatTestItem(100, 1, 10, math.MaxInt32)) }, ErrInvalidCombatEquipment},
		{"disallowed slot", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST))
		}, ErrInvalidCombatEquipment},
		{"zero quality", func(f *combatEquipmentFixture) {
			f.equip(combatTestItem(100, 6, 0, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
		}, combat.ErrInvalidInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCombatEquipmentFixture(t, nil)
			fixture.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
			test.change(fixture)
			armor, err := fixture.resolver.ResolveArmor(fixture.owner)
			require.ErrorIs(t, err, test.err)
			require.Zero(t, armor)
			weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
			require.ErrorIs(t, err, test.err)
			require.Zero(t, weapon)
			require.Zero(t, testing.AllocsPerRun(100, func() {
				_, armorErr := fixture.resolver.ResolveArmor(fixture.owner)
				_, weaponErr := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
				if armorErr != test.err || weaponErr != test.err {
					panic("unexpected state error")
				}
			}))
		})
	}
}

func TestCombatEquipmentNumericErrors(t *testing.T) {
	fixture := newCombatEquipmentFixture(t, nil)
	fixture.equip(combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	for _, strength := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, strength)
		require.ErrorIs(t, err, combat.ErrInvalidInput)
		require.Zero(t, weapon)
	}
	// An invalid losing candidate must fail the entire selection.
	fixture.equip(combatTestItem(100, 4, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND), combatTestItem(101, 3, 0, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	weapon, err := fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 1)
	require.ErrorIs(t, err, combat.ErrInvalidInput)
	require.Zero(t, weapon)
	fixture.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND), combatTestItem(101, 6, 0, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	armor, err := fixture.resolver.ResolveArmor(fixture.owner)
	require.ErrorIs(t, err, combat.ErrInvalidInput)
	require.Zero(t, armor) // Discard a valid earlier contribution.
	definitions := combatTestDefinitions()
	definitions[0].Melee.BaseDamage = math.MaxFloat64
	definitions[5].Armor.BaseArmor = math.MaxFloat64
	fixture = newCombatEquipmentFixture(t, definitions)
	fixture.equip(combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	weapon, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 16)
	require.ErrorIs(t, err, combat.ErrNonFiniteResult)
	require.Zero(t, weapon)
	fixture.equip(combatTestItem(100, 6, 40, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
	armor, err = fixture.resolver.ResolveArmor(fixture.owner)
	require.ErrorIs(t, err, combat.ErrNonFiniteResult)
	require.Zero(t, armor)
	fixture.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND), combatTestItem(101, 6, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	armor, err = fixture.resolver.ResolveArmor(fixture.owner)
	require.ErrorIs(t, err, combat.ErrNonFiniteResult)
	require.Zero(t, armor)
}

func TestCombatEquipmentProductionComposition(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	actions, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), nil)
	require.NoError(t, err)
	definitions := make([]itemdefs.ItemDef, 0, items.Count()+1)
	for _, item := range items.All() {
		definitions = append(definitions, *item)
	}
	definitions = append(definitions, itemdefs.ItemDef{DefID: math.MaxInt32, Key: "test_armor", Allowed: itemdefs.Allowed{EquipmentSlots: []string{"chest"}}, Armor: &itemdefs.ArmorDef{BaseArmor: 4}})
	catalog, err := NewCombatEquipmentCatalog(itemdefs.NewRegistry(definitions))
	require.NoError(t, err)
	world := ecs.NewWorldForTesting()
	owner := world.Spawn(1, nil)
	container := world.SpawnWithoutExternalID()
	axe, exists := items.GetByKey("stone_axe")
	require.True(t, exists)
	ecs.AddComponent(world, container, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryEquipment, Items: []components.InvItem{
		combatTestItem(100, uint32(axe.DefID), 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND),
		combatTestItem(101, math.MaxInt32, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST),
	}})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryEquipment, 1, 0, container)
	resolver, err := NewCombatEquipmentResolver(world, catalog)
	require.NoError(t, err)
	armor, err := resolver.ResolveArmor(owner)
	require.NoError(t, err)
	require.Equal(t, 4.0, armor)
	for id, expected := range map[string]float64{"axe_sweep": 3.6, "axe_strike": 81.0 / 13} {
		definition, exists := actions.Get(id)
		require.True(t, exists)
		action, err := catalog.PrepareMeleeAction(definition)
		require.NoError(t, err)
		weapon, err := resolver.ResolveMeleeWeapon(owner, action, 1)
		require.NoError(t, err)
		damage, err := combat.DamageAfterArmor(weapon.RawDamage, armor)
		require.NoError(t, err)
		require.InDelta(t, expected, damage, 1e-12)
	}
}

func TestCombatEquipmentZeroAllocations(t *testing.T) {
	fixture := newCombatEquipmentFixture(t, nil)
	for _, scenario := range []string{"success", "empty", "absent", "corrupt", "numeric", "owner", "action"} {
		t.Run(scenario, func(t *testing.T) {
			fixture.equip(combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
			index := ecs.GetResource[ecs.InventoryRefIndex](fixture.world)
			index.Add(constt.InventoryEquipment, 1, 0, fixture.container)
			owner, action := fixture.owner, fixture.action
			var armorError, weaponError error
			switch scenario {
			case "empty":
				fixture.equip()
				weaponError = ErrMeleeWeaponUnavailable
			case "absent":
				index.Remove(constt.InventoryEquipment, 1, 0)
				weaponError = ErrMeleeWeaponUnavailable
			case "corrupt":
				fixture.equip(combatTestItem(100, 999, 10, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
				armorError, weaponError = ErrInvalidCombatEquipment, ErrInvalidCombatEquipment
			case "numeric":
				fixture.equip(combatTestItem(100, 6, 0, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND))
				armorError, weaponError = combat.ErrInvalidInput, combat.ErrInvalidInput
			case "owner":
				owner = types.InvalidHandle
				armorError, weaponError = ErrInvalidCombatOwner, ErrInvalidCombatOwner
			case "action":
				action = nil
				weaponError = ErrInvalidMeleeAction
			}
			allocations := testing.AllocsPerRun(1000, func() {
				_, err := fixture.resolver.ResolveArmor(owner)
				if err != armorError {
					panic("unexpected armor error")
				}
				_, err = fixture.resolver.ResolveMeleeWeapon(owner, action, 2.25)
				if err != weaponError {
					panic("unexpected weapon error")
				}
			})
			require.Zero(t, allocations)
		})
	}
}
