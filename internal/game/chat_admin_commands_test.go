package game

import (
	"strconv"
	"strings"
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

func TestHandleTimeReportsRuntimeSeconds(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	ecs.SetResource(world, ecs.TimeState{RuntimeSecondsTotal: 364686})
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)

	playerID := types.EntityID(42)
	if handled := handler.HandleCommand(world, playerID, types.InvalidHandle, "/time"); !handled {
		t.Fatal("expected /time to be recognized")
	}
	if got := mockChat.messages[playerID]; got != "runtime_seconds: 364686" {
		t.Fatalf("unexpected runtime time message: %q", got)
	}
}

func TestDestroyCommandArmsObjectSelection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)

	playerID := types.EntityID(42)
	if handled := handler.HandleCommand(world, playerID, types.InvalidHandle, "/destroy"); !handled {
		t.Fatal("expected /destroy to be recognized")
	}
	if got := mockChat.messages[playerID]; got != "Click an object to permanently destroy it and all of its contents." {
		t.Fatalf("unexpected destroy prompt: %q", got)
	}
	if !ecs.GetResource[ecs.PendingAdminDestroy](world).Get(playerID) {
		t.Fatal("expected /destroy to arm object deletion")
	}
}

func TestDestroyCommandDeletesSelectedObjectAndOwnedInventories(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)
	handler.SetObjectDeleter(destroyTestObjectDeleter{})

	playerID := types.EntityID(42)
	targetID := types.EntityID(777)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{Region: 1})
	})
	rootInventoryHandle := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, rootInventoryHandle, components.InventoryContainer{OwnerID: targetID})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(_const.InventoryGrid, targetID, 0, rootInventoryHandle)
	openState := ecs.GetResource[ecs.OpenContainerState](world)
	openState.SetRootOpened(playerID, targetID)
	openState.OpenRef(playerID, ecs.InventoryRefKey{Kind: _const.InventoryGrid, OwnerID: targetID})

	handler.HandleCommand(world, playerID, types.InvalidHandle, "/destroy")
	handler.ExecutePendingDestroy(world, playerID, targetID)

	if world.Alive(targetHandle) {
		t.Fatal("destroyed object must be removed from ECS")
	}
	if world.Alive(rootInventoryHandle) {
		t.Fatal("destroyed object inventory must be removed from ECS")
	}
	if _, found := ecs.GetResource[ecs.InventoryRefIndex](world).Lookup(_const.InventoryGrid, targetID, 0); found {
		t.Fatal("destroyed object inventory ref must be removed")
	}
	if _, opened := openState.GetOpenedRoot(playerID); opened {
		t.Fatal("destroyed object must be removed from opened-container state")
	}
}

type destroyTestObjectDeleter struct{}

func (destroyTestObjectDeleter) DeleteObject(int, types.EntityID) error { return nil }

func TestDestroyCommandKeepsObjectWhenPermanentDeletionIsUnavailable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)

	playerID := types.EntityID(42)
	targetID := types.EntityID(778)
	targetHandle := world.Spawn(targetID, nil)
	handler.HandleCommand(world, playerID, types.InvalidHandle, "/destroy")
	handler.ExecutePendingDestroy(world, playerID, targetID)

	if !world.Alive(targetHandle) {
		t.Fatal("object must remain when its persistent deletion is unavailable")
	}
	if got := mockChat.messages[playerID]; got != "Object destruction is unavailable." {
		t.Fatalf("unexpected persistence error: %q", got)
	}
}

func TestHandlePosition(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)

	playerID := types.EntityID(42)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 345, Y: 679})
	})

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/pos"); !handled {
		t.Fatal("expected /pos to be recognized")
	}
	if got := mockChat.messages[playerID]; got != "pos: 345, 679" {
		t.Fatalf("unexpected position message: %q", got)
	}
}

func TestHandlePositionWithoutTransform(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)

	playerID := types.EntityID(42)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {})

	if handled := handler.HandleCommand(world, playerID, playerHandle, "/pos"); !handled {
		t.Fatal("expected /pos to be recognized")
	}
	if got := mockChat.messages[playerID]; got != "position unavailable" {
		t.Fatalf("unexpected unavailable-position message: %q", got)
	}
}

