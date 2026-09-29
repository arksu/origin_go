package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"sync"
	"time"

	_const "origin/internal/const"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed preview/index.html
var previewIndexHTML []byte

const (
	previewMaxChunks   = 64
	previewParamRivers = "river"
)

var (
	previewBackgroundColor = color.RGBA{R: 0xf2, G: 0xef, B: 0xe6, A: 0xff}
	previewShallowColor    = color.RGBA{R: 0x6f, G: 0xd0, B: 0xff, A: 0xff}
	previewDeepColor       = color.RGBA{R: 0x0f, G: 0x7f, B: 0xd4, A: 0xff}
)

type previewField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // "float" | "int" | "bool"
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Step    *float64 `json:"step,omitempty"`
	Default any      `json:"default"`
}

type previewGroup struct {
	Title  string         `json:"title"`
	Fields []previewField `json:"fields"`
}

type previewLayerSchema struct {
	Name     string         `json:"name"`
	Title    string         `json:"title"`
	ParamKey string         `json:"param_key"`
	Groups   []previewGroup `json:"groups"`
}

type previewSizeOption struct {
	Chunks int `json:"chunks"`
	Tiles  int `json:"tiles"`
}

type defaultsResponse struct {
	Seed   int64                `json:"seed"`
	Sizes  []previewSizeOption  `json:"sizes"`
	Layers []previewLayerSchema `json:"layers"`
}

type renderRequest struct {
	Seed    int64                      `json:"seed"`
	ChunksX int                        `json:"chunks_x"`
	ChunksY int                        `json:"chunks_y"`
	Layers  []string                   `json:"layers"`
	Params  map[string]json.RawMessage `json:"params"`
}

// renderContext carries everything a layer renderer needs; new layers read
// their own options section instead of growing this struct.
type renderContext struct {
	Seed         int64
	WidthTiles   int
	HeightTiles  int
	RiverOptions RiverOptions
}

type layerRenderer struct {
	name string
	// schema builds this layer's parameter panel description; it receives the
	// server's base options so field defaults follow the loaded preset.
	schema func(base MapgenOptions) previewLayerSchema
	render func(img *image.RGBA, ctx renderContext) error
}

type previewServer struct {
	logger *zap.Logger
	base   MapgenOptions
	mux    *http.ServeMux

	// Serializes buildPreviewPNG; see the comment there.
	mu sync.Mutex
}

func newPreviewServer(logger *zap.Logger, base MapgenOptions) *previewServer {
	s := &previewServer{logger: logger, base: base}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/defaults", s.handleDefaults)
	mux.HandleFunc("POST /api/render", s.handleRender)
	s.mux = mux
	return s
}

func runRiverPreviewServer(logger *zap.Logger, opts MapgenOptions) {
	s := newPreviewServer(logger, opts)
	addr := fmt.Sprintf("127.0.0.1:%d", opts.PreviewPort)
	logger.Info("starting layer preview server", zap.String("address", addr))
	server := &http.Server{Addr: addr, Handler: s.mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		logger.Fatal("layer preview server failed", zap.Error(err))
	}
}

func (s *previewServer) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(previewIndexHTML)
}

