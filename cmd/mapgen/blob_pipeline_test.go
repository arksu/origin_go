package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
	_const "origin/internal/const"
)

func blobBuildTestTerrain(t *testing.T, opts MapgenOptions) *TerrainPrecompute {
	t.Helper()
	terrain, err := BuildTerrainPrecompute(opts, _const.ChunkSize, NewNoiseFields(NewPerlinNoise(opts.Seed), _const.CoordPerTile))
	if err != nil {
		t.Fatal(err)
	}
	return terrain
}

func TestBlobDeterminismFinalBufferAcrossRunsAndWorkers(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 3
	opts.ChunksY = 3
	opts.Seed = 12345
	opts.Threads = 1
	opts.Biome.BlobSeedSpacing = 60
	opts.Biome.BlobSecondarySpacing = 20
	first := blobBuildTestTerrain(t, opts)
	repeated := blobBuildTestTerrain(t, opts)
	opts.Threads = 4
	parallel := blobBuildTestTerrain(t, opts)
	if !bytes.Equal(first.Tiles, repeated.Tiles) || !bytes.Equal(first.Tiles, parallel.Tiles) {
		t.Fatal("terrain depends on run or worker count")
	}
	if first.MainPatches == 0 || first.SecondaryPatches == 0 || first.Islets == 0 {
		t.Fatalf("determinism did not exercise all passes: %d/%d/%d", first.MainPatches, first.SecondaryPatches, first.Islets)
	}
}

func TestBlobPipelineOmittedAndExplicitDefaults(t *testing.T) {
	defaults := DefaultMapgenOptions()
	defaults.ChunksX = 2
	defaults.ChunksY = 2
	defaults.Seed = 12345
	defaults.Threads = 1
	explicit, err := yaml.Marshal(map[string]any{"version": 1, "biomes": defaults.Biome})
	if err != nil {
		t.Fatal(err)
	}
	var terrains []*TerrainPrecompute
	for _, content := range [][]byte{[]byte("version: 1\n"), explicit} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		opts, _, err := LoadMapgenOptionsFromYAML(path, defaults)
		if err != nil {
			t.Fatal(err)
		}
		terrains = append(terrains, blobBuildTestTerrain(t, opts))
	}
	if !bytes.Equal(terrains[0].Tiles, terrains[1].Tiles) {
		t.Fatal("explicit defaults change terrain")
	}
}

func TestBlobPipelineDisabledModesAndProtectedTerrain(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 2
	opts.ChunksY = 2
	opts.Seed = 67890
	opts.Threads = 2
	opts.Biome.MountainRuggedThreshold = 0.38
	for _, master := range []bool{false, true} {
		opts.Biome.Enabled = master
		opts.Biome.BlobEnabled = false
		terrain := blobBuildTestTerrain(t, opts)
		if terrain.MainPatches+terrain.SecondaryPatches+terrain.Islets != 0 || terrain.Timings.Cleanup != 0 {
			t.Fatal("disabled passes ran")
		}
		fields := NewNoiseFields(NewPerlinNoise(opts.Seed), _const.CoordPerTile)
		ground := make([]byte, len(terrain.Tiles))
		expected := make([]byte, len(terrain.Tiles))
		for i := range ground {
			column, row := i%terrain.WidthTiles, i/terrain.WidthTiles
			signals := fields.BiomeSignals(column, row, opts.Biome)
			elevation := float64(terrain.Elevation[i])
			if master {
				ground[i] = classifyBiomeGround(elevation, signals, opts.Biome, opts.Seed, column, row)
			} else {
				ground[i] = classifyBaseTile(elevation, signals.Moisture, signals.Temperature)
			}
			expected[i] = resolveTileType(elevation, ground[i], terrain.RiverClass[i], true)
		}
		applyShorelineSand(expected, ground, terrain.RiverClass, terrain.Elevation, terrain.WidthTiles, terrain.HeightTiles, opts.Seed)
		if !bytes.Equal(expected, terrain.Tiles) {
			t.Fatal("disabled mode changed ground/hydrology")
		}
		if master {
			opts.Biome.BlobEnabled = true
			opts.Biome.BlobSeedSpacing = 50
			active := blobBuildTestTerrain(t, opts)
			locked := buildBiomeStructuralMask(ground, terrain.Elevation, terrain.RiverClass, true)
			mountainCount, waterCount := 0, 0
			for i, isLocked := range locked {
				if !isLocked {
					continue
				}
				resolved := resolveTileType(float64(terrain.Elevation[i]), ground[i], terrain.RiverClass[i], true)
				if resolved == tileWater || resolved == tileWaterDeep {
					waterCount++
					if active.Tiles[i] != resolved {
						t.Fatal("hydrology overwritten")
					}
				}
				if resolved == tileMountain || resolved == tileStone {
					mountainCount++
					if active.Tiles[i] != resolved {
						t.Fatal("structural terrain overwritten")
					}
				}
			}
			if mountainCount == 0 || waterCount == 0 {
				t.Fatal("fixture missed structural terrain")
			}
		}
	}
}

