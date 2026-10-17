package main

import (
	"context"
	"errors"
	_const "origin/internal/const"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"reflect"
	"testing"
)

func spotsBatchFixture() (*TerrainPrecompute, SpotsConfig) {
	config := validSpotsConfig()
	config.DistrictSizeTiles = int64(_const.ChunkSize)
	for index := range config.Spots {
		config.Spots[index].CenterTiles = []int{types.TileGrass}
	}
	districts := spotInsertBatchSize/spotTypeCount + 1
	terrain := &TerrainPrecompute{WidthTiles: districts * _const.ChunkSize, HeightTiles: 1}
	terrain.Tiles = make([]byte, terrain.WidthTiles)
	for index := range terrain.Tiles {
		terrain.Tiles[index] = types.TileGrass
	}
	return terrain, config
}

func TestWriteSpotBatchesBoundedAndComplete(t *testing.T) {
	terrain, config := spotsBatchFixture()
	var sizes []int
	seen := make(map[[3]int]bool)
	stats, err := writeSpotBatches(context.Background(), terrain, config, 42, 7, 321,
		func(_ context.Context, batch repository.InsertSpotsParams) error {
			count := len(batch.SpotTypes)
			sizes = append(sizes, count)
			if count > spotInsertBatchSize || count == 0 || batch.Region != 7 || batch.Layer != 0 || batch.LastRuntimeSeconds != 321 {
				t.Fatalf("unexpected batch metadata: %+v", batch)
			}
			for _, length := range []int{len(batch.DistrictXs), len(batch.DistrictYs), len(batch.CenterXs), len(batch.CenterYs), len(batch.Radii), len(batch.PeakQualities)} {
				if length != count {
					t.Fatal("batch column lengths differ")
				}
			}
			for index, spotType := range batch.SpotTypes {
				key := [3]int{batch.DistrictXs[index], batch.DistrictYs[index], spotTypeIndex(spotType)}
				if seen[key] {
					t.Fatal("a flushed batch was repeated")
				}
				seen[key] = true
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	districts := terrain.WidthTiles / int(config.DistrictSizeTiles)
	total := districts * spotTypeCount
	if !reflect.DeepEqual(sizes, []int{spotInsertBatchSize, total - spotInsertBatchSize}) || len(seen) != total {
		t.Fatalf("batch sizes %v, unique rows %d; want %d", sizes, len(seen), total)
	}
	for _, spotType := range spotTypes {
		if stats.Generated[spotType] != uint64(districts) || stats.Skipped[spotType] != 0 {
			t.Fatalf("incorrect counts: %+v", stats)
		}
	}
}

func TestWriteSpotBatchesStopsOnInsertFailure(t *testing.T) {
	terrain, config := spotsBatchFixture()
	failure := errors.New("insert rejected")
	for _, failAt := range []int{1, 2} {
		calls := 0
		_, err := writeSpotBatches(context.Background(), terrain, config, 42, 7, 321,
			func(context.Context, repository.InsertSpotsParams) error {
				calls++
				if calls == failAt {
					return failure
				}
				return nil
			})
		if !errors.Is(err, failure) || calls != failAt {
			t.Fatalf("failure at %d: calls %d, error %v", failAt, calls, err)
		}
	}
}

func TestWriteSpotBatchesEmptyAndCancelled(t *testing.T) {
	terrain, config := spotsBatchFixture()
	clear(terrain.Tiles)
	calls := 0
	insert := func(context.Context, repository.InsertSpotsParams) error { calls++; return nil }
	if _, err := writeSpotBatches(context.Background(), terrain, config, 42, 7, 321, insert); err != nil || calls != 0 {
		t.Fatalf("empty generation: calls %d, error %v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writeSpotBatches(ctx, terrain, config, 42, 7, 321, insert); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("cancelled generation: calls %d, error %v", calls, err)
	}
}

func TestMapgenSpotsConfigFlag(t *testing.T) {
	defaults, err := ParseMapgenOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.SpotsConfigPath != defaultSpotsConfigPath {
		t.Fatalf("unexpected default spots path %q", defaults.SpotsConfigPath)
	}
	override, err := ParseMapgenOptions([]string{"-spots-config", "custom/spots.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if override.SpotsConfigPath != "custom/spots.yaml" {
		t.Fatalf("flag was lost: %q", override.SpotsConfigPath)
	}
}
