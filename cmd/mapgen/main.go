package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	_const "origin/internal/const"
	"origin/internal/game/behaviors"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"os"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
)

const (
	TreeDensity    = 0.05
	BoulderDensity = 0.0012
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	opts, err := ParseMapgenOptions(os.Args[1:])
	if err != nil {
		logger.Fatal("invalid mapgen options", zap.Error(err))
	}
	if opts.Seed == 0 {
		opts.Seed = time.Now().UnixNano()
	}

	cfg := config.MustLoad(logger)

	itemRegistry, err := itemdefs.LoadFromDirectory("./data/items", logger)
	if err != nil {
		logger.Fatal("failed to load item definitions", zap.Error(err))
	}
	itemdefs.SetGlobal(itemRegistry)

	behaviorRegistry, err := behaviors.DefaultRegistry()
	if err != nil {
		logger.Fatal("failed to initialize behavior registry", zap.Error(err))
	}

	objRegistry, err := objectdefs.LoadFromDirectory("./data/objects", behaviorRegistry, logger)
	if err != nil {
		logger.Fatal("failed to load object definitions", zap.Error(err))
	}

	ctx := context.Background()
	db, err := persistence.NewPostgres(ctx, &cfg.Database, logger)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	perlin := NewPerlinNoise(opts.Seed)
	gen := &MapGenerator{
		cfg:          cfg,
		db:           db,
		logger:       logger,
		chunkSize:    _const.ChunkSize,
		coordPerTile: _const.CoordPerTile,
		region:       cfg.Game.Region,
		perlin:       perlin,
		seed:         opts.Seed,
		objectDefs:   objRegistry,
		options:      opts,
		noiseFields:  NewNoiseFields(perlin, _const.CoordPerTile),
	}

	logger.Info("starting map generation",
		zap.Int("chunks_x", opts.ChunksX),
		zap.Int("chunks_y", opts.ChunksY),
		zap.Int64("seed", opts.Seed),
		zap.Int("region", cfg.Game.Region),
		zap.Int("threads", opts.Threads),
		zap.Bool("river_enabled", opts.River.Enabled),
		zap.Bool("river_layout_draw", opts.River.LayoutDraw),
		zap.Int("river_major_count", opts.River.MajorRiverCount),
		zap.Int("river_lake_count", opts.River.LakeCount),
		zap.Float64("river_lake_border_mix", opts.River.LakeBorderMix),
		zap.Int("river_max_lake_degree", opts.River.MaxLakeDegree),
		zap.Float64("river_source_elevation_min", opts.River.SourceElevationMin),
		zap.Float64("river_source_chance", opts.River.SourceChance),
		zap.Float64("river_meander_strength", opts.River.MeanderStrength),
		zap.Int("river_voronoi_cell_size", opts.River.VoronoiCellSize),
		zap.Float64("river_voronoi_edge_threshold", opts.River.VoronoiEdgeThreshold),
		zap.Float64("river_voronoi_source_boost", opts.River.VoronoiSourceBoost),
		zap.Float64("river_voronoi_bias", opts.River.VoronoiBias),
		zap.Float64("river_sink_lake_chance", opts.River.SinkLakeChance),
		zap.Int("river_lake_min_size", opts.River.LakeMinSize),
		zap.Float64("river_lake_connect_chance", opts.River.LakeConnectChance),
		zap.Int("river_lake_connection_limit", opts.River.LakeConnectionLimit),
		zap.Int("river_lake_link_min_distance", opts.River.LakeLinkMinDistance),
		zap.Int("river_lake_link_max_distance", opts.River.LakeLinkMaxDistance),
		zap.Int("river_width_min", opts.River.RiverWidthMin),
		zap.Int("river_width_max", opts.River.RiverWidthMax),
		zap.Bool("river_grid_enabled", opts.River.GridEnabled),
		zap.Int("river_grid_spacing", opts.River.GridSpacing),
		zap.Int("river_grid_jitter", opts.River.GridJitter),
		zap.Int("river_trunk_count", opts.River.TrunkRiverCount),
		zap.Float64("river_trunk_source_elevation_min", opts.River.TrunkSourceElevation),
		zap.Int("river_trunk_min_length", opts.River.TrunkMinLength),
		zap.Float64("river_coast_sample_chance", opts.River.CoastSampleChance),
		zap.Int("river_flow_shallow_threshold", opts.River.FlowShallowThreshold),
		zap.Int("river_flow_deep_threshold", opts.River.FlowDeepThreshold),
		zap.Int("river_bank_radius", opts.River.BankRadius),
		zap.Int("river_lake_flow_threshold", opts.River.LakeFlowThreshold),
		zap.Bool("png_export", opts.PNG.Export),
		zap.String("png_dir", opts.PNG.OutputDir),
		zap.Int("png_scale", opts.PNG.Scale),
		zap.Bool("png_highlight_rivers", opts.PNG.HighlightRivers),
	)

	if err := gen.Generate(ctx); err != nil {
		logger.Fatal("map generation failed", zap.Error(err))
	}

	logger.Info("map generation completed")
}

