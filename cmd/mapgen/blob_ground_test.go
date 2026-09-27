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
	if classifyBiomeGround(0.7, signals, opts, 1, 0, 0) != tileGrass {
		t.Fatal("nonstructural land must be grass")
	}
	signals.Moisture = 0.21
	signals.Temperature = 0.63
	if classifyBiomeGround(0.7, signals, opts, 1, 0, 0) != tileSand {
		t.Fatal("climate sand missing")
	}
	signals.Moisture = 0.22
	if classifyBiomeGround(0.7, signals, opts, 1, 0, 0) != tileGrass {
		t.Fatal("sand equality")
	}
	signals.Ruggedness = opts.MountainRuggedThreshold
	counts := map[byte]int{}
	for row := 0; row < 256; row++ {
		for column := 0; column < 256; column++ {
			counts[classifyBiomeGround(0.7, signals, opts, 1, column, row)]++
		}
	}
	if counts[tileMountain] == 0 || counts[tileStone] == 0 || len(counts) != 2 {
		t.Fatal(counts)
	}
	opts.Enabled = false
	for _, elevation := range []float64{0.2, 0.3, 0.4, 0.7} {
		if got := classifyBiomeGround(elevation, signals, opts, 1, 0, 0); got != classifyBaseTile(elevation, signals.Moisture, signals.Temperature) {
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
			tile := classifyBiomeGround(0.7, signals, opts, 12345, column, row)
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
			tile := classifyBiomeGround(0.7, signals, opts, 12345, column, row)
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
	for _, key := range []string{"hnh_enabled", "hnh_region_count", "hnh_region_jitter", "hnh_blend_width", "hnh_variant_density", "hnh_forest_share", "hnh_grassland_share", "hnh_wetland_share", "hnh_heath_moor_share", "hnh_mountain_share"} {
		path := filepath.Join(t.TempDir(), "retired.yaml")
		if err := os.WriteFile(path, []byte("version: 1\nbiomes:\n  "+key+": 1\n"), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := LoadMapgenOptionsFromYAML(path, DefaultMapgenOptions())
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("%s: %v", key, err)
		}
	}
}