func TestBlobPipelineFinalSmallSecondarySurvives(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 2
	opts.ChunksY = 2
	opts.Seed = 12345
	opts.River.Enabled = false
	b := &opts.Biome
	b.BlobForestWeight = 0
	b.BlobHeathWeight = 0
	b.BlobMoorWeight = 0
	b.BlobSwampWeight = 0
	b.BlobSkipWeight = 1
	b.BlobSecondarySpacing = 24
	b.BlobDirtDensity = 1
	b.BlobClayDensity = 0
	b.BlobDirtSizeMin = 1
	b.BlobDirtSizeMax = 1
	b.BlobDirtWidth = 0.1
	b.BlobRaggedness = 0
	b.MinPatchTiles = 24
	b.SmoothingPasses = 3
	terrain := blobBuildTestTerrain(t, opts)
	smallComponents := 0
	for _, component := range blobTestComponents(terrain.Tiles, terrain.WidthTiles, terrain.HeightTiles) {
		if component.tile == tileDirt && len(component.indices) < 24 {
			smallComponents++
		}
	}
	if smallComponents == 0 {
		t.Fatal("small secondary detail removed in final pipeline")
	}
}

type blobTestComponent struct {
	tile    byte
	indices []int
}

func blobTestComponents(tiles []byte, width, height int) []blobTestComponent {
	visited := make([]bool, len(tiles))
	var components []blobTestComponent
	for index, tile := range tiles {
		if visited[index] {
			continue
		}
		visited[index] = true
		queue := []int{index}
		for head := 0; head < len(queue); head++ {
			current := queue[head]
			column, row := current%width, current/width
			for _, direction := range blobDirections {
				neighborColumn, neighborRow := column+direction.X, row+direction.Y
				if neighborColumn < 0 || neighborRow < 0 || neighborColumn >= width || neighborRow >= height {
					continue
				}
				neighbor := neighborRow*width + neighborColumn
				if !visited[neighbor] && tiles[neighbor] == tile {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
		components = append(components, blobTestComponent{tile: tile, indices: queue})
	}
	return components
}

func TestBlobPipelineFinalSatellitesSurvive(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX = 3
	opts.ChunksY = 3
	opts.Seed = 12345
	opts.Biome.BlobSeedSpacing = 60
	opts.Biome.MinPatchTiles = 24
	opts.Biome.BlobIsletChance = 0
	without := blobBuildTestTerrain(t, opts)
	opts.Biome.BlobIsletChance = 1
	with := blobBuildTestTerrain(t, opts)
	changed := 0
	smallSurvivors := 0
	for i, tile := range with.Tiles {
		if tile != without.Tiles[i] {
			changed++
		}
	}
	for _, component := range blobTestComponents(with.Tiles, with.WidthTiles, with.HeightTiles) {
		if len(component.indices) > 15 || blobPriority(component.tile) == 0 {
			continue
		}
		for _, index := range component.indices {
			if with.Tiles[index] != without.Tiles[index] {
				smallSurvivors++
				break
			}
		}
	}
	if changed == 0 || smallSurvivors == 0 {
		t.Fatal("accepted satellites did not survive final pipeline")
	}
}
