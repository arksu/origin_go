package main

import (
	"fmt"
	"math"

	"gopkg.in/yaml.v3"
)

func validateJunctionSpacingInput(content []byte, wrapped bool) error {
	type junctionInput struct {
		Spacing *float64 `yaml:"junction_spacing_tiles"`
	}
	var fields junctionInput
	if wrapped {
		var document struct {
			River junctionInput `yaml:"river"`
		}
		if err := yaml.Unmarshal(content, &document); err != nil {
			return fmt.Errorf("river junction input: %w", err)
		}
		fields = document.River
	} else if err := yaml.Unmarshal(content, &fields); err != nil {
		return fmt.Errorf("river junction input: %w", err)
	}
	if fields.Spacing != nil && (math.IsNaN(*fields.Spacing) || math.IsInf(*fields.Spacing, 0) || math.Trunc(*fields.Spacing) != *fields.Spacing) {
		return fmt.Errorf("river.junction_spacing_tiles must be an integer")
	}
	return nil
}

func (opts RiverOptions) validateJunctionOptions() error {
	if math.IsNaN(opts.JunctionChance) || math.IsInf(opts.JunctionChance, 0) || opts.JunctionChance < 0 || opts.JunctionChance > 1 {
		return fmt.Errorf("river.junction_chance must be finite and within [0,1]")
	}
	if opts.JunctionSpacingTiles < 0 || opts.JunctionSpacingTiles > 8192 {
		return fmt.Errorf("river.junction_spacing_tiles must be within [0,8192] tiles")
	}
	if opts.JunctionChance > 0 && (!opts.LayoutDraw || opts.ShapeWavelengthTiles == 0 || opts.FairwayWidthTiles <= 0 || opts.JunctionSpacingTiles == 0) {
		return fmt.Errorf("river.junction_chance requires layout_draw, Option B, positive fairway_width_tiles and junction_spacing_tiles")
	}
	return nil
}

func estimateRiverJunctionBytes(width, height int, opts RiverOptions) (uint64, error) {
	if opts.JunctionChance == 0 {
		return 0, nil
	}
	if width <= 0 || height <= 0 || opts.MajorRiverCount < 0 || opts.LakeConnectionLimit < 0 || opts.LakeCount < 0 {
		return 0, fmt.Errorf("invalid river junction dimensions or budgets")
	}
	dimensions, err := checkedAddUint64(uint64(width), uint64(height))
	if err != nil {
		return 0, err
	}
	routes, err := checkedAddUint64(uint64(opts.MajorRiverCount), uint64(opts.LakeConnectionLimit)+1)
	if err != nil {
		return 0, err
	}
	points, err := checkedMulUint64(dimensions, routes)
	if err != nil {
		return 0, err
	}
	indexBytes, err := checkedMulUint64(points, 512)
	if err != nil {
		return 0, err
	}
	scratch, err := checkedMulUint64(dimensions, 2048)
	if err != nil {
		return 0, err
	}
	lakes, err := checkedMulUint64(uint64(opts.LakeCount), 128)
	if err != nil {
		return 0, err
	}
	scratch, err = checkedAddUint64(scratch, lakes)
	if err != nil {
		return 0, err
	}
	return checkedAddUint64(indexBytes, scratch)
}
