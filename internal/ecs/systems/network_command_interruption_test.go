package systems

import (
	"testing"

	"go.uber.org/zap"
	"origin/internal/ecs"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testInterruptActionRouter struct {
	testActionClickRouter
	interrupts int
}

func (router *testInterruptActionRouter) Interrupt(*ecs.World, types.EntityID, types.Handle) {
	router.interrupts++
}

func TestAdminPrimaryClickConsumptionPrecedesGameplayInterruption(t *testing.T) {
	for _, pending := range []string{"info", "destroy", "teleport", "spawn"} {
		t.Run(pending, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, nil)
			commands := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
			admin, actions := &testAdminObjectInfoHandler{}, &testInterruptActionRouter{}
			commands.SetAdminHandler(admin)
			commands.SetActionService(actions)
			switch pending {
			case "info":
				ecs.GetResource[ecs.PendingAdminObjectInfo](w).Set(1)
			case "destroy":
				ecs.GetResource[ecs.PendingAdminDestroy](w).Set(1)
			case "teleport":
				ecs.GetResource[ecs.PendingAdminTeleport](w).Set(1)
			case "spawn":
				ecs.GetResource[ecs.PendingAdminSpawn](w).Set(1, ecs.AdminSpawnEntry{})
			}
			commands.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 20, TargetEntityId: 99}})
			if actions.interrupts != 0 || admin.calls+admin.destroyCalls+admin.coordinateCalls != 1 {
				t.Fatalf("admin input was rerouted into gameplay: interrupts=%d admin=%+v", actions.interrupts, admin)
			}
		})
	}
}
