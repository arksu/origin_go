package actionanimationdefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSharedFixtures(t *testing.T) {
	contents, err := os.ReadFile("../../tests/fixtures/action_animations/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string          `json:"name"`
		Valid bool            `json:"valid"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(contents, &cases); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range cases {
		t.Run(fixture.Name, func(t *testing.T) {
			directory := t.TempDir()
			filename := filepath.Join(directory, "fixture.json")
			if err := os.WriteFile(filename, fixture.Input, 0600); err != nil {
				t.Fatal(err)
			}
			registry, err := LoadFromDirectory(directory, nil)
			if fixture.Valid {
				if err != nil {
					t.Fatal(err)
				}
				for _, binding := range registry.All() {
					found, ok := registry.Resolve(Source{Kind: " " + binding.Source.Kind + " ", Namespace: binding.Source.Namespace, ID: binding.Source.ID})
					if !ok || found.Key != binding.Key {
						t.Fatal("normalized source lookup failed")
					}
				}
			} else if err == nil || registry != nil || !strings.Contains(err.Error(), "fixture.json") {
				t.Fatalf("invalid input must fail atomically and identify file: registry=%v error=%v", registry, err)
			}
		})
	}
}

func TestDuplicateAcrossFilesAndProduction(t *testing.T) {
	registry, err := LoadFromDirectory("../../data/action_animations", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.All()) == 0 {
		t.Fatal("expected production bindings")
	}
	binding := registry.All()[0]
	duplicate := *binding
	duplicate.SourceFile = "second.json"
	if partial, err := NewRegistry([]Definition{*binding, duplicate}); partial != nil || err == nil || !strings.Contains(err.Error(), "second.json") {
		t.Fatalf("duplicate accepted: %v", err)
	}
	if _, ok := registry.Resolve(Source{Kind: "menu", ID: "unmapped"}); ok {
		t.Fatal("unmapped selector matched")
	}
}

func TestOptionalUnbindSlotsSurviveDefinitionRoundTrip(t *testing.T) {
	contents, err := os.ReadFile("../../tests/fixtures/action_animations/bindings.json")
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := Parse(contents, "fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		encoded, err := json.Marshal(map[string]any{"v": 1, "bindings": []Definition{definition}})
		if err != nil {
			t.Fatal(err)
		}
		reloaded, err := Parse(encoded, "roundtrip.json")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(reloaded[0].UnbindEquipmentSlots, definition.UnbindEquipmentSlots) {
			t.Fatal("unbind slots changed during round trip")
		}
	}
}

func TestCombatSourcesResolveExplicitCombatDefinitions(t *testing.T) {
	registry, err := LoadFromDirectory("../../data/action_animations", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateCombatSources(func(id string) bool { return id == "axe_aoe" || id == "axe_single" }); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateCombatSources(func(id string) bool { return id == "axe_aoe" }); err == nil || !strings.Contains(err.Error(), "axe_single") {
		t.Fatal("missing combat source was accepted", err)
	}
	if err := registry.ValidateCombatSources(nil); err == nil {
		t.Fatal("missing source resolver accepted")
	}
}
