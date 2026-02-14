package game

import (
	"testing"
	"time"

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
