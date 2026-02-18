package behaviors

import (
	"testing"
	"time"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

func TestWorldCoordToChunkIndex(t *testing.T) {
	chunkWorldSize := float64(constt.ChunkWorldSize)
	testCases := []struct {
		name     string
		coord    float64
		expected int
	}{
		{name: "zero", coord: 0, expected: 0},
		{name: "positive within first chunk", coord: chunkWorldSize - 0.1, expected: 0},
		{name: "positive boundary", coord: chunkWorldSize, expected: 1},
		{name: "small negative", coord: -0.1, expected: -1},
		{name: "negative within first chunk", coord: -(chunkWorldSize - 0.1), expected: -1},
		{name: "negative boundary", coord: -chunkWorldSize, expected: -1},
		{name: "negative next chunk", coord: -(chunkWorldSize + 0.1), expected: -2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := worldCoordToChunkIndex(testCase.coord)
			if got != testCase.expected {
				t.Fatalf("worldCoordToChunkIndex(%f): got %d, want %d", testCase.coord, got, testCase.expected)
			}
		})
	}
}

func TestResolveLogFallAxisDirection(t *testing.T) {
	testCases := []struct {
		name         string
		treeX        float64
		treeY        float64
		playerX      float64
		playerY      float64
		expectedDirX float64
		expectedDirY float64
	}{
		{name: "player east", treeX: 0, treeY: 0, playerX: 10, playerY: 1, expectedDirX: -1, expectedDirY: 0},
		{name: "player west", treeX: 0, treeY: 0, playerX: -10, playerY: 1, expectedDirX: 1, expectedDirY: 0},
		{name: "player north", treeX: 0, treeY: 0, playerX: 1, playerY: 10, expectedDirX: 0, expectedDirY: -1},
		{name: "player south", treeX: 0, treeY: 0, playerX: 1, playerY: -10, expectedDirX: 0, expectedDirY: 1},
		{name: "axis tie prefers x", treeX: 0, treeY: 0, playerX: 10, playerY: 10, expectedDirX: -1, expectedDirY: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gotDirX, gotDirY := resolveLogFallAxisDirection(testCase.treeX, testCase.treeY, testCase.playerX, testCase.playerY)
			if gotDirX != testCase.expectedDirX || gotDirY != testCase.expectedDirY {
				t.Fatalf("resolveLogFallAxisDirection() got (%f,%f), want (%f,%f)", gotDirX, gotDirY, testCase.expectedDirX, testCase.expectedDirY)
			}
		})
	}
}

func TestLogSpawnPosition_UsesInitialAndStepOffsets(t *testing.T) {
	treeX := 100.0
	treeY := 200.0
	dirX := 1.0
	dirY := 0.0
	initialOffset := 12
	stepOffset := 10

	x0, y0 := logSpawnPosition(treeX, treeY, dirX, dirY, initialOffset, stepOffset, 0)
	x1, y1 := logSpawnPosition(treeX, treeY, dirX, dirY, initialOffset, stepOffset, 1)
	x2, y2 := logSpawnPosition(treeX, treeY, dirX, dirY, initialOffset, stepOffset, 2)

	if x0 != 112 || y0 != 200 {
		t.Fatalf("index0: got (%f,%f), want (112,200)", x0, y0)
	}
	if x1 != 122 || y1 != 200 {
		t.Fatalf("index1: got (%f,%f), want (122,200)", x1, y1)
	}
	if x2 != 132 || y2 != 200 {
		t.Fatalf("index2: got (%f,%f), want (132,200)", x2, y2)
	}
}

