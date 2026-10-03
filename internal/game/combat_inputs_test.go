package game

import (
	"math"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"testing"
)

func TestCombatWeaponPreferenceAndStrength(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1002, Key: "axe", Tags: []string{"axe"}, Weapon: &itemdefs.Weapon{BaseDamage: 6, Range: 18}}}))
	world := ecs.NewWorldForTesting()
	actor := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{Attributes: characterattrs.Values{characterattrs.STR: 16}})
	})
	equipment := world.Spawn(10, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryEquipment, Items: []components.InvItem{
			{ItemID: 100, TypeID: 1002, Quality: 160, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND},
			{ItemID: 101, TypeID: 1002, Quality: 10, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND},
		}})
	})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryEquipment, 1, 0, equipment)
	definition := &actiondefs.Definition{Requirements: actiondefs.Requirements{Equipment: []actiondefs.EquipmentRequirement{{Slots: []string{"left_hand", "right_hand"}, ItemTag: "axe"}}}}
	weapon, err := resolveCombatWeapon(world, 1, definition)
	if err != nil || weapon.ItemID != 101 || weapon.Quality != 10 || weapon.Slot != "right_hand" {
		t.Fatalf("wrong preference: %+v %v", weapon, err)
	}
	strength, err := baseCombatStrength(world, actor)
	if err != nil || strength != 16 {
		t.Fatalf("wrong base strength: %v %v", strength, err)
	}
	supplied := CombatStrengthProvider(func(*ecs.World, types.Handle) (float64, error) { return 1.5, nil })
	strength, err = supplied(world, actor)
	if err != nil {
		t.Fatal(err)
	}
	damage, err := combat.RawDamage(weapon.Weapon, strength, 1)
	if err != nil || math.Abs(damage-6*math.Pow(1.5, 0.25)) > 1e-12 {
		t.Fatalf("fractional strength lost: %v %v", damage, err)
	}
	ecs.WithComponent(world, equipment, func(container *components.InventoryContainer) { container.Items = container.Items[:1] })
	weapon, err = resolveCombatWeapon(world, 1, definition)
	if err != nil || weapon.ItemID != 100 || weapon.Quality != 160 {
		t.Fatalf("left fallback: %+v %v", weapon, err)
	}
	ecs.WithComponent(world, equipment, func(container *components.InventoryContainer) { container.Items[0].Quality = 0 })
	if _, err = resolveCombatWeapon(world, 1, definition); err == nil {
		t.Fatal("invalid quality accepted")
	}
	ecs.WithComponent(world, equipment, func(container *components.InventoryContainer) { container.Items = nil })
	if _, err = resolveCombatWeapon(world, 1, definition); err == nil {
		t.Fatal("missing weapon accepted")
	}
}
