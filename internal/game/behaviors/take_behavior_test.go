package behaviors

import (
	"testing"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

func TestTakeBehavior_ProvideActionsOnlyWhenRemaining(t *testing.T) {
	const takeDefID = 8101
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 2},
					{ID: "chip_flint", Name: "Chip Flint", ItemDefKey: "stone", Count: 1},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	targetID := types.EntityID(81010)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	ecs.WithComponent(world, targetHandle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, takeBehaviorKey, &components.TakeBehaviorState{
			Taken: map[string]int{
				"chip_stone": 1,
				"chip_flint": 1,
			},
		})
	})

	actions := takeBehavior{}.ProvideActions(&contracts.BehaviorActionListContext{
		World:        world,
		TargetID:     targetID,
		TargetHandle: targetHandle,
	})
	if len(actions) != 1 {
		t.Fatalf("expected one action, got %d", len(actions))
	}
	if actions[0].ActionID != "chip_stone" {
		t.Fatalf("expected chip_stone action, got %q", actions[0].ActionID)
	}
}

func TestTakeBehavior_ValidateActionRejectsUnknownAndExhausted(t *testing.T) {
	const takeDefID = 8102
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take_validate",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 1},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81020)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, State: constt.StateIdle, Speed: constt.PlayerSpeed})
	})
	targetID := types.EntityID(81021)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	unknown := takeBehavior{}.ValidateAction(&contracts.BehaviorActionValidateContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "unknown",
		Phase:        contracts.BehaviorValidationPhasePreview,
	})
	if unknown.OK {
		t.Fatalf("expected unknown action validation failure")
	}

	ecs.WithComponent(world, targetHandle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, takeBehaviorKey, &components.TakeBehaviorState{
			Taken: map[string]int{"chip_stone": 1},
		})
	})
	exhausted := takeBehavior{}.ValidateAction(&contracts.BehaviorActionValidateContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
		Phase:        contracts.BehaviorValidationPhasePreview,
	})
	if exhausted.OK {
		t.Fatalf("expected exhausted action validation failure")
	}
}

func TestTakeBehavior_ValidateActionRejectsWhenAnotherCyclicActionActive(t *testing.T) {
	const takeDefID = 8103
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take_active",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 2},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81030)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, State: constt.StateIdle, Speed: constt.PlayerSpeed})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			BehaviorKey:        "tree",
			ActionID:           "chop",
			CycleDurationTicks: 10,
		})
	})
	targetID := types.EntityID(81031)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	result := takeBehavior{}.ValidateAction(&contracts.BehaviorActionValidateContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
		Phase:        contracts.BehaviorValidationPhaseExecute,
	})
	if result.OK {
		t.Fatalf("expected validation failure when cyclic action is already active")
	}
	if !result.UserVisible || result.ReasonCode != "action_already_active" {
		t.Fatalf("expected action_already_active warning, got %+v", result)
	}
}

func TestTakeBehavior_ExecuteActionStartsCyclicAction(t *testing.T) {
	const takeDefID = 8104
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take_execute",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 2},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81040)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, State: constt.StateIdle, Speed: constt.PlayerSpeed})
	})
	targetID := types.EntityID(81041)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	result := takeBehavior{}.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
	})
	if !result.OK {
		t.Fatalf("expected execute success")
	}

	action, hasAction := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle)
	if !hasAction {
		t.Fatalf("expected active cyclic action")
	}
	if action.BehaviorKey != takeBehaviorKey || action.ActionID != "chip_stone" {
		t.Fatalf("unexpected action payload: %+v", action)
	}
	if action.CycleDurationTicks != uint32(takeCycleDurationTicks) {
		t.Fatalf("unexpected cycle duration: %d", action.CycleDurationTicks)
	}
}

func TestTakeBehavior_OnCycleCompleteContinueAndExhaust(t *testing.T) {
	const takeDefID = 8105
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take_cycle",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 2},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81050)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, State: constt.StateInteracting, Speed: constt.PlayerSpeed})
		ecs.AddComponent(w, h, components.CharacterProfile{Attributes: characterattrs.Default()})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 150, Energy: 1000})
	})
	targetID := types.EntityID(81051)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID, Quality: 88})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	passedQualities := make([]uint32, 0, 2)
	decision1 := takeBehavior{}.OnCycleComplete(&contracts.BehaviorCycleContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
		Deps: &contracts.ExecutionDeps{
			GiveItem: func(_ *ecs.World, _ types.EntityID, _ types.Handle, _ string, _ uint32, quality uint32) contracts.GiveItemOutcome {
				passedQualities = append(passedQualities, quality)
				return contracts.GiveItemOutcome{Success: true}
			},
		},
	})
	if decision1 != contracts.BehaviorCycleDecisionContinue {
		t.Fatalf("expected continue on first take, got %v", decision1)
	}

	decision2 := takeBehavior{}.OnCycleComplete(&contracts.BehaviorCycleContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
		Deps: &contracts.ExecutionDeps{
			GiveItem: func(_ *ecs.World, _ types.EntityID, _ types.Handle, _ string, _ uint32, quality uint32) contracts.GiveItemOutcome {
				passedQualities = append(passedQualities, quality)
				return contracts.GiveItemOutcome{Success: true}
			},
		},
	})
	if decision2 != contracts.BehaviorCycleDecisionComplete {
		t.Fatalf("expected complete on exhausted take count, got %v", decision2)
	}
	if len(passedQualities) != 2 || passedQualities[0] != 88 || passedQualities[1] != 88 {
		t.Fatalf("expected parent quality 88 for each GiveItem call, got %+v", passedQualities)
	}
}

func TestTakeBehavior_OnCycleCompleteLowStaminaCancels(t *testing.T) {
	const takeDefID = 8106
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take_low_stamina",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 2},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81060)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, State: constt.StateInteracting, Speed: constt.PlayerSpeed})
		ecs.AddComponent(w, h, components.CharacterProfile{Attributes: characterattrs.Default()})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 105, Energy: 1000})
	})
	targetID := types.EntityID(81061)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	decision := takeBehavior{}.OnCycleComplete(&contracts.BehaviorCycleContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
		Deps: &contracts.ExecutionDeps{
			GiveItem: func(_ *ecs.World, _ types.EntityID, _ types.Handle, _ string, _ uint32, _ uint32) contracts.GiveItemOutcome {
				return contracts.GiveItemOutcome{Success: true}
			},
		},
	})
	if decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("expected canceled on low stamina, got %v", decision)
	}
}

func TestTakeBehavior_OnCycleCompleteGiveUnavailableCancels(t *testing.T) {
	const takeDefID = 8107
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: takeDefID,
			Key:   "boulder_take_no_give",
			TakeConfig: &objectdefs.TakeBehaviorConfig{
				Items: []objectdefs.TakeConfig{
					{ID: "chip_stone", Name: "Chip Stone", ItemDefKey: "stone", Count: 2},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81070)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, State: constt.StateInteracting, Speed: constt.PlayerSpeed})
		ecs.AddComponent(w, h, components.CharacterProfile{Attributes: characterattrs.Default()})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 150, Energy: 1000})
	})
	targetID := types.EntityID(81071)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: takeDefID})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	decision := takeBehavior{}.OnCycleComplete(&contracts.BehaviorCycleContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "chip_stone",
	})
	if decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("expected canceled when give dependency is missing, got %v", decision)
	}
}
