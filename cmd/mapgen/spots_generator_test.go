package main

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"testing"

	_const "origin/internal/const"
)

func spotsGeneratorTestConfig() SpotsConfig {
	return SpotsConfig{
		Version: 1, DistrictSizeTiles: int64(_const.ChunkSize), RadiusTiles: int64(_const.ChunkSize / 2),
		BaseQuality: 10, PeakQualityMin: 20, PeakQualityMax: 50,
		Spots: []SpotTypeConfig{
			{Type: "www", CenterTiles: []int{int(tileForestPine), int(tileForestLeaf), int(tileGrass), int(tileSwamp)}},
			{Type: "clay", CenterTiles: []int{int(tileClay)}},
			{Type: "soil", CenterTiles: []int{int(tileGrass), int(tileDirt)}},
			{Type: "sand", CenterTiles: []int{int(tileSand)}},
			{Type: "water", CenterTiles: []int{int(tileForestPine), int(tileForestLeaf), int(tileGrass)}},
		},
	}
}

func collectGeneratedSpots(t *testing.T, terrain *TerrainPrecompute, config SpotsConfig, seed int64) ([]GeneratedSpot, SpotGenerationStats) {
	t.Helper()
	var spots []GeneratedSpot
	stats, err := GenerateSpots(context.Background(), terrain, config, seed, func(spot GeneratedSpot) error {
		spots = append(spots, spot)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return spots, stats
}

func TestGenerateSpotsUsesFinalEligibleTilesAndPreservesTerrain(t *testing.T) {
	config := spotsGeneratorTestConfig()
	terrain := &TerrainPrecompute{WidthTiles: 3, HeightTiles: 3, Tiles: []byte{
		tileWaterDeep, tileWater, tileClay,
		tileDirt, tileSand, tileGrass,
		tileForestPine, tileForestLeaf, tileSwamp,
	}}
	original := append([]byte(nil), terrain.Tiles...)
	spots, stats := collectGeneratedSpots(t, terrain, config, 12345)
	if len(spots) != spotTypeCount || stats.Districts != 1 {
		t.Fatalf("expected five spots in one district, got %v %v", spots, stats)
	}
	for _, spot := range spots {
		x, y := spot.CenterX/_const.CoordPerTile, spot.CenterY/_const.CoordPerTile
		tile := terrain.Tiles[y*terrain.WidthTiles+x]
		if tile == tileWater || tile == tileWaterDeep {
			t.Fatalf("%s center landed on water", spot.SpotType)
		}
		allowed := false
		for _, definition := range config.Spots {
			if definition.Type == spot.SpotType {
				for _, tileID := range definition.CenterTiles {
					allowed = allowed || int(tile) == tileID
				}
			}
		}
		if !allowed {
			t.Fatalf("%s center on unsupported tile %d", spot.SpotType, tile)
		}
		if spot.CenterX != x*_const.CoordPerTile+_const.CoordPerTile/2 ||
			spot.CenterY != y*_const.CoordPerTile+_const.CoordPerTile/2 ||
			spot.Radius != int(config.RadiusTiles)*_const.CoordPerTile {
			t.Fatalf("geometry did not use the world coordinate scale: %+v", spot)
		}
		if int64(spot.PeakQuality) < config.PeakQualityMin || int64(spot.PeakQuality) > config.PeakQualityMax {
			t.Fatalf("quality out of inclusive range: %+v", spot)
		}
		if stats.Generated[spot.SpotType] != 1 || stats.Skipped[spot.SpotType] != 0 {
			t.Fatalf("wrong counts for %s: %+v", spot.SpotType, stats)
		}
	}
	if !reflect.DeepEqual(original, terrain.Tiles) {
		t.Fatal("spot generation changed the terrain")
	}
}

func TestGenerateSpotsWaterCentersUseDryLand(t *testing.T) {
	for _, tile := range []byte{tileGrass, tileForestPine, tileForestLeaf, tileSwamp, tileWater, tileWaterDeep} {
		terrain := &TerrainPrecompute{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tile}}
		_, stats := collectGeneratedSpots(t, terrain, spotsGeneratorTestConfig(), 12345)
		want := uint64(0)
		if tile == tileGrass || tile == tileForestPine || tile == tileForestLeaf {
			want = 1
		}
		if stats.Generated["water"] != want || stats.Skipped["water"] != 1-want {
			t.Fatalf("water source on tile %d: got %+v, want %d", tile, stats, want)
		}
	}
}

func TestGenerateSpotsChoosesMinimumHashOverAllEligibleTiles(t *testing.T) {
	config := spotsGeneratorTestConfig()
	terrain := &TerrainPrecompute{WidthTiles: 11, HeightTiles: 7, Tiles: make([]byte, 11*7)}
	for index := range terrain.Tiles {
		terrain.Tiles[index] = tileGrass
	}
	const seed = int64(12345)
	spots, _ := collectGeneratedSpots(t, terrain, config, seed)
	for _, spot := range spots {
		x, y := spot.CenterX/_const.CoordPerTile, spot.CenterY/_const.CoordPerTile
		chosenHash := spotCoordinateHash(seed, x, y, spotTypeSalt(spot.SpotType)^spotCenterSalt)
		for tileY := 0; tileY < terrain.HeightTiles; tileY++ {
			for tileX := 0; tileX < terrain.WidthTiles; tileX++ {
				hash := spotCoordinateHash(seed, tileX, tileY, spotTypeSalt(spot.SpotType)^spotCenterSalt)
				if hash < chosenHash {
					t.Fatalf("%s did not select the minimum tile hash: (%d,%d) beats %+v", spot.SpotType, tileX, tileY, spot)
				}
			}
		}
	}
}

func TestGenerateSpotsSkipsDistrictsWithNoEligibleTiles(t *testing.T) {
	terrain := &TerrainPrecompute{WidthTiles: 3, HeightTiles: 1, Tiles: []byte{tileWater, tileWaterDeep, tileMountain}}
	spots, stats := collectGeneratedSpots(t, terrain, spotsGeneratorTestConfig(), 12345)
	if len(spots) != 0 || stats.Districts != 1 {
		t.Fatalf("unexpected output: %v %+v", spots, stats)
	}
	for _, spotType := range spotTypes {
		if stats.Generated[spotType] != 0 || stats.Skipped[spotType] != 1 {
			t.Fatalf("empty district did not skip %s: %+v", spotType, stats)
		}
	}
}

func TestGenerateSpotsHandlesPartialDistricts(t *testing.T) {
	config := spotsGeneratorTestConfig()
	width, height := _const.ChunkSize+1, _const.ChunkSize+1
	terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: height, Tiles: make([]byte, width*height)}
	for index := range terrain.Tiles {
		terrain.Tiles[index] = tileGrass
	}
	terrain.Tiles[len(terrain.Tiles)-1] = tileClay
	spots, stats := collectGeneratedSpots(t, terrain, config, 12345)
	if stats.Districts != 4 || stats.Generated["www"] != 3 || stats.Generated["soil"] != 3 ||
		stats.Generated["water"] != 3 || stats.Generated["clay"] != 1 || stats.Skipped["sand"] != 4 {
		t.Fatalf("wrong partial district counts: %+v", stats)
	}
	seen := make(map[[3]int]bool)
	for _, spot := range spots {
		x, y := spot.CenterX/_const.CoordPerTile, spot.CenterY/_const.CoordPerTile
		if x/int(config.DistrictSizeTiles) != spot.DistrictX || y/int(config.DistrictSizeTiles) != spot.DistrictY {
			t.Fatalf("center escaped its district: %+v", spot)
		}
		typeIndex := 0
		for index, spotType := range spotTypes {
			if spotType == spot.SpotType {
				typeIndex = index
			}
		}
		key := [3]int{spot.DistrictX, spot.DistrictY, typeIndex}
		if seen[key] {
			t.Fatalf("duplicate spot per type/district: %+v", spot)
		}
		seen[key] = true
		if spot.SpotType == "clay" && (x != width-1 || y != height-1 || spot.DistrictX != 1 || spot.DistrictY != 1) {
			t.Fatalf("lost the one-tile border district: %+v", spot)
		}
	}
}

