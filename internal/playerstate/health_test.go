package playerstate

import (
	"github.com/stretchr/testify/require"
	"origin/internal/ecs/components"
	"testing"
)

func TestFixedDeadlineAndOneTimeCompletion(t *testing.T) {
	for _, hhp := range []float64{.4, 20} {
		health := components.EntityHealth{HHP: hhp}
		StartKnockout(&health, 1000)
		require.Equal(t, int64(61000), health.KOUntilUnixMs)
		require.Equal(t, uint64(1), health.LyingRevision)
		health.SHP = hhp
		ResolveKnockout(&health, 60999)
		require.Equal(t, int64(61000), health.KOUntilUnixMs, "regen must not end KO")
		health.SHP = 0
		StartKnockout(&health, 60999)
		require.Equal(t, int64(61000), health.KOUntilUnixMs, "damage must not extend KO")
		ResolveKnockout(&health, 61000)
		require.Equal(t, min(hhp, 1), health.SHP)
		require.Zero(t, health.KOUntilUnixMs)
		completed := health
		ResolveKnockout(&health, 62000)
		require.Equal(t, completed, health)
		for _, standing := range []bool{false, true} {
			SetLying(&health, !standing)
			health.SHP = 0
			ResolveKnockout(&health, 70000)
			require.True(t, health.IsLying)
			require.Equal(t, int64(130000), health.KOUntilUnixMs)
			ResolveKnockout(&health, 130000)
		}
	}
}

func TestDeathHasPriorityAtDeadline(t *testing.T) {
	health := components.EntityHealth{KOUntilUnixMs: 60000, IsLying: true}
	ResolveKnockout(&health, 60000)
	require.Zero(t, health.SHP)
	require.Zero(t, health.KOUntilUnixMs)
	require.Zero(t, health.LyingRevision)
}
