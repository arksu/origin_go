package systems

import (
	"reflect"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type inventoryOpBridgeSpy struct {
	InventoryOperationExecutor
	InventoryResultSender
	InventorySnapshotSender
	OpenContainerCoordinator
	BuildCommandService

	result         InventoryOpResult
	op             *netproto.InventoryOp
	response       *netproto.S2C_InventoryOpResult
	responseActor  types.EntityID
	broadcast      []*netproto.InventoryState
	broadcastActor types.EntityID
	buildActor     types.EntityID
	buildTarget    types.EntityID
	craftActor     types.EntityID
	craftHandle    types.Handle
	calls          []string
}

func (s *inventoryOpBridgeSpy) ExecuteOperation(_ *ecs.World, _ types.EntityID, _ types.Handle, op *netproto.InventoryOp) InventoryOpResult {
	s.op = op
	return s.result
}

func (s *inventoryOpBridgeSpy) SendInventoryOpResult(actor types.EntityID, result *netproto.S2C_InventoryOpResult) {
	s.responseActor, s.response = actor, result
	s.calls = append(s.calls, "result")
}

func (s *inventoryOpBridgeSpy) BroadcastInventoryUpdates(_ *ecs.World, actor types.EntityID, states []*netproto.InventoryState) {
	s.broadcastActor, s.broadcast = actor, states
	s.calls = append(s.calls, "fanout")
}

func (s *inventoryOpBridgeSpy) SendBuildStateSnapshot(_ *ecs.World, actor, target types.EntityID) {
	s.buildActor, s.buildTarget = actor, target
	s.calls = append(s.calls, "build")
}

func (s *inventoryOpBridgeSpy) SendCraftListSnapshot(_ *ecs.World, actor types.EntityID, handle types.Handle) {
	s.craftActor, s.craftHandle = actor, handle
	s.calls = append(s.calls, "craft")
}

func TestNetworkCommandSystem_InventoryOpBuildRefreshUsesResult(t *testing.T) {
	const (
		playerID = types.EntityID(701)
		buildID  = types.EntityID(702)
	)
	for _, tc := range []struct {
		name        string
		srcKind     netproto.InventoryKind
		dst         *netproto.InventoryRef
		success     bool
		buildTarget types.EntityID
	}{
		{name: "quick deposit without destination", srcKind: netproto.InventoryKind_INVENTORY_KIND_GRID, success: true, buildTarget: buildID},
		{name: "quick takeback without destination", srcKind: netproto.InventoryKind_INVENTORY_KIND_BUILD, success: true, buildTarget: buildID},
		{name: "explicit build deposit", srcKind: netproto.InventoryKind_INVENTORY_KIND_HAND, dst: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(buildID)}, success: true, buildTarget: buildID},
		{name: "non-build transfer", srcKind: netproto.InventoryKind_INVENTORY_KIND_GRID, success: true},
		{name: "failed transfer with build metadata", srcKind: netproto.InventoryKind_INVENTORY_KIND_GRID, buildTarget: buildID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			playerHandle := world.Spawn(playerID, nil)
			spy := &inventoryOpBridgeSpy{result: InventoryOpResult{
				Success:       tc.success,
				BuildTargetID: tc.buildTarget,
				UpdatedContainers: []InventoryContainerState{{
					OwnerID: playerID,
					Kind:    uint8(constt.InventoryGrid),
					Key:     0,
					Version: 5,
					Title:   "Inventory",
					Width:   3,
					Height:  3,
				}},
			}}
			if !tc.success {
				spy.result.ErrorCode = netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL
				spy.result.Message = "Inventory is full"
			}
			move := &netproto.InventoryMoveSpec{Src: &netproto.InventoryRef{Kind: tc.srcKind, OwnerId: uint64(playerID)}, Dst: tc.dst}
			if tc.srcKind == netproto.InventoryKind_INVENTORY_KIND_BUILD {
				move.Src.OwnerId = uint64(buildID)
				move.BuildSlot = new(uint32)
			}
			if tc.dst == nil {
				move.Modifiers = 1
				target := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(buildID)}
				if tc.srcKind == netproto.InventoryKind_INVENTORY_KIND_BUILD {
					target = &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(playerID)}
				} else if tc.success && tc.buildTarget == 0 {
					target = &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(buildID)}
				}
				move.OpenInventories = []*netproto.InventoryRef{move.Src, target}
			}
			op := &netproto.InventoryOp{OpId: 123, Kind: &netproto.InventoryOp_Move{Move: move}}
			system := NewNetworkCommandSystem(nil, nil, nil, nil, spy, spy, nil, 0, zap.NewNop())
			system.SetOpenContainerService(spy)
			system.SetBuildCommandService(spy)
			system.SetInventorySnapshotSender(spy)
			system.handleInventoryOp(world, playerHandle, &network.PlayerCommand{CharacterID: playerID, Payload: op})

			if spy.op != op || spy.response == nil || spy.responseActor != playerID || spy.response.OpId != op.OpId || spy.response.Success != tc.success || spy.response.Error != spy.result.ErrorCode || spy.response.Message != spy.result.Message {
				t.Fatalf("inventory operation/result correlation changed: %+v", spy.response)
			}
			if len(spy.response.Updated) != 1 || spy.response.Updated[0].Ref.OwnerId != uint64(playerID) || spy.response.Updated[0].Revision != 5 {
				t.Fatalf("inventory snapshots not preserved: %+v", spy.response.Updated)
			}
			wantCalls := []string{"result"}
			if tc.success {
				wantCalls = append(wantCalls, "fanout")
				if spy.broadcastActor != playerID || len(spy.broadcast) != 1 || spy.broadcast[0] != spy.response.Updated[0] {
					t.Fatal("successful inventory snapshots were not broadcast")
				}
				if tc.buildTarget > 0 {
					wantCalls = append(wantCalls, "build")
					if spy.buildActor != playerID || spy.buildTarget != tc.buildTarget {
						t.Fatalf("build refresh did not use result metadata: actor=%d target=%d", spy.buildActor, spy.buildTarget)
					}
				}
				wantCalls = append(wantCalls, "craft")
				if spy.craftActor != playerID || spy.craftHandle != playerHandle {
					t.Fatal("successful operation did not refresh actor craft snapshot")
				}
			}
			if !reflect.DeepEqual(spy.calls, wantCalls) {
				t.Fatalf("unexpected refresh order: got %v, want %v", spy.calls, wantCalls)
			}
		})
	}
}
