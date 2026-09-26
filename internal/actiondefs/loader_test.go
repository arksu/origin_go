package actiondefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"origin/internal/itemdefs"

	"go.uber.org/zap"
)

const validAction = `{"v":1,"actions":[{"id":"test_action","presentation":{"label":"Test action","menuIcon":"/assets/cursor/lift.png"},"target":{"kind":"object","cursor":"lift"},"requirements":{"skills":["test_skill"],"equipment":[{"slots":["left_hand","right_hand"],"itemTag":"axe"},{"slots":["back"],"itemKey":"test_pack"}]},"execution":{"ticks":4,"stamina":2.5},"isRepeatable":true}]}`

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
	if err := registry.ValidateHandlers(nil); err == nil || !strings.Contains(err.Error(), "valid.json") {
		t.Fatalf("missing handler must identify its definition file, got %v", err)
	}
	if err := registry.ValidateHandlers([]string{"test_action", "unknown"}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unmatched handler must fail, got %v", err)
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
	if len(registry.All()) != 3 {
		t.Fatalf("expected lift, lift_down and plow_tile, got %d", len(registry.All()))
	}
	if err := registry.ValidateHandlers([]string{"lift", "lift_down", "plow_tile"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lift", "lift_down"} {
		definition, exists := registry.Get(id)
		if !exists || definition.Repeatable() || definition.Execution.Ticks != 0 || definition.Execution.Stamina != 0 || definition.Target.Approach != "" {
			t.Fatalf("invalid production action %q: %#v", id, definition)
		}
	}
	plow, _ := registry.Get("plow_tile")
	if plow.Target.Kind != TargetTile || plow.Target.Approach != ApproachTileCenter || plow.Target.Cursor != "dig" ||
		!plow.Repeatable() || plow.Execution.Ticks != 20 || plow.Execution.Stamina != 250 ||
		len(plow.Requirements.Skills) != 0 || len(plow.Requirements.Equipment) != 0 {
		t.Fatalf("invalid plow definition: %#v", plow)
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