func TestResolveAxisLogDefKey(t *testing.T) {
	testCases := []struct {
		name     string
		baseKey  string
		dirX     float64
		dirY     float64
		expected string
	}{
		{name: "x axis from y key", baseKey: "log_y", dirX: 1, dirY: 0, expected: "log_x"},
		{name: "y axis keeps y key", baseKey: "log_y", dirX: 0, dirY: 1, expected: "log_y"},
		{name: "x axis from base key", baseKey: "log", dirX: -1, dirY: 0, expected: "log_x"},
		{name: "y axis from base key", baseKey: "log", dirX: 0, dirY: 1, expected: "log_y"},
		{name: "y axis from x key", baseKey: "log_x", dirX: 0, dirY: -1, expected: "log_y"},
		{name: "empty key", baseKey: "", dirX: 1, dirY: 0, expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := resolveAxisLogDefKey(testCase.baseKey, testCase.dirX, testCase.dirY)
			if got != testCase.expected {
				t.Fatalf("resolveAxisLogDefKey(%q, %f, %f): got %q, want %q",
					testCase.baseKey, testCase.dirX, testCase.dirY, got, testCase.expected)
			}
		})
	}
}

type testVisionForcer struct {
	forced []types.Handle
}

func (f *testVisionForcer) ForceUpdateForObserver(_ *ecs.World, observerHandle types.Handle) {
	f.forced = append(f.forced, observerHandle)
}

func TestForceVisionUpdatesForAllAliveCharacters(t *testing.T) {
	world := ecs.NewWorldForTesting()
	forcer := &testVisionForcer{}

	playerA := world.Spawn(types.EntityID(1001), nil)
	playerB := world.Spawn(types.EntityID(1002), nil)
	playerDead := world.Spawn(types.EntityID(1003), nil)
	world.Despawn(playerDead)

	characters := ecs.GetResource[ecs.CharacterEntities](world)
	characters.Add(types.EntityID(1001), playerA, time.Time{})
	characters.Add(types.EntityID(1002), playerB, time.Time{})
	characters.Add(types.EntityID(1003), playerDead, time.Time{})

	forceVisionUpdates(world, forcer)

	if len(forcer.forced) != 2 {
		t.Fatalf("expected 2 forced updates, got %d", len(forcer.forced))
	}
	forced := map[types.Handle]bool{
		forcer.forced[0]: true,
		forcer.forced[1]: true,
	}
	if !forced[playerA] || !forced[playerB] {
		t.Fatalf("expected forced updates for alive players only, got %+v", forcer.forced)
	}
	if forced[playerDead] {
		t.Fatalf("did not expect forced update for dead player")
	}
}

func TestApplyGrowthCatchup_RespectsCatchupLimit(t *testing.T) {
	cfg := &objectdefs.TreeBehaviorConfig{
		Stages: []objectdefs.TreeStageConfig{
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: false},
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: false},
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: true},
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: true},
		},
	}

	stage, nextTick, changed := applyGrowthCatchup(cfg, 1, 100, 500, 150)
	if !changed {
		t.Fatalf("expected catch-up to change stage")
	}
	if stage != 3 {
		t.Fatalf("expected stage 3, got %d", stage)
	}
	if nextTick != 300 {
		t.Fatalf("expected next tick 300, got %d", nextTick)
	}
}

func TestStageTransitionDuration_NilConfig(t *testing.T) {
	if got := stageTransitionDuration(nil, 1); got != 0 {
		t.Fatalf("expected zero duration for nil config, got %d", got)
	}
}

func TestApplyGrowthCatchup_NilConfig(t *testing.T) {
	stage, nextTick, changed := applyGrowthCatchup(nil, 2, 100, 300, 0)
	if changed {
		t.Fatalf("expected no change for nil config")
	}
	if stage != 2 || nextTick != 100 {
		t.Fatalf("unexpected result for nil config: stage=%d nextTick=%d", stage, nextTick)
	}
}

func TestApplyGrowthCatchup_ZeroDurationDoesNotForceMaxStage(t *testing.T) {
	cfg := &objectdefs.TreeBehaviorConfig{
		Stages: []objectdefs.TreeStageConfig{
			{ChopPointsTotal: 1, StageDuration: 10, AllowChop: true},
			{ChopPointsTotal: 1, StageDuration: 0, AllowChop: true},
			{ChopPointsTotal: 1, StageDuration: 10, AllowChop: true},
		},
	}

	stage, nextTick, changed := applyGrowthCatchup(cfg, 1, 10, 100, 0)
	if !changed {
		t.Fatalf("expected stage change")
	}
	if stage != 2 {
		t.Fatalf("expected stage to stop at 2, got %d", stage)
	}
	if nextTick != 0 {
		t.Fatalf("expected nextTick to be 0, got %d", nextTick)
	}
}

