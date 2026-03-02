package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	_const "origin/internal/const"
	"strings"
)

const (
	maxPrecomputeBytes = uint64(3) << 30 // 3 GiB safety cap for terrain precompute buffers
	maxOverviewPixels  = uint64(250_000_000)
)

const defaultGenConfigPath = "etc/mapgen/presets/default.yaml"

type RiverOptions struct {
	Enabled                bool    `yaml:"enabled"`
	LayoutDraw             bool    `yaml:"layout_draw"`
	MajorRiverCount        int     `yaml:"major_count"`
	LakeCount              int     `yaml:"lake_count"`
	LakeSizeSmallMin       int     `yaml:"lake_size_small_min"`
	LakeSizeSmallMax       int     `yaml:"lake_size_small_max"`
	LakeSizeMediumMin      int     `yaml:"lake_size_medium_min"`
	LakeSizeMediumMax      int     `yaml:"lake_size_medium_max"`
	LakeSizeLargeMin       int     `yaml:"lake_size_large_min"`
	LakeSizeLargeMax       int     `yaml:"lake_size_large_max"`
	LakeSizeMediumChance   float64 `yaml:"lake_size_medium_chance"`
	LakeSizeLargeChance    float64 `yaml:"lake_size_large_chance"`
	LakeBorderMix          float64 `yaml:"lake_border_mix"`
	MaxLakeDegree          int     `yaml:"max_lake_degree"`
	ShapeLongMeanderScale  float64 `yaml:"shape_long_meander_scale"`
	ShapeShortMeanderScale float64 `yaml:"shape_short_meander_scale"`
	ShapeShortMeanderBias  float64 `yaml:"shape_short_meander_bias"`
	ShapeAmplitudeScale    float64 `yaml:"shape_amplitude_scale"`
	ShapeFrequencyScale    float64 `yaml:"shape_frequency_scale"`
	ShapeNoiseScale        float64 `yaml:"shape_noise_scale"`
	ShapeAlongScale        float64 `yaml:"shape_along_scale"`
	ShapeDistanceCap       float64 `yaml:"shape_distance_cap"`
	ShapeSegmentLength     int     `yaml:"shape_segment_length"`
	SourceElevationMin     float64 `yaml:"source_elevation_min"`
	SourceChance           float64 `yaml:"source_chance"`
	MeanderStrength        float64 `yaml:"meander_strength"`
	VoronoiCellSize        int     `yaml:"voronoi_cell_size"`
	VoronoiEdgeThreshold   float64 `yaml:"voronoi_edge_threshold"`
	VoronoiSourceBoost     float64 `yaml:"voronoi_source_boost"`
	VoronoiBias            float64 `yaml:"voronoi_bias"`
	SinkLakeChance         float64 `yaml:"sink_lake_chance"`
	LakeMinSize            int     `yaml:"lake_min_size"`
	LakeConnectChance      float64 `yaml:"lake_connect_chance"`
	LakeConnectionLimit    int     `yaml:"lake_connection_limit"`
	LakeLinkMinDistance    int     `yaml:"lake_link_min_distance"`
	LakeLinkMaxDistance    int     `yaml:"lake_link_max_distance"`
	RiverWidthMin          int     `yaml:"river_width_min"`
	RiverWidthMax          int     `yaml:"river_width_max"`
	GridEnabled            bool    `yaml:"grid_enabled"`
	GridSpacing            int     `yaml:"grid_spacing"`
	GridJitter             int     `yaml:"grid_jitter"`
	TrunkRiverCount        int     `yaml:"trunk_count"`
	TrunkSourceElevation   float64 `yaml:"trunk_source_elevation_min"`
	TrunkMinLength         int     `yaml:"trunk_min_length"`
	CoastSampleChance      float64 `yaml:"coast_sample_chance"`
	FlowShallowThreshold   int     `yaml:"flow_shallow_threshold"`
	FlowDeepThreshold      int     `yaml:"flow_deep_threshold"`
	BankRadius             int     `yaml:"bank_radius"`
	LakeFlowThreshold      int     `yaml:"lake_flow_threshold"`
}

