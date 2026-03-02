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
	if elevation < sandThreshold {
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
