package entityhealth

import (
	"errors"
	"math"

	"origin/internal/characterattrs"
)

var ErrInvalidPools = errors.New("health pools must be finite and nonnegative")

// ValidatePools checks storage-bound values without modifying or rounding them.
func ValidatePools(shp, hhp float64) error {
	if shp < 0 || hhp < 0 || math.IsNaN(shp) || math.IsNaN(hhp) || math.IsInf(shp, 0) || math.IsInf(hhp, 0) {
		return ErrInvalidPools
	}
	return nil
}

const (
	baseHHPPerSqrtCon   = 25.0
	regenFullMultiplier = 0.002
	regenHungryMul      = 0.001
)

// MaxHHPFromCon derives the health ceiling from CON and the world's fixed multiplier.
// Changing lifeDeathFactor requires a full world wipe: a lower ceiling can permanently
// reduce saved HHP when health is clamped, including when a character logs in after a restart.
func MaxHHPFromCon(con int, lifeDeathFactor float64) float64 {
	if con < characterattrs.DefaultValue {
		con = characterattrs.DefaultValue
	}
	if lifeDeathFactor <= 0 {
		lifeDeathFactor = 1
	}
	return math.Sqrt(float64(con)) * baseHHPPerSqrtCon * lifeDeathFactor
}

func ClampHealth(shp, hhp, mhp float64) (float64, float64) {
	if mhp < 0 {
		mhp = 0
	}
	if hhp < 0 {
		hhp = 0
	} else if hhp > mhp {
		hhp = mhp
	}
	if shp < 0 {
		shp = 0
	} else if shp > hhp {
		shp = hhp
	}
	return shp, hhp
}

func ApplyDamage(shp, hhp, mhp, softDamage, hardDamage float64) (nextSHP, nextHHP float64, knockedOut bool, dead bool) {
	if softDamage < 0 {
		softDamage = 0
	}
	if hardDamage < 0 {
		hardDamage = 0
	}
	nextSHP = shp - softDamage
	nextHHP = hhp - hardDamage
	nextSHP, nextHHP = ClampHealth(nextSHP, nextHHP, mhp)
	dead = nextHHP <= 0
	knockedOut = !dead && nextSHP <= 0
	return nextSHP, nextHHP, knockedOut, dead
}

func ResolveSHPRegenPerInterval(mhp, energy float64) float64 {
	if mhp <= 0 {
		return 0
	}
	switch {
	case energy >= 900:
		return mhp * regenFullMultiplier
	case energy >= 800:
		return mhp * regenHungryMul
	default:
		return 0
	}
}
