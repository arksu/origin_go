package game

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"testing"
)

type combatWorkProbe struct{ calls int }

func (p *combatWorkProbe) HandleStartCraftOne(*ecs.World, types.EntityID, types.Handle, *netproto.C2S_StartCraftOne) {
	p.calls++
}
func (p *combatWorkProbe) HandleStartCraftMany(*ecs.World, types.EntityID, types.Handle, *netproto.C2S_StartCraftMany) {
	p.calls++
}
func (p *combatWorkProbe) HandleStartBuild(*ecs.World, types.EntityID, types.Handle, *netproto.C2S_BuildStart) {
	p.calls++
}
func (p *combatWorkProbe) HandleBuildProgress(*ecs.World, types.EntityID, types.Handle, *netproto.C2S_BuildProgress) {
	p.calls++
}
func (p *combatWorkProbe) HandleBuildTakeBack(*ecs.World, types.EntityID, types.Handle, *netproto.C2S_BuildTakeBack) {
	p.calls++
}
func (p *combatWorkProbe) SendBuildStateSnapshot(*ecs.World, types.EntityID, types.EntityID) {
	p.calls++
}

func TestCombatRawWorkAndObjectClicksCannotBypassCommitment(t *testing.T) {
	fixture := newCombatFixture(t, 500)
	probe := &combatWorkProbe{}
	fixture.commands.SetCraftCommandService(probe)
	fixture.commands.SetBuildCommandService(probe)
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	for _, milliseconds := range []int64{200, 700} {
		fixture.at(milliseconds)
		fixture.service.Update(fixture.world, .1)
		for _, request := range []struct {
			kind    network.CommandType
			payload any
		}{
			{network.CmdStartCraftOne, &netproto.C2S_StartCraftOne{}},
			{network.CmdStartCraftMany, &netproto.C2S_StartCraftMany{}},
			{network.CmdStartBuild, &netproto.C2S_BuildStart{}},
			{network.CmdBuildProgress, &netproto.C2S_BuildProgress{}},
			{network.CmdBuildTakeBack, &netproto.C2S_BuildTakeBack{}},
			{network.CmdSelectContextAction, &netproto.SelectContextAction{}},
		} {
			fixture.send(t, request.kind, request.payload, 1, 0)
		}
		fixture.send(t, network.CmdMapClick, &netproto.MapClick{X: 200, Y: 200, TargetEntityId: 2}, 1, 0)
		movement, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.actor)
		if movement.TargetType != constt.TargetPoint || movement.TargetHandle != 0 || movement.TargetX != 200 {
			t.Fatal("object click became interaction")
		}
		fixture.send(t, network.CmdMapClick, &netproto.MapClick{X: 300, Y: 300, TargetEntityId: 2, Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY}, 1, 0)
		after, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.actor)
		if after != movement || probe.calls != 0 || !components.CombatCommitted(fixture.world, fixture.actor) {
			t.Fatal("busy raw request had effects")
		}
		if ecs.HasComponent[components.PendingInteraction](fixture.world, fixture.actor) || ecs.HasComponent[components.PendingContextAction](fixture.world, fixture.actor) || ecs.HasComponent[components.PendingBuildPlacement](fixture.world, fixture.actor) {
			t.Fatal("busy request queued work")
		}
		lift := &LiftService{}
		if lift.StartLift(fixture.world, 1, fixture.actor, 2, fixture.target).Reason != "ACTION_BUSY" || lift.StartPutDownAt(fixture.world, 1, fixture.actor, 100, 100, 1).Reason != "ACTION_BUSY" {
			t.Fatal("lift shortcut bypassed commitment")
		}
	}
	fixture.at(1000)
	fixture.service.Update(fixture.world, .1)
	fixture.send(t, network.CmdStartCraftOne, &netproto.C2S_StartCraftOne{}, 1, 0)
	if probe.calls != 1 {
		t.Fatal("work remained blocked after recovery")
	}
}
