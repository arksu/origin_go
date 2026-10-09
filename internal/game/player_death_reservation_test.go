package game

import (
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReservedInventoryDoesNotRunKnockoutCleanupForHealthyPlayer(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityHealth{HHP: 20, SHP: 20})
		ecs.AddComponent(w, handle, components.EntityStats{Energy: 900})
		ecs.AddComponent(w, handle, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 42})
	})
	ecs.GetResource[ecs.CharacterEntities](w).Add(1, player, time.Now())
	ecs.SetResource(w, ecs.TimeState{Tick: 1, UnixMs: 1000})
	require.True(t, ecs.ReserveInventoryOwner(w, 1, player))
	require.True(t, playerstate.ItemsLocked(w, player))
	shard := &Shard{world: w, logger: zap.NewNop()}
	system := NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{})
	system.Update(w, 0)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, constt.StateMoving, movement.State)
	require.Equal(t, constt.TargetPoint, movement.TargetType)
	require.Equal(t, 42.0, movement.TargetX)
	require.False(t, playerstate.IsIncapacitated(w, player))

	// Real KO still performs the ordinary cleanup while the reservation survives.
	ecs.WithComponent(w, player, func(health *components.EntityHealth) { health.SHP = 0 })
	system.Update(w, 0)
	movement, _ = ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, constt.StateIdle, movement.State)
	require.Equal(t, constt.TargetNone, movement.TargetType)
	require.True(t, playerstate.IsIncapacitated(w, player))
	require.True(t, ecs.InventoryOwnerReserved(w, 1))
}
