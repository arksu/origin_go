package main

import "testing"

func isWaterTile(tile byte) bool {
	return tile == tileWater || tile == tileWaterDeep
}

func TestResolveTileTypeOceanPrecedenceOverRiver(t *testing.T) {
	got := resolveTileType(0.30, 0.9, 0.9, riverDeep, true)
	if got != tileWater {
		t.Fatalf("expected shallow ocean water to win over river override, got tile=%d", got)
	}
}

func TestResolveTileTypeRiverOverridesLand(t *testing.T) {
	gotDeep := resolveTileType(0.60, 0.2, 0.2, riverDeep, true)
	if gotDeep != tileWaterDeep {
		t.Fatalf("expected deep river override on land, got tile=%d", gotDeep)
	}

	gotShallow := resolveTileType(0.60, 0.2, 0.2, riverShallow, true)
	if gotShallow != tileWater {
		t.Fatalf("expected shallow river override on land, got tile=%d", gotShallow)
	}
}

func TestResolveTileTypeMatchesBiomeWhenNoRiver(t *testing.T) {
	got := resolveTileType(0.70, 0.7, 0.7, riverNone, false)
	if got != tileForestLeaf {
		t.Fatalf("expected forest leaf biome tile, got tile=%d", got)
	}

	got = resolveTileType(0.70, 0.7, 0.4, riverNone, false)
	if got != tileForestPine {
		t.Fatalf("expected forest pine biome tile, got tile=%d", got)
	}

	got = resolveTileType(0.70, 0.2, 0.4, riverNone, false)
	if got != tileGrass {
		t.Fatalf("expected grass biome tile, got tile=%d", got)
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
