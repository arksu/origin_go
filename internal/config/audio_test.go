package config

import (
	"github.com/spf13/viper"
	"math"
	"testing"
)

func TestAudioDefaultsAndValidation(t *testing.T) {
	settings := viper.New()
	setDefaults(settings)
	var configured Config
	if err := settings.Unmarshal(&configured); err != nil {
		t.Fatal(err)
	}
	if configured.Game.Audio != DefaultAudioConfig() {
		t.Fatalf("audio defaults differ: %#v", configured.Game.Audio)
	}
	if err := configured.Game.Audio.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*AudioConfig){
		func(audio *AudioConfig) { audio.BaseHearing = 0 },
		func(audio *AudioConfig) { audio.BaseHearing = -1 },
		func(audio *AudioConfig) { audio.BaseHearing = math.NaN() },
		func(audio *AudioConfig) { audio.ListenerCellSize = math.Inf(1) },
		func(audio *AudioConfig) { audio.MaxEffectiveRadius = math.MaxFloat64 },
		func(audio *AudioConfig) { audio.MaxCellsPerTick = 0 },
		func(audio *AudioConfig) { audio.MaxCandidatesPerTick = -1 },
		func(audio *AudioConfig) { audio.MaxEntriesPerTick = 0 },
		func(audio *AudioConfig) { audio.MaxBatchBytes = 0 },
		func(audio *AudioConfig) { audio.QueueCapacity = 0 },
	} {
		audio := DefaultAudioConfig()
		change(&audio)
		if err := audio.Validate(); err == nil {
			t.Fatalf("accepted invalid configuration %#v", audio)
		}
	}
}
