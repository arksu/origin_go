package actiondefs

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"origin/internal/itemdefs"

	"go.uber.org/zap"
)

const validAction = `{"v":1,"actions":[{"id":"test_action","presentation":{"label":"Test action","menuIcon":"/assets/cursor/lift.png"},"target":{"kind":"object","cursor":"lift"},"requirements":{"skills":["test_skill"],"equipment":[{"slots":["left_hand","right_hand"],"itemTag":"axe"},{"slots":["back"],"itemKey":"test_pack"}]},"execution":{"ticks":4,"stamina":2.5},"isRepeatable":true}]}`

const validCombatAction = `{"v":1,"actions":[{"id":"combat_test","presentation":{"label":"Combat test","menuIcon":"/assets/cursor/atk.png"},"target":{"kind":"direction"},"requirements":{},"execution":{"ticks":6,"recoveryTicks":4,"stamina":60},"cooldown":2000,"isRepeatable":false,"sector":{"range":18,"angleDeg":90},"combat":{"hitMode":"all","damageMultiplier":1.0}}]}`

func writeActionFile(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefinitionsAndValidateHandlers(t *testing.T) {
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1, Key: "test_pack"}}))
	directory := t.TempDir()
	writeActionFile(t, directory, "valid.json", validAction)
	registry, err := LoadFromDirectory(directory, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	definition, exists := registry.Get("test_action")
	if !exists || definition.Target.Kind != TargetObject || !definition.Repeatable() || definition.Execution.Ticks != 4 || definition.Execution.Stamina != 2.5 {
		t.Fatalf("loaded action does not match the definition: %#v", definition)
	}
	if err := registry.ValidateHandlers([]string{"test_action"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateHandlers(nil); err != nil {
		t.Fatalf("definition without handler must load during incremental implementation: %v", err)
	}
	if err := registry.ValidateHandlers([]string{"test_action", "unknown"}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unmatched handler must fail, got %v", err)
	}
	if err := registry.ValidateHandlers([]string{"test_action", "test_action"}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate handler must fail, got %v", err)
	}
}

func TestInvalidDefinitionIdentifiesFile(t *testing.T) {
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1, Key: "test_pack"}}))
	cases := map[string]string{
		"unknown slot":       strings.Replace(validAction, `"back"`, `"unknown"`, 1),
		"bad selector":       strings.Replace(validAction, `"itemTag":"axe"`, `"itemTag":"axe","itemKey":"test_pack"`, 1),
		"negative cost":      strings.Replace(validAction, `"stamina":2.5`, `"stamina":-1`, 1),
		"unsafe icon":        strings.Replace(validAction, `/assets/cursor/lift.png`, `/assets/../outside.png`, 1),
		"external icon":      strings.Replace(validAction, `/assets/cursor/lift.png`, `https://example.com/icon.png`, 1),
		"unknown item":       strings.Replace(validAction, `"itemKey":"test_pack"`, `"itemKey":"missing"`, 1),
		"none cursor":        strings.Replace(validAction, `"kind":"object"`, `"kind":"none"`, 1),
		"none repeatability": strings.Replace(strings.Replace(validAction, `"kind":"object","cursor":"lift"`, `"kind":"none"`, 1), `"isRepeatable":true`, `"isRepeatable":false`, 1),
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			writeActionFile(t, directory, "invalid.json", contents)
			_, err := LoadFromDirectory(directory, zap.NewNop())
			if err == nil || !strings.Contains(err.Error(), "invalid.json") {
				t.Fatalf("expected a file-identifying validation error, got %v", err)
			}
		})
	}
}

func TestDuplicateDefinitionIdentifiesSecondFile(t *testing.T) {
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1, Key: "test_pack"}}))
	directory := t.TempDir()
	writeActionFile(t, directory, "first.json", validAction)
	writeActionFile(t, directory, "second.json", validAction)
	_, err := LoadFromDirectory(directory, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "second.json") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected second file duplicate error, got %v", err)
	}
}

