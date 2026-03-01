package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

func TestNetworkCommandSystem_HandleSetMovementMode_CarryingRunBecomesWalk(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(8101)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Crawl,
			State: constt.StateIdle,
			Speed: 10,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1000,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9901),
		})
	})

	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	system.handleSetMovementMode(world, playerHandle, &network.PlayerCommand{
		CharacterID: playerID,
		Payload: &netproto.C2S_MovementMode{
			Mode: netproto.MovementMode_MOVE_MODE_RUN,
		},
	})

	movement, ok := ecs.GetComponent[components.Movement](world, playerHandle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Walk {
		t.Fatalf("expected carrying run request to resolve to walk, got %v", movement.Mode)
	}
}

func TestNetworkCommandSystem_HandleSetMovementMode_CarryingOverstuffedForcesCrawl(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(8102)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateIdle,
			Speed: 10,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1001,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9902),
		})
	})

	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	system.handleSetMovementMode(world, playerHandle, &network.PlayerCommand{
		CharacterID: playerID,
		Payload: &netproto.C2S_MovementMode{
			Mode: netproto.MovementMode_MOVE_MODE_FAST_RUN,
		},
	})

	movement, ok := ecs.GetComponent[components.Movement](world, playerHandle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Crawl {
		t.Fatalf("expected carrying overstuffed request to resolve to crawl, got %v", movement.Mode)
	}
}
