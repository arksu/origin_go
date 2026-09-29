package main

import "testing"

func isWaterTile(tile byte) bool {
	return tile == tileWater || tile == tileWaterDeep
}

func TestResolveTileTypeOceanPrecedenceOverRiver(t *testing.T) {
	got := resolveTileType(0.30, tileForestLeaf, riverDeep, true, true)
	if got != tileWater {
		t.Fatalf("expected shallow ocean water to win over river override, got tile=%d", got)
	}
}

func TestResolveTileTypeRiverOverridesLand(t *testing.T) {
	gotDeep := resolveTileType(0.60, tileGrass, riverDeep, true, true)
	if gotDeep != tileWaterDeep {
		t.Fatalf("expected deep river override on land, got tile=%d", gotDeep)
	}

	gotShallow := resolveTileType(0.60, tileGrass, riverShallow, true, true)
	if gotShallow != tileWater {
		t.Fatalf("expected shallow river override on land, got tile=%d", gotShallow)
	}
}

func TestResolveTileTypeMatchesBiomeWhenNoRiver(t *testing.T) {
	got := resolveTileType(0.70, tileForestLeaf, riverNone, true, false)
	if got != tileForestLeaf {
		t.Fatalf("expected forest leaf biome tile, got tile=%d", got)
	}

	got = resolveTileType(0.70, tileForestPine, riverNone, true, false)
	if got != tileForestPine {
		t.Fatalf("expected forest pine biome tile, got tile=%d", got)
	}

	got = resolveTileType(0.70, tileGrass, riverNone, true, false)
	if got != tileGrass {
		t.Fatalf("expected grass biome tile, got tile=%d", got)
	}
}

func TestResolveTileTypeDisablesOnlyPerlinWater(t *testing.T) {
	if got := resolveTileType(0.20, tileGrass, riverNone, false, true); got != tileGrass {
		t.Fatalf("disabled Perlin water changed low terrain to tile=%d", got)
	}
	if got := resolveTileType(0.20, tileGrass, riverDeep, false, true); got != tileWaterDeep {
		t.Fatalf("disabled Perlin water removed deep river, got tile=%d", got)
	}
	if got := resolveTileType(0.20, tileGrass, riverShallow, false, true); got != tileWater {
		t.Fatalf("disabled Perlin water removed shallow river, got tile=%d", got)
	}
}

func TestBuildTerrainPrecomputeCanDisablePerlinWater(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 3
	opts.ChunksY = 3
	opts.Threads = 2
	opts.Seed = 12345
	opts.River.Enabled = false
	fields := NewNoiseFields(NewPerlinNoise(opts.Seed), 12)

	withPerlinWater, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		t.Fatalf("BuildTerrainPrecompute with Perlin water error: %v", err)
	}
	opts.PerlinWaterEnabled = false
	withoutPerlinWater, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		t.Fatalf("BuildTerrainPrecompute without Perlin water error: %v", err)
	}

	waterWithFlag := 0
	for index, tile := range withPerlinWater.Tiles {
		if isWaterTile(tile) {
			waterWithFlag++
		}
		if isWaterTile(withoutPerlinWater.Tiles[index]) {
			t.Fatalf("Perlin water remained at tile %d", index)
		}
		if withPerlinWater.Elevation[index] != withoutPerlinWater.Elevation[index] {
			t.Fatalf("water flag changed elevation at tile %d", index)
		}
	}
	if waterWithFlag == 0 {
		t.Fatal("test fixture contains no Perlin water")
	}
}

func TestBuildTerrainPrecomputeKeepsDrawnRiversWithoutPerlinWater(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 3
	opts.ChunksY = 3
	opts.Threads = 2
	opts.Seed = 12345
	opts.PerlinWaterEnabled = false

	terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(opts.Seed), 12))
	if err != nil {
		t.Fatalf("BuildTerrainPrecompute error: %v", err)
	}
	waterTiles := 0
	for index, tile := range terrain.Tiles {
		if !isWaterTile(tile) {
			continue
		}
		waterTiles++
		if terrain.RiverClass[index] == riverNone && (terrain.RiverFairways == nil || !terrain.RiverFairways.Protected[index]) {
			t.Fatalf("water outside the drawn river network at tile %d", index)
		}
	}
	if waterTiles == 0 {
		t.Fatal("drawn river network disappeared with Perlin water disabled")
	}
}

func TestBuildTerrainPrecomputeCreatesRiverConversionsByDefault(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 3
	opts.ChunksY = 3
	opts.Threads = 2
	opts.Seed = 12345

	fields := NewNoiseFields(NewPerlinNoise(opts.Seed), 12)
	withRivers, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		t.Fatalf("BuildTerrainPrecompute with rivers error: %v", err)
	}

	opts.River.Enabled = false
	withoutRivers, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		t.Fatalf("BuildTerrainPrecompute without rivers error: %v", err)
	}

	convertedLandToWater := 0
	for idx := range withRivers.Tiles {
		if isWaterTile(withoutRivers.Tiles[idx]) {
			continue
		}
		if isWaterTile(withRivers.Tiles[idx]) {
			convertedLandToWater++
		}
	}

	if convertedLandToWater == 0 {
		t.Fatalf("expected default river pipeline to convert some land tiles to water")
	}
}