func TestGenerateSpotsAllowsCirclesAcrossWaterDistrictsAndWorldEdges(t *testing.T) {
	config := spotsGeneratorTestConfig()
	config.RadiusTiles = config.DistrictSizeTiles
	width := _const.ChunkSize + 1
	terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: 1, Tiles: make([]byte, width)}
	for index := range terrain.Tiles {
		terrain.Tiles[index] = tileWaterDeep
	}
	terrain.Tiles[width-2], terrain.Tiles[width-1] = tileGrass, tileGrass
	spots, stats := collectGeneratedSpots(t, terrain, config, 12345)
	if len(spots) != 6 || stats.Districts != 2 {
		t.Fatalf("expected both edge centers without an inset: %v %+v", spots, stats)
	}
	for _, spot := range spots {
		if spot.CenterY-spot.Radius >= 0 || spot.CenterY+spot.Radius <= terrain.HeightTiles*_const.CoordPerTile {
			t.Fatalf("world edge unexpectedly constrained radius: %+v", spot)
		}
		if spot.CenterX-spot.Radius >= _const.ChunkWorldSize || spot.CenterX+spot.Radius <= _const.ChunkWorldSize {
			t.Fatalf("district edge unexpectedly constrained radius: %+v", spot)
		}
	}
	if spots[3].CenterX-spots[0].CenterX >= spots[3].Radius+spots[0].Radius {
		t.Fatal("neighboring circles should overlap")
	}
}

