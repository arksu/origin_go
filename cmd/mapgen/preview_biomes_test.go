package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	_const "origin/internal/const"
)

func TestPreviewBiomeSchemaCoversEverySetting(test *testing.T) {
	options := DefaultMapgenOptions()
	options.Biome.TemperatureScale = 12.3456
	options.Biome.BlobForestWeight = 120
	options.Biome.Enabled = false
	options.Biome.BlobForestWidth = 0
	fields := make(map[string]previewField)
	for _, group := range biomesLayerSchema(options).Groups {
		for _, field := range group.Fields {
			if _, exists := fields[field.Key]; exists {
				test.Fatalf("duplicate biome control %s", field.Key)
			}
			if field.Description == "" {
				test.Fatalf("missing explanation for %s", field.Key)
			}
			fields[field.Key] = field
		}
	}
	values := reflect.ValueOf(options.Biome)
	if len(fields) != values.NumField() {
		test.Fatalf("preview has %d controls for %d settings", len(fields), values.NumField())
	}
	for index := 0; index < values.NumField(); index++ {
		key := values.Type().Field(index).Tag.Get("yaml")
		field, exists := fields[key]
		if !exists || !reflect.DeepEqual(field.Default, values.Field(index).Interface()) {
			test.Fatalf("missing or incorrect preset value for %s: %+v", key, field)
		}
		expectedType := map[reflect.Kind]string{reflect.Bool: "bool", reflect.Int: "int", reflect.Float64: "float"}[values.Field(index).Kind()]
		if field.Type != expectedType {
			test.Fatalf("control type for %s: got %s want %s", key, field.Type, expectedType)
		}
	}
	for _, key := range []string{"erosion_scale", "weirdness_scale"} {
		if !strings.Contains(fields[key].Description, "не влияет") {
			test.Fatalf("unused climate field %s has no explanation", key)
		}
	}
}

func TestPreviewBiomeOverridesAndValidation(test *testing.T) {
	base := DefaultMapgenOptions()
	base.River.LayoutDraw = false
	request := renderRequest{Seed: 42, ChunksX: 3, ChunksY: 2, Params: map[string]json.RawMessage{
		"biomes": json.RawMessage(`{"enabled":false,"blob_enabled":false,"blob_raggedness":0,"temperature_scale":12.3456}`),
		"world":  json.RawMessage(`{"terrain_scale":0.00123,"perlin_water_enabled":false}`),
	}}
	resolved, err := resolvePreviewOptions(request, base)
	if err != nil {
		test.Fatal(err)
	}
	if resolved.Biome.Enabled || resolved.Biome.BlobEnabled || resolved.Biome.BlobRaggedness != 0 || resolved.Biome.TemperatureScale != 12.3456 || resolved.PerlinWaterEnabled || resolved.TerrainScale != 0.00123 {
		test.Fatalf("explicit false, zero, or precise values lost: %+v", resolved)
	}
	if resolved.Biome.BlobSeedSpacing != base.Biome.BlobSeedSpacing || resolved.River.LayoutDraw || !base.Biome.Enabled || !base.PerlinWaterEnabled {
		test.Fatal("partial override changed omitted settings, layout mode, or server defaults")
	}
	server := newTestPreviewServer(test)
	for _, testCase := range []struct{ section, value, message string }{
		{"biomes", `{"missing_field":1}`, "missing_field"},
		{"biomes", `{"blob_seed_jitter":1}`, "blob_seed_jitter"},
		{"biomes", `{"blob_dirt_density":0.8,"blob_clay_density":0.8}`, "blob_dirt_density"},
		{"biomes", `{"blob_forest_size_min":100,"blob_forest_size_max":20}`, "size_min/max"},
		{"biomes", `{"mountain_massif_scale":10,"mountain_stone_scale":20}`, "mountain_stone_scale"},
		{"biomes", `{"blob_max_depth":33}`, "blob_max_depth"},
		{"biomes", `{"enabled":false,"temperature_scale":0}`, "temperature_scale"},
		{"biomes", `null`, "must be an object"},
		{"world", `{"terrain_scale":0}`, "terrain_scale"},
		{"world", `{"threads":1}`, "threads"},
		{"world", `[]`, "must be an object"},
	} {
		test.Run(testCase.section+testCase.value, func(test *testing.T) {
			request.Params = map[string]json.RawMessage{testCase.section: json.RawMessage(testCase.value)}
			request.Layers = []string{"biomes"}
			recorder := httptest.NewRecorder()
			server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/render", previewRequestBody(test, request)))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), testCase.message) {
				test.Fatalf("got %d %s, expected %s", recorder.Code, recorder.Body, testCase.message)
			}
		})
	}
}

