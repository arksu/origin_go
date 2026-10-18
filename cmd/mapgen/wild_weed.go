package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"

	_const "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
)

const (
	wildWeedObjectKey = "wild_windsown_weed"
	wildWeedMinGap    = 20
	wildWeedSeedSalt  = uint64(0xE8BA4A1963D5702F)
	wildWeedChance    = 0.0003
)

type wildWeedPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// This grid only lives while filling one spot. Queries inspect nine small
// buckets, rather than every object in the spot for every candidate tile.
type wildWeedPositionGrid map[[2]int][]wildWeedPosition

func wildWeedCell(value int) int {
	cell := value / wildWeedMinGap
	if value < 0 && value%wildWeedMinGap != 0 {
		cell--
	}
	return cell
}

func (grid wildWeedPositionGrid) add(point wildWeedPosition) {
	key := [2]int{wildWeedCell(point.X), wildWeedCell(point.Y)}
	grid[key] = append(grid[key], point)
}

func (grid wildWeedPositionGrid) allows(point wildWeedPosition) bool {
	x, y := wildWeedCell(point.X), wildWeedCell(point.Y)
	for cellY := y - 1; cellY <= y+1; cellY++ {
		for cellX := x - 1; cellX <= x+1; cellX++ {
			for _, other := range grid[[2]int{cellX, cellY}] {
				dx, dy := int64(point.X)-int64(other.X), int64(point.Y)-int64(other.Y)
				if dx*dx+dy*dy < wildWeedMinGap*wildWeedMinGap {
					return false
				}
			}
		}
	}
	return true
}

func (g *MapGenerator) wildWeedDefinition() (*objectdefs.ObjectDef, error) {
	if g.objectDefs != nil {
		if definition, found := g.objectDefs.GetByKey(wildWeedObjectKey); found {
			return definition, nil
		}
	}
	return nil, fmt.Errorf("mapgen requires object definition %q", wildWeedObjectKey)
}

func wildWeedTarget(spot repository.Spot, coordPerTile int) int {
	radiusTiles := float64(spot.Radius) / float64(coordPerTile)
	return int(math.Floor(wildWeedChance * math.Pi * radiusTiles * radiusTiles))
}

func wildWeedQuality(spot repository.Spot, point wildWeedPosition) (int16, bool) {
	dx, dy := int64(point.X)-int64(spot.CenterX), int64(point.Y)-int64(spot.CenterY)
	if dx*dx+dy*dy > int64(spot.Radius)*int64(spot.Radius) {
		return 0, false
	}
	distance := math.Hypot(float64(dx), float64(dy))
	return int16(10 + math.Floor(float64(spot.PeakQuality-10)*(1-distance/float64(spot.Radius)))), true
}

func wildWeedOwnsPoint(spot repository.Spot, overlaps []repository.Spot, point wildWeedPosition) bool {
	quality, inside := wildWeedQuality(spot, point)
	if !inside {
		return false
	}
	for _, other := range overlaps {
		otherQuality, otherInside := wildWeedQuality(other, point)
		if otherInside && (otherQuality > quality || otherQuality == quality &&
			(other.DistrictY < spot.DistrictY || other.DistrictY == spot.DistrictY && other.DistrictX < spot.DistrictX)) {
			return false
		}
	}
	return true
}

func wildWeedCandidates(ctx context.Context, terrain *TerrainPrecompute, spot repository.Spot,
	overlaps []repository.Spot, coordPerTile int, seed int64,
) ([]wildWeedPosition, error) {
	minX := max(0, (spot.CenterX-spot.Radius)/coordPerTile)
	minY := max(0, (spot.CenterY-spot.Radius)/coordPerTile)
	maxX := min(terrain.WidthTiles-1, (spot.CenterX+spot.Radius)/coordPerTile)
	maxY := min(terrain.HeightTiles-1, (spot.CenterY+spot.Radius)/coordPerTile)
	var candidates []wildWeedPosition
	for y := minY; y <= maxY; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := minX; x <= maxX; x++ {
			switch terrain.Tiles[tileIndex(x, y, terrain.WidthTiles)] {
			case 20, 25, 35, 50:
			default:
				continue
			}
			point := wildWeedPosition{X: x*coordPerTile + coordPerTile/2, Y: y*coordPerTile + coordPerTile/2}
			if wildWeedOwnsPoint(spot, overlaps, point) {
				candidates = append(candidates, point)
			}
		}
	}
	rng := rand.New(rand.NewSource(int64(spotCoordinateHash(seed, spot.DistrictX, spot.DistrictY, wildWeedSeedSalt))))
	rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	return candidates, nil
}

func (g *MapGenerator) loadWildWeedPositions(ctx context.Context, queries *repository.Queries,
	spot repository.Spot,
) (wildWeedPositionGrid, error) {
	chunkWorldSize := g.chunkSize * g.coordPerTile
	minX := max(0, spot.CenterX-spot.Radius-wildWeedMinGap) / chunkWorldSize
	minY := max(0, spot.CenterY-spot.Radius-wildWeedMinGap) / chunkWorldSize
	maxX := min(g.terrain.WidthTiles*g.coordPerTile-1, spot.CenterX+spot.Radius+wildWeedMinGap) / chunkWorldSize
	maxY := min(g.terrain.HeightTiles*g.coordPerTile-1, spot.CenterY+spot.Radius+wildWeedMinGap) / chunkWorldSize
	grid := make(wildWeedPositionGrid)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			objects, err := queries.GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{
				Region: g.region, Layer: spot.Layer, ChunkX: x, ChunkY: y,
			})
			if err != nil {
				return nil, fmt.Errorf("load WWW placement objects in chunk (%d,%d): %w", x, y, err)
			}
			for _, object := range objects {
				grid.add(wildWeedPosition{X: object.X, Y: object.Y})
			}
		}
	}
	return grid, nil
}

