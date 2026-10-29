package actiondefs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const naturalBite = `{"v":1,"actions":[{"id":"fox_bite","audience":"npc","presentation":{"label":"Bite","menuIcon":"/assets/cursor/atk.png"},"target":{"kind":"direction"},"requirements":{},"execution":{"windupMs":400},"cooldown":1500,"sector":{"range":12,"angleDeg":60},"combat":{"hitMode":"nearest","rawDamage":6}}]}`

func TestNaturalCombatDefinitionValidation(t *testing.T) {
	for _, test := range []struct{ name, from, to string }{
		{"player_natural", `"audience":"npc"`, `"audience":"player"`},
		{"default_player_natural", `"audience":"npc",`, ``},
		{"unknown_audience", `"audience":"npc"`, `"audience":"everyone"`},
		{"mixed_multiplier", `"rawDamage":6`, `"rawDamage":6,"damageMultiplier":1`},
		{"mixed_equipment", `"requirements":{}`, `"requirements":{"equipment":[{"slots":["right_hand"],"itemTag":"axe","damageSource":true}]}`},
		{"mixed_skills", `"requirements":{}`, `"requirements":{"skills":["melee"]}`},
		{"stamina", `"windupMs":400`, `"windupMs":400,"stamina":1`},
		{"negative_raw", `"rawDamage":6`, `"rawDamage":-1`},
		{"zero_raw", `"rawDamage":6`, `"rawDamage":0`},
		{"null_raw", `"rawDamage":6`, `"rawDamage":null`},
		{"missing_raw", `"rawDamage":6`, `"damageMultiplier":1`},
		{"mixed_duration", `"windupMs":400`, `"windupMs":400,"ticks":4`},
		{"repeating_direction", `"windupMs":400`, `"windupMs":400,"repeat":true`},
		{"untimed", `"windupMs":400`, `"windupMs":0`},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeActionFile(t, dir, "bite.json", strings.Replace(naturalBite, test.from, test.to, 1))
			_, err := LoadFromDirectory(dir, nil)
			require.Error(t, err)
		})
	}
	dir := t.TempDir()
	writeActionFile(t, dir, "bite.json", naturalBite)
	registry, err := LoadFromDirectory(dir, nil)
	require.NoError(t, err)
	definition, ok := registry.Get("fox_bite")
	require.True(t, ok)
	require.False(t, definition.PlayerAvailable())
	require.Equal(t, 6.0, *definition.Combat.RawDamage)
	require.Equal(t, uint32(400), definition.Execution.WindupMs)
	require.True(t, (&Definition{}).PlayerAvailable(), "omitted audience preserves player actions")
}
