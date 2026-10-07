package entityhealth

import (
	"math"
	"testing"
)

func TestValidatePoolsFiniteNonnegativeAndAllocationFree(t *testing.T) {
	for _, value := range []float64{0, math.Copysign(0, -1), math.SmallestNonzeroFloat64, .49, 24.28, math.MaxFloat64, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		want := error(nil)
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			want = ErrInvalidPools
		}
		for _, shp := range []bool{true, false} {
			a, b := value, 1.0
			if !shp {
				a, b = b, a
			}
			if got := ValidatePools(a, b); got != want {
				t.Fatalf("ValidatePools(%v,%v) = %v, want %v", a, b, got, want)
			}
			if allocations := testing.AllocsPerRun(1000, func() {
				if ValidatePools(a, b) != want {
					panic("unexpected validation")
				}
			}); allocations != 0 {
				t.Fatalf("ValidatePools allocated %v times", allocations)
			}
		}
	}
}
