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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_const "origin/internal/const"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed preview/index.html
var previewIndexHTML []byte

const (
	previewMaxChunks          = 64
	previewParamRivers        = "river"
	previewParamBiomes        = "biomes"
	previewParamWorld         = "world"
	previewMaxMajorRiverCount = 2000
)

var (
	previewBackgroundColor = color.RGBA{R: 0xf2, G: 0xef, B: 0xe6, A: 0xff}
	previewShallowColor    = color.RGBA{R: 0x6f, G: 0xd0, B: 0xff, A: 0xff}
	previewDeepColor       = color.RGBA{R: 0x0f, G: 0x7f, B: 0xd4, A: 0xff}
)

type previewField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // "float" | "int" | "bool"
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
	Default     any      `json:"default"`
	Description string   `json:"description,omitempty"`
	RangeMax    *float64 `json:"range_max,omitempty"`
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
	Preset          string               `json:"preset"`
	PresetDirectory string               `json:"preset_directory"`
	ChunksX         int                  `json:"chunks_x"`
	ChunksY         int                  `json:"chunks_y"`
	Seed            int64                `json:"seed"`
	Sizes           []previewSizeOption  `json:"sizes"`
	Layers          []previewLayerSchema `json:"layers"`
	World           previewLayerSchema   `json:"world"`
}

type renderRequest struct {
	Preset  string                     `json:"preset"`
	Seed    int64                      `json:"seed"`
	ChunksX int                        `json:"chunks_x"`
	ChunksY int                        `json:"chunks_y"`
	Layers  []string                   `json:"layers"`
	Params  map[string]json.RawMessage `json:"params"`
}

type renderContext struct {
	WidthTiles  int
	HeightTiles int
	Options     MapgenOptions
	Terrain     *TerrainPrecompute
}

type layerRenderer struct {
	name string
	// schema builds this layer's parameter panel description; it receives the
	// server's base options so field defaults follow the loaded preset.
	schema func(base MapgenOptions) previewLayerSchema
	render func(img *image.RGBA, ctx renderContext) error
}

type previewServer struct {
	logger        *zap.Logger
	base          MapgenOptions
	mux           *http.ServeMux
	presets       *os.Root
	defaultPreset string

	// Serializes buildPreviewPNG; see the comment there.
	mu sync.Mutex
}

func newPreviewServer(logger *zap.Logger, base MapgenOptions) (*previewServer, error) {
	path, err := resolveConfigPath(base.ConfigPath)
	if err != nil {
		return nil, err
	}
	presets, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open preset directory: %w", err)
	}
	s := &previewServer{logger: logger, base: base, presets: presets, defaultPreset: filepath.Base(path)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/defaults", s.handleDefaults)
	mux.HandleFunc("POST /api/render", s.handleRender)
	mux.HandleFunc("POST /api/preset", s.handleSavePreset)
	s.mux = mux
	return s, nil
}

