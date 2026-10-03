package actiondefs

import (
	"go.uber.org/zap"
	"math"
	"origin/internal/itemdefs"
	"path/filepath"
	"strings"
	"testing"
)

const validCombat = `{"v":1,"actions":[{"id":"axe_test","presentation":{"label":"Axe","menuIcon":"/assets/game/items/stone_axe.png"},"target":{"kind":"direction"},"requirements":{"equipment":[{"slots":["right_hand","left_hand"],"itemTag":"axe"}]},"execution":{"stamina":60,"combat":{"selection":"all","sectorAngleDegrees":90,"windupMs":600,"recoveryMs":400,"cooldownMs":2000,"damageMultiplier":1}}}]}`

func loadCombatItems(t *testing.T) *itemdefs.Registry {
	t.Helper()
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	return items
}

func TestCombatPresets(t *testing.T) {
	items := loadCombatItems(t)
	registry, err := LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	for index, id := range []string{"axe_aoe", "axe_single"} {
		definition, ok := registry.Get(id)
		if !ok || definition.Execution.Combat == nil {
			t.Fatalf("missing %s", id)
		}
		profile := definition.Execution.Combat
		if profile.Selection != []string{SelectionAll, SelectionNearest}[index] || profile.DamageMultiplier != []float64{1, 1.5}[index] || profile.SectorAngleDegrees != 90 || profile.WindupMs != 600 || profile.RecoveryMs != 400 || profile.CooldownMs != 2000 || definition.Execution.Stamina != 60 || definition.Target.Kind != TargetDirection || definition.Repeatable() {
			t.Fatalf("unexpected combat preset: %+v %+v", definition, profile)
		}
	}
	axe, ok := items.GetByKey("stone_axe")
	if !ok || axe.Weapon == nil || axe.Weapon.BaseDamage != 6 || axe.Weapon.Range != 18 {
		t.Fatalf("unexpected axe: %+v", axe)
	}
}

func TestCombatDefinitionValidation(t *testing.T) {
	loadCombatItems(t)
	cases := map[string]string{
		"valid":               validCombat,
		"unknown field":       strings.Replace(validCombat, `"windupMs":600`, `"windupMs":600,"windup":600`, 1),
		"legacy zero ticks":   strings.Replace(validCombat, `"stamina":60`, `"ticks":0,"stamina":60`, 1),
		"legacy ticks":        strings.Replace(validCombat, `"stamina":60`, `"ticks":1,"stamina":60`, 1),
		"repeat":              strings.Replace(validCombat, `"stamina":60`, `"repeat":true,"stamina":60`, 1),
		"repeatable":          strings.Replace(validCombat, `"id":"axe_test"`, `"id":"axe_test","isRepeatable":true`, 1),
		"approach":            strings.Replace(validCombat, `"kind":"direction"`, `"kind":"direction","approach":"tile_center"`, 1),
		"unknown weapon":      strings.Replace(validCombat, `"itemTag":"axe"`, `"itemKey":"missing_axe"`, 1),
		"unknown tag":         strings.Replace(validCombat, `"itemTag":"axe"`, `"itemTag":"missing_axe"`, 1),
		"wrong slot":          strings.Replace(validCombat, `"right_hand","left_hand"`, `"back"`, 1),
		"selection":           strings.Replace(validCombat, `"selection":"all"`, `"selection":"random"`, 1),
		"negative cost":       strings.Replace(validCombat, `"stamina":60`, `"stamina":-1`, 1),
		"nonfinite":           strings.Replace(validCombat, `"damageMultiplier":1`, `"damageMultiplier":1e999`, 1),
		"negative multiplier": strings.Replace(validCombat, `"damageMultiplier":1`, `"damageMultiplier":-1`, 1),
		"zero angle":          strings.Replace(validCombat, `"sectorAngleDegrees":90`, `"sectorAngleDegrees":0`, 1),
		"nonconvex angle":     strings.Replace(validCombat, `"sectorAngleDegrees":90`, `"sectorAngleDegrees":181`, 1),
	}
	for _, field := range []string{"windupMs", "recoveryMs", "cooldownMs"} {
		value := map[string]string{"windupMs": "600", "recoveryMs": "400", "cooldownMs": "2000"}[field]
		for _, invalid := range []string{"0", "-1", "1.5", "9223372036854775807"} {
			cases[field+invalid] = strings.Replace(validCombat, `"`+field+`":`+value, `"`+field+`":`+invalid, 1)
		}
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			writeActionFile(t, directory, "combat.json", contents)
			_, err := LoadFromDirectory(directory, zap.NewNop())
			if name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "combat.json") {
				t.Fatalf("expected file-specific error, got %v", err)
			}
		})
	}
	for _, invalid := range []float64{math.NaN(), math.Inf(1)} {
		definition := Definition{Target: Target{Kind: TargetDirection}, Execution: Execution{Combat: &CombatProfile{Selection: SelectionAll, SectorAngleDegrees: invalid}}}
		if validateCombat(&definition) == nil {
			t.Fatal("nonfinite angle accepted")
		}
	}
}
