package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_const "origin/internal/const"
)

func TestBiomeGroundRulesAndFallback(t *testing.T) {
	opts := DefaultMapgenOptions().Biome
	signals := BiomeSignals{Moisture: 0.5, Temperature: 0.5, Continentalness: 0.5, Ruggedness: opts.MountainRuggedThreshold - 0.0001, Wetness: 1}
	if classifyBiomeGround(0.7, signals, opts, true, 1, 0, 0) != tileGrass {
		t.Fatal("nonstructural land must be grass")
	}
	signals.Moisture = 0.21
	signals.Temperature = 0.63
	if classifyBiomeGround(0.7, signals, opts, true, 1, 0, 0) != tileSand {
		t.Fatal("climate sand missing")
	}
	signals.Moisture = 0.22
	if classifyBiomeGround(0.7, signals, opts, true, 1, 0, 0) != tileGrass {
		t.Fatal("sand equality")
	}
	signals.Ruggedness = opts.MountainRuggedThreshold
	counts := map[byte]int{}
	for row := 0; row < 256; row++ {
		for column := 0; column < 256; column++ {
			counts[classifyBiomeGround(0.7, signals, opts, true, 1, column, row)]++
		}
	}
	if counts[tileMountain] == 0 || counts[tileStone] == 0 || len(counts) != 2 {
		t.Fatal(counts)
	}
	opts.Enabled = false
	for _, elevation := range []float64{0.2, 0.3, 0.4, 0.7} {
		if got := classifyBiomeGround(elevation, signals, opts, true, 1, 0, 0); got != classifyBaseTile(elevation, signals.Moisture, signals.Temperature, true) {
			t.Fatal("fallback changed")
		}
	}
}

func TestBiomeGroundMountainMassifsAndStoneCoherence(t *testing.T) {
	opts := DefaultMapgenOptions().Biome
	opts.MountainRuggedThreshold = 0.72
	const width, height = 2048, 2048
	fields := NewNoiseFields(NewPerlinNoise(12345), _const.CoordPerTile)
	regions := make([]byte, width*height)
	for row := 0; row < height; row++ {
		for column := 0; column < width; column++ {
			signals := fields.BiomeSignals(column, row, opts)
			tile := classifyBiomeGround(0.7, signals, opts, true, 12345, column, row)
			if tile == tileMountain || tile == tileStone {
				regions[row*width+column] = 1
			}
		}
	}
	largest := 0
	for _, component := range blobTestComponents(regions, width, height) {
		if component.tile == 1 {
			largest = max(largest, len(component.indices))
		}
	}
	if largest < 50000 {
		t.Fatalf("mountains fragmented before hydrology: largest massif has %d tiles", largest)
	}

	// A stone outcrop's interior should remain continuous, not alternate at each tile.
	signals := BiomeSignals{Moisture: 0.5, Ruggedness: 1}
	transitions, stoneTiles := 0, 0
	for row := 0; row < 256; row++ {
		previous := byte(0)
		for column := 0; column < 256; column++ {
			tile := classifyBiomeGround(0.7, signals, opts, true, 12345, column, row)
			if tile == tileStone {
				stoneTiles++
			}
			if column > 0 && tile != previous {
				transitions++
			}
			previous = tile
		}
	}
	if stoneTiles == 0 || transitions >= 256*255/20 {
		t.Fatalf("stone outcrops lack coherence: %d stone tiles, %d neighbor transitions", stoneTiles, transitions)
	}
}

func TestLoadMapgenOptionsPresetsAndRetiredFields(t *testing.T) {
	for _, name := range []string{"default", "hnh", "minecraft_like"} {
		opts, _, err := LoadMapgenOptionsFromYAML("etc/mapgen/presets/"+name+".yaml", DefaultMapgenOptions())
		if err != nil {
			t.Fatal(err)
		}
		if err := opts.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !opts.Biome.BlobEnabled {
			t.Fatalf("%s has no new layer", name)
		}
	}
	for _, key := range []string{"biomes/hnh_enabled", "biomes/hnh_region_count", "biomes/hnh_region_jitter", "biomes/hnh_blend_width", "biomes/hnh_variant_density", "biomes/hnh_forest_share", "biomes/hnh_grassland_share", "biomes/hnh_wetland_share", "biomes/hnh_heath_moor_share", "biomes/hnh_mountain_share", "river/shape_noise_scale"} {
		section := strings.SplitN(key, "/", 2)
		body := "version: 1\n" + section[0] + ":\n  " + section[1] + ": 1\n"
		path := filepath.Join(t.TempDir(), "retired.yaml")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := LoadMapgenOptionsFromYAML(path, DefaultMapgenOptions())
		if err == nil || !strings.Contains(err.Error(), section[1]) {
			t.Fatalf("%s: %v", key, err)
		}
	}
}