func (s *previewServer) handleDefaults(w http.ResponseWriter, _ *http.Request) {
	response := defaultsResponse{
		Seed:   s.base.Seed,
		Sizes:  previewWorldSizes(),
		Layers: previewLayerSchemas(s.base),
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *previewServer) handleRender(w http.ResponseWriter, r *http.Request) {
	var req renderRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	started := time.Now()
	pngBytes, err := s.buildPreviewPNG(req)
	if err != nil {
		var renderErr renderError
		if errors.As(err, &renderErr) {
			s.logger.Error("layer render failed", zap.Error(err))
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("render failed: %v", err))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("preview render",
		zap.Int64("seed", req.Seed),
		zap.Int("chunks_x", req.ChunksX),
		zap.Int("chunks_y", req.ChunksY),
		zap.Strings("layers", req.Layers),
		zap.Duration("elapsed", time.Since(started)),
		zap.Int("png_bytes", len(pngBytes)),
	)
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(pngBytes)
}

// renderError marks failures of the render pipeline itself, as opposed to
// invalid client input.
type renderError struct{ err error }

func (e renderError) Error() string { return e.err.Error() }
func (e renderError) Unwrap() error { return e.err }

// buildPreviewPNG validates the request against the preset defaults and
// renders the requested layers. It is the single path shared by the HTTP
// handler and tests.
func (s *previewServer) buildPreviewPNG(req renderRequest) ([]byte, error) {
	riverOpts, err := s.resolveRiverOptions(req)
	if err != nil {
		return nil, err
	}
	requested, err := resolveRequestedLayers(req.Layers)
	if err != nil {
		return nil, err
	}
	chunksX, chunksY, err := resolveChunkDimensions(req, s.base)
	if err != nil {
		return nil, err
	}

	// Renders allocate hundreds of MB and are CPU-bound, so they are
	// serialized; the UI drops stale responses by sequence number.
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := renderContext{
		Seed:         req.Seed,
		WidthTiles:   chunksX * _const.ChunkSize,
		HeightTiles:  chunksY * _const.ChunkSize,
		RiverOptions: riverOpts,
	}
	pngBytes, err := renderLayers(requested, ctx)
	if err != nil {
		return nil, renderError{err}
	}
	return pngBytes, nil
}

// resolveRiverOptions starts from the preset defaults and strictly overlays
// the partial params the client sent for each layer.
func (s *previewServer) resolveRiverOptions(req renderRequest) (RiverOptions, error) {
	river := s.base.River
	raw, ok := req.Params[previewParamRivers]
	if !ok || len(raw) == 0 {
		river.LayoutDraw = true
		return river, nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&river); err != nil {
		return RiverOptions{}, fmt.Errorf("river params: %w", err)
	}
	// The elevation-routed layout needs a real elevation field, which the
	// rivers-only preview does not build.
	river.LayoutDraw = true

	opts := s.base
	opts.River = river
	opts.ChunksX = req.ChunksX
	opts.ChunksY = req.ChunksY
	if err := opts.Validate(); err != nil {
		return RiverOptions{}, err
	}
	return river, nil
}

func resolveRequestedLayers(requested []string) ([]layerRenderer, error) {
	if len(requested) == 0 {
		return nil, errors.New("layers must not be empty")
	}
	byName := make(map[string]layerRenderer, len(previewLayers))
	for _, layer := range previewLayers {
		byName[layer.name] = layer
	}
	chosen := make([]layerRenderer, 0, len(requested))
	for _, name := range requested {
		layer, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown layer %q (available: rivers)", name)
		}
		chosen = append(chosen, layer)
	}
	return chosen, nil
}

func resolveChunkDimensions(req renderRequest, base MapgenOptions) (int, int, error) {
	chunksX := req.ChunksX
	chunksY := req.ChunksY
	if chunksX <= 0 {
		chunksX = base.ChunksX
	}
	if chunksY <= 0 {
		chunksY = base.ChunksY
	}
	if chunksX < 1 || chunksX > previewMaxChunks || chunksY < 1 || chunksY > previewMaxChunks {
		return 0, 0, fmt.Errorf("chunks must be within [1,%d], got %dx%d", previewMaxChunks, chunksX, chunksY)
	}
	return chunksX, chunksY, nil
}

// renderLayers paints the paper background and then each requested layer in
// canonical registry order. Later layers must overdraw earlier ones.
func renderLayers(layers []layerRenderer, ctx renderContext) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, ctx.WidthTiles, ctx.HeightTiles))
	fillColor(img, previewBackgroundColor)
	for _, layer := range layers {
		if err := layer.render(img, ctx); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// renderRiversLayer draws the river class mask. Zero elevation is valid input:
// the draw layout ignores the elevation field entirely.
func renderRiversLayer(img *image.RGBA, ctx renderContext) error {
	elevation := make([]float32, ctx.WidthTiles*ctx.HeightTiles)
	network, err := BuildRiverNetwork(elevation, ctx.WidthTiles, ctx.HeightTiles, ctx.Seed, ctx.RiverOptions)
	if err != nil {
		return fmt.Errorf("build river network: %w", err)
	}

	stride := img.Stride
	pix := img.Pix
	for idx, class := range network.Class {
		var paint *color.RGBA
		switch class {
		case riverDeep:
			paint = &previewDeepColor
		case riverShallow:
			paint = &previewShallowColor
		default:
			continue
		}
		offset := (idx/ctx.WidthTiles)*stride + (idx%ctx.WidthTiles)*4
		pix[offset] = paint.R
		pix[offset+1] = paint.G
		pix[offset+2] = paint.B
		pix[offset+3] = paint.A
	}
	return nil
}

func fillColor(img *image.RGBA, c color.RGBA) {
	for y := 0; y < img.Rect.Dy(); y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+img.Rect.Dx()*4]
		for x := 0; x < img.Rect.Dx(); x++ {
			row[x*4] = c.R
			row[x*4+1] = c.G
			row[x*4+2] = c.B
			row[x*4+3] = c.A
		}
	}
}