func TestIsChopAllowedAtStage(t *testing.T) {
	cfg := &objectdefs.TreeBehaviorConfig{
		Stages: []objectdefs.TreeStageConfig{
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: false},
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: false},
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: true},
			{ChopPointsTotal: 1, StageDuration: 100, AllowChop: true},
		},
	}

	if isChopAllowedAtStage(cfg, 2) {
		t.Fatalf("stage 2 should not allow chop")
	}
	if !isChopAllowedAtStage(cfg, 3) {
		t.Fatalf("stage 3 should allow chop")
	}
	if !isChopAllowedAtStage(cfg, 4) {
		t.Fatalf("stage 4 should allow chop")
	}
}

type testMiniAlertSender struct {
	alerts []*netproto.S2C_MiniAlert
}

func (s *testMiniAlertSender) SendMiniAlert(_ types.EntityID, alert *netproto.S2C_MiniAlert) {
	s.alerts = append(s.alerts, alert)
}

func TestSendWarningMiniAlert(t *testing.T) {
	sender := &testMiniAlertSender{}
	sendWarningMiniAlert(types.EntityID(100), sender, "LOW_STAMINA")

	if len(sender.alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(sender.alerts))
	}
	alert := sender.alerts[0]
	if alert.ReasonCode != "LOW_STAMINA" {
		t.Fatalf("expected LOW_STAMINA reason, got %q", alert.ReasonCode)
	}
	if alert.TtlMs != 2000 {
		t.Fatalf("expected ttl 2000, got %d", alert.TtlMs)
	}
}

func TestOnTakeCycleComplete_DoesNotConsumeStaminaWhenGiveUnavailable(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(9101)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateInteracting,
			Speed: constt.PlayerSpeed,
		})
		ecs.AddComponent(w, h, components.CharacterProfile{
			Attributes: characterattrs.Default(),
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 150,
			Energy:  1000,
		})
	})
	targetID := types.EntityID(9102)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	ecs.WithComponent(world, targetHandle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			Stage: 1,
		})
	})

	cfg := &objectdefs.TreeBehaviorConfig{
		Stages: []objectdefs.TreeStageConfig{
			{
				ChopPointsTotal: 1,
				StageDuration:   60,
				AllowChop:       true,
				Take: []objectdefs.TreeTakeConfig{
					{ID: "take_branch", Name: "Take Branch", ItemDefKey: "branch", Count: 1},
				},
			},
		},
	}

	decision := onTakeCycleComplete(&contracts.BehaviorCycleContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     "take_branch",
	}, contracts.ExecutionDeps{}, cfg)
	if decision != contracts.BehaviorCycleDecisionCanceled {
		t.Fatalf("expected canceled decision, got %v", decision)
	}

	stats, hasStats := ecs.GetComponent[components.EntityStats](world, playerHandle)
	if !hasStats {
		t.Fatalf("missing player stats")
	}
	if stats.Stamina != 150 {
		t.Fatalf("expected stamina unchanged when give is unavailable, got %.3f", stats.Stamina)
	}
}

