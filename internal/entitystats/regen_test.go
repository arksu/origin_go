package entitystats

import "testing"

func TestResolveStaminaGainFromEnergy(t *testing.T) {
	if got := ResolveStaminaGainFromEnergy(1); got != 5 {
		t.Fatalf("expected 5 stamina from 1 energy, got %v", got)
	}
	if got := ResolveStaminaGainFromEnergy(0); got != 0 {
		t.Fatalf("expected 0 stamina from 0 energy, got %v", got)
	}
}

func TestRegenerateStamina(t *testing.T) {
	stamina, energy, changed := RegenerateStamina(100, 10, 1000)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if stamina != 105 {
		t.Fatalf("expected stamina=105, got %v", stamina)
	}
	if energy != 9 {
		t.Fatalf("expected energy=9, got %v", energy)
	}
}

func TestRegenerateStamina_ClampToMax(t *testing.T) {
	stamina, energy, changed := RegenerateStamina(998, 10, 1000)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if stamina != 1000 {
		t.Fatalf("expected stamina clamped to 1000, got %v", stamina)
	}
	if energy != 9 {
		t.Fatalf("expected energy=9, got %v", energy)
	}
}

func TestRegenerateStamina_NoEnergy(t *testing.T) {
	stamina, energy, changed := RegenerateStamina(100, 0, 1000)
	if changed {
		t.Fatalf("expected changed=false when energy is zero")
	}
	if stamina != 100 || energy != 0 {
		t.Fatalf("expected unchanged values, got stamina=%v energy=%v", stamina, energy)
	}
}

func TestRegenerateStamina_NoNeedAtMax(t *testing.T) {
	stamina, energy, changed := RegenerateStamina(1000, 10, 1000)
	if changed {
		t.Fatalf("expected changed=false when stamina already max")
	}
	if stamina != 1000 || energy != 10 {
		t.Fatalf("expected unchanged values, got stamina=%v energy=%v", stamina, energy)
	}
}