func TestProductionActionsMatchRegisteredHandlers(t *testing.T) {
	registry, err := LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.All()) != 6 {
		t.Fatalf("expected four existing actions and two axe actions, got %d", len(registry.All()))
	}
	if err := registry.ValidateHandlers([]string{"lift", "lift_down", "plow_tile", "dig"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lift", "lift_down"} {
		definition, exists := registry.Get(id)
		if !exists || definition.Repeatable() || definition.Execution.Ticks != 0 || definition.Execution.Stamina != 0 || definition.Target.Approach != "" {
			t.Fatalf("invalid production action %q: %#v", id, definition)
		}
	}
	plow, _ := registry.Get("plow_tile")
	if plow.Cooldown == 0 || plow.Target.Kind != TargetTile || plow.Target.Approach != ApproachTileCenter || plow.Target.Cursor != "dig" ||
		!plow.Repeatable() || plow.Execution.Repeat || plow.Execution.Ticks != 20 || plow.Execution.Stamina != 250 ||
		len(plow.Requirements.Skills) != 0 || len(plow.Requirements.Equipment) != 0 {
		t.Fatalf("invalid plow definition: %#v", plow)
	}
	dig, _ := registry.Get("dig")
	if dig.Target.Kind != TargetTile || dig.Target.Approach != ApproachTileCenter || dig.Target.Cursor != "dig" ||
		dig.Repeatable() || !dig.Execution.Repeat || dig.Execution.Ticks != 20 || dig.Execution.Stamina != 300 ||
		len(dig.Requirements.Skills) != 0 || len(dig.Requirements.Equipment) != 0 {
		t.Fatalf("invalid dig definition: %#v", dig)
	}
	for _, expected := range []struct {
		id         string
		hitMode    HitMode
		multiplier float64
	}{
		{id: "axe_sweep", hitMode: HitAll, multiplier: 1},
		{id: "axe_strike", hitMode: HitNearest, multiplier: 1.5},
	} {
		definition, exists := registry.Get(expected.id)
		if !exists {
			t.Fatalf("production action %q is missing", expected.id)
		}
		if definition.Target.Kind != TargetDirection || definition.Target.Cursor != "" || definition.Target.Approach != "" ||
			definition.Execution.Ticks != 6 || definition.Execution.RecoveryTicks != 4 || definition.Execution.Stamina != 60 ||
			definition.Execution.Repeat || definition.Repeatable() || definition.Cooldown != 2000 {
			t.Fatalf("invalid axe definition %q: %#v", expected.id, definition)
		}
		if definition.Sector == nil || definition.Sector.Range != 18 || math.Abs(definition.Sector.Angle-math.Pi/2) > 1e-12 ||
			definition.Combat == nil || definition.Combat.HitMode != expected.hitMode || definition.Combat.DamageMultiplier != expected.multiplier {
			t.Fatalf("invalid axe combat parameters %q: sector=%#v combat=%#v", expected.id, definition.Sector, definition.Combat)
		}
		if len(definition.Requirements.Skills) != 0 || len(definition.Requirements.Equipment) != 1 {
			t.Fatalf("invalid axe requirements: %#v", definition.Requirements)
		}
		equipment := definition.Requirements.Equipment[0]
		if equipment.ItemTag != "axe" || equipment.ItemKey != "" || len(equipment.Slots) != 2 ||
			equipment.Slots[0] != "right_hand" || equipment.Slots[1] != "left_hand" {
			t.Fatalf("axe must be allowed in either hand: %#v", equipment)
		}
	}
}

func TestCombatDefinitionLoading(t *testing.T) {
	for _, test := range []struct {
		name          string
		angleDeg      string
		wantAngle     float64
		recoveryTicks string
	}{
		{name: "preset", angleDeg: "90", wantAngle: math.Pi / 2, recoveryTicks: "4"},
		{name: "full circle and no recovery", angleDeg: "360", wantAngle: 2 * math.Pi, recoveryTicks: "0"},
		{name: "fractional degrees", angleDeg: "22.5", wantAngle: math.Pi / 8, recoveryTicks: "4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			contents := strings.Replace(validCombatAction, `"angleDeg":90`, `"angleDeg":`+test.angleDeg, 1)
			contents = strings.Replace(contents, `"recoveryTicks":4`, `"recoveryTicks":`+test.recoveryTicks, 1)
			writeActionFile(t, directory, "combat.json", contents)
			registry, err := LoadFromDirectory(directory, nil)
			if err != nil {
				t.Fatal(err)
			}
			definition, _ := registry.Get("combat_test")
			if definition.Sector == nil || math.Abs(definition.Sector.Angle-test.wantAngle) > 1e-12 {
				t.Fatalf("degrees were not converted to radians: %#v", definition.Sector)
			}
			if err := validateDefinition(definition); err != nil {
				t.Fatal(err)
			}
			if math.Abs(definition.Sector.Angle-test.wantAngle) > 1e-12 {
				t.Fatal("validation converted an already loaded angle again")
			}
		})
	}
}

