package main

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMapgenObjectHealthDefaultsPostgres(t *testing.T) {
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	behaviorRegistry := behaviors.MustDefaultRegistry()
	definitions, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviorRegistry, zap.NewNop())
	require.NoError(t, err)
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(definitions)
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	const coordPerTile = 100
	rules, err := resolveTreePlacementRules(definitions, coordPerTile)
	require.NoError(t, err)

	// Exercise every active terrain/species entry without fixing balance values.
	var tiles []byte
	source := &treePlacementSource{}
	expectedCounts := make(map[int]int)
	for tile, choices := range rules {
		var previous float64
		for _, choice := range choices {
			tiles = append(tiles, byte(tile))
			roll := (previous + choice.threshold) / 2
			source.values = append(source.values, int64(roll*(1<<63)), 0, 0, 0)
			expectedCounts[choice.def.DefID]++
			previous = choice.threshold
		}
	}
	boulder, found := definitions.GetByKey("boulder")
	require.True(t, found)
	tiles = append(tiles, tileSand)
	source.values = append(source.values, 0, 0, 0, 0)
	expectedCounts[boulder.DefID]++
	grass := rules[tileGrass]
	if len(grass) == 0 || grass[len(grass)-1].threshold < 1 {
		// Keep the grass-to-boulder fallback whenever grass can miss its tree roll.
		tiles = append(tiles, tileGrass)
		if len(grass) != 0 {
			total := grass[len(grass)-1].threshold
			roll := (total + 1) / 2
			if roll == 1 {
				roll = total
			}
			source.values = append(source.values, int64(roll*(1<<63)))
		}
		source.values = append(source.values, 0, 0, 0, 0)
		expectedCounts[boulder.DefID]++
	}
	objectCount := len(tiles)
	tiles = append(tiles, types.TileHeath, types.TileMoor)
	chunkSize := 1
	for chunkSize*chunkSize < len(tiles) {
		chunkSize++
	}
	for len(tiles) < chunkSize*chunkSize {
		tiles = append(tiles, types.TileHeath)
	}
	generator := MapGenerator{
		db:             db,
		chunkSize:      chunkSize,
		coordPerTile:   coordPerTile,
		region:         1,
		objectDefs:     definitions,
		treePlacements: rules,
		terrain: &TerrainPrecompute{
			WidthTiles:  chunkSize,
			HeightTiles: chunkSize,
			Tiles:       tiles,
		},
	}
	ctx := context.Background()
	require.NoError(t, generator.generateChunkWithRNG(ctx, 0, 0, rand.New(source)))
	require.Equal(t, len(source.values), source.calls)
	chunk := repository.GetObjectsByChunkParams{Region: 1, ChunkX: 0, ChunkY: 0, Layer: 0}
	objects, err := db.Queries().GetObjectsByChunk(ctx, chunk)
	require.NoError(t, err)
	require.Len(t, objects, objectCount)

	w := ecs.NewWorldForTesting()
	factory := world.NewObjectFactory(nil)
	counts := make(map[int]int)
	for index := range objects {
		raw := &objects[index]
		counts[raw.TypeID]++
		require.False(t, raw.Hp.Valid, "generated objects must keep definition-default HP NULL")
		require.False(t, raw.Data.Valid, "generated objects must not persist a growing tree state")
		require.Equal(t, int64(10), raw.Quality)
		require.True(t, raw.Heading.Valid)
		require.Zero(t, raw.Heading.Int16)
		require.GreaterOrEqual(t, raw.X, 0)
		require.Less(t, raw.X, chunkSize*generator.coordPerTile)
		require.GreaterOrEqual(t, raw.Y, 0)
		require.Less(t, raw.Y, chunkSize*generator.coordPerTile)
		definition, ok := definitions.GetByID(raw.TypeID)
		require.True(t, ok)
		if definition.TreeConfig != nil {
			collider := definition.Components.Collider
			xInTile, yInTile := raw.X%coordPerTile, raw.Y%coordPerTile
			require.GreaterOrEqual(t, float64(xInTile)-collider.W/2, float64(treeMinGap)/2)
			require.GreaterOrEqual(t, float64(yInTile)-collider.H/2, float64(treeMinGap)/2)
			require.GreaterOrEqual(t, float64(coordPerTile-xInTile)-collider.W/2, float64(treeMinGap)/2)
			require.GreaterOrEqual(t, float64(coordPerTile-yInTile)-collider.H/2, float64(treeMinGap)/2)
		}
		handle, err := factory.Build(w, raw, nil)
		require.NoError(t, err)
		state, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
		require.True(t, ok)
		require.True(t, state.HasHP)
		require.Equal(t, float64(definition.HP), state.HP)
		if definition.TreeConfig != nil {
			restored, err := factory.DeserializeObjectState(raw)
			require.NoError(t, err)
			require.Nil(t, restored)
			require.NoError(t, behaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World: w, Handle: handle, EntityID: types.EntityID(raw.ID), EntityType: uint32(raw.TypeID),
				Reason: contracts.ObjectBehaviorInitReasonRestore,
			}, definition.BehaviorOrder))
			behavior, found := behaviorRegistry.GetBehavior("tree")
			require.True(t, found)
			runtime := behavior.(contracts.RuntimeBehavior).ApplyRuntime(&contracts.BehaviorRuntimeContext{
				World: w, Handle: handle, EntityID: types.EntityID(raw.ID), EntityType: uint32(raw.TypeID),
			})
			require.Equal(t, []string{fmt.Sprintf("tree.stage%d", len(definition.TreeConfig.Stages))}, runtime.Flags,
				"%s must load at its mature stage", definition.Key)
		}
		require.False(t, raw.Hp.Valid, "loading must not backfill the raw database row")
	}
	require.Equal(t, expectedCounts, counts)
	require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount(), "mature map trees must not schedule growth")

	objects, err = db.Queries().GetObjectsByChunk(ctx, chunk)
	require.NoError(t, err)
	require.Len(t, objects, objectCount)
	for _, raw := range objects {
		require.False(t, raw.Hp.Valid, "loading must not backfill the database")
	}
}
