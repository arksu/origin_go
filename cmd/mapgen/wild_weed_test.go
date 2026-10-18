package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"testing"

	_const "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestWildWeedCandidatesFinalTerrainAndWorldBoundary(t *testing.T) {
	terrain := &TerrainPrecompute{WidthTiles: 8, HeightTiles: 1,
		Tiles: []byte{20, 25, 35, 50, types.TileDeepWater, types.TileShallowWater, types.TileSand, types.TileDirt}}
	spot := repository.Spot{CenterX: 0, CenterY: 0, Radius: 1000, PeakQuality: 50}
	candidates, err := wildWeedCandidates(context.Background(), terrain, spot, nil, 12, 42)
	require.NoError(t, err)
	require.ElementsMatch(t, []wildWeedPosition{{6, 6}, {18, 6}, {30, 6}, {42, 6}}, candidates)
	for _, point := range candidates {
		require.GreaterOrEqual(t, point.X, 0)
		require.Less(t, point.X, terrain.WidthTiles*12)
		require.Less(t, point.Y, terrain.HeightTiles*12)
	}
}

func TestWildWeedCoordinateGap(t *testing.T) {
	grid := make(wildWeedPositionGrid)
	grid.add(wildWeedPosition{20, 20})
	for _, point := range []wildWeedPosition{{20, 20}, {1, 20}, {20, 1}, {6, 6}, {39, 20}} {
		require.False(t, grid.allows(point), "%+v is less than 20 away", point)
	}
	for _, point := range []wildWeedPosition{{0, 20}, {20, 0}, {40, 20}, {20, 40}, {8, 4}} {
		require.True(t, grid.allows(point), "%+v is at least 20 away", point)
	}
	grid.add(wildWeedPosition{-1, -1})
	// Negative neighboring buckets must obey the same Euclidean rule.
	require.False(t, grid.allows(wildWeedPosition{0, 0}))
	require.True(t, grid.allows(wildWeedPosition{-21, -1}))
}

func TestWildWeedTargetQualityAndOwnership(t *testing.T) {
	spot := repository.Spot{ID: 1, CenterX: 1000, CenterY: 1000, Radius: 64 * 12, PeakQuality: 50, DistrictX: 1}
	require.Equal(t, 128, wildWeedTarget(spot, 12))
	for _, test := range []struct {
		point   wildWeedPosition
		quality int16
		inside  bool
	}{
		{wildWeedPosition{1000, 1000}, 50, true},
		{wildWeedPosition{1384, 1000}, 30, true},
		{wildWeedPosition{1768, 1000}, 10, true},
		{wildWeedPosition{1769, 1000}, 0, false},
	} {
		quality, inside := wildWeedQuality(spot, test.point)
		require.Equal(t, test.inside, inside)
		require.Equal(t, test.quality, quality)
	}
	point := wildWeedPosition{1000, 1000}
	other := spot
	other.ID, other.DistrictX = 2, 0
	require.False(t, wildWeedOwnsPoint(spot, []repository.Spot{other}, point), "equal Q prefers earlier district")
	other.PeakQuality = 49
	require.True(t, wildWeedOwnsPoint(spot, []repository.Spot{other}, point), "higher Q beats district order")
	other.PeakQuality, other.DistrictX, other.DistrictY = 50, 9, -1
	require.False(t, wildWeedOwnsPoint(spot, []repository.Spot{other}, point), "district Y precedes X")
}

func TestWildWeedCandidateOrderIndependentOfSpotID(t *testing.T) {
	terrain := uniformTerrain(128, 128, 35)
	spot := repository.Spot{ID: 1, CenterX: 64 * 12, CenterY: 64 * 12, Radius: 64 * 12, PeakQuality: 50}
	first, err := wildWeedCandidates(context.Background(), terrain, spot, nil, 12, 42)
	require.NoError(t, err)
	spot.ID = 9007199254740993
	second, err := wildWeedCandidates(context.Background(), terrain, spot, nil, 12, 42)
	require.NoError(t, err)
	require.Equal(t, first, second)
	third, err := wildWeedCandidates(context.Background(), terrain, spot, nil, 12, 43)
	require.NoError(t, err)
	require.NotEqual(t, first, third)
	for _, point := range first {
		_, inside := wildWeedQuality(spot, point)
		require.True(t, inside)
	}
}

func TestWildWeedCandidatesCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := wildWeedCandidates(ctx, uniformTerrain(8, 8, 35),
		repository.Spot{CenterX: 30, CenterY: 30, Radius: 100, PeakQuality: 50}, nil, 12, 42)
	require.ErrorIs(t, err, context.Canceled)
}

func uniformTerrain(width, height int, tile byte) *TerrainPrecompute {
	terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: height, Tiles: make([]byte, width*height)}
	for index := range terrain.Tiles {
		terrain.Tiles[index] = tile
	}
	return terrain
}

func wildWeedPostgresGenerator(t *testing.T, terrain *TerrainPrecompute) (*MapGenerator, *persistence.Postgres) {
	t.Helper()
	db := testutil.NewPostgres(t, "ORIGIN_SPOTS_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	generator := &MapGenerator{
		db: db, chunkSize: _const.ChunkSize, coordPerTile: _const.CoordPerTile,
		region: 1, terrain: terrain, seed: 42,
		objectDefs: objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 41, Key: wildWeedObjectKey, HP: 100}}),
	}
	for y := 0; y < terrain.HeightTiles/_const.ChunkSize; y++ {
		for x := 0; x < terrain.WidthTiles/_const.ChunkSize; x++ {
			tiles := make([]byte, _const.ChunkSize*_const.ChunkSize)
			for tileY := 0; tileY < _const.ChunkSize; tileY++ {
				start := (y*_const.ChunkSize+tileY)*terrain.WidthTiles + x*_const.ChunkSize
				copy(tiles[tileY*_const.ChunkSize:], terrain.Tiles[start:start+_const.ChunkSize])
			}
			_, err := db.Queries().UpsertChunk(context.Background(), repository.UpsertChunkParams{
				Region: 1, X: x, Y: y, TilesData: tiles, EntityCount: sql.NullInt32{Valid: true},
			})
			require.NoError(t, err)
		}
	}
	return generator, db
}

func insertWildWeedSpot(ctx context.Context, queries *repository.Queries, center wildWeedPosition) error {
	return queries.InsertSpots(ctx, repository.InsertSpotsParams{
		Region: 1, SpotTypes: []string{"www"}, DistrictXs: []int{0}, DistrictYs: []int{0},
		CenterXs: []int{center.X}, CenterYs: []int{center.Y}, Radii: []int{64 * 12}, PeakQualities: []int16{50},
	})
}

func TestWildWeedMapgenPostgresManifestAndCheckpoint(t *testing.T) {
	generator, db := wildWeedPostgresGenerator(t, uniformTerrain(256, 128, 35))
	ctx := context.Background()
	// Above float64's exact integer range: manifest keys must retain every bit.
	const previousID = uint64(9007199254740992)
	generator.lastEntityID.Store(previousID)
	var generated int
	require.NoError(t, db.WithTx(ctx, func(queries *repository.Queries) error {
		if err := insertWildWeedSpot(ctx, queries, wildWeedPosition{1530, 774}); err != nil {
			return err
		}
		var err error
		generated, err = generator.writeWildWeeds(ctx, queries)
		return err
	}))
	require.Equal(t, 128, generated)
	spots, err := db.Queries().LoadSpotsByRegion(ctx, 1)
	require.NoError(t, err)
	require.Len(t, spots, 1)
	require.Equal(t, int64(2), spots[0].Revision)
	var manifest struct {
		WWW struct {
			Objects map[string]wildWeedPosition `json:"objects"`
		} `json:"www"`
	}
	require.NoError(t, json.Unmarshal(spots[0].State, &manifest))
	require.Len(t, manifest.WWW.Objects, generated)
	grid := make(wildWeedPositionGrid)
	for x := 0; x < 2; x++ {
		objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1, ChunkX: x})
		require.NoError(t, err)
		chunk, err := db.Queries().GetChunk(ctx, repository.GetChunkParams{Region: 1, X: x})
		require.NoError(t, err)
		require.Equal(t, int32(len(objects)), chunk.EntityCount.Int32)
		for _, object := range objects {
			point := wildWeedPosition{object.X, object.Y}
			require.Equal(t, point, manifest.WWW.Objects[strconv.FormatInt(object.ID, 10)])
			require.Greater(t, object.ID, int64(previousID))
			require.Equal(t, 41, object.TypeID)
			require.True(t, object.Data.Valid)
			require.False(t, object.Hp.Valid, "normal default HP comes from the definition")
			quality, inside := wildWeedQuality(spots[0], point)
			require.True(t, inside)
			require.Equal(t, quality, object.Quality)
			require.True(t, grid.allows(point), "WWW must also keep the gap from other WWW")
			grid.add(point)
			var envelope components.ObjectStateEnvelope
			require.NoError(t, json.Unmarshal(object.Data.RawMessage, &envelope))
			require.Equal(t, 1, envelope.Version)
			var state components.WildWeedBehaviorState
			require.NoError(t, json.Unmarshal(envelope.Behaviors["wild_weed"], &state))
			require.Equal(t, spots[0].ID, state.SpotID)
			require.Equal(t, 3, state.Stage)
			require.Zero(t, state.NextGrowthRuntimeSeconds)
		}
	}
	require.Equal(t, int64(previousID)+int64(generated), db.GetGlobalVarLong(ctx, _const.LAST_USED_ID))
}