func TestInfoCommandReportsOnlyMutableRuntimeState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)

	playerID := types.EntityID(42)
	targetID := types.EntityID(777)
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{
			Flags: []string{"lit"},
			State: &components.RuntimeObjectState{Behaviors: map[string]any{
				"burner": &components.BurnerBehaviorState{Fuel: 3},
			}},
		})
	})
	if targetHandle == types.InvalidHandle {
		t.Fatal("expected test target to spawn")
	}

	if handled := handler.HandleCommand(world, playerID, types.InvalidHandle, "/info"); !handled {
		t.Fatal("expected /info to be recognized")
	}
	if !ecs.GetResource[ecs.PendingAdminObjectInfo](world).Get(playerID) {
		t.Fatal("expected /info to arm object inspection")
	}

	handler.ExecutePendingObjectInfo(world, playerID, targetID)

	report := mockChat.messages[playerID]
	for _, expected := range []string{`"entity_id":777`, `"burner"`, `"fuel":3`, `"flags":["lit"]`} {
		if !strings.Contains(report, expected) {
			t.Fatalf("expected inspection report to contain %q, got %q", expected, report)
		}
	}
	if strings.Contains(report, "fuel_capacity") {
		t.Fatalf("inspection report must not include template configuration: %q", report)
	}
	if ecs.GetResource[ecs.PendingAdminObjectInfo](world).Get(playerID) {
		t.Fatal("expected inspection to be one-shot")
	}
}

func TestInfoCommandReportsLiveStationResourcesWhenBehaviorStateIsAbsent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)
	playerID := types.EntityID(45)
	targetID := types.EntityID(779)

	world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{State: &components.RuntimeObjectState{}})
		ecs.AddComponent(w, h, components.StationState{
			CurrentState: "burning",
			Values:       map[string]float64{"temperature": 650},
			Resources:    map[string]uint32{"fuel": 4},
		})
	})
	handler.HandleCommand(world, playerID, types.InvalidHandle, "/info")
	handler.ExecutePendingObjectInfo(world, playerID, targetID)

	report := mockChat.messages[playerID]
	for _, expected := range []string{`"station"`, `"current_state":"burning"`, `"fuel":4`, `"temperature":650`} {
		if !strings.Contains(report, expected) {
			t.Fatalf("expected live station report to contain %q, got %q", expected, report)
		}
	}
}

func TestInfoCommandRejectsArgumentsAndUnavailableTargets(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)
	playerID := types.EntityID(43)

	handler.HandleCommand(world, playerID, types.InvalidHandle, "/info extra")
	if got := mockChat.messages[playerID]; got != "usage: /info" {
		t.Fatalf("expected /info usage error, got %q", got)
	}
	if ecs.GetResource[ecs.PendingAdminObjectInfo](world).Get(playerID) {
		t.Fatal("arguments must not arm inspection")
	}

	handler.HandleCommand(world, playerID, types.InvalidHandle, "/info")
	handler.ExecutePendingObjectInfo(world, playerID, types.EntityID(999))
	if got := mockChat.messages[playerID]; got != "Object inspection target is unavailable." {
		t.Fatalf("expected unavailable target error, got %q", got)
	}
	if ecs.GetResource[ecs.PendingAdminObjectInfo](world).Get(playerID) {
		t.Fatal("unavailable target must consume inspection")
	}
}

func TestInfoCommandReportsRuntimeSerializationFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)
	mockChat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, mockChat, nil, nil, nil, nil, nil, eventBus, logger)
	playerID := types.EntityID(44)
	targetID := types.EntityID(778)

	world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{
			State: &components.RuntimeObjectState{Behaviors: map[string]any{
				"invalid": make(chan struct{}),
			}},
		})
	})
	handler.HandleCommand(world, playerID, types.InvalidHandle, "/info")
	handler.ExecutePendingObjectInfo(world, playerID, targetID)

	if got := mockChat.messages[playerID]; !strings.Contains(got, "runtime state cannot be serialized") {
		t.Fatalf("expected serialization error, got %q", got)
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

func TestDestroySelectionLifecycle(t *testing.T) {
	for _, target := range []types.EntityID{0, 999, 42} {
		t.Run(strconv.FormatUint(uint64(target), 10), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
			h := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t))
			player := w.Spawn(42, nil)
			ecs.GetResource[ecs.CharacterEntities](w).Add(42, player, time.Now())
			h.HandleCommand(w, 42, player, "/destroy")
			h.HandleCommand(w, 43, types.InvalidHandle, "/destroy")
			h.ExecutePendingDestroy(w, 42, target)
			if ecs.GetResource[ecs.PendingAdminDestroy](w).Get(42) {
				t.Fatal("failed selection was not consumed")
			}
			if !ecs.GetResource[ecs.PendingAdminDestroy](w).Get(43) {
				t.Fatal("other selection consumed")
			}
			if !w.Alive(player) {
				t.Fatal("player destroyed")
			}
			if chat.messages[42] == "" || strings.Contains(chat.messages[42], "Destroyed object") {
				t.Fatal("missing error")
			}
			h.HandleCommand(w, 43, types.InvalidHandle, "/info")
			if ecs.GetResource[ecs.PendingAdminDestroy](w).Get(43) || !ecs.GetResource[ecs.PendingAdminObjectInfo](w).Get(43) {
				t.Fatal("new pending command did not replace destroy")
			}
		})
	}
}
