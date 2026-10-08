package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestCreatureDamagePreparationDoesNotPublishHealthMovementOrNotifications(t *testing.T) {
	f := newCreatureDamageFixture(t)
	ecs.AddComponent(f.world, f.owner, components.Movement{State: constt.StateMoving, VelocityX: 3})
	before := f.health()
	movement, _ := ecs.GetComponent[components.Movement](f.world, f.owner)
	writes := 0
	f.world.AddComponentObserver(components.EntityHealthComponentID, func(types.Handle) { writes++ })
	plan, err := f.service.prepareDamage(f.owner, 30)
	require.NoError(t, err)
	require.Equal(t, before, f.health())
	actualMovement, _ := ecs.GetComponent[components.Movement](f.world, f.owner)
	require.Equal(t, movement, actualMovement)
	require.Zero(t, writes)
	require.Zero(t, f.stats.PendingPlayerPushCount())
	require.Zero(t, f.visual.PendingCount())
	require.Equal(t, float64(28), plan.result.After.HHP)
	require.True(t, plan.result.EnteredKO)
	f.service.commitDamage(plan)
	require.Equal(t, plan.result.After, f.health())
	require.Equal(t, 1, writes)
	actualMovement, _ = ecs.GetComponent[components.Movement](f.world, f.owner)
	require.Equal(t, constt.StateIdle, actualMovement.State)
	require.Equal(t, 1, f.stats.PendingPlayerPushCount())
	require.Equal(t, 1, f.visual.PendingCount())
}

func TestCreatureDamagePreparationAllocationsAndZeroCommitNoop(t *testing.T) {
	f := newCreatureDamageFixture(t)
	var plan creatureDamageCommit
	var err error
	for _, draw := range []float64{0, 1, 30, -1} {
		allocs := testing.AllocsPerRun(1000, func() {
			plan, err = f.service.prepareDamage(f.owner, draw)
		})
		require.Zero(t, allocs)
		if draw >= 0 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		require.Equal(t, components.EntityHealth{SHP: 10, HHP: 50}, f.health())
		require.Zero(t, f.stats.PendingPlayerPushCount())
		require.Zero(t, f.visual.PendingCount())
	}
	plan, err = f.service.prepareDamage(f.owner, 0)
	require.NoError(t, err)
	f.world.AddComponentObserver(components.EntityHealthComponentID, func(types.Handle) { t.Fatal("zero commit wrote health") })
	f.service.commitDamage(plan)
}
