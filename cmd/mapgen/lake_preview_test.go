package main

import (
	"bytes"
	"encoding/json"
	"image"
	"reflect"
	"strings"
	"testing"
)

func TestLakePreviewSettingsRoundTrip(test *testing.T) {
	opts, err := decodeMapgenOptions([]byte("version: 1\nriver:\n  lake_irregular_enabled: true\n  fairway_width_tiles: 3\n"), "partial.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	if err := opts.Validate(); err != nil {
		test.Fatal(err)
	}
	if opts.River.LakeIslandRadiusMin != 4 || opts.River.LakeIslandRadiusMax != 12 || opts.River.LakeShallowWidthMin != 1 || opts.River.LakeShallowWidthMax != 5 {
		test.Fatal("omitted lake fields did not inherit defaults")
	}
	schema := lakeDetailPreviewGroup(opts.River)
	if len(schema.Fields) != 12 {
		test.Fatal("incomplete lake preview controls")
	}
	fields := map[string]previewField{}
	for _, group := range riversLayerSchema(opts).Groups {
		for _, field := range group.Fields {
			fields[field.Key] = field
		}
	}
	values := reflect.ValueOf(opts.River)
	for _, field := range schema.Fields {
		if _, ok := fields[field.Key]; !ok || field.Description == "" || field.Label == field.Key {
			test.Fatalf("missing lake control or Russian help: %s", field.Key)
		}
		found := false
		for index := 0; index < values.NumField(); index++ {
			if values.Type().Field(index).Tag.Get("yaml") == field.Key {
				found = true
				if !reflect.DeepEqual(values.Field(index).Interface(), field.Default) {
					test.Fatalf("incorrect preset default %s", field.Key)
				}
			}
		}
		if !found {
			test.Fatalf("unbound preview control %s", field.Key)
		}
	}
	resolved, err := resolvePreviewOptions(renderRequest{Seed: 12345, Params: map[string]json.RawMessage{previewParamRivers: json.RawMessage(`{"lake_irregular_enabled":false,"lake_island_medium_chance":0,"lake_island_large_chance":0,"lake_shallow_width_min":0,"lake_shallow_width_max":0}`)}}, opts)
	if err != nil {
		test.Fatal(err)
	}
	encoded, err := encodePreviewPreset([]byte("version: 1 # preserve comment\nriver:\n  lake_island_medium_chance: 0.15 # probability comment\n"), resolved)
	if err != nil {
		test.Fatal(err)
	}
	reloaded, err := decodeMapgenOptions(encoded, "saved.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	if reloaded.River != resolved.River || reloaded.Biome != resolved.Biome || reloaded.Threads != resolved.Threads || reloaded.PerlinWaterEnabled != resolved.PerlinWaterEnabled {
		test.Fatal("preset round-trip lost settings")
	}
	if !bytes.Contains(encoded, []byte("# preserve comment")) || !bytes.Contains(encoded, []byte("# probability comment")) {
		test.Fatal("preset comments lost")
	}
	for _, raw := range []string{`{"lake_island_large_chance":1.1}`, `{"lake_island_radius_min":20,"lake_island_radius_max":4}`, `{"lake_shallow_width_min":6,"lake_shallow_width_max":5}`} {
		if _, err := resolvePreviewOptions(renderRequest{Params: map[string]json.RawMessage{previewParamRivers: json.RawMessage(raw)}}, opts); err == nil || !strings.Contains(err.Error(), "lake_") {
			test.Fatalf("invalid lake preview options accepted: %s %v", raw, err)
		}
	}
}

func TestLakePreviewMatchesFinalTerrain(test *testing.T) {
	for _, perlin := range []bool{false, true} {
		opts := lakeTestOptions(12345)
		opts.River.LakeIrregularEnabled = true
		opts.River.LakeIslandLargeChance = 1
		opts.PerlinWaterEnabled = perlin
		opts.Biome.Enabled = perlin
		terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(opts.Seed), 12, opts.TerrainScale))
		if err != nil {
			test.Fatal(err)
		}
		bounds := image.Rect(0, 0, terrain.WidthTiles, terrain.HeightTiles)
		fast, full := image.NewRGBA(bounds), image.NewRGBA(bounds)
		context := renderContext{WidthTiles: terrain.WidthTiles, HeightTiles: terrain.HeightTiles, Options: opts}
		if err := renderRiversLayer(fast, context); err != nil {
			test.Fatal(err)
		}
		context.Terrain = terrain
		if err := renderRiversLayer(full, context); err != nil {
			test.Fatal(err)
		}
		if !bytes.Equal(fast.Pix, full.Pix) {
			test.Fatalf("Perlin %v: fast lake preview differs from final terrain", perlin)
		}
	}
}
