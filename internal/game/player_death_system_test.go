package game

import (
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type testPlayerDeathHandler struct {
	calls []testPlayerDeathCall
}

type testPlayerDeathCall struct {
	playerID types.EntityID
	handle   types.Handle
}

func (h *testPlayerDeathHandler) HandlePlayerPermanentDeath(_ *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	h.calls = append(h.calls, testPlayerDeathCall{
		playerID: playerID,
		handle:   playerHandle,
	})
}

func TestPlayerDeathSystem_TriggersOnceWhenHHPZeroOrLess(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81001)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP:    45,
			HHP:    0,
			SHPMax: 100,
			HHPMax: 100,
		})
	})

	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())

	handler := &testPlayerDeathHandler{}
	system := NewPlayerDeathSystem(handler)

	system.Update(world, 0)
	system.Update(world, 0)

	if len(handler.calls) != 1 {
		t.Fatalf("expected one death callback, got %d", len(handler.calls))
	}
	if handler.calls[0].playerID != playerID {
		t.Fatalf("unexpected player id: got %d, want %d", handler.calls[0].playerID, playerID)
	}
	if handler.calls[0].handle != playerHandle {
		t.Fatalf("unexpected handle: got %d, want %d", handler.calls[0].handle, playerHandle)
	}

	if _, exists := ecs.GetResource[ecs.CharacterEntities](world).Map[playerID]; exists {
		t.Fatalf("expected character removed from CharacterEntities after death")
	}
}

func TestPlayerDeathSystem_IgnoresLivingCharacters(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81002)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP:    45,
			HHP:    10,
			SHPMax: 100,
			HHPMax: 100,
		})
	})

	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())

	handler := &testPlayerDeathHandler{}
	system := NewPlayerDeathSystem(handler)
	system.Update(world, 0)

	if len(handler.calls) != 0 {
		t.Fatalf("expected no death callbacks for living character, got %d", len(handler.calls))
	}
	if _, exists := ecs.GetResource[ecs.CharacterEntities](world).Map[playerID]; !exists {
		t.Fatalf("expected living character to remain tracked")
	}
}