// The caller owns the final mapgen transaction. Ordinary objects, the spot's
// manifest, chunk counts and the ID watermark are committed together.
func (g *MapGenerator) writeWildWeeds(ctx context.Context, queries *repository.Queries) (int, error) {
	definition, err := g.wildWeedDefinition()
	if err != nil {
		return 0, err
	}
	spots, err := queries.LoadSpotsByRegion(ctx, g.region)
	if err != nil {
		return 0, fmt.Errorf("load generated spots: %w", err)
	}
	wwwSpots := make([]repository.Spot, 0)
	for _, spot := range spots {
		if spot.SpotType == "www" && spot.Layer == 0 {
			wwwSpots = append(wwwSpots, spot)
		}
	}
	sort.Slice(wwwSpots, func(i, j int) bool {
		left, right := wwwSpots[i], wwwSpots[j]
		return left.DistrictY < right.DistrictY || left.DistrictY == right.DistrictY && left.DistrictX < right.DistrictX
	})
	generated := 0
	for _, spot := range wwwSpots {
		var overlaps []repository.Spot
		for _, other := range wwwSpots {
			if other.ID != spot.ID && math.Hypot(float64(other.CenterX-spot.CenterX), float64(other.CenterY-spot.CenterY)) <= float64(other.Radius+spot.Radius) {
				overlaps = append(overlaps, other)
			}
		}
		candidates, err := wildWeedCandidates(ctx, g.terrain, spot, overlaps, g.coordPerTile, g.seed)
		if err != nil {
			return generated, err
		}
		grid, err := g.loadWildWeedPositions(ctx, queries, spot)
		if err != nil {
			return generated, err
		}
		state, err := json.Marshal(components.WildWeedBehaviorState{SpotID: spot.ID, Stage: 3})
		if err != nil {
			return generated, fmt.Errorf("encode WWW behavior state: %w", err)
		}
		data, err := json.Marshal(components.ObjectStateEnvelope{Version: 1,
			Behaviors: map[string]json.RawMessage{"wild_weed": state}})
		if err != nil {
			return generated, fmt.Errorf("encode WWW object state: %w", err)
		}
		objects := make(map[string]wildWeedPosition)
		chunkCounts := make(map[types.ChunkCoord]int)
		target := wildWeedTarget(spot, g.coordPerTile)
		for _, point := range candidates {
			if len(objects) >= target {
				break
			}
			if err := ctx.Err(); err != nil {
				return generated, err
			}
			if !grid.allows(point) {
				continue
			}
			id := g.lastEntityID.Add(1)
			quality, _ := wildWeedQuality(spot, point)
			coord := types.ChunkCoord{X: point.X / (g.chunkSize * g.coordPerTile), Y: point.Y / (g.chunkSize * g.coordPerTile)}
			if err := queries.UpsertObject(ctx, repository.UpsertObjectParams{
				ID: int64(id), TypeID: definition.DefID, Region: g.region, Layer: spot.Layer,
				X: point.X, Y: point.Y, ChunkX: coord.X, ChunkY: coord.Y,
				Heading: sql.NullInt16{Valid: true}, Quality: quality,
				Data: pqtype.NullRawMessage{RawMessage: data, Valid: true},
			}); err != nil {
				return generated, fmt.Errorf("save WWW object: %w", err)
			}
			grid.add(point)
			objects[strconv.FormatUint(id, 10)] = point
			chunkCounts[coord]++
			generated++
		}
		manifest, err := json.Marshal(map[string]any{
			"www": map[string]any{"objects": objects},
		})
		if err != nil {
			return generated, fmt.Errorf("encode WWW spot state: %w", err)
		}
		affected, err := queries.SaveSpotState(ctx, repository.SaveSpotStateParams{
			ID: spot.ID, Region: g.region, ExpectedRevision: spot.Revision,
			State: manifest, LastRuntimeSeconds: 0,
		})
		if err != nil {
			return generated, fmt.Errorf("save WWW spot state: %w", err)
		}
		if affected != 1 {
			return generated, fmt.Errorf("save WWW spot %d: expected one affected row, got %d", spot.ID, affected)
		}
		for coord, added := range chunkCounts {
			chunk, err := queries.GetChunk(ctx, repository.GetChunkParams{Region: g.region, Layer: spot.Layer, X: coord.X, Y: coord.Y})
			if err != nil {
				return generated, fmt.Errorf("load WWW chunk count: %w", err)
			}
			affected, err := queries.UpsertChunk(ctx, repository.UpsertChunkParams{
				Region: chunk.Region, X: chunk.X, Y: chunk.Y, Layer: chunk.Layer,
				TilesData: chunk.TilesData, LastTick: chunk.LastTick, Version: chunk.Version,
				EntityCount: sql.NullInt32{Int32: chunk.EntityCount.Int32 + int32(added), Valid: true},
			})
			if err != nil {
				return generated, fmt.Errorf("save WWW chunk count: %w", err)
			}
			if affected != 1 {
				return generated, fmt.Errorf("save WWW chunk count: expected one affected row, got %d", affected)
			}
		}
	}
	if err := queries.UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
		Name: _const.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: int64(g.lastEntityID.Load()), Valid: true},
	}); err != nil {
		return generated, fmt.Errorf("save WWW entity ID checkpoint: %w", err)
	}
	return generated, nil
}
