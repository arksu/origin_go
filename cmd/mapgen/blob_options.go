package main

import (
	"fmt"
	"math"
	"reflect"
)

type blobShapeOptions struct {
	name                    string
	minSize, maxSize, width float64
}

func (b BiomeOptions) blobShapes() [7]blobShapeOptions {
	return [7]blobShapeOptions{
		{"forest", b.BlobForestSizeMin, b.BlobForestSizeMax, b.BlobForestWidth},
		{"heath", b.BlobHeathSizeMin, b.BlobHeathSizeMax, b.BlobHeathWidth},
		{"moor", b.BlobMoorSizeMin, b.BlobMoorSizeMax, b.BlobMoorWidth},
		{"swamp", b.BlobSwampSizeMin, b.BlobSwampSizeMax, b.BlobSwampWidth},
		{"thicket", b.BlobThicketSizeMin, b.BlobThicketSizeMax, b.BlobThicketWidth},
		{"dirt", b.BlobDirtSizeMin, b.BlobDirtSizeMax, b.BlobDirtWidth},
		{"clay", b.BlobClaySizeMin, b.BlobClaySizeMax, b.BlobClayWidth},
	}
}

func (b BiomeOptions) validateBlobOptions() error {
	// Inspect every float so newly added settings cannot silently accept NaN/Inf.
	value := reflect.ValueOf(b)
	for i := 0; i < value.NumField(); i++ {
		if value.Field(i).Kind() != reflect.Float64 {
			continue
		}
		number := value.Field(i).Float()
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("biomes.%s must be finite", value.Type().Field(i).Tag.Get("yaml"))
		}
	}
	for _, setting := range []struct {
		name  string
		value int
	}{
		{"blob_seed_spacing", b.BlobSeedSpacing}, {"blob_secondary_spacing", b.BlobSecondarySpacing}, {"blob_islet_spacing", b.BlobIsletSpacing},
	} {
		if setting.value <= 0 {
			return fmt.Errorf("biomes.%s must be > 0", setting.name)
		}
	}
	if b.BlobSeedJitter < 0 || b.BlobSeedJitter >= 1 {
		return fmt.Errorf("biomes.blob_seed_jitter must be within [0,1)")
	}
	for _, shape := range b.blobShapes() {
		if shape.minSize < 1 || shape.maxSize > 4096 || shape.minSize > shape.maxSize {
			return fmt.Errorf("biomes.blob_%s_size_min/max must satisfy 1 <= min <= max <= 4096", shape.name)
		}
		if shape.width < 0 || shape.width > 4096 {
			return fmt.Errorf("biomes.blob_%s_width must be within [0,4096]", shape.name)
		}
	}
	totalWeight := 0.0
	for _, setting := range []struct {
		name  string
		value float64
	}{
		{"forest", b.BlobForestWeight}, {"heath", b.BlobHeathWeight}, {"moor", b.BlobMoorWeight}, {"swamp", b.BlobSwampWeight}, {"skip", b.BlobSkipWeight},
	} {
		if setting.value < 0 {
			return fmt.Errorf("biomes.blob_%s_weight must be >= 0", setting.name)
		}
		totalWeight += setting.value
	}
	if math.IsInf(totalWeight*1.5, 0) {
		return fmt.Errorf("biomes.blob weights sum with climate multiplier must be finite")
	}
	for _, setting := range []struct {
		name  string
		value float64
	}{
		{"blob_thicket_density", b.BlobThicketDensity}, {"blob_dirt_density", b.BlobDirtDensity}, {"blob_clay_density", b.BlobClayDensity},
		{"blob_forest_cold_threshold", b.BlobForestColdThreshold}, {"blob_moor_moisture_max", b.BlobMoorMoistureMax}, {"blob_moor_temperature_max", b.BlobMoorTemperatureMax},
		{"blob_swamp_wetness_min", b.BlobSwampWetnessMin}, {"blob_swamp_moisture_min", b.BlobSwampMoistureMin}, {"blob_clay_wetness_min", b.BlobClayWetnessMin}, {"blob_clay_moisture_min", b.BlobClayMoistureMin},
		{"blob_islet_chance", b.BlobIsletChance},
	} {
		if setting.value < 0 || setting.value > 1 {
			return fmt.Errorf("biomes.%s must be within [0,1]", setting.name)
		}
	}
	if b.BlobDirtDensity+b.BlobClayDensity > 1 {
		return fmt.Errorf("biomes.blob_dirt_density + blob_clay_density must be <= 1")
	}
	if b.BlobRaggedness < 0 || b.BlobRaggedness > 64 {
		return fmt.Errorf("biomes.blob_raggedness must be within [0,64]")
	}
	if b.BlobMaxNodes < 4 || b.BlobMaxNodes > 256 {
		return fmt.Errorf("biomes.blob_max_nodes must be within [4,256]")
	}
	if b.BlobMaxDepth < 3 || b.BlobMaxDepth > 32 {
		return fmt.Errorf("biomes.blob_max_depth must be within [3,32]")
	}
	return nil
}

