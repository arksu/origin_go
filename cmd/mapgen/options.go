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

type RiverOptions struct {
	Enabled              bool
	SourceElevationMin   float64
	SourceChance         float64
	MeanderStrength      float64
	VoronoiCellSize      int
	VoronoiEdgeThreshold float64
	VoronoiSourceBoost   float64
	VoronoiBias          float64
	SinkLakeChance       float64
	LakeMinSize          int
	LakeConnectChance    float64
	LakeConnectionLimit  int
	LakeLinkMinDistance  int
	LakeLinkMaxDistance  int
	RiverWidthMin        int
	RiverWidthMax        int
	GridEnabled          bool
	GridSpacing          int
	GridJitter           int
	TrunkRiverCount      int
	TrunkSourceElevation float64
	TrunkMinLength       int
	CoastSampleChance    float64
	FlowShallowThreshold int
	FlowDeepThreshold    int
	BankRadius           int
	LakeFlowThreshold    int
}

type PNGOptions struct {
	Export          bool
	OutputDir       string
	Scale           int
	HighlightRivers bool
}

type MapgenOptions struct {
	ChunksX int
	ChunksY int
	Seed    int64
	Threads int
	River   RiverOptions
	PNG     PNGOptions
}

func DefaultMapgenOptions() MapgenOptions {
	return MapgenOptions{
		ChunksX: 50,
		ChunksY: 50,
		Seed:    0,
		Threads: 4,
		River: RiverOptions{
			Enabled:              true,
			SourceElevationMin:   0.55,
			SourceChance:         0.00015,
			MeanderStrength:      0.003,
			VoronoiCellSize:      96,
			VoronoiEdgeThreshold: 0.14,
			VoronoiSourceBoost:   0.02,
			VoronoiBias:          0.01,
			SinkLakeChance:       0.08,
			LakeMinSize:          48,
			LakeConnectChance:    0.35,
			LakeConnectionLimit:  64,
			LakeLinkMinDistance:  120,
			LakeLinkMaxDistance:  1800,
			RiverWidthMin:        5,
			RiverWidthMax:        15,
			GridEnabled:          true,
			GridSpacing:          760,
			GridJitter:           64,
			TrunkRiverCount:      8,
			TrunkSourceElevation: 0.62,
			TrunkMinLength:       180,
			CoastSampleChance:    0.012,
			FlowShallowThreshold: 6,
			FlowDeepThreshold:    20,
			BankRadius:           1,
			LakeFlowThreshold:    28,
		},
		PNG: PNGOptions{
			Export:          false,
			OutputDir:       "map_png",
			Scale:           1,
			HighlightRivers: true,
		},
	}
}

