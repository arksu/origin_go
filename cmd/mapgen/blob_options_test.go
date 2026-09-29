package main

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMapgenOptionsBlobDefaultsAndValidation(t *testing.T) {
	defaults := DefaultMapgenOptions()
	if err := defaults.Validate(); err != nil {
		t.Fatal(err)
	}
	if !defaults.Biome.BlobEnabled || defaults.Biome.BlobForestSizeMin != 45 || defaults.Biome.BlobForestSizeMax != 110 || defaults.Biome.BlobForestWidth != 0 {
		t.Fatal("unexpected shape defaults")
	}
	cases := []struct {
		key    string
		mutate func(*BiomeOptions)
	}{
		{"spacing", func(b *BiomeOptions) { b.BlobSeedSpacing = 0 }},
		{"spacing", func(b *BiomeOptions) { b.BlobSecondarySpacing = -1 }},
		{"spacing", func(b *BiomeOptions) { b.BlobIsletSpacing = 0 }},
		{"jitter", func(b *BiomeOptions) { b.BlobSeedJitter = 1 }},
		{"size", func(b *BiomeOptions) { b.BlobForestSizeMin = 0 }},
		{"size", func(b *BiomeOptions) { b.BlobForestSizeMax = 4097 }},
		{"size", func(b *BiomeOptions) { b.BlobClaySizeMin = 8 }},
		{"width", func(b *BiomeOptions) { b.BlobMoorWidth = -1 }},
		{"width", func(b *BiomeOptions) { b.BlobMoorWidth = 4097 }},
		{"weight", func(b *BiomeOptions) { b.BlobSkipWeight = -1 }},
		{"weight", func(b *BiomeOptions) { b.BlobForestWeight = math.MaxFloat64 }},
		{"density", func(b *BiomeOptions) { b.BlobThicketDensity = 1.01 }},
		{"density", func(b *BiomeOptions) { b.BlobDirtDensity = 0.6; b.BlobClayDensity = 0.5 }},
		{"threshold", func(b *BiomeOptions) { b.BlobForestColdThreshold = -0.1 }},
		{"raggedness", func(b *BiomeOptions) { b.BlobRaggedness = 65 }},
		{"max_nodes", func(b *BiomeOptions) { b.BlobMaxNodes = 3 }},
		{"max_nodes", func(b *BiomeOptions) { b.BlobMaxNodes = 257 }},
		{"max_depth", func(b *BiomeOptions) { b.BlobMaxDepth = 2 }},
		{"max_depth", func(b *BiomeOptions) { b.BlobMaxDepth = 33 }},
		{"mountain_massif_scale", func(b *BiomeOptions) { b.MountainMassifScale = 0 }},
		{"mountain_massif_scale", func(b *BiomeOptions) { b.MountainMassifScale = 4097 }},
		{"mountain_stone_scale", func(b *BiomeOptions) { b.MountainStoneScale = 0 }},
		{"mountain_stone_scale", func(b *BiomeOptions) { b.MountainStoneScale = b.MountainMassifScale + 1 }},
	}
	for _, tc := range cases {
		opts := defaults
		opts.Biome.Enabled = false
		opts.Biome.BlobEnabled = false
		tc.mutate(&opts.Biome)
		if err := opts.Validate(); err == nil || !strings.Contains(err.Error(), tc.key) {
			t.Errorf("%s: got %v", tc.key, err)
		}
	}
	for field := 0; field < reflect.TypeOf(defaults.Biome).NumField(); field++ {
		for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			opts := defaults
			value := reflect.ValueOf(&opts.Biome).Elem()
			if value.Field(field).Kind() != reflect.Float64 {
				continue
			}
			value.Field(field).SetFloat(invalid)
			key := value.Type().Field(field).Tag.Get("yaml")
			if err := opts.Validate(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("nonfinite %s: %v", key, err)
			}
		}
	}
	opts := defaults
	opts.Biome.BlobForestWeight = 0
	opts.Biome.BlobHeathWeight = 0
	opts.Biome.BlobMoorWeight = 0
	opts.Biome.BlobSwampWeight = 0
	opts.Biome.BlobSkipWeight = 0
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, dims := range [][2]int{{10000, 10000}, {math.MaxInt, 2}, {100, 100}} {
		opts := defaults
		opts.ChunksX = dims[0]
		opts.ChunksY = dims[1]
		if err := opts.Validate(); err == nil {
			t.Fatalf("accepted excessive allocation %v", dims)
		}
	}
	opts = defaults
	opts.ChunksX, opts.ChunksY = 70, 70
	opts.Biome.BlobSeedSpacing = 1
	if err := opts.Validate(); err == nil {
		t.Fatal("seed records omitted from memory budget")
	}
	opts = defaults
	opts.ChunksX = 90
	opts.ChunksY = 90
	if err := opts.Validate(); err != nil {
		t.Fatalf("baseline without oversized shape scratch: %v", err)
	}
	opts.Biome.BlobForestSizeMax = 4096
	opts.Biome.BlobMaxNodes = 256
	opts.Biome.BlobMaxDepth = 32
	if err := opts.Validate(); err == nil {
		t.Fatal("shape scratch omitted from memory budget")
	}
	if _, err := checkedAddUint64(math.MaxUint64, 1); err == nil {
		t.Fatal("addition overflow")
	}
}

func TestLoadMapgenOptionsBiomeOverlay(t *testing.T) {
	defaults := DefaultMapgenOptions()
	cases := []struct {
		yaml string
		want BiomeOptions
	}{
		{"", defaults.Biome}, {"biomes: null\n", defaults.Biome}, {"biomes: {}\n", defaults.Biome},
	}
	explicit := defaults.Biome
	explicit.BlobRaggedness = 0
	explicit.BlobForestWidth = 0
	explicit.BlobThicketDensity = 0
	explicit.Enabled = false
	explicit.BlobEnabled = false
	cases = append(cases, struct {
		yaml string
		want BiomeOptions
	}{"biomes:\n  blob_raggedness: 0\n  blob_forest_width: 0\n  blob_thicket_density: 0\n  enabled: false\n  blob_enabled: false\n", explicit})
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "preset.yaml")
		if err := os.WriteFile(path, []byte("version: 1\n"+tc.yaml), 0600); err != nil {
			t.Fatal(err)
		}
		got, _, err := LoadMapgenOptionsFromYAML(path, defaults)
		if err != nil {
			t.Fatal(err)
		}
		if got.Biome != tc.want {
			t.Fatalf("overlay %q differs: %+v", tc.yaml, got.Biome)
		}
		if got.River != defaults.River {
			t.Fatal("omitted river changed")
		}
	}
	for _, input := range []string{"version: 1\nbiomes:\n  blob_radius: 4\n", "biomes: {}\n"} {
		path := filepath.Join(t.TempDir(), "bad.yaml")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := LoadMapgenOptionsFromYAML(path, defaults)
		if err == nil {
			t.Fatal("invalid input accepted")
		}
		if strings.Contains(input, "blob_radius") && !strings.Contains(err.Error(), "blob_radius") {
			t.Fatal(err)
		}
	}
}