func TestPreviewBiomesMatchGeneratorAndLayerVisibility(test *testing.T) {
	server := newTestPreviewServer(test)
	server.base.Biome.BlobSeedSpacing = 60
	server.base.Biome.BlobSecondarySpacing = 20
	server.base.River.ShapeWavelengthTiles = 160
	server.base.River.FairwayWidthTiles = 3
	for _, testCase := range []struct {
		name                           string
		enabled, blobs, perlin, rivers bool
	}{
		{"full", true, true, true, true},
		{"without-perlin", true, true, false, true},
		{"signal-ground", true, false, true, true},
		{"fallback", false, true, true, true},
		{"without-rivers", true, true, false, false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			request := renderRequest{Seed: 42, ChunksX: 4, ChunksY: 3, Layers: []string{"biomes", "rivers"}}
			server.base.Biome.Enabled, server.base.Biome.BlobEnabled = testCase.enabled, testCase.blobs
			server.base.PerlinWaterEnabled, server.base.River.Enabled = testCase.perlin, testCase.rivers
			options, err := resolvePreviewOptions(request, server.base)
			if err != nil {
				test.Fatal(err)
			}
			terrain, err := BuildTerrainPrecompute(options, _const.ChunkSize, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(options.Seed), _const.CoordPerTile, options.TerrainScale))
			if err != nil {
				test.Fatal(err)
			}
			combinedPNG, err := server.buildPreviewPNG(request)
			if err != nil {
				test.Fatal(err)
			}
			combined := decodePreviewImage(test, combinedPNG)
			request.Layers = []string{"biomes"}
			biomesPNG, err := server.buildPreviewPNG(request)
			if err != nil {
				test.Fatal(err)
			}
			biomes := decodePreviewImage(test, biomesPNG)
			request.Layers = []string{"rivers"}
			riversPNG, err := server.buildPreviewPNG(request)
			if err != nil {
				test.Fatal(err)
			}
			rivers := decodePreviewImage(test, riversPNG)
			landCount, protectedCount := 0, 0
			for index, tile := range terrain.Tiles {
				protected := terrain.RiverFairways != nil && terrain.RiverFairways.Protected[index]
				riverTile := protected || (len(terrain.RiverClass) > 0 && terrain.RiverClass[index] != riverNone)
				expected := tileColor(tile, riverNone, false)
				if protected {
					protectedCount++
					if tile != tileWaterDeep {
						test.Fatal("fairway lost deep water")
					}
				}
				if riverTile {
					expected = previewShallowColor
					if tile == tileWaterDeep {
						expected = previewDeepColor
					}
				} else {
					landCount++
				}
				column, row := index%terrain.WidthTiles, index/terrain.WidthTiles
				if got := color.RGBAModel.Convert(combined.At(column, row)); got != expected {
					test.Fatalf("preview differs from generator at %d: %v != %v", index, got, expected)
				}
				expectedBiomes, expectedRivers := expected, previewBackgroundColor
				if riverTile {
					expectedBiomes, expectedRivers = previewBackgroundColor, expected
				}
				if color.RGBAModel.Convert(biomes.At(column, row)) != expectedBiomes || color.RGBAModel.Convert(rivers.At(column, row)) != expectedRivers {
					test.Fatalf("layer visibility changes terrain at %d", index)
				}
			}
			if landCount == 0 || (testCase.rivers && protectedCount == 0) {
				test.Fatalf("fixture missed land or fairway: %d / %d", landCount, protectedCount)
			}
			request.Layers = []string{"rivers", "biomes", "rivers"}
			reordered, err := server.buildPreviewPNG(request)
			if err != nil || !bytes.Equal(combinedPNG, reordered) {
				test.Fatalf("render is not deterministic in canonical layer order: %v", err)
			}
		})
	}
}

