package stamina

import (
	"math"
	"origin/internal/characterattrs"
	"testing"
)

func TestMaxStaminaFromCon(t *testing.T) {
	got := MaxStaminaFromCon(4)
	want := 2000.0
	if math.Abs(got-want) > 0.00001 {
		t.Fatalf("MaxStaminaFromCon(4) = %v, want %v", got, want)
	}
}

func TestMaxStaminaFromAttributesUsesCon(t *testing.T) {
	values := characterattrs.Default()
	values[characterattrs.CON] = 9

	got := MaxStaminaFromAttributes(values)
	want := 3000.0
	if math.Abs(got-want) > 0.00001 {
		t.Fatalf("MaxStaminaFromAttributes(CON=9) = %v, want %v", got, want)
	}
}

func TestClampStamina(t *testing.T) {
	if got := ClampStamina(-1, 100); got != 0 {
		t.Fatalf("ClampStamina(-1, 100) = %v, want 0", got)
	}
	if got := ClampStamina(150, 100); got != 100 {
		t.Fatalf("ClampStamina(150, 100) = %v, want 100", got)
	}
	if got := ClampStamina(50, 100); got != 50 {
		t.Fatalf("ClampStamina(50, 100) = %v, want 50", got)
	}
}

func TestRoundToUint32(t *testing.T) {
	if got := RoundToUint32(10.4); got != 10 {
		t.Fatalf("RoundToUint32(10.4) = %v, want 10", got)
	}
	if got := RoundToUint32(10.5); got != 11 {
		t.Fatalf("RoundToUint32(10.5) = %v, want 11", got)
	}
}
