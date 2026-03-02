package main

import "math"

const (
	terrainScale          = 0.002
	deepWaterThreshold    = 0.25
	shallowWaterThreshold = 0.35
	sandThreshold         = 0.42
)

type NoiseFields struct {
	perlin       *PerlinNoise
	coordPerTile int
}

func NewNoiseFields(perlin *PerlinNoise, coordPerTile int) *NoiseFields {
	return &NoiseFields{
		perlin:       perlin,
		coordPerTile: coordPerTile,
	}
}

func (f *NoiseFields) Elevation(tileX, tileY int) float64 {
	worldX := float64(tileX * f.coordPerTile)
	worldY := float64(tileY * f.coordPerTile)
	elevation := f.perlin.Noise2D(worldX*terrainScale, worldY*terrainScale)
	return normalizeNoise(elevation)
}

func (f *NoiseFields) MoistureTemperature(tileX, tileY int) (float64, float64) {
	worldX := float64(tileX * f.coordPerTile)
	worldY := float64(tileY * f.coordPerTile)

	moisture := f.perlin.Noise2D(worldX*terrainScale*0.5+1000, worldY*terrainScale*0.5+1000)
	temperature := f.perlin.Noise2D(worldX*terrainScale*0.3+2000, worldY*terrainScale*0.3+2000)

	return normalizeNoise(moisture), normalizeNoise(temperature)
}

func (f *NoiseFields) BiomeSignals(tileX, tileY int, opts BiomeOptions) BiomeSignals {
	worldX := float64(tileX * f.coordPerTile)
	worldY := float64(tileY * f.coordPerTile)

	warpFreq := terrainScale * 0.08
	warpX := f.perlin.Noise2D(worldX*warpFreq+4300, worldY*warpFreq+4300)
	warpY := f.perlin.Noise2D(worldX*warpFreq+5300, worldY*warpFreq+5300)

	sampleX := worldX + warpX*opts.DomainWarpStrength
	sampleY := worldY + warpY*opts.DomainWarpStrength

	temperature := normalizeNoise(f.perlin.Noise2D(
		sampleX*terrainScale*0.3*opts.TemperatureScale+2000,
		sampleY*terrainScale*0.3*opts.TemperatureScale+2000,
	))
	moisture := normalizeNoise(f.perlin.Noise2D(
		sampleX*terrainScale*0.5*opts.MoistureScale+1000,
		sampleY*terrainScale*0.5*opts.MoistureScale+1000,
	))
	continentalness := normalizeNoise(f.perlin.Noise2D(
		sampleX*terrainScale*0.22*opts.ContinentalnessScale+3000,
		sampleY*terrainScale*0.22*opts.ContinentalnessScale+3000,
	))
	erosion := normalizeNoise(f.perlin.Noise2D(
		sampleX*terrainScale*0.6*opts.ErosionScale+4000,
		sampleY*terrainScale*0.6*opts.ErosionScale+4000,
	))
	weirdness := normalizeNoise(f.perlin.Noise2D(
		sampleX*terrainScale*1.1*opts.WeirdnessScale+5000,
		sampleY*terrainScale*1.1*opts.WeirdnessScale+5000,
	))

	ruggedness := clamp01((math.Abs(weirdness-0.5)*2*0.6 + (1.0-erosion)*0.7 + continentalness*0.35) / 1.65)
	wetness := clamp01((moisture*0.75 + (1.0-continentalness)*0.25) * opts.SwampClumpScale)

	return BiomeSignals{
		Temperature:     temperature,
		Moisture:        moisture,
		Continentalness: continentalness,
		Erosion:         erosion,
		Weirdness:       weirdness,
		Ruggedness:      ruggedness,
		Wetness:         wetness,
	}
}

func normalizeNoise(value float64) float64 {
	return (value + 1) / 2
}

func classifyBaseTile(elevation, moisture, temperature float64) byte {
	if elevation < deepWaterThreshold {
		return tileWaterDeep
	}
	if elevation < shallowWaterThreshold {
		return tileWater
	}

	// Sand is climate-driven instead of an automatic elevation ring around water.
	if moisture < 0.22 && temperature > 0.58 && elevation < 0.62 {
		return tileSand
	}
	if moisture > 0.6 {
		if temperature > 0.5 {
			return tileForestLeaf
		}
		return tileForestPine
	}
	return tileGrass
}

func isOceanTileByElevation(elevation float64) bool {
	return elevation < shallowWaterThreshold
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
