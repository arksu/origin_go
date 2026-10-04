package combat_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"origin/internal/combat"
	"origin/internal/entityhealth"
)

func closeTo(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > math.Abs(want)*1e-12 {
		t.Fatalf("got %.17g, want %.17g", got, want)
	}
}

func TestMeleeRawDamage(t *testing.T) {
	for _, tt := range []struct {
		name                                string
		base, strength, quality, multiplier float64
		want                                float64
	}{
		{"baseline", 6, 1, 10, 1, 6},
		{"action multiplier", 6, 1, 10, 1.5, 9},
		{"strength growth", 6, 16, 10, 1, 12},
		{"quality growth", 6, 1, 160, 1, 12},
		{"combined growth", 6, 16, 160, 1, 24},
		{"fractional strength is not clamped", 6, .0625, 10, 1, 3},
		{"fractional quality", 6, 1, .625, 1, 3},
		{"zero base", 0, 1, 10, 1, 0},
		{"zero multiplier", 6, 1, 10, 0, 0},
		{"large representable product", math.MaxFloat64, 16, 10, .25, math.MaxFloat64 / 2},
		{"small representable product", math.Ldexp(1, -1000), math.Ldexp(1, 400), 10, math.Ldexp(1, -100), math.Ldexp(1, -1000)},
		{"subnormal quality", 1e100, 1, math.SmallestNonzeroFloat64, 1, 8.383901441119440821e18},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := combat.MeleeRawDamage(tt.base, tt.strength, tt.quality, tt.multiplier)
			if err != nil {
				t.Fatal(err)
			}
			closeTo(t, got, tt.want)
		})
	}
}

func TestArmorContribution(t *testing.T) {
	for _, tt := range []struct {
		name                string
		base, quality, want float64
	}{
		{"baseline", 4, 10, 4},
		{"quality scaling", 4, 40, 8},
		{"fractional quality", 4, 2.5, 2},
		{"fractional armor", .25, 10, .25},
		{"zero base", 0, 10, 0},
		{"subnormal quality", 1e200, math.SmallestNonzeroFloat64, 7.028980337440463662e37},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := combat.ArmorContribution(tt.base, tt.quality)
			if err != nil {
				t.Fatal(err)
			}
			closeTo(t, got, tt.want)
		})
	}
}

func TestDamageAfterArmor(t *testing.T) {
	for _, tt := range []struct {
		name             string
		raw, armor, want float64
	}{
		{"zero damage and armor", 0, 0, 0},
		{"zero damage", 0, 4, 0},
		{"unarmored", 6, 0, 6},
		{"baseline", 6, 4, 3.6},
		{"equal damage and armor", 4, 4, 2},
		{"weak hit", 25, 50, 25.0 / 3},
		{"strong hit", 100, 50, 200.0 / 3},
		{"fractional unarmored hit", .4, 0, .4},
		{"large equal inputs", math.MaxFloat64, math.MaxFloat64, math.MaxFloat64 / 2},
		{"large hit", math.MaxFloat64, math.MaxFloat64 / 2, math.MaxFloat64 / 1.5},
		{"large armor", math.MaxFloat64 / 2, math.MaxFloat64, math.MaxFloat64 / 6},
		{"tiny unarmored hit", math.SmallestNonzeroFloat64, 0, math.SmallestNonzeroFloat64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := combat.DamageAfterArmor(tt.raw, tt.armor)
			if err != nil {
				t.Fatal(err)
			}
			closeTo(t, got, tt.want)
		})
	}
}

func TestSplitCreatureDamageAndHealth(t *testing.T) {
	for _, tt := range []struct {
		name                                 string
		damage, shp, hhp, wantSoft, wantHard float64
		activeKO                             bool
		wantSHP, wantHHP                     float64
	}{
		{"ordinary hit", 10, 40, 60, 10, 2, false, 30, 58},
		{"overflow enters KO", 30, 10, 50, 10, 22, false, 0, 28},
		{"exact SHP depletion", 10, 10, 50, 10, 2, false, 0, 48},
		{"KO hit", 12, 0, 28, 0, 12, true, 0, 16},
		{"KO with regenerated SHP", 2, 5, 20, 0, 2, true, 5, 18},
		{"completed KO while still lying", 2, 5, 20, 2, .4, false, 3, 19.6},
		{"depletion after completed KO", 2, 1, 20, 1, 1.2, false, 0, 18.8},
		{"lethal overflow", 30, 10, 20, 10, 22, false, 0, 0},
		{"zero damage", 0, 10, 50, 0, 0, false, 10, 50},
		{"zero damage in KO", 0, 5, 20, 0, 0, true, 5, 20},
		{"zero SHP", 2, 0, 20, 0, 2, false, 0, 18},
		{"SHP is bounded by remaining HHP", 10, 10, 10, 0, 10, true, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			soft, hard, err := combat.SplitCreatureDamage(tt.damage, tt.shp, tt.activeKO)
			if err != nil {
				t.Fatal(err)
			}
			closeTo(t, soft, tt.wantSoft)
			closeTo(t, hard, tt.wantHard)
			shp, hhp, _, _ := entityhealth.ApplyDamage(tt.shp, tt.hhp, tt.hhp, soft, hard)
			closeTo(t, shp, tt.wantSHP)
			closeTo(t, hhp, tt.wantHHP)
		})
	}

	soft, hard, err := combat.SplitCreatureDamage(math.MaxFloat64, math.MaxFloat64, false)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, soft, math.MaxFloat64)
	closeTo(t, hard, math.MaxFloat64*.2)
}

