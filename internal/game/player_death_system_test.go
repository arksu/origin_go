package game

import (
	"math"
	"testing"
	"time"

	_const "origin/internal/const"
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

func TestPlayerDeathSystem_KnockoutSetsStunnedState(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81001)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP: 0,
			HHP: 10,
		})
		ecs.AddComponent(w, h, components.Movement{
			State: _const.StateIdle,
		})
	})

	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 100}

	handler := &testPlayerDeathHandler{}
	system := NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           100,
		StarvationDamageIntervalTicks:   1000,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	health, ok := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if !ok {
		t.Fatalf("missing health component")
	}
	if health.KOUntilTick != 100 {
		t.Fatalf("expected knockout marker tick=100, got %d", health.KOUntilTick)
	}
	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("missing movement component")
	}
	if movement.State != _const.StateStunned {
		t.Fatalf("expected stunned movement state, got %v", movement.State)
	}
}

func TestPlayerDeathSystem_DoesNotAutoWakeFromKnockout(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81002)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP:         0,
			HHP:         20,
			KOUntilTick: 15,
		})
		ecs.AddComponent(w, h, components.Movement{
			State: _const.StateStunned,
		})
	})

	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 15}

	handler := &testPlayerDeathHandler{}
	system := NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           100,
		StarvationDamageIntervalTicks:   1000,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	if len(handler.calls) != 0 {
		t.Fatalf("did not expect respawn callback while waking from KO, got %d", len(handler.calls))
	}

	health, ok := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if !ok {
		t.Fatalf("missing health component")
	}
	if health.KOUntilTick != 15 {
		t.Fatalf("expected KO marker to remain set, got %d", health.KOUntilTick)
	}
	if health.SHP != 0 {
		t.Fatalf("expected SHP to stay at 0 while knocked out, got %v", health.SHP)
	}

	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("missing movement component")
	}
	if movement.State != _const.StateStunned {
		t.Fatalf("expected movement to remain stunned while knocked out")
	}
}

func TestPlayerDeathSystem_KnockoutClearsWhenShpRecovers(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81007)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP:         3,
			HHP:         20,
			KOUntilTick: 15,
		})
		ecs.AddComponent(w, h, components.Movement{
			State: _const.StateStunned,
		})
	})

	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 20}

	system := NewPlayerDeathSystem(&testPlayerDeathHandler{}, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           100,
		StarvationDamageIntervalTicks:   1000,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	health, ok := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if !ok {
		t.Fatalf("missing health component")
	}
	if health.KOUntilTick != 0 {
		t.Fatalf("expected KO marker cleared after SHP recovery, got %d", health.KOUntilTick)
	}
	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("missing movement component")
	}
	if movement.State == _const.StateStunned {
		t.Fatalf("expected movement unstunned after SHP recovery")
	}
}

func TestPlayerDeathSystem_DeathTriggersPermanentDeathOnceAndRemovesCharacter(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81003)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP: 0,
			HHP: 0,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 50,
			Energy:  900,
		})
		ecs.AddComponent(w, h, components.Movement{
			State: _const.StateIdle,
		})
	})
	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())

	handler := &testPlayerDeathHandler{}
	system := NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           100,
		StarvationDamageIntervalTicks:   1000,
		StarvationSoftDamagePerInterval: 10,
	})

	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 10}
	system.Update(world, 0)
	system.Update(world, 0)

	if len(handler.calls) != 1 {
		t.Fatalf("expected exactly one permanent death callback, got %d", len(handler.calls))
	}
	if handler.calls[0].playerID != playerID {
		t.Fatalf("unexpected player id: %d", handler.calls[0].playerID)
	}
	if handler.calls[0].handle != playerHandle {
		t.Fatalf("unexpected handle: %d", handler.calls[0].handle)
	}

	characters := ecs.GetResource[ecs.CharacterEntities](world)
	if _, exists := characters.Map[playerID]; exists {
		t.Fatalf("expected character to be removed after permanent death callback")
	}
}

func TestPlayerDeathSystem_RegenUsesEnergyBands(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81004)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP: 10,
			HHP: 25,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Energy: 950,
		})
	})
	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 100}

	system := NewPlayerDeathSystem(&testPlayerDeathHandler{}, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           100,
		StarvationDamageIntervalTicks:   1000,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	health, _ := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if math.Abs(health.SHP-10.05) > 0.0001 {
		t.Fatalf("expected SHP regen to 10.05, got %v", health.SHP)
	}
}

func TestPlayerDeathSystem_StarvationAppliesSoftDamage(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81005)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP: 5,
			HHP: 25,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Energy: 400,
		})
		ecs.AddComponent(w, h, components.Movement{State: _const.StateIdle})
	})
	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 200}

	system := NewPlayerDeathSystem(&testPlayerDeathHandler{}, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           1000,
		StarvationDamageIntervalTicks:   200,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	health, _ := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.SHP != 0 || health.HHP != 25 {
		t.Fatalf("expected starvation damage to bring SHP to 0 only, got SHP=%v HHP=%v", health.SHP, health.HHP)
	}
	if health.KOUntilTick != 200 {
		t.Fatalf("expected KO marker tick after starvation KO, got %d", health.KOUntilTick)
	}
}

func TestPlayerDeathSystem_ClampsInvariantEachTick(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(81006)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{
			SHP: 100,
			HHP: 100,
		})
	})
	ecs.GetResource[ecs.CharacterEntities](world).Add(playerID, playerHandle, time.Now())
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 1}

	system := NewPlayerDeathSystem(&testPlayerDeathHandler{}, PlayerDeathSystemConfig{
		LifeDeathFactor:                 1,
		ShpRegenIntervalTicks:           1000,
		StarvationDamageIntervalTicks:   1000,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	health, _ := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.HHP != 25 || health.SHP != 25 {
		t.Fatalf("expected clamp to MHP=25, got SHP=%v HHP=%v", health.SHP, health.HHP)
	}
}