type BiomeOptions struct {
	Enabled                 bool    `yaml:"enabled"`
	HNHEnabled              bool    `yaml:"hnh_enabled"`
	VariantDensity          float64 `yaml:"hnh_variant_density"`
	RegionCount             int     `yaml:"hnh_region_count"`
	RegionJitter            int     `yaml:"hnh_region_jitter"`
	BlendWidth              int     `yaml:"hnh_blend_width"`
	SmoothingPasses         int     `yaml:"hnh_smoothing_passes"`
	MinPatchTiles           int     `yaml:"hnh_min_patch_tiles"`
	SwampClumpScale         float64 `yaml:"hnh_swamp_clump_scale"`
	MountainRuggedThreshold float64 `yaml:"hnh_mountain_rugged_threshold"`
	ForestShare             float64 `yaml:"hnh_forest_share"`
	GrasslandShare          float64 `yaml:"hnh_grassland_share"`
	WetlandShare            float64 `yaml:"hnh_wetland_share"`
	HeathMoorShare          float64 `yaml:"hnh_heath_moor_share"`
	MountainShare           float64 `yaml:"hnh_mountain_share"`

	TemperatureScale     float64 `yaml:"temperature_scale"`
	MoistureScale        float64 `yaml:"moisture_scale"`
	ContinentalnessScale float64 `yaml:"continentalness_scale"`
	ErosionScale         float64 `yaml:"erosion_scale"`
	WeirdnessScale       float64 `yaml:"weirdness_scale"`
	DomainWarpStrength   float64 `yaml:"domain_warp_strength"`
}

type EcologyOptions struct {
	Enabled      bool    `yaml:"enabled"`
	TickInterval int     `yaml:"tick_interval"`
	CellsPerTick int     `yaml:"cells_per_tick"`
	ChunkBudget  int     `yaml:"chunk_budget"`
	SpreadScale  float64 `yaml:"spread_scale"`
}

type PNGOptions struct {
	Export          bool   `yaml:"export"`
	OverviewOnly    bool   `yaml:"overview_only"`
	OutputDir       string `yaml:"output_dir"`
	Scale           int    `yaml:"scale"`
	HighlightRivers bool   `yaml:"highlight_rivers"`
}

type MapgenOptions struct {
	ConfigPath string
	ChunksX    int
	ChunksY    int
	Seed       int64
	Threads    int
	River      RiverOptions
	Biome      BiomeOptions
	Ecology    EcologyOptions
	PNG        PNGOptions
}

