package entitystats

import (
	"math"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
)

type MovementTileContext struct {
	TileID  byte
	HasTile bool
}

// ResolveTileStaminaModifier is intentionally a dedicated hook so tile-specific
// stamina formulas (e.g. paved roads) can be introduced without touching callers.
func ResolveTileStaminaModifier(tile MovementTileContext) float64 {
	_ = tile
	return 1.0
}

func ResolveMovementStaminaCostPerTick(
	mode constt.MoveMode,
	attributes characterattrs.Values,
	tile MovementTileContext,
) float64 {
	tileModifier := ResolveTileStaminaModifier(tile)
	if tileModifier <= 0 {
		return 0
	}

	var base float64
	switch mode {
	case constt.Crawl:
		base = constt.MovementStaminaCostCrawlPerTick
	case constt.Walk:
		base = constt.MovementStaminaCostWalkPerTick
	case constt.Run:
		base = constt.MovementStaminaCostRunPerTick
	case constt.FastRun:
		base = constt.MovementStaminaCostFastRunPerTick
	case constt.Swim:
		base = SwimStaminaCostPerTick(characterattrs.Get(attributes, characterattrs.CON))
	default:
		base = constt.MovementStaminaCostStayPerTick
	}
	if base <= 0 {
		return 0
	}
	return base * tileModifier
}

func SwimStaminaCostPerTick(con int) float64 {
	if con < characterattrs.DefaultValue {
		con = characterattrs.DefaultValue
	}
	return 1.0 / math.Sqrt(float64(con)/10.0)
}

func ResolveAllowedMoveMode(mode constt.MoveMode, stamina float64, maxStamina float64) (constt.MoveMode, bool) {
	if mode > constt.Swim {
		mode = constt.Walk
	}
	if maxStamina <= 0 {
		return constt.Crawl, false
	}

	if stamina < 0 {
		stamina = 0
	}

	if stamina < maxStamina*constt.StaminaNoMoveThresholdPercent {
		return constt.Crawl, false
	}
	if stamina < maxStamina*constt.StaminaCrawlOnlyThresholdPercent {
		return constt.Crawl, true
	}
	if stamina < maxStamina*constt.StaminaNoRunThresholdPercent {
		if mode == constt.Run || mode == constt.FastRun {
			return constt.Walk, true
		}
		return mode, true
	}
	if stamina < maxStamina*constt.StaminaNoFastRunThresholdPercent && mode == constt.FastRun {
		return constt.Run, true
	}
	return mode, true
}

func LongActionStaminaFloor(maxStamina float64) float64 {
	if maxStamina <= 0 {
		return 0
	}
	return maxStamina * constt.LongActionStaminaFloorPercent
}

func CanConsumeLongActionStamina(stamina float64, maxStamina float64, cost float64) bool {
	if cost <= 0 {
		return true
	}
	return stamina-cost >= LongActionStaminaFloor(maxStamina)
}