func (o MapgenOptions) estimateTerrainBytes(width, height int) (uint64, error) {
	count, err := checkedMulUint64(uint64(width), uint64(height))
	if err != nil {
		return 0, fmt.Errorf("tile count overflow: %w", err)
	}
	total, err := estimatePrecomputeBytes(count, o.River.Enabled, o.Biome.Enabled)
	if err != nil {
		return 0, err
	}
	if !o.Biome.Enabled || !o.Biome.BlobEnabled {
		return total, nil
	}
	// Lock, cleanup visited bitmap, and a single reusable component queue.
	extra, err := checkedMulUint64(count, 10)
	if err != nil {
		return 0, err
	}
	total, err = checkedAddUint64(total, extra)
	if err != nil {
		return 0, err
	}
	b := o.Biome
	columns := (width-1)/b.BlobSeedSpacing + 1
	rows := (height-1)/b.BlobSeedSpacing + 1
	records, err := checkedMulUint64(uint64(columns), uint64(rows))
	if err != nil {
		return 0, err
	}
	records, err = checkedMulUint64(records, 64)
	if err != nil {
		return 0, err
	}
	total, err = checkedAddUint64(total, records)
	if err != nil {
		return 0, err
	}
	maxSize, maxWidth := 0.0, 0.0
	for _, shape := range b.blobShapes() {
		maxSize = math.Max(maxSize, shape.maxSize)
		width := shape.width
		if width == 0 {
			width = shape.maxSize / 2
		}
		maxWidth = math.Max(maxWidth, width)
	}
	// Tree reach + outline offset + Hermite tangent overshoot + displacement.
	reach := float64(min(b.BlobMaxDepth, b.BlobMaxNodes-1))*1.1*maxSize + 2*maxWidth + b.BlobRaggedness + 2
	side := int(math.Ceil(2*reach)) + 1
	scratchTiles, err := checkedMulUint64(uint64(min(width, side)), uint64(min(height, side)))
	if err != nil {
		return 0, err
	}
	// Two reusable raster buffers during growth plus a queue of tile indices.
	scratch, err := checkedMulUint64(scratchTiles, 18)
	if err != nil {
		return 0, err
	}
	// At most three outline vertices per node; conservative chord/subdivision bound.
	samples := uint64(3*b.BlobMaxNodes) * uint64(math.Ceil((2.2*maxSize+4.4*maxWidth)/3)+1)
	geometry, err := checkedMulUint64(samples, 64)
	if err != nil {
		return 0, err
	}
	for _, size := range []uint64{scratch, geometry} {
		total, err = checkedAddUint64(total, size)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func checkedAddUint64(a, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, fmt.Errorf("precompute memory addition overflow")
	}
	return a + b, nil
}
