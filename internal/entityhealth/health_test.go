package entityhealth

import "testing"

func TestMaxHHPFromCon(t *testing.T) {
	if got := MaxHHPFromCon(4, 1.0); got != 50 {
		t.Fatalf("MaxHHPFromCon(4, 1.0) = %v, want 50", got)
	}
	if got := MaxHHPFromCon(4, 1.2); got != 60 {
		t.Fatalf("MaxHHPFromCon(4, 1.2) = %v, want 60", got)
	}
	if got := MaxHHPFromCon(0, 1.0); got != 25 {
		t.Fatalf("MaxHHPFromCon(0, 1.0) = %v, want 25 (clamped CON default)", got)
	}
}

func TestClampHealth(t *testing.T) {
	shp, hhp := ClampHealth(120, 110, 100)
	if shp != 100 || hhp != 100 {
		t.Fatalf("ClampHealth(120,110,100) = (%v,%v), want (100,100)", shp, hhp)
	}

	shp, hhp = ClampHealth(-5, -3, 100)
	if shp != 0 || hhp != 0 {
		t.Fatalf("ClampHealth(-5,-3,100) = (%v,%v), want (0,0)", shp, hhp)
	}
}

func TestApplyDamageTransitions(t *testing.T) {
	shp, hhp, knockedOut, dead := ApplyDamage(5, 10, 10, 6, 0)
	if shp != 0 || hhp != 10 {
		t.Fatalf("KO damage expected (0,10), got (%v,%v)", shp, hhp)
	}
	if !knockedOut || dead {
		t.Fatalf("expected knockedOut=true dead=false, got knockedOut=%v dead=%v", knockedOut, dead)
	}

	shp, hhp, knockedOut, dead = ApplyDamage(3, 2, 10, 1, 3)
	if shp != 0 || hhp != 0 {
		t.Fatalf("Death damage expected (0,0), got (%v,%v)", shp, hhp)
	}
	if knockedOut || !dead {
		t.Fatalf("expected knockedOut=false dead=true, got knockedOut=%v dead=%v", knockedOut, dead)
	}
}

func TestResolveSHPRegenPerInterval(t *testing.T) {
	if got := ResolveSHPRegenPerInterval(1000, 950); got != 2 {
		t.Fatalf("ResolveSHPRegenPerInterval(1000,950) = %v, want 2", got)
	}
	if got := ResolveSHPRegenPerInterval(1000, 850); got != 1 {
		t.Fatalf("ResolveSHPRegenPerInterval(1000,850) = %v, want 1", got)
	}
	if got := ResolveSHPRegenPerInterval(1000, 799); got != 0 {
		t.Fatalf("ResolveSHPRegenPerInterval(1000,799) = %v, want 0", got)
	}
}
