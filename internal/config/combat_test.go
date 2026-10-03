package config

import (
	"github.com/spf13/viper"
	"testing"
)

func TestCombatTestingIsOptInAndRejectsInvalidFlag(t *testing.T) {
	settings := viper.New()
	setDefaults(settings)
	var configured Config
	if err := settings.Unmarshal(&configured); err != nil || configured.Game.CombatTestEnabled {
		t.Fatal("combat must default off", err)
	}
	settings.Set("game.combat_test_enabled", true)
	if err := settings.Unmarshal(&configured); err != nil || !configured.Game.CombatTestEnabled {
		t.Fatal("explicit enable failed", err)
	}
	settings.Set("game.combat_test_enabled", "not-a-bool")
	if err := settings.Unmarshal(&configured); err == nil {
		t.Fatal("invalid flag accepted")
	}
}
