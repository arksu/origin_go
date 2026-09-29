package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func previewSaveRequest(test *testing.T, server *previewServer, payload savePresetRequest) *httptest.ResponseRecorder {
	test.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8099/api/preset", previewRequestBody(test, payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:8099")
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, request)
	return recorder
}

func TestPreviewPresetLoadAndRenderIsolation(test *testing.T) {
	server := newTestPreviewServer(test)
	content, err := server.presets.ReadFile("test.yaml")
	if err != nil {
		test.Fatal(err)
	}
	opts := server.base
	opts.Seed, opts.ChunksX, opts.ChunksY = 731, 3, 4
	opts.River.ShapeAmplitudeScale = 1.7
	variant, err := encodePreviewPreset(content, opts)
	if err != nil {
		test.Fatal(err)
	}
	if err := server.presets.WriteFile("variant space.yaml", variant, 0o644); err != nil {
		test.Fatal(err)
	}
	for _, path := range []string{"variant space.yaml", filepath.Join(server.presets.Name(), "variant space.yaml")} {
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/defaults?preset="+url.QueryEscape(path), nil))
		if recorder.Code != http.StatusOK {
			test.Fatalf("load %s: %d %s", path, recorder.Code, recorder.Body)
		}
		var defaults defaultsResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &defaults); err != nil {
			test.Fatal(err)
		}
		expectedSchema, err := json.Marshal(previewLayerSchemas(opts))
		if err != nil {
			test.Fatal(err)
		}
		actualSchema, err := json.Marshal(defaults.Layers)
		if err != nil {
			test.Fatal(err)
		}
		if defaults.Preset != "variant space.yaml" || defaults.Seed != 731 || defaults.ChunksX != 3 || defaults.ChunksY != 4 || !bytes.Equal(actualSchema, expectedSchema) {
			test.Fatalf("loaded wrong preset: %+v", defaults)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			test.Fatal("preset responses can be cached")
		}
	}
	request := renderRequest{Preset: "variant space.yaml", Seed: 731, Layers: []string{"rivers"}}
	actual, err := server.buildPreviewPNG(request)
	if err != nil {
		test.Fatal(err)
	}
	expected, err := renderLayers(previewLayers, renderContext{Seed: 731, WidthTiles: 384, HeightTiles: 512, RiverOptions: opts.River})
	if err != nil || !bytes.Equal(actual, expected) {
		test.Fatalf("render ignored selected preset: %v", err)
	}
	if server.base.Seed != 42 || server.base.River.ShapeAmplitudeScale == 1.7 {
		test.Fatal("one request mutated another tab's defaults")
	}
	opts.Seed = 732
	variant, err = encodePreviewPreset(content, opts)
	if err != nil {
		test.Fatal(err)
	}
	if err := server.presets.WriteFile("variant space.yaml", variant, 0o644); err != nil {
		test.Fatal(err)
	}
	loaded, _, _, err := server.loadPreset("variant space.yaml")
	if err != nil || loaded.Seed != 732 {
		test.Fatalf("did not reload file: %+v %v", loaded, err)
	}
}

func TestPreviewPresetSaveRoundTrip(test *testing.T) {
	server := newTestPreviewServer(test)
	source, err := server.presets.ReadFile("test.yaml")
	if err != nil {
		test.Fatal(err)
	}
	source = bytes.Replace(source, []byte("version: 1"), []byte("version: 1 # keep this comment"), 1)
	source = bytes.Replace(source, []byte("shape_amplitude_scale: 1"), []byte("shape_amplitude_scale: 1 # shape comment"), 1)
	if err := server.presets.WriteFile("test.yaml", source, 0o644); err != nil {
		test.Fatal(err)
	}
	request := savePresetRequest{Path: "good variant.yaml", renderRequest: renderRequest{
		Preset: "test.yaml", Seed: 8123, ChunksX: 3, ChunksY: 4, Layers: []string{"rivers"},
		Params: map[string]json.RawMessage{previewParamRivers: json.RawMessage(`{"shape_amplitude_scale":1.8,"lake_border_mix":0}`)},
	}}
	recorder := previewSaveRequest(test, server, request)
	if recorder.Code != http.StatusOK {
		test.Fatalf("save: %d %s", recorder.Code, recorder.Body)
	}
	loaded, saved, _, err := server.loadPreset("good variant.yaml")
	if err != nil {
		test.Fatal(err)
	}
	if loaded.Seed != 8123 || loaded.ChunksX != 3 || loaded.ChunksY != 4 || loaded.River.ShapeAmplitudeScale != 1.8 || loaded.River.LakeBorderMix != 0 || !loaded.River.LayoutDraw {
		test.Fatalf("saved values differ: %+v", loaded)
	}
	if loaded.Threads != server.base.Threads || loaded.River.SourceChance != server.base.River.SourceChance || loaded.Biome != server.base.Biome || loaded.Ecology != server.base.Ecology || loaded.PNG != server.base.PNG {
		test.Fatal("save changed settings outside the preview")
	}
	if !bytes.Contains(saved, []byte("# keep this comment")) || !bytes.Contains(saved, []byte("# shape comment")) {
		test.Fatal("save lost comments")
	}
	unchanged, err := server.presets.ReadFile("test.yaml")
	if err != nil || !bytes.Equal(unchanged, source) {
		test.Fatal("saving a variant changed the source")
	}
	request.Preset, request.Path, request.Seed = "good variant.yaml", "good variant.yaml", 99
	recorder = previewSaveRequest(test, server, request)
	if recorder.Code != http.StatusOK {
		test.Fatalf("overwrite: %d %s", recorder.Code, recorder.Body)
	}
	loaded, _, _, err = server.loadPreset("good variant.yaml")
	if err != nil || loaded.Seed != 99 {
		test.Fatalf("overwrite was not persisted: %v", err)
	}
	request.Params[previewParamRivers] = json.RawMessage(`{"shape_octaves":99}`)
	recorder = previewSaveRequest(test, server, request)
	if recorder.Code != http.StatusBadRequest {
		test.Fatalf("invalid save: %d %s", recorder.Code, recorder.Body)
	}
	loaded, _, _, err = server.loadPreset("good variant.yaml")
	if err != nil || loaded.Seed != 99 || loaded.River.ShapeOctaves == 99 {
		test.Fatal("invalid save damaged existing file")
	}
	entries, err := os.ReadDir(server.presets.Name())
	if err != nil || len(entries) != 2 {
		test.Fatalf("temporary files left behind: %v %v", entries, err)
	}
}

