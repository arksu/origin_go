package main

import (
	"fmt"
	"math"
)

func (opts RiverOptions) validateLakeOptions() error {
	for name, value := range map[string]float64{
		"lake_island_small_chance":  opts.LakeIslandSmallChance,
		"lake_island_medium_chance": opts.LakeIslandMediumChance,
		"lake_island_large_chance":  opts.LakeIslandLargeChance,
		"lake_island_second_chance": opts.LakeIslandSecondChance,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("river.%s must be finite and within [0,1]", name)
		}
	}
	if opts.LakePeninsulaCountMax < 0 || opts.LakePeninsulaCountMax > 8 {
		return fmt.Errorf("river.lake_peninsula_count_max must be within [0,8]")
	}
	if math.IsNaN(opts.LakePeninsulaDepthRatio) || math.IsInf(opts.LakePeninsulaDepthRatio, 0) || opts.LakePeninsulaDepthRatio < 0 || opts.LakePeninsulaDepthRatio > 0.6 {
		return fmt.Errorf("river.lake_peninsula_depth_ratio must be finite and within [0,0.6]")
	}
	if opts.LakeShoreVariationTiles < 0 || opts.LakeShoreVariationTiles > 16 {
		return fmt.Errorf("river.lake_shore_variation_tiles must be within [0,16] tiles")
	}
	if opts.LakeIslandRadiusMin != 0 || opts.LakeIslandRadiusMax != 0 || opts.LakeIrregularEnabled {
		if opts.LakeIslandRadiusMin < 2 || opts.LakeIslandRadiusMax < opts.LakeIslandRadiusMin || opts.LakeIslandRadiusMax > 64 {
			return fmt.Errorf("river.lake_island_radius_min/max must satisfy 2 <= min <= max <= 64 tiles")
		}
	}
	if opts.LakeShallowWidthMin < 0 || opts.LakeShallowWidthMax < opts.LakeShallowWidthMin || opts.LakeShallowWidthMax > 32 {
		return fmt.Errorf("river.lake_shallow_width_min/max must satisfy 0 <= min <= max <= 32 tiles")
	}
	if opts.LakeIrregularEnabled && (!opts.LayoutDraw || opts.FairwayWidthTiles <= 0) {
		return fmt.Errorf("river.lake_irregular_enabled requires layout_draw and positive fairway_width_tiles")
	}
	return nil
}

func estimateLakeGeometryBytes(width, height int, opts RiverOptions) (uint64, error) {
	pixels, err := checkedMulUint64(uint64(width), uint64(height))
	if err != nil {
		return 0, err
	}
	profile := resolveLakeSizeProfile(width, height, opts)
	radius := maxInt(profile.SmallMax, maxInt(profile.MediumMax, profile.LargeMax))
	side, err := checkedMulUint64(uint64(radius), 4)
	if err != nil {
		return 0, err
	}
	side, err = checkedAddUint64(side, uint64(2*(opts.LakeShoreVariationTiles+opts.FairwayWidthTiles+opts.LakeShallowWidthMax+4)))
	if err != nil {
		return 0, err
	}
	localPixels, err := checkedMulUint64(minUint64(uint64(width), side), minUint64(uint64(height), side))
	if err != nil {
		return 0, err
	}
	lakeBuffers, err := checkedMulUint64(uint64(opts.LakeCount), 5)
	if err != nil {
		return 0, err
	}
	retained, err := checkedMulUint64(localPixels, lakeBuffers)
	if err != nil {
		return 0, err
	}
	scratch, err := checkedMulUint64(localPixels, 64)
	if err != nil {
		return 0, err
	}
	global, err := checkedMulUint64(pixels, 2)
	if err != nil {
		return 0, err
	}
	total, err := checkedAddUint64(global, retained)
	if err != nil {
		return 0, err
	}
	return checkedAddUint64(total, scratch)
}

func minUint64(first, second uint64) uint64 {
	if first < second {
		return first
	}
	return second
}
