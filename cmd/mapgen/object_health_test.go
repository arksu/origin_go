package main

import (
	"context"
	"math/rand"
	"path/filepath"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/stretchr/testify/require"
)

// Always choose the spawn branch so both forest and boulder tiles are covered.
type objectHealthZeroSource struct{}

func (objectHealthZeroSource) Int63() int64 { return 0 }
func (objectHealthZeroSource) Seed(int64)   {}

func TestMapgenObjectHealthDefaultsPostgres(t *testing.T) {
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	definitions := objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "tree_birch", HP: 125},
		{DefID: 2, Key: "boulder", HP: 320},
	})
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(definitions)
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })

	generator := MapGenerator{
		db:           db,
		chunkSize:    2,
		coordPerTile: 100,
		region:       1,
		objectDefs:   definitions,
		terrain: &TerrainPrecompute{
			WidthTiles:  2,
			HeightTiles: 2,
			Tiles:       []byte{tileForestPine, tileForestLeaf, tileGrass, tileSand},
		},
	}
	ctx := context.Background()
	require.NoError(t, generator.generateChunkWithRNG(ctx, 0, 0, rand.New(objectHealthZeroSource{})))
	chunk := repository.GetObjectsByChunkParams{Region: 1, ChunkX: 0, ChunkY: 0, Layer: 0}
	objects, err := db.Queries().GetObjectsByChunk(ctx, chunk)
	require.NoError(t, err)
	require.Len(t, objects, 4)

	w := ecs.NewWorldForTesting()
	factory := world.NewObjectFactory(nil)
	counts := make(map[int]int)
	for index := range objects {
		raw := &objects[index]
		counts[raw.TypeID]++
		require.False(t, raw.Hp.Valid, "generated objects must keep definition-default HP NULL")
		definition, ok := definitions.GetByID(raw.TypeID)
		require.True(t, ok)
		handle, err := factory.Build(w, raw, nil)
		require.NoError(t, err)
		state, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
		require.True(t, ok)
		require.True(t, state.HasHP)
		require.Equal(t, float64(definition.HP), state.HP)
		require.False(t, raw.Hp.Valid, "loading must not backfill the raw database row")
	}
	require.Equal(t, map[int]int{1: 2, 2: 2}, counts)

	objects, err = db.Queries().GetObjectsByChunk(ctx, chunk)
	require.NoError(t, err)
	require.Len(t, objects, 4)
	for _, raw := range objects {
		require.False(t, raw.Hp.Valid, "loading must not backfill the database")
	}
}