func TestInvalidNumericInputs(t *testing.T) {
	for _, fn := range []struct {
		name     string
		args     []float64
		positive []bool
		call     func([]float64) (float64, error)
	}{
		{"melee", []float64{6, 1, 10, 1}, []bool{false, true, true, false}, func(a []float64) (float64, error) {
			return combat.MeleeRawDamage(a[0], a[1], a[2], a[3])
		}},
		{"armor contribution", []float64{4, 10}, []bool{false, true}, func(a []float64) (float64, error) {
			return combat.ArmorContribution(a[0], a[1])
		}},
		{"damage after armor", []float64{6, 4}, []bool{false, false}, func(a []float64) (float64, error) {
			return combat.DamageAfterArmor(a[0], a[1])
		}},
	} {
		for index := range fn.args {
			invalid := []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)}
			if fn.positive[index] {
				invalid = append(invalid, 0)
			}
			for _, value := range invalid {
				t.Run(fmt.Sprintf("%s/argument%d/%g", fn.name, index, value), func(t *testing.T) {
					args := append([]float64(nil), fn.args...)
					args[index] = value
					got, err := fn.call(args)
					if got != 0 || !errors.Is(err, combat.ErrInvalidInput) {
						t.Fatalf("got (%g, %v), want (0, ErrInvalidInput)", got, err)
					}
				})
			}
		}
	}

	for _, activeKO := range []bool{false, true} {
		for index := 0; index < 2; index++ {
			for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
				t.Run(fmt.Sprintf("split/KO%t/argument%d/%g", activeKO, index, value), func(t *testing.T) {
					args := [2]float64{2, 5}
					args[index] = value
					soft, hard, err := combat.SplitCreatureDamage(args[0], args[1], activeKO)
					if soft != 0 || hard != 0 || !errors.Is(err, combat.ErrInvalidInput) {
						t.Fatalf("got (%g, %g, %v), want (0, 0, ErrInvalidInput)", soft, hard, err)
					}
				})
			}
		}
	}
}

func TestZeroFactorsStillValidateOtherInputs(t *testing.T) {
	for _, call := range []func() (float64, error){
		func() (float64, error) { return combat.MeleeRawDamage(0, math.NaN(), 10, 1) },
		func() (float64, error) { return combat.MeleeRawDamage(6, 1, 0, 0) },
		func() (float64, error) { return combat.ArmorContribution(0, math.Inf(1)) },
		func() (float64, error) { return combat.DamageAfterArmor(0, math.NaN()) },
	} {
		got, err := call()
		if got != 0 || !errors.Is(err, combat.ErrInvalidInput) {
			t.Fatalf("got (%g, %v), want (0, ErrInvalidInput)", got, err)
		}
	}
	soft, hard, err := combat.SplitCreatureDamage(0, math.NaN(), true)
	if soft != 0 || hard != 0 || !errors.Is(err, combat.ErrInvalidInput) {
		t.Fatalf("got (%g, %g, %v), want (0, 0, ErrInvalidInput)", soft, hard, err)
	}
}

func TestNonFiniteResults(t *testing.T) {
	for _, call := range []func() (float64, error){
		func() (float64, error) { return combat.MeleeRawDamage(math.MaxFloat64, 16, 10, 1) },
		func() (float64, error) { return combat.ArmorContribution(math.MaxFloat64, 40) },
	} {
		got, err := call()
		if got != 0 || !errors.Is(err, combat.ErrNonFiniteResult) {
			t.Fatalf("got (%g, %v), want (0, ErrNonFiniteResult)", got, err)
		}
	}
	got, err := combat.MeleeRawDamage(math.MaxFloat64, math.MaxFloat64, math.MaxFloat64, 0)
	if got != 0 || err != nil {
		t.Fatalf("zero multiplier got (%g, %v), want (0, nil)", got, err)
	}
}

var (
	allocationResult, allocationSoft, allocationHard float64
	allocationError                                  error
)

func TestCalculationsDoNotAllocate(t *testing.T) {
	for _, tt := range []struct {
		name string
		call func()
	}{
		{"melee success", func() { allocationResult, allocationError = combat.MeleeRawDamage(6, 16, 10, 1.5) }},
		{"melee invalid", func() { allocationResult, allocationError = combat.MeleeRawDamage(6, math.NaN(), 10, 1) }},
		{"melee overflow", func() { allocationResult, allocationError = combat.MeleeRawDamage(math.MaxFloat64, 16, 10, 1) }},
		{"armor success", func() { allocationResult, allocationError = combat.ArmorContribution(4, 40) }},
		{"armor invalid", func() { allocationResult, allocationError = combat.ArmorContribution(4, 0) }},
		{"armor overflow", func() { allocationResult, allocationError = combat.ArmorContribution(math.MaxFloat64, 40) }},
		{"after armor success", func() { allocationResult, allocationError = combat.DamageAfterArmor(6, 4) }},
		{"after armor invalid", func() { allocationResult, allocationError = combat.DamageAfterArmor(6, math.Inf(1)) }},
		{"split success", func() { allocationSoft, allocationHard, allocationError = combat.SplitCreatureDamage(30, 10, false) }},
		{"split KO", func() { allocationSoft, allocationHard, allocationError = combat.SplitCreatureDamage(30, 10, true) }},
		{"split invalid", func() { allocationSoft, allocationHard, allocationError = combat.SplitCreatureDamage(30, -1, false) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(100, tt.call); got != 0 {
				t.Fatalf("got %g allocations per call, want zero", got)
			}
		})
	}
}