func TestGenerateSpotsDeterministicAcrossConfigOrderAndIndependentOfObjectRandomness(t *testing.T) {
	config := spotsGeneratorTestConfig()
	width, height := _const.ChunkSize+3, _const.ChunkSize+2
	terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: height, Tiles: make([]byte, width*height)}
	tileCycle := []byte{tileGrass, tileClay, tileDirt, tileSand, tileForestPine, tileForestLeaf, tileSwamp}
	for index := range terrain.Tiles {
		terrain.Tiles[index] = tileCycle[index%len(tileCycle)]
	}
	const seed = int64(12345)
	first, firstStats := collectGeneratedSpots(t, terrain, config, seed)
	reordered := config
	reordered.Spots = append([]SpotTypeConfig(nil), config.Spots...)
	for index := range reordered.Spots {
		definition := &reordered.Spots[index]
		definition.CenterTiles = append([]int(nil), definition.CenterTiles...)
		for left, right := 0, len(definition.CenterTiles)-1; left < right; left, right = left+1, right-1 {
			definition.CenterTiles[left], definition.CenterTiles[right] = definition.CenterTiles[right], definition.CenterTiles[left]
		}
	}
	for left, right := 0, len(reordered.Spots)-1; left < right; left, right = left+1, right-1 {
		reordered.Spots[left], reordered.Spots[right] = reordered.Spots[right], reordered.Spots[left]
	}
	second, secondStats := collectGeneratedSpots(t, terrain, reordered, seed)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstStats, secondStats) {
		t.Fatal("type or allow-list ordering changed generated spots")
	}
	different, _ := collectGeneratedSpots(t, terrain, config, seed+1)
	if reflect.DeepEqual(first, different) {
		t.Fatal("different seed produced the same centers and quality")
	}

	// Object scatter owns a local random source per chunk. Interleaving the spot
	// pass must neither touch that source nor change the chunk seed stream.
	rng := rand.New(rand.NewSource(deterministicChunkSeed(seed, 1, 2)))
	baseline := rand.New(rand.NewSource(deterministicChunkSeed(seed, 1, 2)))
	for range 10 {
		if rng.Int63() != baseline.Int63() {
			t.Fatal("object scatter source mismatch before spots")
		}
	}
	collectGeneratedSpots(t, terrain, config, seed)
	for range 10 {
		if rng.Int63() != baseline.Int63() {
			t.Fatal("spot generation changed object scatter random state")
		}
	}
}

func TestGenerateSpotsQualityRangeIncludesBothEndpoints(t *testing.T) {
	config := spotsGeneratorTestConfig()
	config.PeakQualityMin, config.PeakQualityMax = 20, 21
	terrain := &TerrainPrecompute{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tileGrass}}
	seen := map[int16]bool{}
	for seed := int64(1); seed <= 16; seed++ {
		spots, _ := collectGeneratedSpots(t, terrain, config, seed)
		for _, spot := range spots {
			if int64(spot.PeakQuality) < config.PeakQualityMin || int64(spot.PeakQuality) > config.PeakQualityMax {
				t.Fatalf("quality outside configured range: %+v", spot)
			}
			seen[spot.PeakQuality] = true
		}
	}
	if !seen[int16(config.PeakQualityMin)] || !seen[int16(config.PeakQualityMax)] {
		t.Fatalf("did not produce both inclusive endpoints: %v", seen)
	}
	config.PeakQualityMin, config.PeakQualityMax = 32767, 32767
	spots, _ := collectGeneratedSpots(t, terrain, config, 1)
	for _, spot := range spots {
		if spot.PeakQuality != 32767 {
			t.Fatalf("fixed upper quality was not preserved: %+v", spot)
		}
	}
}