func TestPreviewPresetMissingSections(test *testing.T) {
	server := newTestPreviewServer(test)
	if err := server.presets.WriteFile("minimal.yml", []byte("version: 1\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	recorder := previewSaveRequest(test, server, savePresetRequest{Path: "minimal.yml", renderRequest: renderRequest{Preset: "minimal.yml", Seed: 91, ChunksX: 3, ChunksY: 4}})
	if recorder.Code != http.StatusOK {
		test.Fatalf("minimal preset: %d %s", recorder.Code, recorder.Body)
	}
	loaded, _, _, err := server.loadPreset("minimal.yml")
	if err != nil || loaded.River != DefaultMapgenOptions().River || loaded.Seed != 91 || loaded.ChunksY != 4 {
		test.Fatalf("missing defaults lost on save: %v", err)
	}
}

func TestPreviewPresetPathsAndWriteProtection(test *testing.T) {
	server := newTestPreviewServer(test)
	outside := filepath.Join(test.TempDir(), "outside.yaml")
	if err := os.WriteFile(outside, []byte("version: 1\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	if err := server.presets.Symlink(outside, "escape.yaml"); err != nil {
		test.Fatal(err)
	}
	for _, path := range []string{"../outside.yaml", outside, "escape.yaml", "missing.yaml", "test.txt"} {
		if _, _, _, err := server.loadPreset(path); err == nil {
			test.Errorf("loaded forbidden/missing path %q", path)
		}
	}
	for _, path := range []string{"../outside.yaml", outside, "escape.yaml", "test.txt", ""} {
		recorder := previewSaveRequest(test, server, savePresetRequest{Path: path, renderRequest: renderRequest{Preset: "test.yaml", Seed: 9}})
		if recorder.Code == http.StatusOK {
			test.Errorf("saved forbidden path %q", path)
		}
	}
	unchanged, err := os.ReadFile(outside)
	if err != nil || string(unchanged) != "version: 1\n" {
		test.Fatal("modified file outside preset directory")
	}
	for _, scenario := range []struct{ host, origin, mediaType, fetchSite string }{
		{"127.0.0.1:8099", "https://attacker.invalid", "application/json", ""},
		{"attacker.invalid", "http://attacker.invalid", "application/json", ""},
		{"127.0.0.1:8099", "", "text/plain", ""},
		{"127.0.0.1:8099", "", "application/json", "cross-site"},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://"+scenario.host+"/api/preset", strings.NewReader(`{}`))
		request.Header.Set("Origin", scenario.origin)
		request.Header.Set("Content-Type", scenario.mediaType)
		request.Header.Set("Sec-Fetch-Site", scenario.fetchSite)
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			test.Fatalf("unsafe write accepted: %+v: %d", scenario, recorder.Code)
		}
	}
}

func TestPreviewPresetInvalidFilesAndBodies(test *testing.T) {
	server := newTestPreviewServer(test)
	for _, content := range []string{"version: 2\n", "version: 1\nunknown: true\n", "version: 1\n---\nversion: 1\n"} {
		if err := server.presets.WriteFile("invalid.yaml", []byte(content), 0o644); err != nil {
			test.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/defaults?preset=invalid.yaml", nil))
		if recorder.Code != http.StatusBadRequest {
			test.Fatalf("invalid file accepted: %q", content)
		}
	}
	for _, body := range []string{`{"unexpected":true}`, `{} {}`, strings.Repeat(" ", previewMaxPresetBytes) + `{}`} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8099/api/preset", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			test.Fatalf("invalid body accepted: %d", recorder.Code)
		}
	}
}
