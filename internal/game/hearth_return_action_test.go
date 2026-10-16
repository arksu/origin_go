package game

import (
	"fmt"
	"testing"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/hearth"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func hearthReturnTestDefinitions() *actiondefs.Registry {
	return actiondefs.NewRegistry([]actiondefs.Definition{{ID: hearth.ReturnActionID,
		Target:    actiondefs.Target{Kind: actiondefs.TargetNone},
		Execution: actiondefs.Execution{Ticks: 50}, Cooldown: 360000,
	}})
}

func newHearthReturnActionTest(t *testing.T) (*hearthReturnActionHandler, types.Handle, *testActionSender) {
	t.Helper()
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).UnixMs = 100000
	player := w.Spawn(10, nil)
	ecs.AddComponent(w, player, components.EntityHealth{HHP: 20, SHP: 10})
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 100})
	ecs.AddComponent(w, player, components.Movement{Mode: constt.Walk, State: constt.StateIdle})
	require.True(t, ecs.GetResource[ecs.CombatActivityState](w).Prepare(player, 10))
	client := &network.Client{ID: 1, CharacterID: 10}
	client.InWorld.Store(true)
	shard := &Shard{world: w, Clients: map[types.EntityID]*network.Client{10: client}}
	h := &hearthReturnActionHandler{shard: shard, index: hearth.NewIndex()}
	require.NoError(t, h.index.LoadIfUnknown(10, &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 200}, X: 200, Y: 200}))
	sender := &testActionSender{}
	var err error
	shard.actionService, err = NewActionService(w, hearthReturnTestDefinitions(), map[string]ActionHandler{hearth.ReturnActionID: h}, sender)
	require.NoError(t, err)
	return h, player, sender
}

func TestHearthReturnEligibilityAndOrdinaryProgress(t *testing.T) {
	for _, mode := range []string{"standing", "lying", "resolved_ko", "unknown", "no_home", "terminal", "combat", "swimming", "active_ko", "unresolved_ko", "dead", "cooldown"} {
		t.Run(mode, func(t *testing.T) {
			h, player, sender := newHearthReturnActionTest(t)
			w, service := h.shard.world, h.shard.actionService
			switch mode {
			case "lying", "resolved_ko":
				ecs.WithComponent(w, player, func(health *components.EntityHealth) {
					health.IsLying = true
					if mode == "resolved_ko" {
						health.KOUntilUnixMs, health.SHP = 100000, 0
						playerstate.ResolveKnockout(health, 100000)
					}
				})
			case "unknown":
				h.index = hearth.NewIndex()
			case "no_home":
				_, err := h.index.RevokeHome(10)
				require.NoError(t, err)
			case "terminal":
				require.NoError(t, h.index.MarkTerminalPending(10))
			case "combat":
				ecs.GetResource[ecs.CombatActivityState](w).RecordPreparedEvent(player, 99999)
			case "swimming":
				ecs.WithComponent(w, player, func(m *components.Movement) { m.Mode = constt.Swim })
			case "active_ko":
				ecs.WithComponent(w, player, func(health *components.EntityHealth) { health.KOUntilUnixMs = 100001 })
			case "unresolved_ko":
				ecs.WithComponent(w, player, func(health *components.EntityHealth) { health.SHP = 0 })
			case "dead":
				ecs.WithComponent(w, player, func(health *components.EntityHealth) { health.HHP = 0 })
			case "cooldown":
				ecs.AddComponent(w, player, components.ActionCooldowns{ByAction: map[string]components.ActionCooldown{hearth.ReturnActionID: {ExpiresAtMs: 100001}}})
			}
			service.Activate(w, 10, player, hearth.ReturnActionID)
			accepted := mode == "standing" || mode == "lying" || mode == "resolved_ko"
			require.Equal(t, accepted, ecs.HasComponent[components.ActiveGameAction](w, player))
			if !accepted {
				require.NotEmpty(t, sender.alerts)
				return
			}
			cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.True(t, exists)
			require.EqualValues(t, 50, cycle.CycleDurationTicks)
			for range 49 {
				cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](w, player)
				service.AdvanceCycle(w, 10, player, cycle, sender)
			}
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.EqualValues(t, 100, stats.Stamina)
			require.False(t, service.isOnCooldown(w, player, hearth.ReturnActionID))
			// Combat appearing before completion cancels through ordinary recheck.
			ecs.GetResource[ecs.CombatActivityState](w).RecordPreparedEvent(player, 100000)
			service.Recheck(w, 10, player)
			require.False(t, ecs.HasComponent[components.ActiveGameAction](w, player))
		})
	}
}

