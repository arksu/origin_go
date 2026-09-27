package main

import "testing"

func TestCleanBiomeBorderArtifactsRemovesSingleTileSpike(t *testing.T) {
	width := 5
	height := 5
	tiles := make([]byte, width*height)
	for i := range tiles {
		tiles[i] = tileGrass
	}

	center := tileIndex(2, 2, width)
	tiles[center] = tileForestLeaf

	cleanBiomeBorderArtifacts(tiles, testStructuralLocks(tiles), width, height, 1)

	if tiles[center] != tileGrass {
		t.Fatalf("expected isolated border spike to be removed, got tile=%d", tiles[center])
	}
}

func TestCleanBiomeBorderArtifactsKeepsLockedCoastTiles(t *testing.T) {
	width := 5
	height := 5
	tiles := make([]byte, width*height)
	for i := range tiles {
		tiles[i] = tileForestLeaf
	}

	center := tileIndex(2, 2, width)
	tiles[center] = tileWater

	cleanBiomeBorderArtifacts(tiles, testStructuralLocks(tiles), width, height, 2)

	if tiles[center] != tileWater {
		t.Fatalf("expected locked coast tile to stay unchanged, got tile=%d", tiles[center])
	}
}

func testStructuralLocks(tiles []byte) []bool {
	locked := make([]bool, len(tiles))
	for i, tile := range tiles {
		locked[i] = tile == tileWater || tile == tileWaterDeep || tile == tileSand || tile == tileMountain || tile == tileStone
	}
	return locked
}
