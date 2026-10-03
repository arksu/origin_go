package actionanimationdefs

import (
	"os"
	"strings"
	"testing"
)

func TestMenuTargetSoundSourceAvailability(t *testing.T) {
	contents, err := os.ReadFile("../../tests/fixtures/action_animations/bindings.json")
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := Parse(contents, "menu-cues.json")
	if err != nil {
		t.Fatal(err)
	}
	binding := bindings[0]
	binding.Source = Source{Kind: "menu", ID: "sample_menu_action"}
	binding.SoundCues = []SoundCue{{ID: "impact", Phase: .6, SoundKey: "chop", Source: "target"}}
	registry, err := NewRegistry([]Definition{binding})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"object", "tile"} {
		if err := registry.ValidateMenuSoundTargets(func(actionID string) (string, bool) {
			if actionID != binding.Source.ID {
				t.Fatalf("unexpected action %s", actionID)
			}
			return kind, true
		}); err != nil {
			t.Fatalf("valid %s target: %v", kind, err)
		}
	}
	for _, kind := range []string{"none", "", "unknown"} {
		err := registry.ValidateMenuSoundTargets(func(string) (string, bool) { return kind, true })
		if err == nil || !strings.Contains(err.Error(), "sample_menu_action") || !strings.Contains(err.Error(), "impact") {
			t.Fatalf("invalid target %q: %v", kind, err)
		}
	}
	if err := registry.ValidateMenuSoundTargets(func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("accepted missing menu definition")
	}
	if err := registry.ValidateMenuSoundTargets(nil); err == nil {
		t.Fatal("accepted missing resolver")
	}
	binding.SoundCues[0].Source = "actor"
	registry, err = NewRegistry([]Definition{binding})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateMenuSoundTargets(func(string) (string, bool) { return "none", true }); err != nil {
		t.Fatalf("actor-only cue rejected: %v", err)
	}
	binding.Source.Kind = "context"
	binding.SoundCues[0].Source = "target"
	registry, err = NewRegistry([]Definition{binding})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateMenuSoundTargets(nil); err != nil {
		t.Fatalf("context target rejected: %v", err)
	}
}
