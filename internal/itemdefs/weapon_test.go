package itemdefs

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeaponValidation(t *testing.T) {
	for _, weapon := range []Weapon{{BaseDamage: -1, Range: 18}, {BaseDamage: 6}, {BaseDamage: 6, Range: -1}, {BaseDamage: math.NaN(), Range: 18}, {BaseDamage: 6, Range: math.Inf(1)}} {
		item := ItemDef{DefID: 1, Key: "axe", Name: "Axe", Size: Size{1, 1}, Weapon: &weapon}
		err := validateItem(&item, "weapons.json")
		if err == nil || !strings.Contains(err.Error(), "weapons.json") {
			t.Fatalf("accepted invalid weapon %+v: %v", weapon, err)
		}
	}
	filename := filepath.Join(t.TempDir(), "weapons.json")
	if err := os.WriteFile(filename, []byte(`{"v":1,"items":[{"defId":1,"key":"axe","name":"Axe","size":{"w":1,"h":1},"weapon":{"baseDamage":6,"range":18,"typo":1}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFile(filename); err == nil || !strings.Contains(err.Error(), "weapons.json") {
		t.Fatalf("unknown weapon field accepted: %v", err)
	}
}
