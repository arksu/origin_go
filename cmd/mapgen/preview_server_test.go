package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func newTestPreviewServer(t *testing.T) *previewServer {
	t.Helper()
	opts := DefaultMapgenOptions()
	opts.Seed = 42
	// Small enough for every link to carry water on a 512-tile test world.
	opts.River.LakeCount = 30
	opts.River.MajorRiverCount = 8
	opts.River.LakeConnectionLimit = 10
	opts.River.LakeSizeSmallMin = 8
	opts.River.LakeSizeSmallMax = 16
	opts.River.LakeSizeMediumMin = 20
	opts.River.LakeSizeMediumMax = 40
	opts.River.LakeSizeLargeMin = 48
	opts.River.LakeSizeLargeMax = 80
	content, err := yaml.Marshal(mapgenConfigFile{Version: 1,
		World: &worldConfig{ChunksX: opts.ChunksX, ChunksY: opts.ChunksY, Seed: opts.Seed, Threads: opts.Threads, TerrainScale: opts.TerrainScale, PerlinWaterEnabled: opts.PerlinWaterEnabled},
		River: &opts.River, Biomes: &opts.Biome, Ecology: &opts.Ecology, PNG: &opts.PNG})
	if err != nil {
		t.Fatal(err)
	}
	opts.ConfigPath = filepath.Join(t.TempDir(), "test.yaml")
	if err := os.WriteFile(opts.ConfigPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	server, err := newPreviewServer(zap.NewNop(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.presets.Close() })
	return server
}

func previewRequestBody(t *testing.T, body any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return bytes.NewReader(raw)
}

func TestRenderLayersRiversPNG(t *testing.T) {
	s := newTestPreviewServer(t)
	request := renderRequest{Seed: 42, ChunksX: 4, ChunksY: 4, Layers: []string{"rivers"}}

	first, err := s.buildPreviewPNG(request)
	if err != nil {
		t.Fatalf("render layers: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if got := decoded.Bounds(); got.Dx() != 512 || got.Dy() != 512 {
		t.Fatalf("unexpected png size: %dx%d", got.Dx(), got.Dy())
	}

	allowed := map[[4]byte]bool{
		{0xf2, 0xef, 0xe6, 0xff}: true,
		{0x6f, 0xd0, 0xff, 0xff}: true,
		{0x0f, 0x7f, 0xd4, 0xff}: true,
	}
	seen := map[[4]byte]bool{}
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			r, g, b, a := decoded.At(x, y).RGBA()
			key := [4]byte{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
			if !allowed[key] {
				t.Fatalf("unexpected color %v at (%d,%d)", key, x, y)
			}
			seen[key] = true
		}
	}
	if !seen[[4]byte{0x6f, 0xd0, 0xff, 0xff}] || !seen[[4]byte{0x0f, 0x7f, 0xd4, 0xff}] {
		t.Fatal("expected both shallow and deep river colors in output")
	}

	second, err := s.buildPreviewPNG(request)
	if err != nil {
		t.Fatalf("re-render layers: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical requests must produce byte-identical pngs")
	}
}

func TestPreviewRenderHandler(t *testing.T) {
	s := newTestPreviewServer(t)

	valid := previewRequestBody(t, map[string]any{
		"seed":     42,
		"chunks_x": 4,
		"chunks_y": 4,
		"layers":   []string{"rivers"},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/render", valid)
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid request: got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type: got %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty png body")
	}

	unknownLayer := previewRequestBody(t, map[string]any{
		"seed": 42, "chunks_x": 4, "chunks_y": 4, "layers": []string{"nope"},
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/render", unknownLayer)
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown layer") {
		t.Fatalf("unknown layer: got %d: %s", rec.Code, rec.Body.String())
	}

	unknownField := previewRequestBody(t, map[string]any{
		"seed": 42, "chunks_x": 4, "chunks_y": 4, "layers": []string{"rivers"},
		"params": map[string]any{"river": map[string]any{"shape_unknown_field": 1}},
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/render", unknownField)
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "shape_unknown_field") {
		t.Fatalf("unknown river field: got %d: %s", rec.Code, rec.Body.String())
	}

	outOfRange := previewRequestBody(t, map[string]any{
		"seed": 42, "chunks_x": 4, "chunks_y": 4, "layers": []string{"rivers"},
		"params": map[string]any{"river": map[string]any{"shape_octaves": 99}},
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/render", outOfRange)
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "shape_octaves") {
		t.Fatalf("out-of-range param: got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPreviewDefaultsHandler(t *testing.T) {
	s := newTestPreviewServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/defaults", nil)
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("defaults: got %d", rec.Code)
	}

	var payload struct {
		Seed   int64 `json:"seed"`
		Layers []struct {
			Name   string `json:"name"`
			Groups []struct {
				Fields []struct {
					Key     string   `json:"key"`
					Max     *float64 `json:"max"`
					Default float64  `json:"default"`
				} `json:"fields"`
			} `json:"groups"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode defaults: %v", err)
	}
	if payload.Seed != 42 {
		t.Fatalf("seed default: got %d", payload.Seed)
	}
	if len(payload.Layers) != 1 || payload.Layers[0].Name != "rivers" {
		t.Fatalf("expected exactly the rivers layer, got %+v", payload.Layers)
	}
	found := false
	majorCountMaximum := float64(0)
	for _, group := range payload.Layers[0].Groups {
		for _, field := range group.Fields {
			if field.Key == "shape_waves_per_link" && field.Default > 0 {
				found = true
			}
			if field.Key == "major_count" && field.Max != nil {
				majorCountMaximum = *field.Max
			}
		}
	}
	if !found {
		t.Fatal("defaults must expose shape_waves_per_link with a positive default")
	}
	if majorCountMaximum != previewMaxMajorRiverCount {
		t.Fatalf("major_count maximum: got %v want %d", majorCountMaximum, previewMaxMajorRiverCount)
	}
}
