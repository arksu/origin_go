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

func TestPlayerDeathSystem_KnockoutBoundaryAndIndependentStun(t *testing.T) {
	for _, stunned := range []bool{false, true} {
		world := ecs.NewWorldForTesting()
		state := _const.StateMoving
		if stunned {
			state = _const.StateStunned
		}
		id := types.EntityID(81001)
		handle := world.Spawn(id, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityHealth{HHP: 20})
			ecs.AddComponent(w, h, components.EntityStats{Energy: 950})
			ecs.AddComponent(w, h, components.Movement{State: state, TargetType: _const.TargetPoint, TargetX: 42})
		})
		ecs.GetResource[ecs.CharacterEntities](world).Add(id, handle, time.Now())
		system := NewPlayerDeathSystem(&testPlayerDeathHandler{}, PlayerDeathSystemConfig{})
		clock := ecs.GetResource[ecs.TimeState](world)
		*clock = ecs.TimeState{Tick: 100, UnixMs: 1000}
		system.Update(world, 0)
		health, _ := ecs.GetComponent[components.EntityHealth](world, handle)
		if health.KOUntilUnixMs != 61_000 || !health.IsLying || health.SHP <= 0 {
			t.Fatalf("depletion lost to regen: %+v", health)
		}
		// Regenerated SHP must not shorten the deadline; later damage must not extend it.
		clock.UnixMs = 60_999
		clock.Tick = 101
		system.Update(world, 0)
		health, _ = ecs.GetComponent[components.EntityHealth](world, handle)
		if health.KOUntilUnixMs != 61_000 {
			t.Fatal("KO ended before 60 seconds")
		}
		ecs.WithComponent(world, handle, func(h *components.EntityHealth) { h.SHP = 0 })
		clock.UnixMs = 61_000
		system.Update(world, 0)
		health, _ = ecs.GetComponent[components.EntityHealth](world, handle)
		if health.KOUntilUnixMs != 0 || health.SHP != 1 || !health.IsLying {
			t.Fatalf("bad expiry: %+v", health)
		}
		system.Update(world, 0)
		unchanged, _ := ecs.GetComponent[components.EntityHealth](world, handle)
		if unchanged != health {
			t.Fatal("completion grant repeated")
		}
		movement, _ := ecs.GetComponent[components.Movement](world, handle)
		expectedState := _const.StateIdle
		if stunned {
			expectedState = _const.StateStunned
		}
		if movement.State != expectedState || movement.TargetType != _const.TargetNone {
			t.Fatalf("KO must stop movement and preserve independent stun: %+v", movement)
		}
		ecs.WithComponent(world, handle, func(h *components.EntityHealth) { h.SHP = 0 })
		system.Update(world, 0)
		health, _ = ecs.GetComponent[components.EntityHealth](world, handle)
		if health.KOUntilUnixMs != 121_000 || !health.IsLying {
			t.Fatalf("repeat KO not started: %+v", health)
		}
	}
}

func TestPlayerDeathSystem_BoundaryDamageBeforeCompletion(t *testing.T) {
	for _, hhp := range []float64{0, .4, 20} {
		world := ecs.NewWorldForTesting()
		id := types.EntityID(81002)
		handle := world.Spawn(id, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityHealth{SHP: 5, HHP: hhp, KOUntilUnixMs: 60_000, IsLying: true})
			ecs.AddComponent(w, h, components.EntityStats{Energy: 400})
		})
		ecs.GetResource[ecs.CharacterEntities](world).Add(id, handle, time.Now())
		*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 200, UnixMs: 60_000}
		handler := &testPlayerDeathHandler{}
		system := NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{ShpRegenIntervalTicks: 1000, StarvationDamageIntervalTicks: 200})
		system.Update(world, 0)
		health, _ := ecs.GetComponent[components.EntityHealth](world, handle)
		if health.SHP != math.Min(hhp, 1) || health.KOUntilUnixMs != 0 {
			t.Fatalf("damage did not precede expiry: %+v", health)
		}
		if (len(handler.calls) == 1) != (hhp == 0) {
			t.Fatal("death lost priority")
		}
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
	if health.KOUntilUnixMs != 60_000 {
		t.Fatalf("expected KO marker tick after starvation KO, got %d", health.KOUntilUnixMs)
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
