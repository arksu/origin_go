package combat

import (
	"fmt"
	"math"
)

const (
	StrengthNormalization = 1.0
	QualityNormalization  = 10.0
)

type Weapon struct {
	BaseDamage float64
	Range      float64
	Quality    float64
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func (weapon Weapon) Validate() error {
	if !finite(weapon.BaseDamage) || weapon.BaseDamage < 0 || !finite(weapon.Range) || weapon.Range <= 0 || !finite(weapon.Quality) || weapon.Quality <= 0 {
		return fmt.Errorf("weapon requires finite non-negative damage, positive range and quality")
	}
	return nil
}

func RawDamage(weapon Weapon, strength, multiplier float64) (float64, error) {
	if err := weapon.Validate(); err != nil {
		return 0, err
	}
	if !finite(strength) || strength < 0 || !finite(multiplier) || multiplier < 0 {
		return 0, fmt.Errorf("strength and damage multiplier must be finite and non-negative")
	}
	damage := weapon.BaseDamage * math.Pow(strength/StrengthNormalization, 0.25) * math.Pow(weapon.Quality/QualityNormalization, 0.25) * multiplier
	if !finite(damage) {
		return 0, fmt.Errorf("raw damage overflow")
	}
	return damage, nil
}

func ArmorDamage(raw, armor float64) (float64, error) {
	if !finite(raw) || raw < 0 || !finite(armor) || armor < 0 {
		return 0, fmt.Errorf("raw damage and armor must be finite and non-negative")
	}
	if raw == 0 {
		return 0, nil
	}
	// The equivalent ratio avoids overflowing either raw squared or raw + armor.
	if armor > raw {
		ratio := raw / armor
		return raw * ratio / (1 + ratio), nil
	}
	return raw / (1 + armor/raw), nil
}
