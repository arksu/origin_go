package sounddefs

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func TestSharedValidationFixtures(t *testing.T) {
	contents, err := os.ReadFile("../../tests/fixtures/sounds/cases.json")
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
			_, err := Parse(fixture.Input, "fixture.json")
			if (err == nil) != fixture.Valid {
				t.Fatalf("valid=%v, error=%v", fixture.Valid, err)
			}
		})
	}
}

func TestDefinitionLoadingNeedsNoMountedAudio(t *testing.T) {
	registry, err := LoadFromDirectory("../../data/sounds", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = registry.ValidateHearing(1, 4096); err != nil {
		t.Fatal(err)
	}
	if err = registry.ValidateHearing(4, 4096); err == nil {
		t.Fatal("accepted effective radius over limit")
	}
	chop, ok := registry.Get("chop")
	if !ok || chop.Mode != ModeWorld || chop.Loudness != 1000 || chop.Volume != .9 {
		t.Fatalf("unexpected chop %#v", chop)
	}
	footstep, ok := registry.Get("footstep")
	if !ok || footstep.Mode != ModeLocal || len(footstep.Files) != 4 {
		t.Fatalf("unexpected step %#v", footstep)
	}
}

func TestFiniteHearingAndRadius(t *testing.T) {
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := EffectiveRadius(1000, value, 4096); err == nil {
			t.Fatalf("accepted hearing %g", value)
		}
		if _, err := EffectiveRadius(value, 1, 4096); err == nil {
			t.Fatalf("accepted loudness %g", value)
		}
	}
	if radius, err := EffectiveRadius(1000, 1.5, 4096); err != nil || radius != 1500 {
		t.Fatalf("radius=%g err=%v", radius, err)
	}
	registry, err := LoadFromDirectory("../../data/sounds", nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := *registry.all[0]
	profile.Loudness = math.NaN()
	if _, err := NewRegistry([]Profile{profile}); err == nil {
		t.Fatal("accepted NaN definition")
	}
}

func TestLocalNearDistanceRejectsNonfiniteValues(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		distance float64
	}{
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			profile := localNearDistanceProfile(fixture.distance)
			_, err := NewRegistry([]Profile{profile})
			if err == nil || !strings.Contains(err.Error(), "near_distance") {
				t.Fatalf("distance=%g error=%v", fixture.distance, err)
			}
		})
	}
}

func TestLocalNearDistanceRequiresEffectiveFadeInterval(t *testing.T) {
	registry, err := NewRegistry([]Profile{localNearDistanceProfile(16)})
	if err != nil {
		t.Fatal(err)
	}
	for _, hearing := range []float64{.0625, .125} {
		if err = registry.ValidateHearing(hearing, 4096); err == nil || !strings.Contains(err.Error(), "near_distance") {
			t.Fatalf("accepted hearing=%g with collapsed fade interval: %v", hearing, err)
		}
	}
	for _, hearing := range []float64{.25, 1, 2} {
		if err = registry.ValidateHearing(hearing, 4096); err != nil {
			t.Fatalf("rejected hearing=%g: %v", hearing, err)
		}
	}
	legacy, err := NewRegistry([]Profile{localNearDistanceProfile(0)})
	if err != nil {
		t.Fatal(err)
	}
	if err = legacy.ValidateHearing(.0625, 4096); err != nil {
		t.Fatalf("zero near distance must preserve legacy hearing scaling: %v", err)
	}
}

func localNearDistanceProfile(distance float64) Profile {
	return Profile{
		Key: "footstep", Mode: ModeLocal, Loudness: 128, Volume: .35,
		Files: []string{"sound/steps/step_01.wav"}, MaxVoices: 12, MaxVoicesPerSource: 2,
		LocalAttenuation: &LocalAttenuation{NearGain: .9, FarGain: 0, Shape: 4, NearDistance: distance},
	}
}
