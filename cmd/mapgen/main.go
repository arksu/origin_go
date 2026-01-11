package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"origin/internal/utils"
)

const (
	ObjectTypeTree = 1
	TreeHP         = 100
	TreeDensity    = 0.15
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	var (
		chunksX = flag.Int("chunks-x", 10, "number of chunks in X direction")
		chunksY = flag.Int("chunks-y", 10, "number of chunks in Y direction")
		seed    = flag.Int64("seed", 0, "random seed (0 = use current time)")
	)
	flag.Parse()

	cfg := config.MustLoad(logger)

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(*seed))

	ctx := context.Background()
	db, err := persistence.NewPostgres(ctx, &cfg.Database, logger)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	gen := &MapGenerator{
		cfg:          cfg,
		db:           db,
		logger:       logger,
		rng:          rng,
		chunkSize:    utils.ChunkSize,
		coordPerTile: utils.CoordPerTile,
		region:       cfg.Game.Region,
		perlin:       NewPerlinNoise(*seed),
	}

	logger.Info("starting map generation",
		zap.Int("chunks_x", *chunksX),
		zap.Int("chunks_y", *chunksY),
		zap.Int64("seed", *seed),
		zap.Int32("region", cfg.Game.Region))

	if err := gen.Generate(ctx, *chunksX, *chunksY); err != nil {
		logger.Fatal("map generation failed", zap.Error(err))
	}

	logger.Info("map generation completed")
}

type MapGenerator struct {
	cfg          *config.Config
	db           *persistence.Postgres
	logger       *zap.Logger
	rng          *rand.Rand
	chunkSize    int
	coordPerTile int
	region       int32
	perlin       *PerlinNoise
	lastEntityID uint64
}

func (g *MapGenerator) Generate(ctx context.Context, chunksX, chunksY int) error {
	g.logger.Info("truncating existing data for region", zap.Int32("region", g.region))
	if err := g.db.Queries().DeleteChunksByRegion(ctx, g.region); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if err := g.db.Queries().HardDeleteObjectsByRegion(ctx, g.region); err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	g.lastEntityID = uint64(g.db.GetGlobalVarLong(ctx, "last_used_id"))
	g.logger.Info("loaded last entity ID", zap.Uint64("last_entity_id", g.lastEntityID))

	totalChunks := chunksX * chunksY
	generated := 0

	for cy := 0; cy < chunksY; cy++ {
		for cx := 0; cx < chunksX; cx++ {
			if err := g.generateChunk(ctx, int32(cx), int32(cy)); err != nil {
				return fmt.Errorf("generate chunk (%d,%d): %w", cx, cy, err)
			}
			generated++
			if generated%10 == 0 {
				g.logger.Info("progress", zap.Int("generated", generated), zap.Int("total", totalChunks))
			}
		}
	}

	if err := g.db.SetGlobalVarLong(ctx, "last_used_id", int64(g.lastEntityID)); err != nil {
		return fmt.Errorf("save last entity ID: %w", err)
	}
	g.logger.Info("saved last entity ID", zap.Uint64("last_entity_id", g.lastEntityID))

	return nil
}

func (g *MapGenerator) generateChunk(ctx context.Context, chunkX, chunkY int32) error {
	tilesPerChunk := g.chunkSize
	tiles := make([]byte, tilesPerChunk*tilesPerChunk)
	var entities []repository.UpsertObjectParams

	worldOffsetX := float64(int(chunkX) * g.chunkSize * g.coordPerTile)
	worldOffsetY := float64(int(chunkY) * g.chunkSize * g.coordPerTile)

	for ty := 0; ty < g.chunkSize; ty++ {
		for tx := 0; tx < g.chunkSize; tx++ {
			worldX := worldOffsetX + float64(tx*g.coordPerTile)
			worldY := worldOffsetY + float64(ty*g.coordPerTile)

			tile := g.getTileType(worldX, worldY)
			tiles[ty*g.chunkSize+tx] = tile

			if (tile == types.TileForestPine || tile == types.TileForestLeaf) && g.rng.Float64() < TreeDensity {
				g.lastEntityID++
				tileWorldX := int32(worldOffsetX) + int32(tx*g.coordPerTile) + int32(g.rng.Intn(g.coordPerTile))
				tileWorldY := int32(worldOffsetY) + int32(ty*g.coordPerTile) + int32(g.rng.Intn(g.coordPerTile))

				entities = append(entities, repository.UpsertObjectParams{
					ID:         int64(g.lastEntityID),
					ObjectType: ObjectTypeTree,
					Region:     g.region,
					X:          tileWorldX,
					Y:          tileWorldY,
					Layer:      0,
					ChunkX:     chunkX,
					ChunkY:     chunkY,
					Heading:    sql.NullInt16{Int16: int16(g.rng.Intn(8)), Valid: true},
					Quality:    sql.NullInt16{},
					HpCurrent:  sql.NullInt32{Int32: TreeHP, Valid: true},
					HpMax:      sql.NullInt32{Int32: TreeHP, Valid: true},
					IsStatic:   sql.NullBool{Bool: true, Valid: true},
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

func (g *MapGenerator) getTileType(worldX, worldY float64) byte {
	scale := 0.002

	elevation := g.perlin.Noise2D(worldX*scale, worldY*scale)
	moisture := g.perlin.Noise2D(worldX*scale*0.5+1000, worldY*scale*0.5+1000)
	temperature := g.perlin.Noise2D(worldX*scale*0.3+2000, worldY*scale*0.3+2000)

	elevation = (elevation + 1) / 2
	moisture = (moisture + 1) / 2
	temperature = (temperature + 1) / 2

	if elevation < 0.25 {
		return types.TileWaterDeep
	}
	if elevation < 0.35 {
		return types.TileWater
	}

	if elevation < 0.42 {
		return types.TileSand
	}

	if moisture > 0.6 {
		if temperature > 0.5 {
			return types.TileForestLeaf
		}
		return types.TileForestPine
	}

	return types.TileGrass
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