func DefaultMapgenOptions() MapgenOptions {
	return MapgenOptions{
		ConfigPath: defaultGenConfigPath,
		ChunksX:    50,
		ChunksY:    50,
		Seed:       0,
		Threads:    4,
		River: RiverOptions{
			Enabled:                true,
			LayoutDraw:             true,
			MajorRiverCount:        34,
			LakeCount:              220,
			LakeSizeSmallMin:       10,
			LakeSizeSmallMax:       30,
			LakeSizeMediumMin:      36,
			LakeSizeMediumMax:      88,
			LakeSizeLargeMin:       96,
			LakeSizeLargeMax:       240,
			LakeSizeMediumChance:   0.24,
			LakeSizeLargeChance:    0.04,
			LakeBorderMix:          0.32,
			MaxLakeDegree:          2,
			ShapeLongMeanderScale:  0.55,
			ShapeShortMeanderScale: 2.2,
			ShapeShortMeanderBias:  0.0035,
			ShapeAmplitudeScale:    1.0,
			ShapeFrequencyScale:    1.0,
			ShapeNoiseScale:        0.30,
			ShapeAlongScale:        0.16,
			ShapeDistanceCap:       0.40,
			ShapeSegmentLength:     70,
			SourceElevationMin:     0.55,
			SourceChance:           0.00015,
			MeanderStrength:        0.003,
			VoronoiCellSize:        96,
			VoronoiEdgeThreshold:   0.14,
			VoronoiSourceBoost:     0.02,
			VoronoiBias:            0.01,
			SinkLakeChance:         0.03,
			LakeMinSize:            48,
			LakeConnectChance:      0.72,
			LakeConnectionLimit:    140,
			LakeLinkMinDistance:    180,
			LakeLinkMaxDistance:    1800,
			RiverWidthMin:          5,
			RiverWidthMax:          15,
			GridEnabled:            true,
			GridSpacing:            760,
			GridJitter:             64,
			TrunkRiverCount:        8,
			TrunkSourceElevation:   0.62,
			TrunkMinLength:         180,
			CoastSampleChance:      0.012,
			FlowShallowThreshold:   6,
			FlowDeepThreshold:      20,
			BankRadius:             1,
			LakeFlowThreshold:      28,
		},
		Biome: BiomeOptions{
			Enabled:                 true,
			HNHEnabled:              true,
			VariantDensity:          0.55,
			RegionCount:             140,
			RegionJitter:            220,
			BlendWidth:              96,
			SmoothingPasses:         2,
			MinPatchTiles:           20,
			SwampClumpScale:         1.0,
			MountainRuggedThreshold: 0.66,
			ForestShare:             0.30,
			GrasslandShare:          0.30,
			WetlandShare:            0.14,
			HeathMoorShare:          0.14,
			MountainShare:           0.12,
			TemperatureScale:        1.0,
			MoistureScale:           1.0,
			ContinentalnessScale:    1.0,
			ErosionScale:            1.0,
			WeirdnessScale:          1.0,
			DomainWarpStrength:      48.0,
		},
		Ecology: EcologyOptions{
			Enabled:      true,
			TickInterval: 300,
			CellsPerTick: 5000,
			ChunkBudget:  12,
			SpreadScale:  1.0,
		},
		PNG: PNGOptions{
			Export:          false,
			OverviewOnly:    false,
			OutputDir:       "map_png",
			Scale:           1,
			HighlightRivers: true,
		},
	}
}

func ParseMapgenOptions(args []string) (MapgenOptions, error) {
	defaults := DefaultMapgenOptions()

	var (
		genConfigPath      = defaultGenConfigPath
		chunksX            = defaults.ChunksX
		chunksY            = defaults.ChunksY
		seed               = defaults.Seed
		threads            = defaults.Threads
		pngExport          = defaults.PNG.Export
		pngOverviewOnly    = defaults.PNG.OverviewOnly
		pngDir             = defaults.PNG.OutputDir
		pngScale           = defaults.PNG.Scale
		pngHighlightRivers = defaults.PNG.HighlightRivers
	)

	fs := flag.NewFlagSet("mapgen", flag.ContinueOnError)
	fs.StringVar(&genConfigPath, "gen-config", genConfigPath, "path to YAML generation preset")
	fs.IntVar(&chunksX, "chunks-x", chunksX, "override chunks in X direction")
	fs.IntVar(&chunksY, "chunks-y", chunksY, "override chunks in Y direction")
	fs.Int64Var(&seed, "seed", seed, "override random seed (0 = use current time)")
	fs.IntVar(&threads, "threads", threads, "override worker thread count")
	fs.BoolVar(&pngExport, "png-export", pngExport, "override png export toggle")
	fs.BoolVar(&pngOverviewOnly, "png-overview-only", pngOverviewOnly, "export only overview.png (implies -png-export=true, skips DB writes)")
	fs.StringVar(&pngDir, "png-dir", pngDir, "override png output directory")
	fs.IntVar(&pngScale, "png-scale", pngScale, "override png scale in pixels per tile")
	fs.BoolVar(&pngHighlightRivers, "png-highlight-rivers", pngHighlightRivers, "override river highlighting in PNG exports")

	if err := fs.Parse(args); err != nil {
		return MapgenOptions{}, err
	}

	overrides := map[string]struct{}{}
	fs.Visit(func(f *flag.Flag) {
		overrides[f.Name] = struct{}{}
	})

	opts, resolvedConfigPath, err := LoadMapgenOptionsFromYAML(genConfigPath, defaults)
	if err != nil {
		return MapgenOptions{}, err
	}
	opts.ConfigPath = resolvedConfigPath

	if _, ok := overrides["chunks-x"]; ok {
		opts.ChunksX = chunksX
	}
	if _, ok := overrides["chunks-y"]; ok {
		opts.ChunksY = chunksY
	}
	if _, ok := overrides["seed"]; ok {
		opts.Seed = seed
	}
	if _, ok := overrides["threads"]; ok {
		opts.Threads = threads
	}
	if _, ok := overrides["png-export"]; ok {
		opts.PNG.Export = pngExport
	}
	if _, ok := overrides["png-overview-only"]; ok {
		opts.PNG.OverviewOnly = pngOverviewOnly
	}
	if _, ok := overrides["png-dir"]; ok {
		opts.PNG.OutputDir = pngDir
	}
	if _, ok := overrides["png-scale"]; ok {
		opts.PNG.Scale = pngScale
	}
	if _, ok := overrides["png-highlight-rivers"]; ok {
		opts.PNG.HighlightRivers = pngHighlightRivers
	}
	if opts.PNG.OverviewOnly {
		opts.PNG.Export = true
	}

	if err := opts.Validate(); err != nil {
		return MapgenOptions{}, err
	}

	return opts, nil
}

