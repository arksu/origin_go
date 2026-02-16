package entitystats

import (
	"math"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
)

func MaxStaminaFromCon(con int) float64 {
	if con < characterattrs.DefaultValue {
		con = characterattrs.DefaultValue
	}

	return math.Sqrt(float64(con)) * constt.StaminaScalePerCon
}

func MaxStaminaFromAttributes(values characterattrs.Values) float64 {
	return MaxStaminaFromCon(characterattrs.Get(values, characterattrs.CON))
}

func ClampStamina(value float64, max float64) float64 {
	if max < 0 {
		max = 0
	}
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

func RoundToUint32(value float64) uint32 {
	if value <= 0 {
		return 0
	}
	if value >= math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(math.Round(value))
}