func decodePreviewImage(test *testing.T, encoded []byte) image.Image {
	test.Helper()
	decoded, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		test.Fatal(err)
	}
	return decoded
}

func TestPreviewSavesHiddenBiomeAndWorldSettings(test *testing.T) {
	server := newTestPreviewServer(test)
	source, err := server.presets.ReadFile("test.yaml")
	if err != nil {
		test.Fatal(err)
	}
	source = bytes.Replace(source, []byte("blob_enabled: true"), []byte("blob_enabled: true # сохранить комментарий"), 1)
	if err := server.presets.WriteFile("test.yaml", source, 0o644); err != nil {
		test.Fatal(err)
	}
	request := savePresetRequest{Path: "biomes.yaml", renderRequest: renderRequest{
		Preset: "test.yaml", Seed: 12345, ChunksX: 4, ChunksY: 3, Layers: []string{"rivers"},
		Params: map[string]json.RawMessage{
			"biomes": json.RawMessage(`{"blob_enabled":false,"blob_islet_chance":0,"blob_raggedness":0,"blob_forest_width":0,"hnh_smoothing_passes":0,"temperature_scale":12.3456}`),
			"world":  json.RawMessage(`{"terrain_scale":0.00123,"perlin_water_enabled":false}`),
		},
	}}
	expected, err := resolvePreviewOptions(request.renderRequest, server.base)
	if err != nil {
		test.Fatal(err)
	}
	recorder := previewSaveRequest(test, server, request)
	if recorder.Code != http.StatusOK {
		test.Fatalf("save: %d %s", recorder.Code, recorder.Body)
	}
	loaded, saved, _, err := server.loadPreset("biomes.yaml")
	if err != nil {
		test.Fatal(err)
	}
	if loaded.Biome != expected.Biome || loaded.TerrainScale != expected.TerrainScale || loaded.PerlinWaterEnabled != expected.PerlinWaterEnabled || loaded.River != expected.River || loaded.Ecology != expected.Ecology || loaded.PNG != expected.PNG {
		test.Fatalf("saved settings differ: %+v", loaded)
	}
	if !bytes.Contains(saved, []byte("# сохранить комментарий")) {
		test.Fatal("biome comment lost")
	}
	unchanged, err := server.presets.ReadFile("test.yaml")
	if err != nil || !bytes.Equal(source, unchanged) {
		test.Fatal("saving changed the source preset")
	}
	recorder = httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/defaults?preset=biomes.yaml", nil))
	expectedDefaults := defaultsResponse{World: worldPreviewSchema(loaded), Layers: previewLayerSchemas(loaded)}
	var actual defaultsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &actual); err != nil {
		test.Fatal(err)
	}
	expectedJSON, _ := json.Marshal([]any{expectedDefaults.World, expectedDefaults.Layers})
	actualJSON, _ := json.Marshal([]any{actual.World, actual.Layers})
	if recorder.Code != http.StatusOK || !bytes.Equal(expectedJSON, actualJSON) {
		test.Fatal("reloaded panel does not match saved biome and world settings")
	}
}

func TestPreviewMemoryIncludesImageBuffers(test *testing.T) {
	options := DefaultMapgenOptions()
	options.ChunksX, options.ChunksY = 64, 64
	options.River.MajorRiverCount = 550
	options.River.FairwayWidthTiles = 3
	if err := options.Validate(); err != nil {
		test.Fatalf("fixture must fit the terrain budget before image buffers: %v", err)
	}
	if err := validatePreviewMemory(options); err == nil || !strings.Contains(err.Error(), "PNG buffers") {
		test.Fatalf("preview did not reserve image memory: %v", err)
	}
	options.ChunksX, options.ChunksY = 4, 3
	if err := validatePreviewMemory(options); err != nil {
		test.Fatalf("small preview rejected: %v", err)
	}
}