func (o MapgenOptions) WorldTileDimensions() (int, int, error) {
	widthTiles, err := checkedMulInt(o.ChunksX, _const.ChunkSize)
	if err != nil {
		return 0, 0, fmt.Errorf("world width in tiles: %w", err)
	}
	heightTiles, err := checkedMulInt(o.ChunksY, _const.ChunkSize)
	if err != nil {
		return 0, 0, fmt.Errorf("world height in tiles: %w", err)
	}
	return widthTiles, heightTiles, nil
}

func (o MapgenOptions) Validate() error {
	if o.ChunksX <= 0 {
		return errors.New("chunks-x must be > 0")
	}
	if o.ChunksY <= 0 {
		return errors.New("chunks-y must be > 0")
	}
	if o.Threads <= 0 {
		return errors.New("threads must be > 0")
	}
	if o.PNG.Scale <= 0 {
		return errors.New("png-scale must be > 0")
	}

	if o.River.SourceElevationMin < 0 || o.River.SourceElevationMin > 1 {
		return errors.New("river.source_elevation_min must be within [0,1]")
	}
	if o.River.MajorRiverCount <= 0 {
		return errors.New("river.major_count must be > 0")
	}
	if o.River.LakeCount <= 1 {
		return errors.New("river.lake_count must be > 1")
	}
	if o.River.LakeSizeSmallMin < 0 || o.River.LakeSizeSmallMax < 0 {
		return errors.New("river lake small size bounds must be >= 0")
	}
	if o.River.LakeSizeSmallMin > 0 && o.River.LakeSizeSmallMax > 0 && o.River.LakeSizeSmallMax < o.River.LakeSizeSmallMin {
		return errors.New("river.lake_size_small_max must be >= river.lake_size_small_min")
	}
	if o.River.LakeSizeMediumMin < 0 || o.River.LakeSizeMediumMax < 0 {
		return errors.New("river lake medium size bounds must be >= 0")
	}
	if o.River.LakeSizeMediumMin > 0 && o.River.LakeSizeMediumMax > 0 && o.River.LakeSizeMediumMax < o.River.LakeSizeMediumMin {
		return errors.New("river.lake_size_medium_max must be >= river.lake_size_medium_min")
	}
	if o.River.LakeSizeLargeMin < 0 || o.River.LakeSizeLargeMax < 0 {
		return errors.New("river lake large size bounds must be >= 0")
	}
	if o.River.LakeSizeLargeMin > 0 && o.River.LakeSizeLargeMax > 0 && o.River.LakeSizeLargeMax < o.River.LakeSizeLargeMin {
		return errors.New("river.lake_size_large_max must be >= river.lake_size_large_min")
	}
	if o.River.LakeSizeMediumChance < 0 || o.River.LakeSizeMediumChance > 1 {
		return errors.New("river.lake_size_medium_chance must be within [0,1]")
	}
	if o.River.LakeSizeLargeChance < 0 || o.River.LakeSizeLargeChance > 1 {
		return errors.New("river.lake_size_large_chance must be within [0,1]")
	}
	if o.River.LakeSizeMediumChance+o.River.LakeSizeLargeChance > 0.95 {
		return errors.New("river lake size chance sum must be <= 0.95")
	}
	if o.River.LakeBorderMix < 0 || o.River.LakeBorderMix > 1 {
		return errors.New("river.lake_border_mix must be within [0,1]")
	}
	if o.River.MaxLakeDegree <= 0 {
		return errors.New("river.max_lake_degree must be > 0")
	}
	if o.River.ShapeLongMeanderScale <= 0 || o.River.ShapeLongMeanderScale > 4 {
		return errors.New("river.shape_long_meander_scale must be within (0,4]")
	}
	if o.River.ShapeShortMeanderScale <= 0 || o.River.ShapeShortMeanderScale > 8 {
		return errors.New("river.shape_short_meander_scale must be within (0,8]")
	}
	if o.River.ShapeShortMeanderBias < 0 || o.River.ShapeShortMeanderBias > 0.05 {
		return errors.New("river.shape_short_meander_bias must be within [0,0.05]")
	}
	if o.River.ShapeAmplitudeScale <= 0 || o.River.ShapeAmplitudeScale > 3 {
		return errors.New("river.shape_amplitude_scale must be within (0,3]")
	}
	if o.River.ShapeFrequencyScale <= 0 || o.River.ShapeFrequencyScale > 3 {
		return errors.New("river.shape_frequency_scale must be within (0,3]")
	}
	if o.River.ShapeNoiseScale < 0 || o.River.ShapeNoiseScale > 1 {
		return errors.New("river.shape_noise_scale must be within [0,1]")
	}
	if o.River.ShapeAlongScale < 0 || o.River.ShapeAlongScale > 1 {
		return errors.New("river.shape_along_scale must be within [0,1]")
	}
	if o.River.ShapeDistanceCap <= 0 || o.River.ShapeDistanceCap > 0.9 {
		return errors.New("river.shape_distance_cap must be within (0,0.9]")
	}
	if o.River.ShapeSegmentLength < 20 || o.River.ShapeSegmentLength > 300 {
		return errors.New("river.shape_segment_length must be within [20,300]")
	}
	if o.River.SourceChance < 0 || o.River.SourceChance > 1 {
		return errors.New("river.source_chance must be within [0,1]")
	}
	if o.River.VoronoiCellSize <= 1 {
		return errors.New("river.voronoi_cell_size must be > 1")
	}
	if o.River.VoronoiEdgeThreshold <= 0 {
		return errors.New("river.voronoi_edge_threshold must be > 0")
	}
	if o.River.VoronoiSourceBoost < 0 || o.River.VoronoiSourceBoost > 1 {
		return errors.New("river.voronoi_source_boost must be within [0,1]")
	}
	if o.River.VoronoiBias < 0 || o.River.VoronoiBias > 1 {
		return errors.New("river.voronoi_bias must be within [0,1]")
	}
	if o.River.SinkLakeChance < 0 || o.River.SinkLakeChance > 1 {
		return errors.New("river.sink_lake_chance must be within [0,1]")
	}
	if o.River.LakeMinSize <= 0 {
		return errors.New("river.lake_min_size must be > 0")
	}
	if o.River.LakeConnectChance < 0 || o.River.LakeConnectChance > 1 {
		return errors.New("river.lake_connect_chance must be within [0,1]")
	}
	if o.River.LakeConnectionLimit < 0 {
		return errors.New("river.lake_connection_limit must be >= 0")
	}
	if o.River.LakeLinkMinDistance < 0 {
		return errors.New("river.lake_link_min_distance must be >= 0")
	}
	if o.River.LakeLinkMaxDistance <= 0 {
		return errors.New("river.lake_link_max_distance must be > 0")
	}
	if o.River.LakeLinkMaxDistance <= o.River.LakeLinkMinDistance {
		return errors.New("river.lake_link_max_distance must be > river.lake_link_min_distance")
	}
	if o.River.RiverWidthMin <= 0 {
		return errors.New("river.river_width_min must be > 0")
	}
	if o.River.RiverWidthMax < o.River.RiverWidthMin {
		return errors.New("river.river_width_max must be >= river.river_width_min")
	}
	if o.River.GridSpacing <= 0 {
		return errors.New("river.grid_spacing must be > 0")
	}
	if o.River.GridJitter < 0 {
		return errors.New("river.grid_jitter must be >= 0")
	}
	if o.River.GridJitter >= o.River.GridSpacing/2 {
		return errors.New("river.grid_jitter must be < river.grid_spacing/2")
	}
	if o.River.TrunkRiverCount < 0 {
		return errors.New("river.trunk_count must be >= 0")
	}
	if o.River.TrunkSourceElevation < 0 || o.River.TrunkSourceElevation > 1 {
		return errors.New("river.trunk_source_elevation_min must be within [0,1]")
	}
	if o.River.TrunkMinLength <= 0 {
		return errors.New("river.trunk_min_length must be > 0")
	}
	if o.River.CoastSampleChance <= 0 || o.River.CoastSampleChance > 1 {
		return errors.New("river.coast_sample_chance must be within (0,1]")
	}
	if o.River.FlowShallowThreshold <= 0 {
		return errors.New("river.flow_shallow_threshold must be > 0")
	}
	if o.River.FlowDeepThreshold <= o.River.FlowShallowThreshold {
		return errors.New("river.flow_deep_threshold must be > river.flow_shallow_threshold")
	}
	if o.River.BankRadius < 0 {
		return errors.New("river.bank_radius must be >= 0")
	}
	if o.River.LakeFlowThreshold < o.River.FlowShallowThreshold {
		return errors.New("river.lake_flow_threshold must be >= river.flow_shallow_threshold")
	}

	if o.Biome.RegionCount <= 0 {
		return errors.New("biomes.hnh_region_count must be > 0")
	}
	if o.Biome.RegionJitter < 0 {
		return errors.New("biomes.hnh_region_jitter must be >= 0")
	}
	if o.Biome.BlendWidth < 0 {
		return errors.New("biomes.hnh_blend_width must be >= 0")
	}
	if o.Biome.SmoothingPasses < 0 {
		return errors.New("biomes.hnh_smoothing_passes must be >= 0")
	}
	if o.Biome.MinPatchTiles <= 0 {
		return errors.New("biomes.hnh_min_patch_tiles must be > 0")
	}
	if o.Biome.VariantDensity < 0 || o.Biome.VariantDensity > 1 {
		return errors.New("biomes.hnh_variant_density must be within [0,1]")
	}
	if o.Biome.SwampClumpScale <= 0 || o.Biome.SwampClumpScale > 4 {
		return errors.New("biomes.hnh_swamp_clump_scale must be within (0,4]")
	}
	if o.Biome.MountainRuggedThreshold < 0 || o.Biome.MountainRuggedThreshold > 1 {
		return errors.New("biomes.hnh_mountain_rugged_threshold must be within [0,1]")
	}
	if err := validateBiomeShare("biomes.hnh_forest_share", o.Biome.ForestShare); err != nil {
		return err
	}
	if err := validateBiomeShare("biomes.hnh_grassland_share", o.Biome.GrasslandShare); err != nil {
		return err
	}
	if err := validateBiomeShare("biomes.hnh_wetland_share", o.Biome.WetlandShare); err != nil {
		return err
	}
	if err := validateBiomeShare("biomes.hnh_heath_moor_share", o.Biome.HeathMoorShare); err != nil {
		return err
	}
	if err := validateBiomeShare("biomes.hnh_mountain_share", o.Biome.MountainShare); err != nil {
		return err
	}
	landShare := o.Biome.ForestShare + o.Biome.GrasslandShare + o.Biome.WetlandShare + o.Biome.HeathMoorShare + o.Biome.MountainShare
	if landShare > 1.0 {
		return errors.New("sum of biome land shares must be <= 1.0")
	}
	if o.Biome.TemperatureScale <= 0 {
		return errors.New("biomes.temperature_scale must be > 0")
	}
	if o.Biome.MoistureScale <= 0 {
		return errors.New("biomes.moisture_scale must be > 0")
	}
	if o.Biome.ContinentalnessScale <= 0 {
		return errors.New("biomes.continentalness_scale must be > 0")
	}
	if o.Biome.ErosionScale <= 0 {
		return errors.New("biomes.erosion_scale must be > 0")
	}
	if o.Biome.WeirdnessScale <= 0 {
		return errors.New("biomes.weirdness_scale must be > 0")
	}
	if o.Biome.DomainWarpStrength < 0 || o.Biome.DomainWarpStrength > 512 {
		return errors.New("biomes.domain_warp_strength must be within [0,512]")
	}

	if o.Ecology.TickInterval <= 0 {
		return errors.New("ecology.tick_interval must be > 0")
	}
	if o.Ecology.CellsPerTick <= 0 {
		return errors.New("ecology.cells_per_tick must be > 0")
	}
	if o.Ecology.ChunkBudget <= 0 {
		return errors.New("ecology.chunk_budget must be > 0")
	}
	if o.Ecology.SpreadScale <= 0 || o.Ecology.SpreadScale > 10 {
		return errors.New("ecology.spread_scale must be within (0,10]")
	}

	if strings.TrimSpace(o.PNG.OutputDir) == "" {
		return errors.New("png-dir must not be empty")
	}

	widthTiles, heightTiles, err := o.WorldTileDimensions()
	if err != nil {
		return err
	}
	if widthTiles == 0 || heightTiles == 0 {
		return errors.New("world tile dimensions must be > 0")
	}

	tileCount, err := checkedMulUint64(uint64(widthTiles), uint64(heightTiles))
	if err != nil {
		return fmt.Errorf("tile count overflow: %w", err)
	}

	estimatedBytes, err := estimatePrecomputeBytes(tileCount, o.River.Enabled, o.Biome.Enabled)
	if err != nil {
		return err
	}
	if estimatedBytes > maxPrecomputeBytes {
		return fmt.Errorf("estimated precompute memory %d bytes exceeds limit %d bytes", estimatedBytes, maxPrecomputeBytes)
	}

	if o.PNG.Export {
		if err := validateOverviewImageSize(widthTiles, heightTiles, o.PNG.Scale); err != nil {
			return err
		}
	}

	return nil
}

