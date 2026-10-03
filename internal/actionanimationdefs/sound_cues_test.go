package actionanimationdefs

import (
	"origin/internal/sounddefs"
	"testing"
)

func TestPrepareSoundCueOwnershipAndMissingReference(t *testing.T) {
	bindings, err := LoadFromDirectory("../../data/action_animations", nil)
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := sounddefs.LoadFromDirectory("../../data/sounds", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := bindings.PrepareSoundCues(sounds); err != nil {
		t.Fatal(err)
	}
	chop, ok := bindings.Get("tree_chop")
	if !ok {
		t.Fatal("missing chop")
	}
	if len(chop.WorldSoundCues) != 1 || chop.WorldSoundCues[0].Phase != .6 || chop.WorldSoundCues[0].Source != "target" || len(chop.LocalSoundCues) != 0 {
		t.Fatalf("unexpected cues %#v", chop)
	}
	changed := *chop
	changed.SoundCues = []SoundCue{{ID: "missing", Phase: .2, Source: "actor", SoundKey: "missing"}}
	registry, err := NewRegistry([]Definition{changed})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.PrepareSoundCues(sounds); err == nil {
		t.Fatal("accepted missing sound")
	}
	changed.SoundCues = []SoundCue{{ID: "quiet", Phase: .2, Source: "actor", SoundKey: "footstep"}}
	registry, err = NewRegistry([]Definition{changed})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.PrepareSoundCues(sounds); err != nil {
		t.Fatal(err)
	}
	prepared, _ := registry.Get(changed.Key)
	if len(prepared.LocalSoundCues) != 1 || len(prepared.WorldSoundCues) != 0 {
		t.Fatal("local cue entered world set")
	}
}
