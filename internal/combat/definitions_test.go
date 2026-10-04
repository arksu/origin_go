package combat_test

import (
	"os"
	"path/filepath"
	"testing"

	"origin/internal/actiondefs"
	"origin/internal/combat"
	"origin/internal/itemdefs"

	"go.uber.org/zap"
)

func TestMeleeDamageFromDefinitions(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	actions, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	weapon, ok := items.GetByKey("stone_axe")
	if !ok || weapon.Melee == nil {
		t.Fatal("missing melee weapon definition")
	}
	for _, tt := range []struct {
		actionID string
		want     float64
	}{
		{"axe_sweep", 3.6},
		{"axe_strike", 81.0 / 13},
	} {
		t.Run(tt.actionID, func(t *testing.T) {
			action, ok := actions.Get(tt.actionID)
			if !ok || action.Combat == nil {
				t.Fatal("missing combat action definition")
			}
			raw, err := combat.MeleeRawDamage(weapon.Melee.BaseDamage, 1, 10, action.Combat.DamageMultiplier)
			if err != nil {
				t.Fatal(err)
			}
			damage, err := combat.DamageAfterArmor(raw, 4)
			if err != nil {
				t.Fatal(err)
			}
			closeTo(t, damage, tt.want)
		})
	}
}

func TestDifferentMeleeItemsShareCalculations(t *testing.T) {
	dir := t.TempDir()
	definitions := `{"v":1,"items":[
		{"defId":1,"key":"test_axe","name":"Test axe","tags":["axe"],"size":{"w":1,"h":1},"melee":{"baseDamage":6}},
		{"defId":2,"key":"test_sword","name":"Test sword","tags":["sword"],"size":{"w":1,"h":1},"melee":{"baseDamage":8}},
		{"defId":3,"key":"test_knife","name":"Test knife","tags":["knife"],"size":{"w":1,"h":1},"melee":{"baseDamage":4}},
		{"defId":4,"key":"test_pike","name":"Test pike","tags":["pike"],"size":{"w":1,"h":1},"melee":{"baseDamage":10},"armor":{"baseArmor":4}},
		{"defId":5,"key":"test_armor","name":"Test armor","size":{"w":1,"h":1},"armor":{"baseArmor":8}}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "items.json"), []byte(definitions), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := itemdefs.LoadFromDirectory(dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		key  string
		want float64
	}{
		{"test_axe", 9},
		{"test_sword", 12},
		{"test_knife", 6},
		{"test_pike", 15},
	} {
		t.Run(tt.key, func(t *testing.T) {
			item, ok := items.GetByKey(tt.key)
			if !ok || item.Melee == nil {
				t.Fatal("missing melee parameters")
			}
			raw, err := combat.MeleeRawDamage(item.Melee.BaseDamage, 1, 10, 1.5)
			if err != nil {
				t.Fatal(err)
			}
			closeTo(t, raw, tt.want)
		})
	}

	armor := 0.0
	for _, key := range []string{"test_pike", "test_armor"} {
		item, ok := items.GetByKey(key)
		if !ok || item.Armor == nil {
			t.Fatal("missing armor parameters")
		}
		contribution, err := combat.ArmorContribution(item.Armor.BaseArmor, 10)
		if err != nil {
			t.Fatal(err)
		}
		armor += contribution
	}
	closeTo(t, armor, 12)
	damage, err := combat.DamageAfterArmor(9, armor)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, damage, 27.0/7)
}