func validateBiomeShare(name string, value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("%s must be within [0,1]", name)
	}
	return nil
}

func estimatePrecomputeBytes(tileCount uint64, riverEnabled, biomeEnabled bool) (uint64, error) {
	// elevation float32 + final tile byte
	bytesPerTile := uint64(5)
	if riverEnabled {
		// flow uint32 + river class byte
		bytesPerTile += 5
	}
	if biomeEnabled {
		// base tile byte + climate fields (temperature, moisture, continentalness, erosion, weirdness, ruggedness)
		bytesPerTile += 1 + 6*4
	}
	return checkedMulUint64(tileCount, bytesPerTile)
}

func validateOverviewImageSize(widthTiles, heightTiles, scale int) error {
	widthPx, err := checkedMulUint64(uint64(widthTiles), uint64(scale))
	if err != nil {
		return fmt.Errorf("overview width overflow: %w", err)
	}
	heightPx, err := checkedMulUint64(uint64(heightTiles), uint64(scale))
	if err != nil {
		return fmt.Errorf("overview height overflow: %w", err)
	}
	if widthPx == 0 || heightPx == 0 {
		return errors.New("overview image dimensions must be > 0")
	}
	pixels, err := checkedMulUint64(widthPx, heightPx)
	if err != nil {
		return fmt.Errorf("overview pixel count overflow: %w", err)
	}
	if pixels > maxOverviewPixels {
		return fmt.Errorf("overview pixel count %d exceeds limit %d", pixels, maxOverviewPixels)
	}
	return nil
}

func checkedMulInt(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, errors.New("multiplication inputs must be >= 0")
	}
	product, err := checkedMulUint64(uint64(a), uint64(b))
	if err != nil {
		return 0, err
	}
	if product > uint64(math.MaxInt) {
		return 0, errors.New("multiplication result exceeds max int")
	}
	return int(product), nil
}

func checkedMulUint64(a, b uint64) (uint64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxUint64/b {
		return 0, errors.New("multiplication overflow")
	}
	return a * b, nil
}