func TestWildWeedMapgenPostgresCrossChunkGapAndWater(t *testing.T) {
	for _, gap := range []int{19, 20} {
		t.Run(strconv.Itoa(gap), func(t *testing.T) {
			terrain := uniformTerrain(256, 128, types.TileDeepWater)
			point := wildWeedPosition{127*12 + 6, 20*12 + 6}
			terrain.Tiles[tileIndex(127, 20, terrain.WidthTiles)] = 35
			generator, db := wildWeedPostgresGenerator(t, terrain)
			ctx := context.Background()
			require.NoError(t, db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
				ID: 1, TypeID: 1, Region: 1, X: point.X + gap, Y: point.Y, ChunkX: 1,
			}))
			generator.lastEntityID.Store(1)
			require.NoError(t, insertWildWeedSpot(ctx, db.Queries(), point))
			var generated int
			require.NoError(t, db.WithTx(ctx, func(queries *repository.Queries) error {
				var err error
				generated, err = generator.writeWildWeeds(ctx, queries)
				return err
			}))
			if gap == 19 {
				require.Zero(t, generated)
			} else {
				require.Equal(t, 1, generated)
			}
			objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1})
			require.NoError(t, err)
			require.Len(t, objects, generated, "all other circle tiles are water")
			spots, err := db.Queries().LoadSpotsByRegion(ctx, 1)
			require.NoError(t, err)
			if generated == 0 {
				require.JSONEq(t, `{"www":{"objects":{}}}`, string(spots[0].State))
			}
		})
	}
}

func TestWildWeedMapgenPostgresFinalTransactionRollback(t *testing.T) {
	generator, db := wildWeedPostgresGenerator(t, uniformTerrain(128, 128, 35))
	ctx := context.Background()
	const checkpoint = 1234
	generator.lastEntityID.Store(checkpoint)
	require.NoError(t, db.SetGlobalVarLong(ctx, _const.LAST_USED_ID, checkpoint))
	failure := errors.New("final mapgen stage failed")
	err := db.WithTx(ctx, func(queries *repository.Queries) error {
		if err := insertWildWeedSpot(ctx, queries, wildWeedPosition{774, 774}); err != nil {
			return err
		}
		if _, err := generator.writeWildWeeds(ctx, queries); err != nil {
			return err
		}
		return failure
	})
	require.ErrorIs(t, err, failure)
	spots, err := db.Queries().LoadSpotsByRegion(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, spots)
	objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	require.Empty(t, objects)
	chunk, err := db.Queries().GetChunk(ctx, repository.GetChunkParams{Region: 1})
	require.NoError(t, err)
	require.Zero(t, chunk.EntityCount.Int32)
	require.Equal(t, int64(checkpoint), db.GetGlobalVarLong(ctx, _const.LAST_USED_ID))
	require.Greater(t, generator.lastEntityID.Load(), uint64(checkpoint), "only committed ID watermark must remain unchanged")
}

func TestWildWeedGapUsesCoordinatesOnly(t *testing.T) {
	grid := make(wildWeedPositionGrid)
	grid.add(wildWeedPosition{100, 100})
	require.False(t, grid.allows(wildWeedPosition{119, 100}))
	require.True(t, grid.allows(wildWeedPosition{120, 100}))
	// Squared Euclidean distance, rather than an axis-aligned 20x20 box.
	require.True(t, grid.allows(wildWeedPosition{115, 115}))
	require.Greater(t, math.Hypot(15, 15), float64(wildWeedMinGap))
}
