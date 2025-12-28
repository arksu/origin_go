package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"

	"origin/internal/config"
	"origin/internal/db"
	"origin/internal/game"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// PerlinNoise generates 2D Perlin noise
type PerlinNoise struct {
	perm []int
}

func NewPerlinNoise(seed int64) *PerlinNoise {
	rng := rand.New(rand.NewSource(seed))
	p := make([]int, 512)
	for i := 0; i < 256; i++ {
		p[i] = i
	}
	// Shuffle
	for i := 255; i > 0; i-- {
		j := rng.Intn(i + 1)
		p[i], p[j] = p[j], p[i]
	}
	// Duplicate
	for i := 0; i < 256; i++ {
		p[256+i] = p[i]
	}
	return &PerlinNoise{perm: p}
}

func (pn *PerlinNoise) fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func (pn *PerlinNoise) lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}

func (pn *PerlinNoise) grad(hash int, x, y float64) float64 {
	h := hash & 15
	u := x
	if h >= 8 {
		u = y
	}
	v := y
	if h >= 8 {
		v = x
	}
	var gu, gv float64
	if h&1 == 0 {
		gu = u
	} else {
		gu = -u
	}
	if h&2 == 0 {
		gv = v
	} else {
		gv = -v
	}
	return gu + gv
}

func (pn *PerlinNoise) Noise(x, y float64) float64 {
	X := int(math.Floor(x)) & 255
	Y := int(math.Floor(y)) & 255
	x -= math.Floor(x)
	y -= math.Floor(y)
	u := pn.fade(x)
	v := pn.fade(y)

	a := pn.perm[X] + Y
	b := pn.perm[X+1] + Y

	return pn.lerp(v,
		pn.lerp(u, pn.grad(pn.perm[a], x, y), pn.grad(pn.perm[b], x-1, y)),
		pn.lerp(u, pn.grad(pn.perm[a+1], x, y-1), pn.grad(pn.perm[b+1], x-1, y-1)))
}

// OctaveNoise generates layered Perlin noise
func (pn *PerlinNoise) OctaveNoise(x, y float64, octaves int, persistence float64) float64 {
	total := 0.0
	frequency := 1.0
	amplitude := 1.0
	maxValue := 0.0

	for i := 0; i < octaves; i++ {
		total += pn.Noise(x*frequency, y*frequency) * amplitude
		maxValue += amplitude
		amplitude *= persistence
		frequency *= 2
	}

	return total / maxValue
}

func getTileType(elevation float64) game.TileType {
	if elevation < 0.3 {
		return game.TileWaterDeep
	} else if elevation < 0.4 {
		return game.TileWater
	} else if elevation < 0.45 {
		return game.TileSand
	} else if elevation < 0.65 {
		return game.TileGrass
	} else if elevation < 0.8 {
		return game.TileForestPine
	}
	return game.TileStone
}

func generateChunk(perlin *PerlinNoise, region, chunkX, chunkY, layer int) []byte {
	// Create tile data buffer
	// Each tile: 1 byte type + 2 bytes data = 3 bytes per tile
	chunkSize := game.CHUNK_SIZE
	tileCount := chunkSize * chunkSize
	data := make([]byte, tileCount*3)

	for y := 0; y < chunkSize; y++ {
		for x := 0; x < chunkSize; x++ {
			// World coordinates
			worldX := float64(chunkX*chunkSize + x)
			worldY := float64(chunkY*chunkSize + y)

			// Generate elevation using Perlin noise
			elevation := perlin.OctaveNoise(worldX*0.01, worldY*0.01, 4, 0.5)
			elevation = (elevation + 1) / 2 // Normalize to 0-1

			tileType := getTileType(elevation)

			// Write tile data
			idx := (y*chunkSize + x) * 3
			data[idx] = byte(tileType)
			binary.LittleEndian.PutUint16(data[idx+1:], 0) // extra data
		}
	}

	return data
}

func main() {
	cfg := config.Load()

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	// Set schema
	_, err = pool.Exec(ctx, fmt.Sprintf("SET search_path TO %s", cfg.DBSchema))
	if err != nil {
		log.Fatalf("Unable to set schema: %v", err)
	}

	// Initialize database queries with database/sql compatible connection
	sqlDB := stdlib.OpenDBFromPool(pool)
	queries := db.New(sqlDB)

	// Truncate all chunks before start
	log.Println("Truncating existing chunks...")
	err = queries.TruncateChunks(ctx)
	if err != nil {
		log.Fatalf("Unable to truncate chunks: %v", err)
	}

	// Initialize Perlin noise
	perlin := NewPerlinNoise(12345)

	region := cfg.RegionID
	layer := 0

	// Generate chunks for the region
	for chunkY := 0; chunkY < cfg.RegionHeightChunks; chunkY++ {
		for chunkX := 0; chunkX < cfg.RegionWidthChunks; chunkX++ {
			log.Printf("Generating chunk (%d, %d) for region %d", chunkX, chunkY, region)

			data := generateChunk(perlin, region, chunkX, chunkY, layer)

			// Save to database using SaveChunk sqlc function
			err := queries.SaveChunk(ctx, db.SaveChunkParams{
				Region:   int32(region),
				X:        int32(chunkX),
				Y:        int32(chunkY),
				Layer:    int32(layer),
				LastTick: 0,
				Data:     data,
			})

			if err != nil {
				log.Fatalf("Failed to save chunk (%d, %d): %v", chunkX, chunkY, err)
			}
		}
	}

	log.Printf("Successfully generated %d x %d chunks for region %d",
		cfg.RegionWidthChunks, cfg.RegionHeightChunks, region)
}
