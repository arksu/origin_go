package behaviors

import (
	"testing"
	"time"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
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
		GrowthStageMax:       4,
		GrowthStageDurations: []int{100, 100, 100},
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

func TestIsChopAllowedAtStage(t *testing.T) {
	cfg := &objectdefs.TreeBehaviorConfig{
		GrowthStageMax:    4,
		AllowedChopStages: []int{3, 4},
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

func TestSendLowStaminaMiniAlert(t *testing.T) {
	sender := &testMiniAlertSender{}
	sendLowStaminaMiniAlert(types.EntityID(100), sender)

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
