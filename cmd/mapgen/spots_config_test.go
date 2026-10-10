package main

import (
	"math"
	_const "origin/internal/const"
	"origin/internal/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validSpotsConfig() SpotsConfig {
	return SpotsConfig{
		Version:           1,
		DistrictSizeTiles: int64(_const.ChunkSize) * 4,
		RadiusTiles:       int64(_const.ChunkSize) / 2,
		BaseQuality:       10,
		PeakQualityMin:    20,
		PeakQualityMax:    50,
		Spots: []SpotTypeConfig{
			{Type: "www", CenterTiles: []int{types.TileConiferousForest, types.TileBroadleafForest, types.TileGrass, types.TileSwamp1}},
			{Type: "clay", CenterTiles: []int{types.TileClay}},
			{Type: "soil", CenterTiles: []int{types.TileGrass, types.TileDirt}},
			{Type: "sand", CenterTiles: []int{types.TileSand}},
			{Type: "water", CenterTiles: []int{types.TileConiferousForest, types.TileBroadleafForest, types.TileGrass}},
		},
	}
}

func TestLoadSpotsConfig(t *testing.T) {
	config, resolved, err := LoadSpotsConfig(defaultSpotsConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("resolved path %q is not absolute", resolved)
	}
	if config.DistrictSizeTiles != 512 || config.RadiusTiles != 64 || config.BaseQuality != 10 ||
		config.PeakQualityMin != 20 || config.PeakQualityMax != 50 {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	if len(config.Spots) != spotTypeCount {
		t.Fatalf("got %d definitions", len(config.Spots))
	}
	if err := config.ValidateForWorld(_const.ChunkSize, _const.ChunkSize); err != nil {
		t.Fatal(err)
	}
	for _, definition := range config.Spots {
		if definition.Type == "water" {
			for _, tile := range definition.CenterTiles {
				if tile != types.TileGrass && tile != types.TileConiferousForest && tile != types.TileBroadleafForest {
					t.Fatalf("default water center tile %d is not grass or forest", tile)
				}
			}
		}
	}
}

func TestLoadSpotsConfigRejectsMissingFile(t *testing.T) {
	if _, _, err := LoadSpotsConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected missing-file error")
	}
}

