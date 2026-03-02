package main

import "testing"

func TestApplyShorelineSandRemovesInlandSandRing(t *testing.T) {
	width := 9
	height := 9
	tileCount := width * height

	tiles := make([]byte, tileCount)
	baseTiles := make([]byte, tileCount)
	elevation := make([]float32, tileCount)
	riverClass := make([]RiverClass, tileCount)

	for i := range tiles {
		tiles[i] = tileStone
		baseTiles[i] = tileStone
		elevation[i] = 0.8
	}

	cx, cy := 4, 4
	centerIdx := tileIndex(cx, cy, width)
	tiles[centerIdx] = tileWaterDeep
	riverClass[centerIdx] = riverDeep

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x := cx + dx
			y := cy + dy
			idx := tileIndex(x, y, width)
			tiles[idx] = tileSand
			baseTiles[idx] = tileStone
		}
	}

	applyShorelineSand(tiles, baseTiles, riverClass, elevation, width, height, 1234)

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x := cx + dx
			y := cy + dy
			idx := tileIndex(x, y, width)
			if tiles[idx] == tileSand {
				t.Fatalf("expected inland sand ring to be removed at (%d,%d)", x, y)
			}
		}
	}
}

func TestApplyShorelineSandAddsOccasionalRiverShoreSand(t *testing.T) {
	width := 48
	height := 48
	tileCount := width * height

	tiles := make([]byte, tileCount)
	baseTiles := make([]byte, tileCount)
	elevation := make([]float32, tileCount)
	riverClass := make([]RiverClass, tileCount)

	for i := range tiles {
		tiles[i] = tileGrass
		baseTiles[i] = tileGrass
		elevation[i] = 0.78
	}

	for y := 4; y < height-4; y++ {
		x := width / 2
		idx := tileIndex(x, y, width)
		tiles[idx] = tileWater
		riverClass[idx] = riverShallow
	}

	applyShorelineSand(tiles, baseTiles, riverClass, elevation, width, height, 99)

	sandCount := 0
	for _, tile := range tiles {
		if tile == tileSand {
			sandCount++
		}
	}
	if sandCount == 0 {
		t.Fatalf("expected occasional shore sand near river")
	}
}
