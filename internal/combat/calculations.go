// Package combat calculates damage from caller-supplied combat parameters.
// Resolving equipment, effective attributes and action parameters belongs to
// the caller; these functions do not access or mutate game state.
package combat

import (
	"errors"
	"math"
)

const (
	// StrengthReference and QualityReference are shared damage model normalizations.
	StrengthReference = 1.0
	QualityReference  = 10.0
	// HardDamageFraction is the HHP share of ordinary damage absorbed by SHP.
	HardDamageFraction = 0.20
)

var (
	ErrInvalidInput    = errors.New("combat: invalid numeric input")
	ErrNonFiniteResult = errors.New("combat: non-finite result")
)

// MeleeRawDamage calculates B * (STR/S0)^0.25 * (Quality/Q0)^0.25 * multiplier
// for any melee weapon. Strength and quality must be positive; base damage and
// multiplier may be zero. All inputs must be finite and are validated even when
// a zero factor makes the damage zero.
func MeleeRawDamage(baseDamage, strength, quality, multiplier float64) (float64, error) {
	if !nonnegativeFinite(baseDamage) || !positiveFinite(strength) ||
		!positiveFinite(quality) || !nonnegativeFinite(multiplier) {
		return 0, ErrInvalidInput
	}
	if baseDamage == 0 || multiplier == 0 {
		return 0, nil
	}

	// Take roots before normalization so a positive subnormal quality is not
	// lost by division by Q0. Two square roots give the shared quarter power.
	strengthFactor := math.Sqrt(math.Sqrt(strength)) / math.Sqrt(math.Sqrt(StrengthReference))
	qualityFactor := math.Sqrt(math.Sqrt(quality)) / math.Sqrt(math.Sqrt(QualityReference))

	// Multiply scaled mantissas to avoid intermediate overflow or underflow
	// when the complete product is still representable.
	product, exponent := math.Frexp(baseDamage)
	for _, factor := range [...]float64{strengthFactor, qualityFactor, multiplier} {
		mantissa, factorExponent := math.Frexp(factor)
		product *= mantissa
		exponent += factorExponent
	}
	return finiteResult(math.Ldexp(product, exponent))
}

// ArmorContribution calculates BaseArmor * sqrt(Quality/Q0) for one item.
// Base armor may be zero; quality must be positive. Both inputs must be finite.
func ArmorContribution(baseArmor, quality float64) (float64, error) {
	if !nonnegativeFinite(baseArmor) || !positiveFinite(quality) {
		return 0, ErrInvalidInput
	}
	return finiteResult(baseArmor * (math.Sqrt(quality) / math.Sqrt(QualityReference)))
}

// DamageAfterArmor calculates Draw^2 / (Draw + A), including zero damage when
// both inputs are zero. Inputs must be finite and nonnegative.
func DamageAfterArmor(rawDamage, armor float64) (float64, error) {
	if !nonnegativeFinite(rawDamage) || !nonnegativeFinite(armor) {
		return 0, ErrInvalidInput
	}
	if rawDamage == 0 || armor == 0 {
		return rawDamage, nil
	}

	// Normalize by the larger input to avoid overflowing the square or sum.
	if rawDamage >= armor {
		return finiteResult(rawDamage / (1 + armor/rawDamage))
	}
	ratio := rawDamage / armor
	return finiteResult(rawDamage * ratio / (1 + ratio))
}

// SplitCreatureDamage returns the SHP and HHP damage to pass to the health API.
// In active KO all damage goes to HHP, even if SHP has regenerated. The caller
// supplies KO state independently of the creature's lying pose.
func SplitCreatureDamage(damage, shp float64, activeKO bool) (softDamage, hardDamage float64, err error) {
	if !nonnegativeFinite(damage) || !nonnegativeFinite(shp) {
		return 0, 0, ErrInvalidInput
	}
	if activeKO {
		return 0, damage, nil
	}
	softDamage = math.Min(damage, shp)
	hardDamage = HardDamageFraction*softDamage + (damage - softDamage)
	if !finite(softDamage) || !finite(hardDamage) {
		return 0, 0, ErrNonFiniteResult
	}
	return softDamage, hardDamage, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func positiveFinite(value float64) bool {
	return value > 0 && finite(value)
}

func nonnegativeFinite(value float64) bool {
	return value >= 0 && finite(value)
}

func finiteResult(value float64) (float64, error) {
	if !finite(value) {
		return 0, ErrNonFiniteResult
	}
	return value, nil
}