func TestHearthReturnSourceAndResultGuards(t *testing.T) {
	for _, change := range []string{"none", "canceled", "new_generation", "new_session", "combat", "swimming", "terminal", "new_handle"} {
		t.Run(change, func(t *testing.T) {
			h, player, _ := newHearthReturnActionTest(t)
			w, service := h.shard.world, h.shard.actionService
			service.Activate(w, 10, player, hearth.ReturnActionID)
			active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
			req := h.transferRequest(10, player, active.Generation)
			require.NoError(t, req.SourcePolicy(h.shard, player, h.shard.Clients[10]))
			switch change {
			case "canceled":
				service.Cancel(w, 10, player)
			case "new_generation":
				active.Generation++
				ecs.AddComponent(w, player, active)
			case "new_session":
				h.shard.Clients[10] = &network.Client{ID: 2, CharacterID: 10}
				h.shard.Clients[10].InWorld.Store(true)
			case "combat":
				ecs.GetResource[ecs.CombatActivityState](w).RecordPreparedEvent(player, 100000)
			case "swimming":
				ecs.WithComponent(w, player, func(m *components.Movement) { m.Mode = constt.Swim })
			case "terminal":
				require.NoError(t, h.index.MarkTerminalPending(10))
			case "new_handle":
				require.True(t, w.Despawn(player))
				player = w.Spawn(10, nil)
				ecs.AddComponent(w, player, active)
			}
			err := req.SourcePolicy(h.shard, player, h.shard.Clients[10])
			if change == "none" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			req.OnResult(h.shard, types.InvalidHandle, fmt.Errorf("transfer failed"))
			preserved := change == "new_generation" || change == "new_session" || change == "new_handle"
			require.Equal(t, preserved, ecs.HasComponent[components.ActiveGameAction](w, player))
			require.False(t, service.isOnCooldown(w, player, hearth.ReturnActionID))
		})
	}
}

func TestHearthReturnCooldownResultIsOnceAndIndependent(t *testing.T) {
	h, player, _ := newHearthReturnActionTest(t)
	w, service := h.shard.world, h.shard.actionService
	service.Activate(w, 10, player, hearth.ReturnActionID)
	active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
	req := h.transferRequest(10, player, active.Generation)
	ecs.AddComponent(w, player, components.ActionCooldowns{ByAction: map[string]components.ActionCooldown{"other": {StartedAtMs: 90000, ExpiresAtMs: 500000}}})
	req.OnResult(h.shard, player, nil)
	ecs.GetResource[ecs.TimeState](w).UnixMs++
	req.OnResult(h.shard, player, nil)
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](w, player)
	require.Equal(t, components.ActionCooldown{StartedAtMs: 100000, ExpiresAtMs: 460000}, cooldowns.ByAction[hearth.ReturnActionID])
	require.Equal(t, components.ActionCooldown{StartedAtMs: 90000, ExpiresAtMs: 500000}, cooldowns.ByAction["other"])
}

func TestHearthReturnAdmissionRetiresBuildAndPersonalCraft(t *testing.T) {
	for _, outcome := range []string{"cancel", "failure", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			h, player, sender := newHearthReturnActionTest(t)
			w, service := h.shard.world, h.shard.actionService
			h.shard.buildService = &BuildService{}
			h.shard.contextActions = &ContextActionService{world: w, cyclicOut: sender}
			ecs.AddComponent(w, player, components.PendingBuildPlacement{BuildKey: "hearth_fire", TargetX: 300})
			ecs.AddComponent(w, player, components.Collider{Phantom: &components.PhantomCollider{WorldX: 300, HalfWidth: 6, HalfHeight: 6}})
			ecs.AddComponent(w, player, components.PendingInteraction{TargetEntityID: 200})
			ecs.AddComponent(w, player, components.PendingContextAction{ActionID: "open"})
			ecs.AddComponent(w, player, components.ActiveCraft{CraftKey: "personal_craft", RemainingCycles: 3})
			ecs.AddComponent(w, player, components.ActiveCyclicAction{ActionID: "craft", BehaviorKey: "craft", CycleDurationTicks: 20})
			ecs.WithComponent(w, player, func(movement *components.Movement) { movement.SetTargetPoint(300, 0) })
			if outcome == "rejected" {
				ecs.AddComponent(w, player, components.ActionCooldowns{ByAction: map[string]components.ActionCooldown{hearth.ReturnActionID: {ExpiresAtMs: 100001}}})
			}
			service.Activate(w, 10, player, hearth.ReturnActionID)
			if outcome == "rejected" {
				require.True(t, ecs.HasComponent[components.PendingBuildPlacement](w, player))
				require.True(t, ecs.HasComponent[components.ActiveCraft](w, player))
				require.Empty(t, sender.finished)
				return
			}
			require.Len(t, sender.finished, 1)
			require.Equal(t, "craft", sender.finished[0].ActionId)
			require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED, sender.finished[0].Result)
			cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.True(t, exists)
			require.Equal(t, hearth.ReturnActionID, cycle.ActionID)
			if outcome == "cancel" {
				service.Cancel(w, 10, player)
			} else {
				ecs.GetResource[ecs.CombatActivityState](w).RecordPreparedEvent(player, 100000)
				service.Recheck(w, 10, player)
			}
			require.False(t, ecs.HasComponent[components.PendingBuildPlacement](w, player))
			require.False(t, ecs.HasComponent[components.PendingInteraction](w, player))
			require.False(t, ecs.HasComponent[components.PendingContextAction](w, player))
			require.False(t, ecs.HasComponent[components.ActiveCraft](w, player))
			require.False(t, ecs.HasComponent[components.ActiveCyclicAction](w, player))
			collider, _ := ecs.GetComponent[components.Collider](w, player)
			require.Nil(t, collider.Phantom)
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			require.Equal(t, constt.TargetNone, movement.TargetType)
		})
	}
}
