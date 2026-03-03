package entityhealth

import (
	"math"

	"origin/internal/characterattrs"
)

const (
	baseHHPPerSqrtCon   = 25.0
	regenFullMultiplier = 0.002
	regenHungryMul      = 0.001
)

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
