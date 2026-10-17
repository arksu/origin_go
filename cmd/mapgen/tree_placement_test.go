package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func mapgenTreeDefinitions() []objectdefs.ObjectDef {
	definitions := []objectdefs.ObjectDef{
		{DefID: 1, Key: "tree_birch"},
		{DefID: 5, Key: "tree_pine"},
		{DefID: 20, Key: "tree_oak"},
		{DefID: 22, Key: "tree_apple"},
		{DefID: 24, Key: "tree_maple"},
		{DefID: 26, Key: "tree_yew"},
		{DefID: 28, Key: "tree_elm"},
		{DefID: 30, Key: "tree_hazel"},
		{DefID: 32, Key: "tree_willow"},
		{DefID: 34, Key: "tree_fir"},
		{DefID: 39, Key: "tree_mulberry"},
	}
	for index := range definitions {
		definitions[index].Components = &objectdefs.Components{
			Collider: &objectdefs.ColliderDef{W: 10, H: 10},
		}
	}
	return definitions
}

type treePlacementSource struct {
	values []int64
	calls  int
}

func (source *treePlacementSource) Seed(int64) {}

func (source *treePlacementSource) Int63() int64 {
	value := source.values[source.calls]
	source.calls++
	return value
}

func configuredTreeDefinitions() []objectdefs.ObjectDef {
	used := make(map[string]bool)
	for _, choices := range treePlacementRules {
		for _, choice := range choices {
			if choice.chance > 0 {
				used[choice.key] = true
			}
		}
	}
	var definitions []objectdefs.ObjectDef
	for _, definition := range mapgenTreeDefinitions() {
		if used[definition.Key] {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func TestTreePlacementConfiguredEntries(t *testing.T) {
	registry := objectdefs.NewRegistry(mapgenTreeDefinitions())
	rules, err := resolveTreePlacementRules(registry, 12)
	require.NoError(t, err)
	generator := MapGenerator{treePlacements: rules}
	for tile, choices := range treePlacementRules {
		var previous float64
		active := 0
		for _, choice := range choices {
			if choice.chance == 0 {
				continue
			}
			roll := previous + choice.chance/2
			source := &treePlacementSource{values: []int64{int64(roll * (1 << 63))}}
			placement := generator.treePlacementForTile(byte(tile), rand.New(source))
			want, found := registry.GetByKey(choice.key)
			require.True(t, found)
			require.NotNil(t, placement)
			require.Same(t, want, placement.def, "tile %d, tree %s", tile, choice.key)
			require.Equal(t, 1, source.calls)
			previous += choice.chance
			active++
		}
		require.Len(t, rules[tile], active, "tile %d", tile)
	}
}

func setTreePlacementTestRules(t *testing.T, choices []treePlacementRule) {
	t.Helper()
	previous := treePlacementRules
	t.Cleanup(func() { treePlacementRules = previous })
	treePlacementRules = [256][]treePlacementRule{types.TileConiferousForest: choices}
}

func TestTreePlacementAbsoluteChanceBoundaries(t *testing.T) {
	setTreePlacementTestRules(t, []treePlacementRule{
		{key: "tree_pine", chance: 0.125},
		{key: "tree_yew", chance: 0},
		{key: "tree_fir", chance: 0.25},
	})
	rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), 12)
	require.NoError(t, err)
	generator := MapGenerator{treePlacements: rules}
	for _, tc := range []struct {
		roll float64
		key  string
	}{
		{0, "tree_pine"},
		{math.Nextafter(0.125, 0), "tree_pine"},
		{0.125, "tree_fir"},
		{math.Nextafter(0.375, 0), "tree_fir"},
		{0.375, ""},
		{math.Nextafter(0.375, 1), ""},
		{math.Nextafter(1, 0), ""},
	} {
		t.Run(fmt.Sprintf("roll_%g", tc.roll), func(t *testing.T) {
			source := &treePlacementSource{values: []int64{int64(tc.roll * (1 << 63))}}
			placement := generator.treePlacementForTile(types.TileConiferousForest, rand.New(source))
			if tc.key == "" {
				require.Nil(t, placement)
			} else {
				require.NotNil(t, placement)
				require.Equal(t, tc.key, placement.def.Key)
			}
			require.Equal(t, 1, source.calls, "selection and no-tree outcome use one roll")
		})
	}
}

func TestTreePlacementChangingOneChanceDoesNotRescaleOthers(t *testing.T) {
	for _, pineChance := range []float64{0.125, 0.25} {
		t.Run(fmt.Sprintf("pine_%g", pineChance), func(t *testing.T) {
			setTreePlacementTestRules(t, []treePlacementRule{
				{key: "tree_pine", chance: pineChance},
				{key: "tree_fir", chance: 0.25},
			})
			rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), 12)
			require.NoError(t, err)
			generator := MapGenerator{treePlacements: rules}
			counts := make(map[string]int)
			// Exhaust an evenly spaced grid: these binary probabilities have exact
			// counts, avoiding statistical tolerances and random sampling failures.
			for index := 0; index < 1024; index++ {
				source := &treePlacementSource{values: []int64{int64(index) << 53}}
				placement := generator.treePlacementForTile(types.TileConiferousForest, rand.New(source))
				key := "none"
				if placement != nil {
					key = placement.def.Key
				}
				counts[key]++
			}
			require.Equal(t, map[string]int{
				"tree_pine": int(pineChance * 1024), "tree_fir": 256,
				"none": 768 - int(pineChance*1024),
			}, counts)
		})
	}
}

