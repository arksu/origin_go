package game

import (
	"testing"
	"time"

	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap/zaptest"
)

func TestHandleOnline(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)

	// Initialize resources
	ecs.InitResource(world, ecs.CharacterEntities{Map: make(map[types.EntityID]ecs.CharacterEntity)})

	// Create mock chat delivery service
	mockChat := &mockChatDeliveryService{
		messages: make(map[types.EntityID]string),
	}

	// Create handler
	handler := NewChatAdminCommandHandler(
		nil, // inventoryExecutor
		nil, // inventoryResultSender
		mockChat,
		nil, // alertSender
		nil, // entityIDAllocator
		nil, // chunkProvider
		nil, // visionForcer
		nil, // behaviorRegistry
		eventBus,
		logger,
	)

	// Test 1: No players online
	handler.HandleCommand(world, 1, types.InvalidHandle, "/online")

	if len(mockChat.messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(mockChat.messages))
	}

	expected := "Online players: 0"
	if mockChat.messages[1] != expected {
		t.Errorf("Expected message %q, got %q", expected, mockChat.messages[1])
	}

	// Test 2: Add some players
	characterEntities := ecs.GetResource[ecs.CharacterEntities](world)

	// Create test players
	player1Name := "TestPlayer1"
	player2Name := "TestPlayer2"

	player1Handle := world.Spawn(100, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Appearance{Name: &player1Name})
	})
	characterEntities.Add(100, player1Handle, testTime())

	player2Handle := world.Spawn(200, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Appearance{Name: &player2Name})
	})
	characterEntities.Add(200, player2Handle, testTime())

	// Test online command with players
	mockChat.messages = make(map[types.EntityID]string)
	handler.HandleCommand(world, 1, types.InvalidHandle, "/online")

	if len(mockChat.messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(mockChat.messages))
	}

	message := mockChat.messages[1]
	if message == "" {
		t.Error("Expected non-empty message")
	}

	// Check that message contains expected content
	if message != "Online players: 2" {
		t.Errorf("Expected message 'Online players: 2', got %q", message)
	}
}

func testTime() time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestHandleErrorWarn(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)

	mockAlert := &mockAdminAlertSender{}
	handler := NewChatAdminCommandHandler(
		nil, // inventoryExecutor
		nil, // inventoryResultSender
		nil, // chatDelivery
		mockAlert,
		nil, // entityIDAllocator
		nil, // chunkProvider
		nil, // visionForcer
		nil, // behaviorRegistry
		eventBus,
		logger,
	)

	playerID := types.EntityID(42)
	handled := handler.HandleCommand(world, playerID, types.InvalidHandle, "/error test error message")
	if !handled {
		t.Fatal("expected /error to be recognized")
	}
	if mockAlert.lastErrorEntityID != playerID {
		t.Fatalf("expected error for player %d, got %d", playerID, mockAlert.lastErrorEntityID)
	}
	if mockAlert.lastErrorCode != netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR {
		t.Fatalf("unexpected error code: %v", mockAlert.lastErrorCode)
	}
	if mockAlert.lastErrorMessage != "test error message" {
		t.Fatalf("unexpected error message: %q", mockAlert.lastErrorMessage)
	}

	handled = handler.HandleCommand(world, playerID, types.InvalidHandle, "/warn test warning message")
	if !handled {
		t.Fatal("expected /warn to be recognized")
	}
	if mockAlert.lastWarningEntityID != playerID {
		t.Fatalf("expected warning for player %d, got %d", playerID, mockAlert.lastWarningEntityID)
	}
	if mockAlert.lastWarningCode != netproto.WarningCode_WARN_INPUT_QUEUE_OVERFLOW {
		t.Fatalf("unexpected warning code: %v", mockAlert.lastWarningCode)
	}
	if mockAlert.lastWarningMessage != "test warning message" {
		t.Fatalf("unexpected warning message: %q", mockAlert.lastWarningMessage)
	}
}

func TestHandleTeleportPendingClick(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	mockTeleport := &mockAdminTeleportExecutor{}

	handler := NewChatAdminCommandHandler(
		nil,
		nil,
		mockChat,
		nil,
		nil,
		nil,
		nil,
		nil,
		eventBus,
		logger,
	)
	handler.SetAllowReviveCommand(true)
	handler.SetTeleportExecutor(mockTeleport)

	playerID := types.EntityID(77)
	if handled := handler.HandleCommand(world, playerID, types.InvalidHandle, "/tp"); !handled {
		t.Fatal("expected /tp to be recognized")
	}

	pending := ecs.GetResource[ecs.PendingAdminTeleport](world)
	if !pending.Get(playerID) {
		t.Fatal("expected pending teleport after /tp")
	}
}

func TestHandleTeleportImmediate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	mockTeleport := &mockAdminTeleportExecutor{}

	handler := NewChatAdminCommandHandler(
		nil,
		nil,
		mockChat,
		nil,
		nil,
		nil,
		nil,
		nil,
		eventBus,
		logger,
	)
	handler.SetTeleportExecutor(mockTeleport)

	playerID := types.EntityID(78)
	if handled := handler.HandleCommand(world, playerID, types.InvalidHandle, "/tp 100 200 2"); !handled {
		t.Fatal("expected /tp with coords to be recognized")
	}
	if mockTeleport.calls != 1 {
		t.Fatalf("expected 1 teleport call, got %d", mockTeleport.calls)
	}
	if mockTeleport.lastPlayerID != playerID {
		t.Fatalf("unexpected player id: %d", mockTeleport.lastPlayerID)
	}
	if mockTeleport.lastX != 100 || mockTeleport.lastY != 200 {
		t.Fatalf("unexpected coords: (%d,%d)", mockTeleport.lastX, mockTeleport.lastY)
	}
	if mockTeleport.lastTargetLayer == nil || *mockTeleport.lastTargetLayer != 2 {
		t.Fatalf("expected target layer 2, got %+v", mockTeleport.lastTargetLayer)
	}
}

