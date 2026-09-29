package main

import "testing"

func TestNoiseFieldsTerrainScale(t *testing.T) {
	perlin := NewPerlinNoise(42817)
	legacy := NewNoiseFields(perlin, 12)
	explicitDefault := NewNoiseFieldsWithTerrainScale(perlin, 12, defaultTerrainScale)
	custom := NewNoiseFieldsWithTerrainScale(perlin, 12, defaultTerrainScale*2)

	defaultElevation := legacy.Elevation(137, 269)
	if defaultElevation != explicitDefault.Elevation(137, 269) {
		t.Fatal("explicit default changed legacy elevation")
	}
	if defaultElevation == custom.Elevation(137, 269) {
		t.Fatal("custom terrain scale did not change elevation sampling")
	}
}