func previewWorldSizes() []previewSizeOption {
	sizes := []previewSizeOption{}
	for _, chunks := range []int{10, 20, 50} {
		sizes = append(sizes, previewSizeOption{Chunks: chunks, Tiles: chunks * _const.ChunkSize})
	}
	return sizes
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// Headers are already sent; nothing to recover, log-worthy only in
		// server wrappers that instrument writes.
		_ = err
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// Panel labels are the yaml keys themselves (user tunes presets by config name).
func floatField(key string, min, max, step, value float64) previewField {
	minCopy, maxCopy, stepCopy := min, max, step
	return previewField{Key: key, Label: key, Type: "float", Min: &minCopy, Max: &maxCopy, Step: &stepCopy, Default: value}
}

func intField(key string, min, max, value int) previewField {
	minCopy, maxCopy := float64(min), float64(max)
	step := 1.0
	return previewField{Key: key, Label: key, Type: "int", Min: &minCopy, Max: &maxCopy, Step: &step, Default: value}
}

func previewLayerSchemas(base MapgenOptions) []previewLayerSchema {
	schemas := make([]previewLayerSchema, 0, len(previewLayers))
	for _, layer := range previewLayers {
		schemas = append(schemas, layer.schema(base))
	}
	return schemas
}

func riversLayerSchema(base MapgenOptions) previewLayerSchema {
	river := base.River
	return previewLayerSchema{
		Name:     "rivers",
		Title:    "Rivers",
		ParamKey: previewParamRivers,
		Groups: []previewGroup{
			{
				Title: "Channel shape",
				Fields: []previewField{
					floatField("shape_waves_per_link", 0.5, 12, 0.1, river.ShapeWavesPerLink),
					floatField("shape_frequency_scale", 0.05, 3, 0.05, river.ShapeFrequencyScale),
					intField("shape_octaves", 1, 4, river.ShapeOctaves),
					floatField("shape_octave_gain", 0.05, 0.6, 0.01, river.ShapeOctaveGain),
					floatField("shape_amplitude_scale", 0.05, 3, 0.05, river.ShapeAmplitudeScale),
					floatField("shape_distance_cap", 0.01, 0.9, 0.01, river.ShapeDistanceCap),
					floatField("shape_along_scale", 0, 1, 0.01, river.ShapeAlongScale),
					intField("shape_segment_length", 20, 300, river.ShapeSegmentLength),
					floatField("meander_strength", 0, 0.2, 0.001, river.MeanderStrength),
					floatField("shape_long_meander_scale", 0.05, 4, 0.05, river.ShapeLongMeanderScale),
					floatField("shape_short_meander_scale", 0.05, 8, 0.05, river.ShapeShortMeanderScale),
					floatField("shape_short_meander_bias", 0, 0.05, 0.001, river.ShapeShortMeanderBias),
				},
			},
			{
				Title: "Network",
				Fields: []previewField{
					intField("major_count", 1, 500, river.MajorRiverCount),
					floatField("lake_border_mix", 0, 1, 0.01, river.LakeBorderMix),
					intField("max_lake_degree", 1, 8, river.MaxLakeDegree),
					floatField("lake_connect_chance", 0, 1, 0.01, river.LakeConnectChance),
					intField("lake_connection_limit", 0, 500, river.LakeConnectionLimit),
					intField("lake_link_min_distance", 0, 5000, river.LakeLinkMinDistance),
					intField("lake_link_max_distance", 1, 10000, river.LakeLinkMaxDistance),
				},
			},
			{
				Title: "Lakes",
				Fields: []previewField{
					intField("lake_count", 2, 1000, river.LakeCount),
					floatField("lake_size_medium_chance", 0, 1, 0.01, river.LakeSizeMediumChance),
					floatField("lake_size_large_chance", 0, 1, 0.01, river.LakeSizeLargeChance),
					intField("lake_size_small_min", 0, 200, river.LakeSizeSmallMin),
					intField("lake_size_small_max", 0, 200, river.LakeSizeSmallMax),
					intField("lake_size_medium_min", 0, 400, river.LakeSizeMediumMin),
					intField("lake_size_medium_max", 0, 400, river.LakeSizeMediumMax),
					intField("lake_size_large_min", 0, 800, river.LakeSizeLargeMin),
					intField("lake_size_large_max", 0, 800, river.LakeSizeLargeMax),
				},
			},
			{
				Title: "Width & depth",
				Fields: []previewField{
					intField("river_width_min", 1, 64, river.RiverWidthMin),
					intField("river_width_max", 1, 64, river.RiverWidthMax),
					intField("bank_radius", 0, 8, river.BankRadius),
					intField("flow_shallow_threshold", 1, 200, river.FlowShallowThreshold),
					intField("flow_deep_threshold", 2, 400, river.FlowDeepThreshold),
				},
			},
		},
	}
}

var previewLayers = []layerRenderer{
	{
		name:   "rivers",
		schema: riversLayerSchema,
		render: renderRiversLayer,
	},
}