func TestInvalidCombatDefinitions(t *testing.T) {
	for _, test := range []struct {
		name        string
		original    string
		replacement string
	}{
		{name: "missing sector", original: `,"sector":{"range":18,"angleDeg":90}`},
		{name: "null sector", original: `"sector":{"range":18,"angleDeg":90}`, replacement: `"sector":null`},
		{name: "missing combat", original: `,"combat":{"hitMode":"all","damageMultiplier":1.0}`},
		{name: "null combat", original: `"combat":{"hitMode":"all","damageMultiplier":1.0}`, replacement: `"combat":null`},
		{name: "ordinary target", original: `"kind":"direction"`, replacement: `"kind":"object"`},
		{name: "missing windup", original: `"ticks":6,`},
		{name: "zero windup", original: `"ticks":6`, replacement: `"ticks":0`},
		{name: "negative windup", original: `"ticks":6`, replacement: `"ticks":-1`},
		{name: "missing recovery", original: `"recoveryTicks":4,`},
		{name: "null recovery", original: `"recoveryTicks":4`, replacement: `"recoveryTicks":null`},
		{name: "negative recovery", original: `"recoveryTicks":4`, replacement: `"recoveryTicks":-1`},
		{name: "fractional recovery", original: `"recoveryTicks":4`, replacement: `"recoveryTicks":0.5`},
		{name: "missing range", original: `"range":18,`},
		{name: "zero range", original: `"range":18`, replacement: `"range":0`},
		{name: "negative range", original: `"range":18`, replacement: `"range":-1`},
		{name: "protocol float overflow", original: `"range":18`, replacement: `"range":1e300`},
		{name: "nonfinite range", original: `"range":18`, replacement: `"range":1e999`},
		{name: "missing angle", original: `,"angleDeg":90`},
		{name: "null angle", original: `"angleDeg":90`, replacement: `"angleDeg":null`},
		{name: "zero angle", original: `"angleDeg":90`, replacement: `"angleDeg":0`},
		{name: "negative angle", original: `"angleDeg":90`, replacement: `"angleDeg":-1`},
		{name: "angle exceeds circle", original: `"angleDeg":90`, replacement: `"angleDeg":361`},
		{name: "missing hit mode", original: `"hitMode":"all",`},
		{name: "unknown hit mode", original: `"hitMode":"all"`, replacement: `"hitMode":"unknown"`},
		{name: "missing multiplier", original: `,"damageMultiplier":1.0`},
		{name: "null multiplier", original: `"damageMultiplier":1.0`, replacement: `"damageMultiplier":null`},
		{name: "zero multiplier", original: `"damageMultiplier":1.0`, replacement: `"damageMultiplier":0`},
		{name: "negative multiplier", original: `"damageMultiplier":1.0`, replacement: `"damageMultiplier":-1`},
		{name: "cursor", original: `"kind":"direction"`, replacement: `"kind":"direction","cursor":"atk"`},
		{name: "approach", original: `"kind":"direction"`, replacement: `"kind":"direction","approach":"tile_center"`},
		{name: "execution repeat", original: `"ticks":6`, replacement: `"ticks":6,"repeat":true`},
		{name: "repeatable", original: `"isRepeatable":false`, replacement: `"isRepeatable":true`},
		{name: "unknown sector field", original: `"angleDeg":90`, replacement: `"angleDeg":90,"angle":1.5`},
		{name: "unknown combat field", original: `"hitMode":"all"`, replacement: `"hitMode":"all","baseDamage":6`},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeActionFile(t, directory, "invalid_combat.json", strings.Replace(validCombatAction, test.original, test.replacement, 1))
			_, err := LoadFromDirectory(directory, nil)
			if err == nil || !strings.Contains(err.Error(), "invalid_combat.json") {
				t.Fatalf("expected a file-identifying error, got %v", err)
			}
		})
	}
}