func TestOnScheduledTick_CancelsWhenCatchupReturnsZeroNextTickNonFinal(t *testing.T) {
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	treeDefID := 7001
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: treeDefID,
			Key:   "tree_test_scheduler_cancel",
			TreeConfig: &objectdefs.TreeBehaviorConfig{
				Stages: []objectdefs.TreeStageConfig{
					{ChopPointsTotal: 1, StageDuration: 10, AllowChop: true},
					{ChopPointsTotal: 1, StageDuration: 0, AllowChop: true},
					{ChopPointsTotal: 1, StageDuration: 10, AllowChop: true},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(70010)
	handle := world.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: uint32(treeDefID)})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	ecs.WithComponent(world, handle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     1,
			Stage:          1,
			NextGrowthTick: 10,
		})
	})
	ecs.ScheduleBehaviorTick(world, entityID, treeBehaviorKey, 10)

	behavior := treeBehavior{}
	result, err := behavior.OnScheduledTick(&contracts.BehaviorTickContext{
		World:       world,
		Handle:      handle,
		EntityID:    entityID,
		EntityType:  uint32(treeDefID),
		BehaviorKey: treeBehaviorKey,
		CurrentTick: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.StateChanged {
		t.Fatalf("expected state change")
	}

	schedule := ecs.GetResource[ecs.BehaviorTickSchedule](world)
	if schedule.PendingCount() != 0 {
		t.Fatalf("expected no pending ticks, got %d", schedule.PendingCount())
	}

	internalState, ok := ecs.GetComponent[components.ObjectInternalState](world, handle)
	if !ok {
		t.Fatalf("missing internal state")
	}
	treeState, ok := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey)
	if !ok || treeState == nil {
		t.Fatalf("missing tree state")
	}
	if treeState.Stage != 2 {
		t.Fatalf("expected stage 2, got %d", treeState.Stage)
	}
	if treeState.NextGrowthTick != 0 {
		t.Fatalf("expected next growth tick 0, got %d", treeState.NextGrowthTick)
	}
}

func TestInitializeRestoredTreeState_CancelsTickWhenCatchupReturnsZeroNextTickNonFinal(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(7301)
	handle := world.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	ecs.WithComponent(world, handle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     1,
			Stage:          1,
			NextGrowthTick: 10,
		})
	})
	ecs.ScheduleBehaviorTick(world, entityID, treeBehaviorKey, 10)

	cfg := &objectdefs.TreeBehaviorConfig{
		Stages: []objectdefs.TreeStageConfig{
			{ChopPointsTotal: 1, StageDuration: 10, AllowChop: true},
			{ChopPointsTotal: 1, StageDuration: 0, AllowChop: true},
			{ChopPointsTotal: 1, StageDuration: 10, AllowChop: true},
		},
	}
	initializeRestoredTreeState(world, handle, entityID, cfg, 100)

	schedule := ecs.GetResource[ecs.BehaviorTickSchedule](world)
	if schedule.PendingCount() != 0 {
		t.Fatalf("expected no pending ticks after restore catch-up, got %d", schedule.PendingCount())
	}

	internalState, ok := ecs.GetComponent[components.ObjectInternalState](world, handle)
	if !ok {
		t.Fatalf("missing internal state")
	}
	treeState, ok := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey)
	if !ok || treeState == nil {
		t.Fatalf("missing tree state")
	}
	if treeState.Stage != 2 {
		t.Fatalf("expected stage 2, got %d", treeState.Stage)
	}
	if treeState.NextGrowthTick != 0 {
		t.Fatalf("expected next growth tick 0, got %d", treeState.NextGrowthTick)
	}
}

func TestExecuteAction_ChopKeepsExistingChopPoints(t *testing.T) {
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	treeDefID := 7002
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: treeDefID,
			Key:   "tree_test_execute",
			TreeConfig: &objectdefs.TreeBehaviorConfig{
				Stages: []objectdefs.TreeStageConfig{
					{ChopPointsTotal: 6, StageDuration: 60, AllowChop: true},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(7201)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateIdle,
			Speed: constt.PlayerSpeed,
		})
	})
	targetID := types.EntityID(7202)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: uint32(treeDefID)})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	ecs.WithComponent(world, targetHandle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     2,
			Taken:          map[string]int{"take_branch": 1},
			Stage:          1,
			NextGrowthTick: 77,
		})
	})

	result := treeBehavior{}.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     actionChop,
	})
	if !result.OK {
		t.Fatalf("expected execute success")
	}

	internalState, ok := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !ok {
		t.Fatalf("missing internal state")
	}
	treeState, ok := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey)
	if !ok || treeState == nil {
		t.Fatalf("missing tree state")
	}
	if treeState.ChopPoints != 2 {
		t.Fatalf("expected chop points to remain 2, got %d", treeState.ChopPoints)
	}
}