func runRiverPreviewServer(logger *zap.Logger, opts MapgenOptions) {
	s, err := newPreviewServer(logger, opts)
	if err != nil {
		logger.Fatal("preview configuration failed", zap.Error(err))
		return
	}
	defer s.presets.Close()
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

func (s *previewServer) handleDefaults(w http.ResponseWriter, request *http.Request) {
	preset := request.URL.Query().Get("preset")
	if preset == "" {
		preset = s.defaultPreset
	}
	opts, _, path, err := s.loadPreset(preset)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, _, err := resolveChunkDimensions(renderRequest{}, opts); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response := defaultsResponse{
		Preset:          path,
		PresetDirectory: s.presets.Name(),
		ChunksX:         opts.ChunksX,
		ChunksY:         opts.ChunksY,
		Seed:            opts.Seed,
		Sizes:           previewWorldSizes(),
		Layers:          previewLayerSchemas(opts),
		World:           worldPreviewSchema(opts),
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func (s *previewServer) handleRender(w http.ResponseWriter, r *http.Request) {
	var req renderRequest
	if err := decodePreviewRequest(w, r, &req); err != nil {
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
	base, err := s.requestBase(req.Preset)
	if err != nil {
		return nil, err
	}
	opts, err := resolvePreviewOptions(req, base)
	if err != nil {
		return nil, err
	}
	requested, err := resolveRequestedLayers(req.Layers)
	if err != nil {
		return nil, err
	}
	if err := validatePreviewMemory(opts); err != nil {
		return nil, err
	}

	// Renders allocate hundreds of MB and are CPU-bound, so they are
	// serialized; the UI drops stale responses by sequence number.
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := renderContext{
		WidthTiles:  opts.ChunksX * _const.ChunkSize,
		HeightTiles: opts.ChunksY * _const.ChunkSize,
		Options:     opts,
	}
	pngBytes, err := renderLayers(requested, ctx)
	if err != nil {
		return nil, renderError{err}
	}
	return pngBytes, nil
}

func resolvePreviewOptions(req renderRequest, base MapgenOptions) (MapgenOptions, error) {
	opts := base
	world := previewWorldOptions{TerrainScale: base.TerrainScale, PerlinWaterEnabled: base.PerlinWaterEnabled}
	sections := map[string]any{
		previewParamRivers: &opts.River,
		previewParamBiomes: &opts.Biome,
		previewParamWorld:  &world,
	}
	for key := range req.Params {
		if _, known := sections[key]; !known {
			return MapgenOptions{}, fmt.Errorf("unknown parameter section %q", key)
		}
	}
	for _, key := range []string{previewParamRivers, previewParamBiomes, previewParamWorld} {
		raw, ok := req.Params[key]
		if !ok {
			continue
		}
		if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
			return MapgenOptions{}, fmt.Errorf("%s params must be an object", key)
		}
		if key == previewParamRivers {
			if err := validateJunctionSpacingInput(raw, false); err != nil {
				return MapgenOptions{}, err
			}
		}
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(sections[key]); err != nil {
			return MapgenOptions{}, fmt.Errorf("%s params: %w", key, err)
		}
	}
	opts.TerrainScale = world.TerrainScale
	opts.PerlinWaterEnabled = world.PerlinWaterEnabled
	opts.Seed = req.Seed
	var err error
	opts.ChunksX, opts.ChunksY, err = resolveChunkDimensions(req, base)
	if err != nil {
		return MapgenOptions{}, err
	}
	if err := opts.Validate(); err != nil {
		return MapgenOptions{}, err
	}
	return opts, nil
}

func resolveRequestedLayers(requested []string) ([]layerRenderer, error) {
	if len(requested) == 0 {
		return nil, errors.New("layers must not be empty")
	}
	byName := make(map[string]layerRenderer, len(previewLayers))
	for _, layer := range previewLayers {
		byName[layer.name] = layer
	}
	selected := make(map[string]bool, len(requested))
	available := make([]string, 0, len(previewLayers))
	for _, layer := range previewLayers {
		available = append(available, layer.name)
	}
	for _, name := range requested {
		_, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown layer %q (available: %s)", name, strings.Join(available, ", "))
		}
		selected[name] = true
	}
	chosen := make([]layerRenderer, 0, len(selected))
	for _, layer := range previewLayers {
		if selected[layer.name] {
			chosen = append(chosen, layer)
		}
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
	for _, layer := range layers {
		if layer.name == previewParamBiomes || !ctx.Options.River.LayoutDraw {
			fields := NewNoiseFieldsWithTerrainScale(NewPerlinNoise(ctx.Options.Seed), _const.CoordPerTile, ctx.Options.TerrainScale)
			terrain, err := BuildTerrainPrecompute(ctx.Options, _const.ChunkSize, fields)
			if err != nil {
				return nil, fmt.Errorf("build preview terrain: %w", err)
			}
			ctx.Terrain = terrain
			break
		}
	}
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

func renderRiversLayer(img *image.RGBA, ctx renderContext) error {
	if !ctx.Options.River.Enabled {
		return nil
	}
	var classes []RiverClass
	var fairways *riverFairways
	if ctx.Terrain != nil {
		classes, fairways = ctx.Terrain.RiverClass, ctx.Terrain.RiverFairways
	} else {
		elevation := make([]float32, ctx.WidthTiles*ctx.HeightTiles)
		network, err := BuildRiverNetwork(elevation, ctx.WidthTiles, ctx.HeightTiles, ctx.Options.Seed, ctx.Options.River)
		if err != nil {
			return fmt.Errorf("build river network: %w", err)
		}
		classes, fairways = network.Class, network.Fairways
	}

	fields := NewNoiseFieldsWithTerrainScale(NewPerlinNoise(ctx.Options.Seed), _const.CoordPerTile, ctx.Options.TerrainScale)
	stride := img.Stride
	pix := img.Pix
	for index, class := range classes {
		protected := fairways != nil && fairways.Protected[index]
		if class == riverNone && !protected {
			continue
		}
		var tile byte
		if ctx.Terrain != nil {
			tile = ctx.Terrain.Tiles[index]
		} else {
			elevation := 1.0
			if ctx.Options.PerlinWaterEnabled {
				elevation = float64(float32(fields.Elevation(index%ctx.WidthTiles, index/ctx.WidthTiles)))
			}
			tile = resolveTileType(elevation, tileGrass, class, ctx.Options.PerlinWaterEnabled, true)
			if protected {
				tile = tileWaterDeep
			}
		}
		var paint *color.RGBA
		switch tile {
		case tileWaterDeep:
			paint = &previewDeepColor
		case tileWater:
			paint = &previewShallowColor
		default:
			continue
		}
		offset := (index/ctx.WidthTiles)*stride + (index%ctx.WidthTiles)*4
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

func legacyRiverWavesField(value float64) previewField {
	field := floatField("shape_waves_per_link", 0.5, 12, 0.1, value)
	field.Label = "shape_waves_per_link (legacy; wavelength = 0)"
	return field
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
					intField("shape_wavelength_tiles", 0, 8192, river.ShapeWavelengthTiles),
					legacyRiverWavesField(river.ShapeWavesPerLink),
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
					intField("major_count", 1, previewMaxMajorRiverCount, river.MajorRiverCount),
					floatField("lake_border_mix", 0, 1, 0.01, river.LakeBorderMix),
					intField("max_lake_degree", 1, 8, river.MaxLakeDegree),
					floatField("lake_connect_chance", 0, 1, 0.01, river.LakeConnectChance),
					intField("lake_connection_limit", 0, 500, river.LakeConnectionLimit),
					intField("lake_link_min_distance", 0, 5000, river.LakeLinkMinDistance),
					intField("lake_link_max_distance", 1, 10000, river.LakeLinkMaxDistance),
					floatField("tributary_ratio", 0, 1, 0.01, river.TributaryRatio),
					intField("tributary_spacing_tiles", 0, 8192, river.TributarySpacingTiles),
					intField("tributary_length_min", 0, 8192, river.TributaryLengthMin),
					intField("tributary_length_max", 0, 8192, river.TributaryLengthMax),
				},
			},
			junctionPreviewGroup(river),
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
			lakeDetailPreviewGroup(river),
			{
				Title: "Width & depth",
				Fields: []previewField{
					describedField(intField("river_width_min", 1, 64, river.RiverWidthMin), "Минимальная глубокая ширина основных рисуемых рек при fairway_width_tiles > 0; у притоков это максимум. В старом режиме — ширина всего коридора."),
					intField("fairway_width_tiles", 0, 63, river.FairwayWidthTiles),
					describedField(intField("river_width_max", 1, 64, river.RiverWidthMax), "Максимальная глубокая ширина основных рисуемых рек с фарватером. Мелководье добавляется снаружи; на поворотах сечения объединяются."),
					describedField(intField("width_variation_scale", 0, 8192, river.WidthVariationScale), "Масштаб изменения глубокой части вдоль русла, тайлы: 16–8192; 0 — постоянная ширина связи. Для рисуемых рек с fairway_width_tiles > 0."),
					describedField(intField("shallow_width_min", 0, 32, river.ShallowWidthMin), "Минимальная полоса мелководья с каждого берега, тайлы. Глубокий фарватер сохраняется."),
					describedField(intField("shallow_width_max", 0, 32, river.ShallowWidthMax), "Максимальная полоса мелководья, тайлы; >= min. Равные min/max — постоянная толщина; оба 0 — без полосы у рек."),
					describedField(intField("shallow_variation_scale", 0, 8192, river.ShallowVariationScale), "Масштаб независимых изменений берегов, 16–8192 тайлов. 0 допустим при одинаковых shallow_width_min/max."),
					describedField(intField("bank_radius", 0, 8, river.BankRadius), "Дополнительный радиус старого режима; у рисуемых рек с фарватером ограничен shallow_width_min."),
					intField("flow_shallow_threshold", 1, 200, river.FlowShallowThreshold),
					intField("flow_deep_threshold", 2, 400, river.FlowDeepThreshold),
				},
			},
		},
	}
}

var previewLayers = []layerRenderer{
	{
		name:   "biomes",
		schema: biomesLayerSchema,
		render: renderBiomesLayer,
	},
	{
		name:   "rivers",
		schema: riversLayerSchema,
		render: renderRiversLayer,
	},
}
