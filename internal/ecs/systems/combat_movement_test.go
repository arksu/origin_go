package systems

import (
	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"testing"
	"time"
)

func TestCombatMovementCapAndRecovery(t *testing.T) {
	for _, directional := range []bool{false, true} {
		fixture := newDirectionalFixture(t)
		w, actor := fixture.world, fixture.player
		ecs.AddComponent(w, actor, components.EntityStats{Stamina: 500, Energy: 1000})
		ecs.AddComponent(w, actor, components.CombatState{Execution: &components.CombatExecution{}})
		ecs.WithComponent(w, actor, func(m *components.Movement) { m.Mode = constt.Run; m.SetTargetPoint(200, 100) })
		if directional {
			fixture.send(1, 0, 1, 0)
		}
		before := fixture.state().Direction
		fixture.movement.Update(w, .1)
		if fixture.state().Mode != constt.Run || fixture.state().VelocityX != 16 {
			t.Fatal("windup must cap speed without changing selection")
		}
		if directional && fixture.state().Direction != before {
			t.Fatal("combat changed hold identity or expiry")
		}
		transform := NewTransformUpdateSystem(w, nil, nil, zap.NewNop())
		transform.applyMovementStaminaTick(w, actor, 100, 100, 101, 100)
		stats, _ := ecs.GetComponent[components.EntityStats](w, actor)
		if stats.Stamina != 500 {
			t.Fatal("windup used run cost")
		}
		ecs.WithComponent(w, actor, func(state *components.CombatState) { state.Execution.StrikeResolved = true })
		fixture.movement.Update(w, .1)
		if fixture.state().Mode != constt.Run || fixture.state().VelocityX != 48 {
			t.Fatal("recovery did not restore selected speed")
		}
		transform.applyMovementStaminaTick(w, actor, 101, 100, 102, 100)
		stats, _ = ecs.GetComponent[components.EntityStats](w, actor)
		if stats.Stamina >= 500 {
			t.Fatal("recovery did not restore movement cost")
		}
		if directional {
			fixture.advance(time.Second)
			fixture.movement.Update(w, .1)
			if fixture.state().TargetType != constt.TargetNone {
				t.Fatal("combat extended expired hold")
			}
		}
	}
}

func TestCombatZeroStaminaAndCollisionCost(t *testing.T) {
	fixture := newDirectionalFixture(t)
	w, actor := fixture.world, fixture.player
	ecs.AddComponent(w, actor, components.CombatState{Execution: &components.CombatExecution{}})
	ecs.AddComponent(w, actor, components.EntityStats{Stamina: 0, Energy: 1000})
	fixture.send(1, 0, 1, 0)
	fixture.movement.Update(w, .1)
	if fixture.state().VelocityX != 0 || !components.CombatCommitted(w, actor) {
		t.Fatal("zero stamina must stop movement, not combat")
	}
	ecs.WithComponent(w, actor, func(stats *components.EntityStats) { stats.Stamina = 100 })
	transform := NewTransformUpdateSystem(w, nil, nil, zap.NewNop())
	transform.applyMovementStaminaTick(w, actor, 100, 100, 100, 100)
	stats, _ := ecs.GetComponent[components.EntityStats](w, actor)
	if stats.Stamina != 100 {
		t.Fatal("collision with no displacement spent stamina")
	}
}