func TestExecuteAction_InitializesMissingStateAtFinalStage(t *testing.T) {
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	treeDefID := 7003
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: treeDefID,
			Key:   "tree_test_missing_state",
			TreeConfig: &objectdefs.TreeBehaviorConfig{
				Stages: []objectdefs.TreeStageConfig{
					{ChopPointsTotal: 2, StageDuration: 60, AllowChop: true},
					{ChopPointsTotal: 6, StageDuration: 60, AllowChop: true},
				},
			},
		},
	}))

	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(7401)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateIdle,
			Speed: constt.PlayerSpeed,
		})
	})
	targetID := types.EntityID(7402)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: uint32(treeDefID)})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	result := treeBehavior{}.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World:        world,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     actionChop,
	})
	if !result.OK {
		t.Fatalf("expected execute success")
	}

	internalState, ok := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !ok {
		t.Fatalf("missing internal state")
	}
	treeState, ok := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey)
	if !ok || treeState == nil {
		t.Fatalf("missing tree state")
	}
	if treeState.Stage != 2 {
		t.Fatalf("expected final stage 2 for missing state init, got %d", treeState.Stage)
	}
	if treeState.ChopPoints != 6 {
		t.Fatalf("expected chop points from final stage (6), got %d", treeState.ChopPoints)
	}
}

func TestConsumePlayerStaminaForTreeCycle_FailsBelowFloor(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(3001)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateInteracting,
			Speed: constt.PlayerSpeed,
		})
		ecs.AddComponent(w, h, components.CharacterProfile{
			Attributes: characterattrs.Default(),
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 105,
			Energy:  1000,
		})
	})

	if consumePlayerStaminaForTreeCycle(world, playerHandle, 10) {
		t.Fatalf("expected consume to fail below long-action floor")
	}

	stats, ok := ecs.GetComponent[components.EntityStats](world, playerHandle)
	if !ok {
		t.Fatalf("missing stats component")
	}
	if stats.Stamina != 105 {
		t.Fatalf("expected stamina unchanged after failed consume, got %.3f", stats.Stamina)
	}
	if ecs.GetResource[ecs.EntityStatsUpdateState](world).PendingPlayerPushCount() != 0 {
		t.Fatalf("expected no player stats push schedule on failed consume")
	}
}

func TestConsumePlayerStaminaForTreeCycle_SuccessMarksDirty(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(3002)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Walk,
			State: constt.StateInteracting,
			Speed: constt.PlayerSpeed,
		})
		ecs.AddComponent(w, h, components.CharacterProfile{
			Attributes: characterattrs.Default(),
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 150,
			Energy:  1000,
		})
	})

	if !consumePlayerStaminaForTreeCycle(world, playerHandle, 10) {
		t.Fatalf("expected consume to succeed")
	}

	stats, ok := ecs.GetComponent[components.EntityStats](world, playerHandle)
	if !ok {
		t.Fatalf("missing stats component")
	}
	if stats.Stamina != 140 {
		t.Fatalf("expected stamina 140 after consume, got %.3f", stats.Stamina)
	}
	if ecs.GetResource[ecs.EntityStatsUpdateState](world).PendingPlayerPushCount() != 1 {
		t.Fatalf("expected one player stats push schedule after successful consume")
	}
}

func TestTreeBehavior_DeclaredActionsIncludeOnlyChop(t *testing.T) {
	behavior := treeBehavior{}
	declared := behavior.DeclaredActions()

	if len(declared) != 1 {
		t.Fatalf("expected one declared action, got %d", len(declared))
	}
	if declared[0].ActionID != actionChop || !declared[0].StartsCyclic {
		t.Fatalf("expected chop to be declared as cyclic")
	}
}
