package game

import (
	"testing"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const (
	testTeachItemDefID = 91001
	testTeachItemKey   = "teach_test_item"
)

type testTeachAlertSender struct {
	byEntity map[types.EntityID][]*netproto.S2C_MiniAlert
}

func (s *testTeachAlertSender) SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert) {
	if s.byEntity == nil {
		s.byEntity = make(map[types.EntityID][]*netproto.S2C_MiniAlert)
	}
	s.byEntity[entityID] = append(s.byEntity[entityID], alert)
}

func setTeachTestItemRegistry(t *testing.T) {
	t.Helper()
	prev := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{
			DefID: testTeachItemDefID,
			Key:   testTeachItemKey,
			Name:  "Teach Test Item",
		},
	}))
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(prev)
	})
}

func newTeachTestService(world *ecs.World, alerts miniAlertSender, cyclicOut cyclicActionFinishSender) *ContextActionService {
	return NewContextActionService(world, nil, nil, nil, alerts, cyclicOut, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)
}

func spawnTeachTestPlayer(
	world *ecs.World,
	playerID types.EntityID,
	discovery []string,
	stamina float64,
	withHand bool,
) (types.Handle, types.Handle) {
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateIdle,
			Speed: 1,
		})
		ecs.AddComponent(w, h, components.CharacterProfile{
			Attributes: characterattrs.Default(),
			Discovery:  append([]string(nil), discovery...),
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: stamina,
			Energy:  1000,
		})
	})
	if !withHand {
		return playerHandle, types.InvalidHandle
	}

	handHandle := world.Spawn(types.EntityID(uint64(playerID)+100000), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{
			OwnerID: playerID,
			Kind:    constt.InventoryHand,
			Key:     0,
			Items: []components.InvItem{
				{TypeID: uint32(testTeachItemDefID)},
			},
		})
	})
	ecs.AddComponent(world, playerHandle, components.InventoryOwner{
		Inventories: []components.InventoryLink{
			{
				Kind:    constt.InventoryHand,
				Key:     0,
				OwnerID: playerID,
				Handle:  handHandle,
			},
		},
	})
	return playerHandle, handHandle
}

func hasContextAction(actions []systems.ContextAction, actionID string) bool {
	for _, action := range actions {
		if action.ActionID == actionID {
			return true
		}
	}
	return false
}

func TestContextActionService_ComputeActions_AppendsTeachForPlayerTargetWithHandItem(t *testing.T) {
	setTeachTestItemRegistry(t)
	world := ecs.NewWorldForTesting()
	service := newTeachTestService(world, nil, nil)

	teacherID := types.EntityID(1001)
	learnerID := types.EntityID(1002)
	teacherHandle, _ := spawnTeachTestPlayer(world, teacherID, nil, 300, true)
	learnerHandle, _ := spawnTeachTestPlayer(world, learnerID, nil, 300, false)

	actions := service.ComputeActions(world, teacherID, teacherHandle, learnerID, learnerHandle)
	if !hasContextAction(actions, teachContextActionID) {
		t.Fatalf("expected synthetic teach action in context menu, got %+v", actions)
	}
}