func TestTreePlacementDisabledAndCertain(t *testing.T) {
	t.Run("zero skips missing definition and random draw", func(t *testing.T) {
		setTreePlacementTestRules(t, []treePlacementRule{{key: "tree_missing", chance: 0}})
		rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(nil), 12)
		require.NoError(t, err)
		generator := MapGenerator{treePlacements: rules}
		source := &treePlacementSource{}
		require.Nil(t, generator.treePlacementForTile(types.TileConiferousForest, rand.New(source)))
		require.Zero(t, source.calls)
	})
	t.Run("one always selects", func(t *testing.T) {
		setTreePlacementTestRules(t, []treePlacementRule{{key: "tree_pine", chance: 1}})
		rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), 12)
		require.NoError(t, err)
		generator := MapGenerator{treePlacements: rules}
		for _, roll := range []float64{0, 0.5, math.Nextafter(1, 0)} {
			source := &treePlacementSource{values: []int64{int64(roll * (1 << 63))}}
			require.Equal(t, "tree_pine", generator.treePlacementForTile(types.TileConiferousForest, rand.New(source)).def.Key)
		}
	})
}

func TestTreePlacementInvalidChancesFailBeforeDatabaseAccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		choices   []treePlacementRule
		errorText string
	}{
		{"negative", []treePlacementRule{{key: "tree_pine", chance: -0.1}}, "[0,1]"},
		{"greater than one", []treePlacementRule{{key: "tree_pine", chance: 1.1}}, "[0,1]"},
		{"nan", []treePlacementRule{{key: "tree_pine", chance: math.NaN()}}, "finite"},
		{"infinity", []treePlacementRule{{key: "tree_pine", chance: math.Inf(1)}}, "finite"},
		{"sum exceeds one", []treePlacementRule{{key: "tree_pine", chance: 0.75}, {key: "tree_fir", chance: 0.5}}, "total tree chance"},
		{"duplicate", []treePlacementRule{{key: "tree_pine", chance: 0.1}, {key: "tree_pine", chance: 0.2}}, "duplicate tree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setTreePlacementTestRules(t, tc.choices)
			generator := spotStageGenerator(nil, spotStageOptions(t))
			err := generator.Generate(context.Background())
			require.ErrorContains(t, err, tc.errorText)
			require.ErrorContains(t, err, "tile 20")
			require.Nil(t, generator.terrain)
		})
	}
}

func TestTreePlacementExcludedTilesConsumeNoRandomNumbers(t *testing.T) {
	rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), 12)
	require.NoError(t, err)
	generator := MapGenerator{treePlacements: rules}
	eligible := map[byte]bool{
		types.TileConiferousForest: true, types.TileBroadleafForest: true,
		types.TileThicket: true, types.TileGrass: true,
		types.TileSwamp1: true, types.TileSwamp2: true, types.TileSwamp3: true,
	}
	for tile := 0; tile < 256; tile++ {
		if eligible[byte(tile)] {
			continue
		}
		source := &treePlacementSource{}
		require.Nil(t, generator.treePlacementForTile(byte(tile), rand.New(source)), "tile %d", tile)
		require.Zero(t, source.calls, "tile %d", tile)
	}
}

func TestTreePlacementDeterministicAcrossChunkOrder(t *testing.T) {
	setTreePlacementTestRules(t, []treePlacementRule{{key: "tree_pine", chance: 0.125}, {key: "tree_fir", chance: 0.25}})
	rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), 12)
	require.NoError(t, err)
	generator := MapGenerator{treePlacements: rules}
	tiles := []byte{types.TileConiferousForest, types.TileBroadleafForest, types.TileThicket, types.TileSwamp1, types.TileGrass}
	sample := func(chunkX, chunkY int) []string {
		rng := rand.New(rand.NewSource(deterministicChunkSeed(12345, chunkX, chunkY)))
		var result []string
		for index := 0; index < 128*128; index++ {
			if placement := generator.treePlacementForTile(tiles[index%len(tiles)], rng); placement != nil {
				x, y := placement.offset(rng)
				result = append(result, fmt.Sprintf("%d:%s:%d,%d:%d", index, placement.def.Key, x, y, rng.Intn(8)))
			}
		}
		return result
	}
	coordinates := [][2]int{{0, 0}, {1, 2}, {5, 7}}
	want := make(map[[2]int][]string)
	for _, coordinate := range coordinates {
		want[coordinate] = sample(coordinate[0], coordinate[1])
		require.NotEmpty(t, want[coordinate])
	}
	for index := len(coordinates) - 1; index >= 0; index-- {
		coordinate := coordinates[index]
		require.Equal(t, want[coordinate], sample(coordinate[0], coordinate[1]))
	}
	require.NotEqual(t, want[coordinates[0]], want[coordinates[1]], "different chunks must use different random streams")
}

func TestTreePlacementPreflightBeforeDatabaseAccess(t *testing.T) {
	for _, missing := range configuredTreeDefinitions() {
		t.Run(missing.Key, func(t *testing.T) {
			var definitions []objectdefs.ObjectDef
			for _, definition := range mapgenTreeDefinitions() {
				if definition.Key != missing.Key {
					definitions = append(definitions, definition)
				}
			}
			generator := spotStageGenerator(nil, spotStageOptions(t))
			generator.objectDefs = objectdefs.NewRegistry(definitions)
			// A nil DB would panic if preflight reached any DB operation.
			err := generator.Generate(context.Background())
			require.ErrorContains(t, err, missing.Key)
			require.ErrorContains(t, err, "missing object definition")
			require.Nil(t, generator.terrain)
		})
	}
	_, err := resolveTreePlacementRules(nil, 12)
	require.ErrorContains(t, err, "object definitions are not initialized")
}
