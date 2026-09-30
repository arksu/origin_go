package main

import (
	"bytes"
	"encoding/json"
	"image"
	"reflect"
	"strings"
	"testing"
)

func TestRiverJunctionPreviewRoundTrip(test *testing.T) {
	opts := junctionGoldenOptions(12345)
	opts.River.JunctionChance = 0.25
	fields := junctionPreviewGroup(opts.River).Fields
	if len(fields) != 2 {
		test.Fatal("missing controls")
	}
	for _, field := range fields {
		found := false
		for _, group := range riversLayerSchema(opts).Groups {
			for _, control := range group.Fields {
				if control.Key == field.Key {
					found = reflect.DeepEqual(field, control)
				}
			}
		}
		if !found || field.Description == "" || field.Label == field.Key {
			test.Fatalf("missing Russian control %s", field.Key)
		}
	}
	for _, chance := range []string{"0", "1"} {
		resolved, err := resolvePreviewOptions(renderRequest{Seed: opts.Seed, Params: map[string]json.RawMessage{previewParamRivers: json.RawMessage(`{"junction_chance":` + chance + `}`)}}, opts)
		if err != nil {
			test.Fatal(err)
		}
		encoded, err := encodePreviewPreset([]byte("version: 1 # keep me\nriver:\n  junction_chance: 0.25 # Russian probability\n"), resolved)
		if err != nil {
			test.Fatal(err)
		}
		reloaded, err := decodeMapgenOptions(encoded, "junction.yaml", DefaultMapgenOptions())
		if err != nil {
			test.Fatal(err)
		}
		if reloaded.River != resolved.River || reloaded.Biome != resolved.Biome || reloaded.PerlinWaterEnabled != resolved.PerlinWaterEnabled || !bytes.Contains(encoded, []byte("# keep me")) || !bytes.Contains(encoded, []byte("# Russian probability")) {
			test.Fatal("round trip lost settings/comments")
		}
		if resolved.River.JunctionSpacingTiles != 180 {
			test.Fatal("partial request discarded spacing")
		}
	}
	for _, raw := range []string{`{"junction_chance":-0.01}`, `{"junction_chance":1.01}`, `{"junction_spacing_tiles":0}`, `{"junction_spacing_tiles":1.5}`, `{"junction_spacing_tiles":8193}`} {
		if _, err := resolvePreviewOptions(renderRequest{Params: map[string]json.RawMessage{previewParamRivers: json.RawMessage(raw)}}, opts); err == nil || !strings.Contains(err.Error(), "junction") {
			test.Fatalf("invalid junction accepted: %s %v", raw, err)
		}
	}
	opts.River.MajorRiverCount = int(^uint(0) >> 1)
	if err := validatePreviewMemory(opts); err == nil {
		test.Fatal("preview overflow accepted")
	}
	opts.River.MajorRiverCount = 2000
	opts.ChunksX, opts.ChunksY = 100, 100
	if err := validatePreviewMemory(opts); err == nil {
		test.Fatal("preview overbudget accepted")
	}
}

func TestRiverJunctionPreviewParityAndDeterminism(test *testing.T) {
	for _, chance := range []float64{0, 1} {
		for _, perlin := range []bool{false, true} {
			opts := junctionGoldenOptions(67890)
			opts.ChunksX, opts.ChunksY = 8, 8
			opts.River.MajorRiverCount, opts.River.LakeConnectionLimit = 4, 40
			opts.River.LakeConnectChance = 1
			opts.River.JunctionChance = chance
			opts.River.JunctionSpacingTiles = 60
			opts.River.LakeSizeLargeMin, opts.River.LakeSizeLargeMax = 60, 80
			opts.River.LakeSizeMediumMin, opts.River.LakeSizeMediumMax = 30, 45
			opts.PerlinWaterEnabled, opts.Biome.Enabled = perlin, perlin
			if perlin {
				opts.River.ShallowWidthMin, opts.River.ShallowWidthMax = 3, 3
				opts.River.FairwayWidthTiles = 5
			}
			terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(opts.Seed), 12, opts.TerrainScale))
			if err != nil {
				test.Fatal(err)
			}
			if chance > 0 {
				test.Logf("Perlin=%v width=%d: %v", perlin, opts.River.FairwayWidthTiles, riverRouteStats(terrain.RiverFairways))
				if len(terrain.RiverFairways.Junctions.Accepted) == 0 {
					test.Fatal("positive parity fixture must exercise accepted junctions")
				}
				plan := terrain.RiverFairways
				if plan.MainCount > opts.River.MajorRiverCount+opts.River.LakeConnectionLimit {
					test.Fatal("main budget exceeded")
				}
				degrees := make([]int, len(plan.Lakes))
				for routeID, origins := range plan.Junctions.Origins {
					if plan.Routes[routeID].Role != "main" {
						test.Fatal("nonordinary parent registered")
					}
					for _, lakeID := range origins {
						if lakeID >= 0 {
							degrees[lakeID]++
						}
					}
				}
				for _, junction := range plan.Junctions.Accepted {
					degrees[junction.Source]++
				}
				for _, degree := range degrees {
					if degree > opts.River.MaxLakeDegree {
						test.Fatal("lake degree exceeded")
					}
				}
				if plan.Junctions.Grid != nil {
					test.Fatal("generation index retained after generation")
				}
				junction := plan.Junctions.Accepted[0]
				entrance := plan.Routes[junction.Route].Path[0]
				original := terrain.Tiles[entrance]
				terrain.Tiles[entrance] = tileWater
				validationErr := plan.validate(terrain.WidthTiles, terrain.HeightTiles, opts.River.FairwayWidthTiles, opts.Seed, func(index int) bool { return terrain.Tiles[index] == tileWaterDeep })
				terrain.Tiles[entrance] = original
				if validationErr == nil || !strings.Contains(validationErr.Error(), "junction branch") {
					test.Fatalf("final source entrance corruption: %v", validationErr)
				}
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
				test.Fatalf("fast/full parity chance %g Perlin %v", chance, perlin)
			}
			opts.Threads = 4
			repeated, err := BuildTerrainPrecompute(opts, 128, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(opts.Seed), 12, opts.TerrainScale))
			if err != nil {
				test.Fatal(err)
			}
			if !bytes.Equal(terrain.Tiles, repeated.Tiles) || !reflect.DeepEqual(terrain.RiverClass, repeated.RiverClass) || !reflect.DeepEqual(terrain.RiverFairways, repeated.RiverFairways) {
				test.Fatal("seed/worker determinism failed")
			}
		}
	}
}
