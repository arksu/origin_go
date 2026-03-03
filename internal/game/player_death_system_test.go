package game

import (
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
	mhp      float64
}

func (h *testPlayerDeathHandler) HandlePlayerDeathRespawn(_ *ecs.World, playerID types.EntityID, playerHandle types.Handle, mhp float64) bool {
	h.calls = append(h.calls, testPlayerDeathCall{
		playerID: playerID,
		handle:   playerHandle,
		mhp:      mhp,
	})
	return true
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
		KnockoutDurationTicks:           30,
		DeathRespawnDelayTicks:          3,
		DeathRespawnHHPPercent:          0.25,
		DeathRespawnEnergy:              1000,
		DeathRespawnStamina:             0,
		StarvationSoftDamagePerInterval: 10,
	})
	system.Update(world, 0)

	health, ok := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if !ok {
		t.Fatalf("missing health component")
	}
	if health.KOUntilTick != 130 {
		t.Fatalf("expected knockout until tick 130, got %d", health.KOUntilTick)
	}
	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("missing movement component")
	}
	if movement.State != _const.StateStunned {
		t.Fatalf("expected stunned movement state, got %v", movement.State)
	}
}

func TestPlayerDeathSystem_WakesUpAfterKnockout(t *testing.T) {
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
		KnockoutDurationTicks:           30,
		DeathRespawnDelayTicks:          3,
		DeathRespawnHHPPercent:          0.25,
		DeathRespawnEnergy:              1000,
		DeathRespawnStamina:             0,
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
	if health.KOUntilTick != 0 {
		t.Fatalf("expected KO timer cleared, got %d", health.KOUntilTick)
	}
	if health.SHP != 2 {
		t.Fatalf("expected wakeup SHP to be 2, got %v", health.SHP)
	}

	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("missing movement component")
	}
	if movement.State == _const.StateStunned {
		t.Fatalf("expected movement unstunned after KO wake")
	}
}

func TestPlayerDeathSystem_DeathSchedulesAndRequestsRespawn(t *testing.T) {
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
		KnockoutDurationTicks:           30,
		DeathRespawnDelayTicks:          3,
		DeathRespawnHHPPercent:          0.25,
		DeathRespawnEnergy:              1000,
		DeathRespawnStamina:             0,
		StarvationSoftDamagePerInterval: 10,
	})

	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 10}
	system.Update(world, 0)
	health, _ := ecs.GetComponent[components.EntityHealth](world, playerHandle)
	if health.RespawnDueTick != 13 {
		t.Fatalf("expected respawn due tick=13, got %d", health.RespawnDueTick)
	}

	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 13}
	system.Update(world, 0)
	system.Update(world, 0)

	if len(handler.calls) != 1 {
		t.Fatalf("expected exactly one respawn callback, got %d", len(handler.calls))
	}
	if handler.calls[0].playerID != playerID {
		t.Fatalf("unexpected player id: %d", handler.calls[0].playerID)
	}
	if handler.calls[0].handle != playerHandle {
		t.Fatalf("unexpected handle: %d", handler.calls[0].handle)
	}
}