func TestDecodeSpotsConfigStrictYAML(t *testing.T) {
	configYAML, err := yaml.Marshal(validSpotsConfig())
	if err != nil {
		t.Fatal(err)
	}
	valid := string(configYAML)
	cases := map[string]string{
		"empty":                 "",
		"unknown field":         valid + "unexpected: true\n",
		"unknown nested field":  strings.Replace(valid, "type: www", "type: www\n      unexpected: true", 1),
		"second document":       valid + "---\nversion: 1\n",
		"empty second document": valid + "---\n",
		"duplicate key":         valid + "version: 1\n",
		"fractional quality":    strings.Replace(valid, "base_quality: 10", "base_quality: 10.9", 1),
		"fractional radius":     strings.Replace(valid, "radius_tiles:", "radius_tiles: 1.5 #", 1),
		"fractional tile":       strings.Replace(valid, "- 35", "- 35.1", 1),
		"malformed":             "version: [\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeSpotsConfig([]byte(input)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "spots.yaml")
	if err := os.WriteFile(path, configYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadSpotsConfig(path); err != nil {
		t.Fatal(err)
	}
}

func TestSpotsConfigValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SpotsConfig)
	}{
		{"version", func(c *SpotsConfig) { c.Version = 2 }},
		{"zero district", func(c *SpotsConfig) { c.DistrictSizeTiles = 0 }},
		{"negative district", func(c *SpotsConfig) { c.DistrictSizeTiles = -int64(_const.ChunkSize) }},
		{"nonchunk district", func(c *SpotsConfig) { c.DistrictSizeTiles++ }},
		{"zero radius", func(c *SpotsConfig) { c.RadiusTiles = 0 }},
		{"negative radius", func(c *SpotsConfig) { c.RadiusTiles = -1 }},
		{"radius exceeds district", func(c *SpotsConfig) { c.RadiusTiles = c.DistrictSizeTiles + 1 }},
		{"base below floor", func(c *SpotsConfig) { c.BaseQuality = 9 }},
		{"base exceeds min", func(c *SpotsConfig) { c.BaseQuality = c.PeakQualityMin + 1 }},
		{"min exceeds max", func(c *SpotsConfig) { c.PeakQualityMin = c.PeakQualityMax + 1 }},
		{"peak exceeds smallint", func(c *SpotsConfig) { c.PeakQualityMax = math.MaxInt16 + 1 }},
		{"missing type", func(c *SpotsConfig) { c.Spots = c.Spots[:len(c.Spots)-1] }},
		{"duplicate type", func(c *SpotsConfig) { c.Spots[4].Type = c.Spots[0].Type }},
		{"unknown type", func(c *SpotsConfig) { c.Spots[4].Type = "stone" }},
		{"empty tiles", func(c *SpotsConfig) { c.Spots[0].CenterTiles = nil }},
		{"duplicate tile", func(c *SpotsConfig) { c.Spots[0].CenterTiles = []int{types.TileGrass, types.TileGrass} }},
		{"unknown tile", func(c *SpotsConfig) { c.Spots[0].CenterTiles = []int{2} }},
		{"negative tile", func(c *SpotsConfig) { c.Spots[0].CenterTiles = []int{-1} }},
		{"deep water", func(c *SpotsConfig) { c.Spots[0].CenterTiles = []int{types.TileDeepWater} }},
		{"shallow water", func(c *SpotsConfig) { c.Spots[4].CenterTiles = []int{types.TileShallowWater} }},
		{"district postgres overflow", func(c *SpotsConfig) {
			c.DistrictSizeTiles = (int64(math.MaxInt32)/int64(_const.CoordPerTile)/int64(_const.ChunkSize) + 1) * int64(_const.ChunkSize)
		}},
		{"district int64 overflow", func(c *SpotsConfig) {
			c.DistrictSizeTiles = math.MaxInt64 / int64(_const.ChunkSize) * int64(_const.ChunkSize)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := validSpotsConfig()
			tc.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	config := validSpotsConfig()
	config.RadiusTiles = config.DistrictSizeTiles
	config.BaseQuality = math.MaxInt16
	config.PeakQualityMin = math.MaxInt16
	config.PeakQualityMax = math.MaxInt16
	if err := config.Validate(); err != nil {
		t.Fatalf("inclusive limits: %v", err)
	}
}

func TestSpotWorldConversions(t *testing.T) {
	tileSize := int64(_const.CoordPerTile)
	for _, tile := range []int64{-int64(_const.ChunkSize) - 1, -1, 0, 1, int64(_const.ChunkSize)} {
		center, err := spotTileCenter(tile)
		if err != nil {
			t.Fatal(err)
		}
		want := tile*tileSize + tileSize/2
		if int64(center) != want || spotWorldToTile(int64(center)) != tile {
			t.Fatalf("tile %d: center %d want %d, inverse %d", tile, center, want, spotWorldToTile(int64(center)))
		}
	}
	for _, tc := range []struct{ world, want int64 }{
		{-tileSize - 1, -2}, {-tileSize, -1}, {-1, -1}, {0, 0}, {tileSize - 1, 0}, {tileSize, 1},
	} {
		if got := spotWorldToTile(tc.world); got != tc.want {
			t.Fatalf("world %d: tile %d want %d", tc.world, got, tc.want)
		}
	}
	config := validSpotsConfig()
	radius, err := config.RadiusWorld()
	if err != nil || int64(radius) != config.RadiusTiles*tileSize {
		t.Fatalf("radius %d error %v", radius, err)
	}
	district, err := config.DistrictWorldSize()
	if err != nil || int64(district) != config.DistrictSizeTiles*tileSize {
		t.Fatalf("district %d error %v", district, err)
	}
	for _, world := range []int64{math.MinInt32, math.MaxInt32} {
		if got, err := spotPostgresCoordinate(world); err != nil || int64(got) != world {
			t.Fatalf("integer boundary %d: got %d err %v", world, got, err)
		}
	}
	for _, tile := range []int64{math.MinInt64, math.MaxInt64, int64(math.MaxInt32)/tileSize + 1, (int64(math.MinInt32)-tileSize/2)/tileSize - 1} {
		if _, err := spotTileCenter(tile); err == nil {
			t.Fatalf("expected coordinate overflow for tile %d", tile)
		}
	}
	if _, err := spotTileLengthWorld(0); err == nil {
		t.Fatal("expected invalid tile length")
	}
}

func TestSpotsConfigWorldPreflight(t *testing.T) {
	config := validSpotsConfig()
	tileSize := int64(_const.CoordPerTile)
	maxTiles := int((int64(math.MaxInt32)-tileSize/2)/tileSize + 1)
	if err := config.ValidateForWorld(maxTiles, maxTiles); err != nil {
		t.Fatalf("last representable center: %v", err)
	}
	for _, dims := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {maxTiles + 1, 1}, {1, maxTiles + 1}} {
		if err := config.ValidateForWorld(dims[0], dims[1]); err == nil {
			t.Fatalf("expected invalid dimensions %v", dims)
		}
	}
	config.Version++
	if err := config.ValidateForWorld(1, 1); err == nil {
		t.Fatal("world preflight must include config validation")
	}
}
