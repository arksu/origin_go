package main

import (
	"fmt"
	"image"
)

type previewWorldOptions struct {
	TerrainScale       float64 `yaml:"terrain_scale"`
	PerlinWaterEnabled bool    `yaml:"perlin_water_enabled"`
}

func validatePreviewMemory(opts MapgenOptions) error {
	width, height, err := opts.WorldTileDimensions()
	if err != nil {
		return err
	}
	estimated, err := opts.estimateTerrainBytes(width, height)
	if err != nil {
		return err
	}
	if opts.River.Enabled && opts.River.FairwayWidthTiles > 0 {
		extra, err := estimateRiverBendsBytes(width, height, opts.River)
		if err != nil {
			return err
		}
		estimated, err = checkedAddUint64(estimated, extra)
		if err != nil {
			return err
		}
	}
	pixels, err := checkedMulUint64(uint64(width), uint64(height))
	if err != nil {
		return err
	}
	imageBytes, err := checkedMulUint64(pixels, 12)
	if err != nil {
		return err
	}
	estimated, err = checkedAddUint64(estimated, imageBytes)
	if err != nil {
		return err
	}
	if estimated > maxPrecomputeBytes {
		return fmt.Errorf("estimated preview memory including RGBA and PNG buffers %d bytes exceeds limit %d bytes", estimated, maxPrecomputeBytes)
	}
	return nil
}

func renderBiomesLayer(img *image.RGBA, ctx renderContext) error {
	if ctx.Terrain == nil {
		return fmt.Errorf("biome preview requires precomputed terrain")
	}
	for index, tile := range ctx.Terrain.Tiles {
		if len(ctx.Terrain.RiverClass) > 0 && ctx.Terrain.RiverClass[index] != riverNone {
			continue
		}
		if ctx.Terrain.RiverFairways != nil && ctx.Terrain.RiverFairways.Protected[index] {
			continue
		}
		paint := tileColor(tile, riverNone, false)
		offset := (index/ctx.WidthTiles)*img.Stride + (index%ctx.WidthTiles)*4
		img.Pix[offset] = paint.R
		img.Pix[offset+1] = paint.G
		img.Pix[offset+2] = paint.B
		img.Pix[offset+3] = paint.A
	}
	return nil
}
