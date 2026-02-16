package entitystats

import constt "origin/internal/const"

func ResolveStaminaGainFromEnergy(energySpent float64) float64 {
	if energySpent <= 0 {
		return 0
	}
	return energySpent * constt.StaminaPerEnergyUnit
}

func RegenerateStamina(
	currentStamina float64,
	currentEnergy float64,
	maxStamina float64,
) (nextStamina float64, nextEnergy float64, changed bool) {
	if maxStamina < 0 {
		maxStamina = 0
	}

	clampedStamina := ClampStamina(currentStamina, maxStamina)
	clampedEnergy := currentEnergy
	if clampedEnergy < 0 {
		clampedEnergy = 0
	}

	nextStamina = clampedStamina
	nextEnergy = clampedEnergy
	changed = nextStamina != currentStamina || nextEnergy != currentEnergy

	if nextEnergy <= 0 || nextStamina >= maxStamina {
		return nextStamina, nextEnergy, changed
	}

	energySpent := constt.RegenEnergySpendPerTick
	if energySpent > nextEnergy {
		energySpent = nextEnergy
	}
	if energySpent <= 0 {
		return nextStamina, nextEnergy, changed
	}

	nextEnergy -= energySpent
	nextStamina = ClampStamina(nextStamina+ResolveStaminaGainFromEnergy(energySpent), maxStamina)
	return nextStamina, nextEnergy, true
}