type MapGenerator struct {
	cfg          *config.Config
	db           *persistence.Postgres
	logger       *zap.Logger
	chunkSize    int
	coordPerTile int
	region       int
	perlin       *PerlinNoise
	noiseFields  *NoiseFields
	seed         int64
	objectDefs   *objectdefs.Registry
	options      MapgenOptions
	terrain      *TerrainPrecompute

	lastEntityID atomic.Uint64
	generated    atomic.Int32
}

type ChunkTask struct {
	X, Y int
}

func (g *MapGenerator) Generate(ctx context.Context) error {
	g.logger.Info("precomputing terrain")
	terrain, err := BuildTerrainPrecompute(g.options, g.chunkSize, g.noiseFields)
	if err != nil {
		return fmt.Errorf("precompute terrain: %w", err)
	}
	g.terrain = terrain

	g.logger.Info("terrain precompute completed",
		zap.Int("width_tiles", terrain.WidthTiles),
		zap.Int("height_tiles", terrain.HeightTiles),
		zap.Int("river_sources", terrain.RiverSources),
		zap.Int("river_shallow_tiles", terrain.RiverShallowTiles),
		zap.Int("river_deep_tiles", terrain.RiverDeepTiles),
	)

	if g.options.PNG.Export {
		g.logger.Info("exporting map pngs",
			zap.String("dir", g.options.PNG.OutputDir),
			zap.Int("scale", g.options.PNG.Scale),
		)
		if err := ExportMapPNGs(terrain.Tiles, terrain.RiverClass, terrain.WidthTiles, terrain.HeightTiles, g.chunkSize, g.options.PNG); err != nil {
			return fmt.Errorf("export map pngs: %w", err)
		}
	}

	g.logger.Info("truncating existing data for region", zap.Int("region", g.region))
	if err := g.db.Queries().DeleteChunksByRegion(ctx, g.region); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if err := g.db.Queries().HardDeleteObjectsByRegion(ctx, g.region); err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	g.lastEntityID.Store(uint64(g.db.GetGlobalVarLong(ctx, "last_used_id")))
	g.logger.Info("loaded last entity ID", zap.Uint64("last_entity_id", g.lastEntityID.Load()))

	totalChunks := g.options.ChunksX * g.options.ChunksY
	g.generated.Store(0)

	tasks := make(chan ChunkTask, g.options.Threads*2)
	errs := make(chan error, g.options.Threads)

	for i := 0; i < g.options.Threads; i++ {
		go g.worker(ctx, tasks, errs)
	}

	go func() {
		defer close(tasks)
		for cy := 0; cy < g.options.ChunksY; cy++ {
			for cx := 0; cx < g.options.ChunksX; cx++ {
				tasks <- ChunkTask{X: cx, Y: cy}
			}
		}
	}()

	completed := 0
	for completed < totalChunks {
		select {
		case err := <-errs:
			return err
		default:
			if g.generated.Load() > int32(completed) {
				completed = int(g.generated.Load())
				if completed%10 == 0 || completed == totalChunks {
					g.logger.Info("progress", zap.Int("generated", completed), zap.Int("total", totalChunks))
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	if err := g.db.SetGlobalVarLong(ctx, "last_used_id", int64(g.lastEntityID.Load())); err != nil {
		return fmt.Errorf("save last entity ID: %w", err)
	}
	g.logger.Info("saved last entity ID", zap.Uint64("last_entity_id", g.lastEntityID.Load()))

	return nil
}

func (g *MapGenerator) worker(ctx context.Context, tasks <-chan ChunkTask, errs chan<- error) {
	for task := range tasks {
		if err := g.generateChunk(ctx, task.X, task.Y); err != nil {
			errs <- fmt.Errorf("generate chunk (%d,%d): %w", task.X, task.Y, err)
			return
		}
		g.generated.Add(1)
	}
}

func (g *MapGenerator) generateChunk(ctx context.Context, chunkX, chunkY int) error {
	seed := deterministicChunkSeed(g.seed, chunkX, chunkY)
	rng := rand.New(rand.NewSource(seed))
	return g.generateChunkWithRNG(ctx, chunkX, chunkY, rng)
}

func deterministicChunkSeed(seed int64, chunkX, chunkY int) int64 {
	mix := uint64(seed)
	mix ^= uint64(int64(chunkX)) * 0x9E3779B185EBCA87
	mix ^= uint64(int64(chunkY)) * 0xC2B2AE3D27D4EB4F
	mixed := splitMix64(mix)
	if mixed == 0 {
		mixed = 1
	}
	return int64(mixed & math.MaxInt64)
}

func (g *MapGenerator) generateChunkWithRNG(ctx context.Context, chunkX, chunkY int, rng *rand.Rand) error {
	if g.terrain == nil {
		return fmt.Errorf("terrain precompute is not initialized")
	}

	tilesPerChunk := g.chunkSize
	tiles := make([]byte, tilesPerChunk*tilesPerChunk)
	var entities []repository.UpsertObjectParams

	worldOffsetX := float64(chunkX * g.chunkSize * g.coordPerTile)
	worldOffsetY := float64(chunkY * g.chunkSize * g.coordPerTile)

	for ty := 0; ty < g.chunkSize; ty++ {
		for tx := 0; tx < g.chunkSize; tx++ {
			globalTileX := chunkX*g.chunkSize + tx
			globalTileY := chunkY*g.chunkSize + ty
			tile := g.terrain.Tiles[tileIndex(globalTileX, globalTileY, g.terrain.WidthTiles)]
			tiles[ty*g.chunkSize+tx] = tile

			if treeDef := g.treeDefForTile(tile); treeDef != nil && rng.Float64() < TreeDensity {
				entityID := g.lastEntityID.Add(1)
				tileWorldX := int(worldOffsetX) + tx*g.coordPerTile + rng.Intn(g.coordPerTile)
				tileWorldY := int(worldOffsetY) + ty*g.coordPerTile + rng.Intn(g.coordPerTile)

				entities = append(entities, repository.UpsertObjectParams{
					ID:         int64(entityID),
					TypeID:     treeDef.DefID,
					Region:     g.region,
					X:          tileWorldX,
					Y:          tileWorldY,
					Layer:      0,
					ChunkX:     chunkX,
					ChunkY:     chunkY,
					Heading:    sql.NullInt16{Int16: int16(rng.Intn(8)), Valid: true},
					Quality:    10,
					Hp:         sql.NullInt32{Int32: int32(treeDef.HP), Valid: true},
					OwnerID:    sql.NullInt64{},
					CreateTick: 0,
					LastTick:   0,
				})
			} else if boulderDef := g.boulderDefForTile(tile); boulderDef != nil && rng.Float64() < BoulderDensity {
				entityID := g.lastEntityID.Add(1)
				tileWorldX := int(worldOffsetX) + tx*g.coordPerTile + rng.Intn(g.coordPerTile)
				tileWorldY := int(worldOffsetY) + ty*g.coordPerTile + rng.Intn(g.coordPerTile)

				entities = append(entities, repository.UpsertObjectParams{
					ID:         int64(entityID),
					TypeID:     boulderDef.DefID,
					Region:     g.region,
					X:          tileWorldX,
					Y:          tileWorldY,
					Layer:      0,
					ChunkX:     chunkX,
					ChunkY:     chunkY,
					Heading:    sql.NullInt16{Int16: int16(rng.Intn(8)), Valid: true},
					Quality:    10,
					Hp:         sql.NullInt32{Int32: int32(boulderDef.HP), Valid: true},
					OwnerID:    sql.NullInt64{},
					CreateTick: 0,
					LastTick:   0,
				})
			}
		}
	}

	if err := g.db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{
		Region:      g.region,
		X:           chunkX,
		Y:           chunkY,
		Layer:       0,
		TilesData:   tiles,
		LastTick:    0,
		EntityCount: sql.NullInt32{Int32: int32(len(entities)), Valid: true},
	}); err != nil {
		return fmt.Errorf("upsert chunk: %w", err)
	}

	for _, entity := range entities {
		if err := g.db.Queries().UpsertObject(ctx, entity); err != nil {
			return fmt.Errorf("upsert object: %w", err)
		}
	}

	return nil
}

func (g *MapGenerator) boulderDefForTile(tile byte) *objectdefs.ObjectDef {
	switch tile {
	case types.TileGrass, types.TileSand:
		def, _ := g.objectDefs.GetByKey("boulder")
		return def
	}
	return nil
}

func (g *MapGenerator) treeDefForTile(tile byte) *objectdefs.ObjectDef {
	switch tile {
	case types.TileConiferousForest:
		def, _ := g.objectDefs.GetByKey("tree_birch")
		return def
	case types.TileBroadleafForest:
		def, _ := g.objectDefs.GetByKey("tree_birch")
		return def
	}
	return nil
}

type PerlinNoise struct {
	perm [512]int
}

func NewPerlinNoise(seed int64) *PerlinNoise {
	p := &PerlinNoise{}
	rng := rand.New(rand.NewSource(seed))

	perm := make([]int, 256)
	for i := range perm {
		perm[i] = i
	}
	rng.Shuffle(len(perm), func(i, j int) {
		perm[i], perm[j] = perm[j], perm[i]
	})

	for i := 0; i < 512; i++ {
		p.perm[i] = perm[i%256]
	}

	return p
}

func (p *PerlinNoise) Noise2D(x, y float64) float64 {
	X := int(math.Floor(x)) & 255
	Y := int(math.Floor(y)) & 255

	x -= math.Floor(x)
	y -= math.Floor(y)

	u := fade(x)
	v := fade(y)

	A := p.perm[X] + Y
	B := p.perm[X+1] + Y

	return lerp(v,
		lerp(u, grad(p.perm[A], x, y), grad(p.perm[B], x-1, y)),
		lerp(u, grad(p.perm[A+1], x, y-1), grad(p.perm[B+1], x-1, y-1)))
}

func fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}

func grad(hash int, x, y float64) float64 {
	h := hash & 3
	switch h {
	case 0:
		return x + y
	case 1:
		return -x + y
	case 2:
		return x - y
	default:
		return -x - y
	}
}
