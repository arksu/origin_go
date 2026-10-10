package main

import (
	"context"
	"fmt"
	"math/bits"
)

const (
	spotCenterSalt  = uint64(0xD4E12C77A3296B51)
	spotQualitySalt = uint64(0x872A619BF43D05CE)
	spotSpawnSalt   = uint64(0xA7615FBDF0932C48)
)

// GeneratedSpot contains the immutable initial geometry and quality of one spot.
// CenterX, CenterY, and Radius are absolute world units, not tile coordinates.
type GeneratedSpot struct {
	SpotType    string
	DistrictX   int
	DistrictY   int
	CenterX     int
	CenterY     int
	Radius      int
	PeakQuality int16
}

type SpotGenerationStats struct {
	Districts       uint64
	Generated       map[string]uint64
	Skipped         map[string]uint64
	SkippedByChance map[string]uint64
}

type spotCenterCandidate struct {
	found bool
	x, y  int
	hash  uint64
}

func (candidate *spotCenterCandidate) consider(x, y int, hash uint64) {
	if !candidate.found || hash < candidate.hash || hash == candidate.hash &&
		(y < candidate.y || y == candidate.y && x < candidate.x) {
		*candidate = spotCenterCandidate{found: true, x: x, y: y, hash: hash}
	}
}

// GenerateSpots streams spots in district (y, x), then canonical type order.
// It only reads the final terrain and keeps five candidates for the current
// district. Neither map size nor the number of eligible tiles grows its memory.
func GenerateSpots(ctx context.Context, terrain *TerrainPrecompute, config SpotsConfig, seed int64, emit func(GeneratedSpot) error) (SpotGenerationStats, error) {
	stats := SpotGenerationStats{
		Generated:       make(map[string]uint64, spotTypeCount),
		Skipped:         make(map[string]uint64, spotTypeCount),
		SkippedByChance: make(map[string]uint64, spotTypeCount),
	}
	for _, spotType := range spotTypes {
		stats.Generated[spotType] = 0
		stats.Skipped[spotType] = 0
		stats.SkippedByChance[spotType] = 0
	}
	if ctx == nil || terrain == nil || emit == nil {
		return stats, fmt.Errorf("spot generation requires context, terrain, and output callback")
	}
	if err := config.ValidateForWorld(terrain.WidthTiles, terrain.HeightTiles); err != nil {
		return stats, err
	}
	tileCount, err := checkedMulInt(terrain.WidthTiles, terrain.HeightTiles)
	if err != nil {
		return stats, fmt.Errorf("spot terrain tile count: %w", err)
	}
	if len(terrain.Tiles) != tileCount {
		return stats, fmt.Errorf("spot terrain has %d tiles, expected %d", len(terrain.Tiles), tileCount)
	}
	radius, err := config.RadiusWorld()
	if err != nil {
		return stats, err
	}

	// Each byte-sized tile ID maps to a bit per eligible type. This fixed lookup
	// avoids five allow-list searches for every tile without a world-sized mask.
	var eligibleTypes [256]uint8
	var typeSalts [spotTypeCount]uint64
	var spawnChances [spotTypeCount]float64
	for index, spotType := range spotTypes {
		typeSalts[index] = spotTypeSalt(spotType)
		for _, definition := range config.Spots {
			if definition.Type == spotType {
				spawnChances[index] = definition.spawnChance()
				for _, tileID := range definition.CenterTiles {
					eligibleTypes[tileID] |= 1 << index
				}
				break
			}
		}
	}

	districtSize := int(config.DistrictSizeTiles)
	districtY := 0
	for startY := 0; startY < terrain.HeightTiles; districtY++ {
		endY := startY + minInt(districtSize, terrain.HeightTiles-startY)
		districtX := 0
		for startX := 0; startX < terrain.WidthTiles; districtX++ {
			if err := ctx.Err(); err != nil {
				return stats, err
			}
			endX := startX + minInt(districtSize, terrain.WidthTiles-startX)
			// A single independent roll per district/type, never one per tile.
			// Rejected types still record eligibility but do not hash candidates.
			var spawnMask, foundEligibleMask uint8
			for index, chance := range spawnChances {
				if chance == 1 || chance > 0 && coordHash01(seed, districtX, districtY, typeSalts[index]^spotSpawnSalt) < chance {
					spawnMask |= 1 << index
				}
			}
			var candidates [spotTypeCount]spotCenterCandidate
			for y := startY; y < endY; y++ {
				if err := ctx.Err(); err != nil {
					return stats, err
				}
				row := y * terrain.WidthTiles
				for x := startX; x < endX; x++ {
					eligible := eligibleTypes[terrain.Tiles[row+x]]
					foundEligibleMask |= eligible
					mask := eligible & spawnMask
					for mask != 0 {
						index := bits.TrailingZeros8(mask)
						hash := spotCoordinateHash(seed, x, y, typeSalts[index]^spotCenterSalt)
						candidates[index].consider(x, y, hash)
						mask &= mask - 1
					}
				}
			}
			stats.Districts++
			for index, candidate := range candidates {
				if err := ctx.Err(); err != nil {
					return stats, err
				}
				spotType := spotTypes[index]
				if foundEligibleMask&(1<<index) == 0 {
					stats.Skipped[spotType]++
					continue
				}
				if spawnMask&(1<<index) == 0 {
					stats.SkippedByChance[spotType]++
					continue
				}
				centerX, err := spotTileCenter(int64(candidate.x))
				if err != nil {
					return stats, err
				}
				centerY, err := spotTileCenter(int64(candidate.y))
				if err != nil {
					return stats, err
				}
				qualityHash := spotCoordinateHash(seed, districtX, districtY, typeSalts[index]^spotQualitySalt)
				qualityRange := uint64(config.PeakQualityMax - config.PeakQualityMin + 1)
				quality := config.PeakQualityMin + int64(qualityHash%qualityRange)
				spot := GeneratedSpot{
					SpotType: spotType, DistrictX: districtX, DistrictY: districtY,
					CenterX: centerX, CenterY: centerY, Radius: radius, PeakQuality: int16(quality),
				}
				if err := emit(spot); err != nil {
					return stats, fmt.Errorf("emit %s spot in district (%d,%d): %w", spotType, districtX, districtY, err)
				}
				stats.Generated[spotType]++
				if err := ctx.Err(); err != nil {
					return stats, err
				}
			}
			startX = endX
		}
		startY = endY
	}
	return stats, ctx.Err()
}

// Type keys are stable when config order or the canonical output order changes.
func spotTypeSalt(spotType string) uint64 {
	var key uint64
	for index := range len(spotType) {
		key = key<<8 | uint64(spotType[index])
	}
	return splitMix64(key)
}

// Keep the existing coordinate-mixing algorithm, retaining all 64 output bits
// instead of converting to the 53-bit floating point value used by coordHash01.
func spotCoordinateHash(seed int64, x, y int, salt uint64) uint64 {
	mix := uint64(seed)
	mix ^= uint64(int64(x)) * 0x9E3779B185EBCA87
	mix ^= uint64(int64(y)) * 0xC2B2AE3D27D4EB4F
	return splitMix64(mix ^ salt)
}