func TestOrdinaryActionRejectsCombatFields(t *testing.T) {
	for _, field := range []string{`"sector":null`, `"combat":null`, `"execution":{"recoveryTicks":0}`} {
		t.Run(field, func(t *testing.T) {
			directory := t.TempDir()
			contents := `{"v":1,"actions":[{"id":"ordinary","presentation":{"label":"Ordinary","menuIcon":"/assets/test.png"},"target":{"kind":"object"},"requirements":{},"execution":{},` + field + `}]}`
			if strings.HasPrefix(field, `"execution"`) {
				contents = strings.Replace(contents, `"execution":{},`, "", 1)
			}
			writeActionFile(t, directory, "ordinary.json", contents)
			_, err := LoadFromDirectory(directory, nil)
			if err == nil || !strings.Contains(err.Error(), "ordinary.json") || !strings.Contains(err.Error(), "ordinary") {
				t.Fatalf("ordinary action accepted combat fields: %v", err)
			}
		})
	}
}

func TestTileCenterApproachValidation(t *testing.T) {
	for _, kind := range []TargetKind{TargetNone, TargetObject, TargetTile} {
		t.Run(string(kind), func(t *testing.T) {
			definition := Definition{ID: "test", Presentation: Presentation{Label: "Test", MenuIcon: "/assets/test.png"}, Target: Target{Kind: kind, Approach: ApproachTileCenter}}
			err := validateDefinition(&definition)
			if (err == nil) != (kind == TargetTile) {
				t.Fatalf("unexpected approach validation: %v", err)
			}
		})
	}
	definition := Definition{ID: "test", Presentation: Presentation{Label: "Test", MenuIcon: "/assets/test.png"}, Target: Target{Kind: TargetTile, Approach: "unknown"}}
	if err := validateDefinition(&definition); err == nil {
		t.Fatal("unknown approach accepted")
	}
}

func TestExecutionRepeatValidation(t *testing.T) {
	base := `{"v":1,"actions":[{"id":"repeat_test","presentation":{"label":"Repeat test","menuIcon":"/assets/cursor/dig.png"},"target":{"kind":"tile","cursor":"dig"},"requirements":{},"execution":{"ticks":4,"stamina":2}}]}`
	tests := []struct {
		name       string
		contents   string
		wantRepeat bool
		wantError  bool
	}{
		{name: "omitted", contents: base},
		{name: "false", contents: strings.Replace(base, `"stamina":2`, `"stamina":2,"repeat":false`, 1)},
		{name: "true", contents: strings.Replace(base, `"stamina":2`, `"stamina":2,"repeat":true`, 1), wantRepeat: true},
		{name: "targetless", contents: strings.Replace(strings.Replace(base, `"kind":"tile","cursor":"dig"`, `"kind":"none"`, 1), `"stamina":2`, `"stamina":2,"repeat":true`, 1), wantError: true},
		{name: "instant", contents: strings.Replace(strings.Replace(base, `"ticks":4`, `"ticks":0`, 1), `"stamina":2`, `"stamina":2,"repeat":true`, 1), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeActionFile(t, directory, "repeat.json", test.contents)
			registry, err := LoadFromDirectory(directory, zap.NewNop())
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "repeat.json") || !strings.Contains(err.Error(), "execution.repeat") {
					t.Fatalf("expected file-named execution.repeat error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			definition, found := registry.Get("repeat_test")
			if !found || definition.Execution.Repeat != test.wantRepeat {
				t.Fatalf("unexpected repeat setting: %#v", definition)
			}
		})
	}
}

func TestCooldownDefinitionValidation(t *testing.T) {
	for _, value := range []string{"0", "2000", "4294967295", "-1", "0.5", "4294967296", "null", "\"2000\""} {
		t.Run(value, func(t *testing.T) {
			directory := t.TempDir()
			contents := `{"v":1,"actions":[{"id":"test","presentation":{"label":"Test","menuIcon":"/assets/test.png"},"target":{"kind":"none"},"requirements":{},"execution":{},"cooldown":` + value + `}]}`
			writeActionFile(t, directory, "cooldown.json", contents)
			_, err := LoadFromDirectory(directory, nil)
			valid := value == "0" || value == "2000" || value == "4294967295"
			if valid != (err == nil) {
				t.Fatalf("cooldown %s: %v", value, err)
			}
			if err != nil && !strings.Contains(err.Error(), "cooldown.json") {
				t.Fatalf("error omitted source file: %v", err)
			}
		})
	}
}