func TestGenerateSpotsDeterministicAcrossTerrainWorkerCounts(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX, opts.ChunksY = 2, 2
	opts.Seed = 12345
	opts.River.Enabled = false
	opts.Biome.Enabled = false
	opts.Threads = 1
	build := func() *TerrainPrecompute {
		t.Helper()
		fields := NewNoiseFieldsWithTerrainScale(NewPerlinNoise(opts.Seed), _const.CoordPerTile, opts.TerrainScale)
		terrain, err := BuildTerrainPrecompute(opts, _const.ChunkSize, fields)
		if err != nil {
			t.Fatal(err)
		}
		return terrain
	}
	firstTerrain := build()
	opts.Threads = 4
	secondTerrain := build()
	first, firstStats := collectGeneratedSpots(t, firstTerrain, spotsGeneratorTestConfig(), opts.Seed)
	second, secondStats := collectGeneratedSpots(t, secondTerrain, spotsGeneratorTestConfig(), opts.Seed)
	if len(first) == 0 {
		t.Fatal("worker-count fixture must produce spots")
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstStats, secondStats) {
		t.Fatal("terrain worker count changed generated spots")
	}
}

func TestSpotCenterCandidateBreaksTiesByRowThenColumn(t *testing.T) {
	var candidate spotCenterCandidate
	candidate.consider(4, 3, 5)
	candidate.consider(8, 2, 5)
	candidate.consider(1, 2, 5)
	candidate.consider(0, 0, 6)
	if candidate.x != 1 || candidate.y != 2 || candidate.hash != 5 {
		t.Fatalf("wrong hash/tie selection: %+v", candidate)
	}
	candidate.consider(9, 9, 4)
	if candidate.x != 9 || candidate.y != 9 || candidate.hash != 4 {
		t.Fatalf("lower hash must beat coordinate order: %+v", candidate)
	}
}

func TestGenerateSpotsPropagatesCallbackErrorsAndCancellation(t *testing.T) {
	terrain := &TerrainPrecompute{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tileGrass}}
	config := spotsGeneratorTestConfig()
	outputErr := errors.New("output failed")
	emissions := 0
	stats, err := GenerateSpots(context.Background(), terrain, config, 1, func(GeneratedSpot) error {
		emissions++
		return outputErr
	})
	if !errors.Is(err, outputErr) || emissions != 1 || stats.Generated["www"] != 0 {
		t.Fatalf("callback failure not propagated: stats=%+v emissions=%d err=%v", stats, emissions, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	emissions = 0
	_, err = GenerateSpots(ctx, terrain, config, 1, func(GeneratedSpot) error {
		emissions++
		return nil
	})
	if !errors.Is(err, context.Canceled) || emissions != 0 {
		t.Fatalf("canceled generation emitted output: emissions=%d err=%v", emissions, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	emissions = 0
	_, err = GenerateSpots(ctx, terrain, config, 1, func(GeneratedSpot) error {
		emissions++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || emissions != 1 {
		t.Fatalf("generation continued after output cancellation: emissions=%d err=%v", emissions, err)
	}
	// Also catch cancellation by the final emitted type in the final district.
	terrain.Tiles[0] = tileSand
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	emissions = 0
	_, err = GenerateSpots(ctx, terrain, config, 1, func(GeneratedSpot) error {
		emissions++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || emissions != 1 {
		t.Fatalf("final output cancellation lost: emissions=%d err=%v", emissions, err)
	}
}

func TestGenerateSpotsRejectsMalformedTerrainBeforeOutput(t *testing.T) {
	for _, terrain := range []*TerrainPrecompute{
		nil,
		{WidthTiles: 0, HeightTiles: 1},
		{WidthTiles: 1, HeightTiles: 0},
		{WidthTiles: 1, HeightTiles: 1},
		{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tileGrass, tileGrass}},
	} {
		emitted := false
		_, err := GenerateSpots(context.Background(), terrain, spotsGeneratorTestConfig(), 1, func(GeneratedSpot) error {
			emitted = true
			return nil
		})
		if err == nil || emitted {
			t.Fatalf("invalid terrain accepted or emitted: %+v err=%v emitted=%v", terrain, err, emitted)
		}
	}
}

func TestGenerateSpotsAllocationsDoNotGrowWithDistrictCount(t *testing.T) {
	config := spotsGeneratorTestConfig()
	emit := func(GeneratedSpot) error { return nil }
	allocations := func(width, height int) float64 {
		terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: height, Tiles: make([]byte, width*height)}
		for index := range terrain.Tiles {
			terrain.Tiles[index] = tileGrass
		}
		return testing.AllocsPerRun(3, func() {
			if _, err := GenerateSpots(context.Background(), terrain, config, 1, emit); err != nil {
				t.Fatal(err)
			}
		})
	}
	small := allocations(1, 1)
	large := allocations(_const.ChunkSize*3+1, _const.ChunkSize*2+1)
	if large > small {
		t.Fatalf("allocation count grew with districts: small=%v large=%v", small, large)
	}
}