func TestContextActionService_ExecuteTeach_StartsSyntheticCyclicAction(t *testing.T) {
	setTeachTestItemRegistry(t)
	world := ecs.NewWorldForTesting()
	service := newTeachTestService(world, nil, nil)

	teacherID := types.EntityID(2001)
	learnerID := types.EntityID(2002)
	teacherHandle, _ := spawnTeachTestPlayer(world, teacherID, nil, 300, true)
	learnerHandle, _ := spawnTeachTestPlayer(world, learnerID, nil, 300, false)

	if handled := service.ExecuteAction(world, teacherID, teacherHandle, learnerID, learnerHandle, teachContextActionID); !handled {
		t.Fatalf("expected teach action to be handled")
	}

	action, hasAction := ecs.GetComponent[components.ActiveCyclicAction](world, teacherHandle)
	if !hasAction {
		t.Fatalf("expected teach to start active cyclic action")
	}
	if action.ActionID != teachContextActionID {
		t.Fatalf("unexpected action id: %q", action.ActionID)
	}
	if action.CycleDurationTicks != teachCycleDurationTicks {
		t.Fatalf("unexpected teach cycle duration: got %d want %d", action.CycleDurationTicks, teachCycleDurationTicks)
	}
	if action.TargetID != learnerID {
		t.Fatalf("unexpected target id: got %d want %d", action.TargetID, learnerID)
	}
	if action.BehaviorKey != "" {
		t.Fatalf("expected synthetic teach action to have empty behavior key, got %q", action.BehaviorKey)
	}

	learnerProfile, _ := ecs.GetComponent[components.CharacterProfile](world, learnerHandle)
	if len(learnerProfile.Discovery) != 0 {
		t.Fatalf("expected no immediate discovery mutation on cast start, got %+v", learnerProfile.Discovery)
	}

	movement, _ := ecs.GetComponent[components.Movement](world, teacherHandle)
	if movement.State != constt.StateInteracting {
		t.Fatalf("expected movement state interacting, got %d", movement.State)
	}
}

func TestContextActionService_ExecuteTeach_AlreadyKnown_NoCastStarted(t *testing.T) {
	setTeachTestItemRegistry(t)
	world := ecs.NewWorldForTesting()
	service := newTeachTestService(world, nil, nil)

	teacherID := types.EntityID(3001)
	learnerID := types.EntityID(3002)
	teacherHandle, _ := spawnTeachTestPlayer(world, teacherID, nil, 300, true)
	learnerHandle, _ := spawnTeachTestPlayer(world, learnerID, []string{testTeachItemKey}, 300, false)

	if handled := service.ExecuteAction(world, teacherID, teacherHandle, learnerID, learnerHandle, teachContextActionID); !handled {
		t.Fatalf("expected teach action to be handled")
	}
	if _, hasAction := ecs.GetComponent[components.ActiveCyclicAction](world, teacherHandle); hasAction {
		t.Fatalf("did not expect active cyclic action when learner already knows discovery")
	}
}

func TestContextActionService_SyntheticTeachCycleComplete_SuccessConsumesStaminaAndAlerts(t *testing.T) {
	setTeachTestItemRegistry(t)
	world := ecs.NewWorldForTesting()
	alerts := &testTeachAlertSender{}
	service := newTeachTestService(world, alerts, nil)

	teacherID := types.EntityID(4001)
	learnerID := types.EntityID(4002)
	teacherHandle, handHandle := spawnTeachTestPlayer(world, teacherID, nil, 300, true)
	learnerHandle, _ := spawnTeachTestPlayer(world, learnerID, nil, 300, false)

	decision := service.handleSyntheticTeachCycleComplete(world, teacherID, teacherHandle, components.ActiveCyclicAction{
		ActionID:     teachContextActionID,
		TargetID:     learnerID,
		TargetHandle: learnerHandle,
	})
	if decision != contracts.BehaviorCycleDecisionComplete {
		t.Fatalf("expected teach cycle to complete, got %v", decision)
	}

	learnerProfile, _ := ecs.GetComponent[components.CharacterProfile](world, learnerHandle)
	if !service.learnerHasDiscoveryKey(world, learnerHandle, testTeachItemKey) {
		t.Fatalf("expected learner discovery to contain taught item, got %+v", learnerProfile.Discovery)
	}
	if learnerProfile.Experience.LP != 0 {
		t.Fatalf("expected learner LP unchanged, got %d", learnerProfile.Experience.LP)
	}

	stats, _ := ecs.GetComponent[components.EntityStats](world, teacherHandle)
	if stats.Stamina != 200 {
		t.Fatalf("expected teacher stamina 200 after teach, got %.3f", stats.Stamina)
	}

	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)
	if len(handContainer.Items) != 1 || handContainer.Items[0].TypeID != uint32(testTeachItemDefID) {
		t.Fatalf("expected hand item unchanged after teach, got %+v", handContainer.Items)
	}

	if len(alerts.byEntity[teacherID]) != 1 {
		t.Fatalf("expected one alert for teacher, got %d", len(alerts.byEntity[teacherID]))
	}
	if alerts.byEntity[teacherID][0].ReasonCode != teachSuccessTeacherReasonCode {
		t.Fatalf("unexpected teacher alert reason: %s", alerts.byEntity[teacherID][0].ReasonCode)
	}
	if len(alerts.byEntity[learnerID]) != 1 {
		t.Fatalf("expected one alert for learner, got %d", len(alerts.byEntity[learnerID]))
	}
	if alerts.byEntity[learnerID][0].ReasonCode != teachSuccessLearnerReasonCode {
		t.Fatalf("unexpected learner alert reason: %s", alerts.byEntity[learnerID][0].ReasonCode)
	}
}