func ParseMapgenOptions(args []string) (MapgenOptions, error) {
	opts := DefaultMapgenOptions()

	fs := flag.NewFlagSet("mapgen", flag.ContinueOnError)
	fs.IntVar(&opts.ChunksX, "chunks-x", opts.ChunksX, "number of chunks in X direction")
	fs.IntVar(&opts.ChunksY, "chunks-y", opts.ChunksY, "number of chunks in Y direction")
	fs.Int64Var(&opts.Seed, "seed", opts.Seed, "random seed (0 = use current time)")
	fs.IntVar(&opts.Threads, "threads", opts.Threads, "number of worker threads")

	fs.BoolVar(&opts.River.Enabled, "river-enabled", opts.River.Enabled, "enable procedural river generation")
	fs.Float64Var(&opts.River.SourceElevationMin, "river-source-elevation-min", opts.River.SourceElevationMin, "minimum elevation [0..1] for river sources")
	fs.Float64Var(&opts.River.SourceChance, "river-source-chance", opts.River.SourceChance, "chance [0..1] per tile to become a river source")
	fs.Float64Var(&opts.River.MeanderStrength, "river-meander-strength", opts.River.MeanderStrength, "meander noise strength used as a tie-breaker")
	fs.IntVar(&opts.River.VoronoiCellSize, "river-voronoi-cell-size", opts.River.VoronoiCellSize, "voronoi cell size in tiles used to shape river network")
	fs.Float64Var(&opts.River.VoronoiEdgeThreshold, "river-voronoi-edge-threshold", opts.River.VoronoiEdgeThreshold, "edge sensitivity for voronoi-guided channeling (>0)")
	fs.Float64Var(&opts.River.VoronoiSourceBoost, "river-voronoi-source-boost", opts.River.VoronoiSourceBoost, "extra source chance added on voronoi edges")
	fs.Float64Var(&opts.River.VoronoiBias, "river-voronoi-bias", opts.River.VoronoiBias, "downhill step bias towards voronoi edges")
	fs.Float64Var(&opts.River.SinkLakeChance, "river-sink-lake-chance", opts.River.SinkLakeChance, "chance [0..1] to form a sink lake when sink flow threshold is met")
	fs.IntVar(&opts.River.LakeMinSize, "river-lake-min-size", opts.River.LakeMinSize, "minimum inland lake size in tiles for lake-link generation")
	fs.Float64Var(&opts.River.LakeConnectChance, "river-lake-connect-chance", opts.River.LakeConnectChance, "chance [0..1] for each inland lake to create an outgoing link")
	fs.IntVar(&opts.River.LakeConnectionLimit, "river-lake-connection-limit", opts.River.LakeConnectionLimit, "maximum number of generated lake-to-lake river links")
	fs.IntVar(&opts.River.LakeLinkMinDistance, "river-lake-link-min-distance", opts.River.LakeLinkMinDistance, "minimum lake center distance in tiles for linking")
	fs.IntVar(&opts.River.LakeLinkMaxDistance, "river-lake-link-max-distance", opts.River.LakeLinkMaxDistance, "maximum lake center distance in tiles for linking")
	fs.IntVar(&opts.River.RiverWidthMin, "river-width-min", opts.River.RiverWidthMin, "minimum generated river width in tiles")
	fs.IntVar(&opts.River.RiverWidthMax, "river-width-max", opts.River.RiverWidthMax, "maximum generated river width in tiles")
	fs.BoolVar(&opts.River.GridEnabled, "river-grid-enabled", opts.River.GridEnabled, "enable uniform grid river stage")
	fs.IntVar(&opts.River.GridSpacing, "river-grid-spacing", opts.River.GridSpacing, "grid spacing in tiles for uniform river distribution")
	fs.IntVar(&opts.River.GridJitter, "river-grid-jitter", opts.River.GridJitter, "per-step jitter in tiles applied to grid rivers")
	fs.IntVar(&opts.River.TrunkRiverCount, "river-trunk-count", opts.River.TrunkRiverCount, "number of major trunk rivers routed to coastline")
	fs.Float64Var(&opts.River.TrunkSourceElevation, "river-trunk-source-elevation-min", opts.River.TrunkSourceElevation, "minimum elevation [0..1] for trunk river sources")
	fs.IntVar(&opts.River.TrunkMinLength, "river-trunk-min-length", opts.River.TrunkMinLength, "minimum path length in tiles for trunk river carving")
	fs.Float64Var(&opts.River.CoastSampleChance, "river-coast-sample-chance", opts.River.CoastSampleChance, "chance [0..1] to keep a coastline tile as trunk target sample")
	fs.IntVar(&opts.River.FlowShallowThreshold, "river-flow-shallow-threshold", opts.River.FlowShallowThreshold, "flow threshold for shallow river tiles")
	fs.IntVar(&opts.River.FlowDeepThreshold, "river-flow-deep-threshold", opts.River.FlowDeepThreshold, "flow threshold for deep river tiles")
	fs.IntVar(&opts.River.BankRadius, "river-bank-radius", opts.River.BankRadius, "bank expansion radius around deep river tiles")
	fs.IntVar(&opts.River.LakeFlowThreshold, "river-lake-flow-threshold", opts.River.LakeFlowThreshold, "minimum sink flow needed to form a lake")

	fs.BoolVar(&opts.PNG.Export, "png-export", opts.PNG.Export, "export generated chunks and overview image to PNG")
	fs.StringVar(&opts.PNG.OutputDir, "png-dir", opts.PNG.OutputDir, "output directory for PNG exports")
	fs.IntVar(&opts.PNG.Scale, "png-scale", opts.PNG.Scale, "PNG scale in pixels per tile")
	fs.BoolVar(&opts.PNG.HighlightRivers, "png-highlight-rivers", opts.PNG.HighlightRivers, "highlight river channels in PNG exports")

	if err := fs.Parse(args); err != nil {
		return MapgenOptions{}, err
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
		return errors.New("river-source-elevation-min must be within [0,1]")
	}
	if o.River.SourceChance < 0 || o.River.SourceChance > 1 {
		return errors.New("river-source-chance must be within [0,1]")
	}
	if o.River.VoronoiCellSize <= 1 {
		return errors.New("river-voronoi-cell-size must be > 1")
	}
	if o.River.VoronoiEdgeThreshold <= 0 {
		return errors.New("river-voronoi-edge-threshold must be > 0")
	}
	if o.River.VoronoiSourceBoost < 0 || o.River.VoronoiSourceBoost > 1 {
		return errors.New("river-voronoi-source-boost must be within [0,1]")
	}
	if o.River.VoronoiBias < 0 || o.River.VoronoiBias > 1 {
		return errors.New("river-voronoi-bias must be within [0,1]")
	}
	if o.River.SinkLakeChance < 0 || o.River.SinkLakeChance > 1 {
		return errors.New("river-sink-lake-chance must be within [0,1]")
	}
	if o.River.LakeMinSize <= 0 {
		return errors.New("river-lake-min-size must be > 0")
	}
	if o.River.LakeConnectChance < 0 || o.River.LakeConnectChance > 1 {
		return errors.New("river-lake-connect-chance must be within [0,1]")
	}
	if o.River.LakeConnectionLimit < 0 {
		return errors.New("river-lake-connection-limit must be >= 0")
	}
	if o.River.LakeLinkMinDistance < 0 {
		return errors.New("river-lake-link-min-distance must be >= 0")
	}
	if o.River.LakeLinkMaxDistance <= 0 {
		return errors.New("river-lake-link-max-distance must be > 0")
	}
	if o.River.LakeLinkMaxDistance <= o.River.LakeLinkMinDistance {
		return errors.New("river-lake-link-max-distance must be > river-lake-link-min-distance")
	}
	if o.River.RiverWidthMin <= 0 {
		return errors.New("river-width-min must be > 0")
	}
	if o.River.RiverWidthMax < o.River.RiverWidthMin {
		return errors.New("river-width-max must be >= river-width-min")
	}
	if o.River.GridSpacing <= 0 {
		return errors.New("river-grid-spacing must be > 0")
	}
	if o.River.GridJitter < 0 {
		return errors.New("river-grid-jitter must be >= 0")
	}
	if o.River.GridJitter >= o.River.GridSpacing/2 {
		return errors.New("river-grid-jitter must be < river-grid-spacing/2")
	}
	if o.River.TrunkRiverCount < 0 {
		return errors.New("river-trunk-count must be >= 0")
	}
	if o.River.TrunkSourceElevation < 0 || o.River.TrunkSourceElevation > 1 {
		return errors.New("river-trunk-source-elevation-min must be within [0,1]")
	}
	if o.River.TrunkMinLength <= 0 {
		return errors.New("river-trunk-min-length must be > 0")
	}
	if o.River.CoastSampleChance <= 0 || o.River.CoastSampleChance > 1 {
		return errors.New("river-coast-sample-chance must be within (0,1]")
	}
	if o.River.FlowShallowThreshold <= 0 {
		return errors.New("river-flow-shallow-threshold must be > 0")
	}
	if o.River.FlowDeepThreshold <= o.River.FlowShallowThreshold {
		return errors.New("river-flow-deep-threshold must be > river-flow-shallow-threshold")
	}
	if o.River.BankRadius < 0 {
		return errors.New("river-bank-radius must be >= 0")
	}
	if o.River.LakeFlowThreshold < o.River.FlowShallowThreshold {
		return errors.New("river-lake-flow-threshold must be >= river-flow-shallow-threshold")
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

	estimatedBytes, err := estimatePrecomputeBytes(tileCount, o.River.Enabled)
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

func estimatePrecomputeBytes(tileCount uint64, riverEnabled bool) (uint64, error) {
	bytesPerTile := uint64(5) // elevation float32 + final tile byte
	if riverEnabled {
		bytesPerTile += 5 // flow uint32 + river class byte
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