func TestHandleHealthCommands(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}

	handler := NewChatAdminCommandHandler(
		nil,
		nil,
		mockChat,
		nil,
		nil,
		nil,
		nil,
		nil,
		eventBus,
		logger,
	)
	handler.SetAllowReviveCommand(true)

	playerID := types.EntityID(9001)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP: 20,
			HHP: 30,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 50,
			Energy:  900,
		})
		ecs.AddComponent(w, h, components.Movement{
			State: _const.StateStunned,
		})
	})

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/shp 12.5"); !handled {
		t.Fatal("expected /shp to be recognized")
	}
	health, _ := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.SHP != 12.5 {
		t.Fatalf("expected SHP=12.5 after /shp, got %v", health.SHP)
	}

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/hhp 10"); !handled {
		t.Fatal("expected /hhp to be recognized")
	}
	health, _ = ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.HHP != 10 {
		t.Fatalf("expected HHP=10 after /hhp, got %v", health.HHP)
	}
	if health.SHP > health.HHP {
		t.Fatalf("expected SHP<=HHP after /hhp, got SHP=%v HHP=%v", health.SHP, health.HHP)
	}

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/damage 3 4"); !handled {
		t.Fatal("expected /damage to be recognized")
	}
	health, _ = ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.SHP != 6 || health.HHP != 6 {
		t.Fatalf("expected (/damage 3 4) => SHP=6 HHP=6, got SHP=%v HHP=%v", health.SHP, health.HHP)
	}

	ecs.WithComponent(world, playerHandle, func(h *components.EntityHealth) {
		h.SHP = 0
		h.HHP = 0
		h.KOUntilTick = 100
	})

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/revive"); !handled {
		t.Fatal("expected /revive to be recognized")
	}
	health, _ = ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.HHP <= 0 || health.SHP <= 0 {
		t.Fatalf("expected positive HP after /revive, got SHP=%v HHP=%v", health.SHP, health.HHP)
	}
	if health.KOUntilTick != 0 {
		t.Fatalf("expected KO marker cleared after /revive, got KO=%d", health.KOUntilTick)
	}
	movement, _ := ecs.GetComponent[components.Movement](world, playerHandle)
	if movement.State == _const.StateStunned {
		t.Fatalf("expected movement unstunned after /revive")
	}

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/health"); !handled {
		t.Fatal("expected /health to be recognized")
	}
	if mockChat.messages[playerID] == "" {
		t.Fatalf("expected /health snapshot message")
	}
}

func TestHandleReviveDisabledOutsideDev(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}

	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)
	handler.SetAllowReviveCommand(false)

	playerID := types.EntityID(9002)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{SHP: 1, HHP: 1})
	})

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/revive"); !handled {
		t.Fatal("expected /revive to be recognized even when disabled")
	}
	if got := mockChat.messages[playerID]; got != "revive command is disabled" {
		t.Fatalf("unexpected message: %q", got)
	}
}

// mockChatDeliveryService implements ChatDeliveryService for testing
type mockChatDeliveryService struct {
	messages map[types.EntityID]string
}

func (m *mockChatDeliveryService) SendChatMessage(entityID types.EntityID, channel netproto.ChatChannel, fromEntityID types.EntityID, fromName, text string) {
	m.messages[entityID] = text
}

func (m *mockChatDeliveryService) BroadcastChatMessage(entityIDs []types.EntityID, channel netproto.ChatChannel, fromEntityID types.EntityID, fromName, text string) {
	// Not used in this test
}

type mockAdminAlertSender struct {
	lastErrorEntityID   types.EntityID
	lastErrorCode       netproto.ErrorCode
	lastErrorMessage    string
	lastWarningEntityID types.EntityID
	lastWarningCode     netproto.WarningCode
	lastWarningMessage  string
}

func (m *mockAdminAlertSender) SendError(entityID types.EntityID, errorCode netproto.ErrorCode, message string) {
	m.lastErrorEntityID = entityID
	m.lastErrorCode = errorCode
	m.lastErrorMessage = message
}

func (m *mockAdminAlertSender) SendWarning(entityID types.EntityID, warningCode netproto.WarningCode, message string) {
	m.lastWarningEntityID = entityID
	m.lastWarningCode = warningCode
	m.lastWarningMessage = message
}

type mockAdminTeleportExecutor struct {
	calls           int
	lastPlayerID    types.EntityID
	lastSourceLayer int
	lastX           int
	lastY           int
	lastTargetLayer *int
}

func (m *mockAdminTeleportExecutor) RequestAdminTeleport(
	playerID types.EntityID,
	sourceLayer int,
	targetX, targetY int,
	targetLayer *int,
) error {
	m.calls++
	m.lastPlayerID = playerID
	m.lastSourceLayer = sourceLayer
	m.lastX = targetX
	m.lastY = targetY
	m.lastTargetLayer = targetLayer
	return nil
}