func TestContextActionService_SyntheticTeachCycleComplete_AlreadyKnown_NoOpNoStamina(t *testing.T) {
	setTeachTestItemRegistry(t)
	world := ecs.NewWorldForTesting()
	alerts := &testTeachAlertSender{}
	service := newTeachTestService(world, alerts, nil)

	teacherID := types.EntityID(5001)
	learnerID := types.EntityID(5002)
	teacherHandle, _ := spawnTeachTestPlayer(world, teacherID, nil, 300, true)
	learnerHandle, _ := spawnTeachTestPlayer(world, learnerID, []string{testTeachItemKey}, 300, false)

	decision := service.handleSyntheticTeachCycleComplete(world, teacherID, teacherHandle, components.ActiveCyclicAction{
		ActionID:     teachContextActionID,
		TargetID:     learnerID,
		TargetHandle: learnerHandle,
	})
	if decision != contracts.BehaviorCycleDecisionComplete {
		t.Fatalf("expected no-op teach cycle to complete, got %v", decision)
	}

	stats, _ := ecs.GetComponent[components.EntityStats](world, teacherHandle)
	if stats.Stamina != 300 {
		t.Fatalf("expected stamina unchanged on already-known no-op, got %.3f", stats.Stamina)
	}
	if len(alerts.byEntity) != 0 {
		t.Fatalf("expected no alerts on already-known no-op, got %+v", alerts.byEntity)
	}
}

func TestContextActionService_SyntheticTeachCycleComplete_LowStaminaCancelsAndWarns(t *testing.T) {
	setTeachTestItemRegistry(t)
	world := ecs.NewWorldForTesting()
	alerts := &testTeachAlertSender{}
	service := newTeachTestService(world, alerts, nil)

	teacherID := types.EntityID(6001)
	learnerID := types.EntityID(6002)
	teacherHandle, _ := spawnTeachTestPlayer(world, teacherID, nil, 105, true)
	learnerHandle, _ := spawnTeachTestPlayer(world, learnerID, nil, 300, false)

	decision := service.handleSyntheticTeachCycleComplete(world, teacherID, teacherHandle, components.ActiveCyclicAction{
		ActionID:     teachContextActionID,
		TargetID:     learnerID,
		TargetHandle: learnerHandle,
	})
	if decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("expected low-stamina teach cycle to cancel, got %v", decision)
	}

	stats, _ := ecs.GetComponent[components.EntityStats](world, teacherHandle)
	if stats.Stamina != 105 {
		t.Fatalf("expected stamina unchanged on low-stamina cancel, got %.3f", stats.Stamina)
	}

	learnerProfile, _ := ecs.GetComponent[components.CharacterProfile](world, learnerHandle)
	if len(learnerProfile.Discovery) != 0 {
		t.Fatalf("expected learner discovery unchanged on low-stamina cancel, got %+v", learnerProfile.Discovery)
	}

	if len(alerts.byEntity[teacherID]) != 1 {
		t.Fatalf("expected one warning alert for teacher, got %d", len(alerts.byEntity[teacherID]))
	}
	alert := alerts.byEntity[teacherID][0]
	if alert.ReasonCode != "LOW_STAMINA" {
		t.Fatalf("unexpected low-stamina alert reason: %s", alert.ReasonCode)
	}
	if alert.Severity != netproto.AlertSeverity_ALERT_SEVERITY_WARNING {
		t.Fatalf("unexpected low-stamina alert severity: %v", alert.Severity)
	}
}
