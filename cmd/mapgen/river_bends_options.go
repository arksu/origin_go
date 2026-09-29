package main

import (
	"fmt"
	"math"
)

func (opts RiverOptions) validateBends() error {
	if opts.ShapeWavelengthTiles != 0 && (opts.ShapeWavelengthTiles < 64 || opts.ShapeWavelengthTiles > 8192) {
		return fmt.Errorf("river.shape_wavelength_tiles must be zero or within [64,8192]")
	}
	if opts.FairwayWidthTiles < 0 || opts.FairwayWidthTiles > 63 || (opts.FairwayWidthTiles > 0 && (opts.FairwayWidthTiles%2 == 0 || opts.FairwayWidthTiles > opts.RiverWidthMin)) {
		return fmt.Errorf("river.fairway_width_tiles must be zero or odd within [1,63] and <= river.river_width_min")
	}
	if math.IsNaN(opts.TributaryRatio) || math.IsInf(opts.TributaryRatio, 0) || opts.TributaryRatio < 0 || opts.TributaryRatio > 1 {
		return fmt.Errorf("river.tributary_ratio must be finite and within [0,1]")
	}
	for name, value := range map[string]int{
		"tributary_spacing_tiles": opts.TributarySpacingTiles,
		"tributary_length_min":    opts.TributaryLengthMin,
		"tributary_length_max":    opts.TributaryLengthMax,
	} {
		if value < 0 || value > 8192 {
			return fmt.Errorf("river.%s must be within [0,8192]", name)
		}
	}
	if opts.FairwayWidthTiles > 0 && !opts.LayoutDraw {
		return fmt.Errorf("river.fairway_width_tiles requires layout_draw")
	}
	if opts.ShapeWavelengthTiles > 0 {
		if !opts.LayoutDraw || opts.FairwayWidthTiles == 0 {
			return fmt.Errorf("river.shape_wavelength_tiles requires layout_draw and positive fairway_width_tiles")
		}
		for name, value := range map[string]float64{
			"shape_frequency_scale": opts.ShapeFrequencyScale, "shape_amplitude_scale": opts.ShapeAmplitudeScale,
			"shape_octave_gain": opts.ShapeOctaveGain, "shape_distance_cap": opts.ShapeDistanceCap,
			"shape_along_scale": opts.ShapeAlongScale, "meander_strength": opts.MeanderStrength,
			"shape_long_meander_scale": opts.ShapeLongMeanderScale, "shape_short_meander_scale": opts.ShapeShortMeanderScale,
			"shape_short_meander_bias": opts.ShapeShortMeanderBias,
		} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("river.%s must be finite", name)
			}
		}
		for name, value := range map[string]float64{
			"shape_frequency_scale": opts.ShapeFrequencyScale,
			"shape_amplitude_scale": opts.ShapeAmplitudeScale,
			"shape_distance_cap":    opts.ShapeDistanceCap,
		} {
			if value <= 0 {
				return fmt.Errorf("river.%s must be > 0", name)
			}
		}
		if opts.ShapeOctaves < 1 || opts.ShapeOctaves > 4 {
			return fmt.Errorf("river.shape_octaves must be within [1,4]")
		}
		if opts.ShapeOctaveGain < 0.05 || opts.ShapeOctaveGain > 0.6 {
			return fmt.Errorf("river.shape_octave_gain must be within [0.05,0.6]")
		}
	}
	if opts.TributaryRatio > 0 && (opts.ShapeWavelengthTiles == 0 || opts.TributarySpacingTiles == 0 || opts.TributaryLengthMin == 0 || opts.TributaryLengthMax < opts.TributaryLengthMin) {
		return fmt.Errorf("river.tributary_ratio requires Option B, positive tributary_spacing_tiles and ordered positive tributary_length_min/max")
	}
	return nil
}

func estimateRiverBendsBytes(width, height int, opts RiverOptions) (uint64, error) {
	count, err := checkedMulUint64(uint64(width), uint64(height))
	if err != nil {
		return 0, err
	}
	routeBudget, err := checkedAddUint64(uint64(opts.MajorRiverCount), uint64(opts.LakeConnectionLimit))
	if err != nil {
		return 0, err
	}
	routeBudget, err = checkedAddUint64(routeBudget, 1)
	if err != nil {
		return 0, err
	}
	dimensionSum, err := checkedAddUint64(uint64(width), uint64(height))
	if err != nil {
		return 0, err
	}
	bytesPerRoute, err := checkedMulUint64(dimensionSum, 256)
	if err != nil {
		return 0, err
	}
	routeBytes, err := checkedMulUint64(routeBudget, bytesPerRoute)
	if err != nil {
		return 0, err
	}
	scratch, err := checkedMulUint64(count, 2)
	if err != nil {
		return 0, err
	}
	return checkedAddUint64(routeBytes, scratch)
}
