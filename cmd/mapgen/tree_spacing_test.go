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

func TestTreePlacementTrunksKeepGapAcrossChunkBoundaries(t *testing.T) {
	setTreePlacementTestRules(t, []treePlacementRule{
		{key: "tree_pine", chance: 0.5}, {key: "tree_fir", chance: 0.5},
	})
	definitions := []objectdefs.ObjectDef{
		{DefID: 5, Key: "tree_pine", Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 8, H: 10}}},
		{DefID: 34, Key: "tree_fir", Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10, H: 8}}},
	}
	const tileSize = 12
	rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(definitions), tileSize)
	require.NoError(t, err)
	placements := rules[types.TileConiferousForest]

	// All four tile positions meet at the corner of four different chunks.
	tiles := [][2]int{{127, 127}, {128, 127}, {127, 128}, {128, 128}}
	extremes := [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}}
	for first := 0; first < len(tiles); first++ {
		for second := first + 1; second < len(tiles); second++ {
			for firstSpecies := range placements {
				for secondSpecies := range placements {
					firstTree, secondTree := &placements[firstSpecies], &placements[secondSpecies]
					for _, firstExtreme := range extremes {
						for _, secondExtreme := range extremes {
							firstX, firstY := treeSpacingExtremeOffset(t, firstTree, firstExtreme[0], firstExtreme[1])
							secondX, secondY := treeSpacingExtremeOffset(t, secondTree, secondExtreme[0], secondExtreme[1])
							firstX += tiles[first][0] * tileSize
							firstY += tiles[first][1] * tileSize
							secondX += tiles[second][0] * tileSize
							secondY += tiles[second][1] * tileSize
							firstCollider, secondCollider := firstTree.def.Components.Collider, secondTree.def.Components.Collider
							gapX := math.Max(0, math.Abs(float64(firstX-secondX))-(firstCollider.W+secondCollider.W)/2)
							gapY := math.Max(0, math.Abs(float64(firstY-secondY))-(firstCollider.H+secondCollider.H)/2)
							require.GreaterOrEqual(t, math.Hypot(gapX, gapY), float64(treeMinGap),
								"tiles %v and %v, species %s and %s", tiles[first], tiles[second], firstTree.def.Key, secondTree.def.Key)
						}
					}
				}
			}
		}
	}
}

func treeSpacingExtremeOffset(t *testing.T, tree *resolvedTreePlacementRule, maximumX, maximumY bool) (int, int) {
	t.Helper()
	var x, y int
	if maximumX {
		x = tree.rangeX - 1
	}
	if maximumY {
		y = tree.rangeY - 1
	}
	// Intn uses the upper 31 bits of Int63 for these small ranges. Values below
	// the range exercise its extrema without triggering rejection sampling.
	source := &treePlacementSource{values: []int64{int64(x) << 32, int64(y) << 32}}
	offsetX, offsetY := tree.offset(rand.New(source))
	require.Equal(t, 2, source.calls)
	return offsetX, offsetY
}

func TestTreePlacementCurrentTrunksUseTileCenters(t *testing.T) {
	rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), 12)
	require.NoError(t, err)
	for _, choices := range rules {
		for index := range choices {
			for _, maximum := range []bool{false, true} {
				x, y := treeSpacingExtremeOffset(t, &choices[index], maximum, maximum)
				require.Equal(t, 6, x, choices[index].def.Key)
				require.Equal(t, 6, y, choices[index].def.Key)
			}
		}
	}
}

func TestTreePlacementSpacingInvalidBeforeDatabaseAccess(t *testing.T) {
	for _, tc := range []struct {
		name       string
		components *objectdefs.Components
		tileSize   int
	}{
		{"missing components", nil, 12},
		{"missing collider", &objectdefs.Components{}, 12},
		{"zero width", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 0, H: 10}}, 12},
		{"negative height", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10, H: -1}}, 12},
		{"nan width", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: math.NaN(), H: 10}}, 12},
		{"infinite height", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10, H: math.Inf(1)}}, 12},
		{"oversized width", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 11, H: 10}}, 12},
		{"oversized height", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10, H: 11}}, 12},
		{"no integer center", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10.5, H: 10}}, 13},
		{"zero tile size", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10, H: 10}}, 0},
		{"negative tile size", &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 10, H: 10}}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setTreePlacementTestRules(t, []treePlacementRule{{key: "tree_pine", chance: 1}})
			generator := spotStageGenerator(nil, spotStageOptions(t))
			generator.coordPerTile = tc.tileSize
			generator.objectDefs = objectdefs.NewRegistry([]objectdefs.ObjectDef{
				{DefID: 5, Key: "tree_pine", Components: tc.components},
			})
			// A nil database would panic if validation reached destructive writes.
			err := generator.Generate(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "mapgen tree placement")
			require.Nil(t, generator.terrain)
		})
	}
}

func TestTreePlacementDisabledSpeciesSkipsSpacingValidation(t *testing.T) {
	setTreePlacementTestRules(t, []treePlacementRule{
		{key: "tree_pine", chance: 0}, {key: "tree_fir", chance: 1},
	})
	definitions := mapgenTreeDefinitions()
	for index := range definitions {
		if definitions[index].Key == "tree_pine" {
			definitions[index].Components = nil
		}
	}
	rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(definitions), 12)
	require.NoError(t, err)
	require.Len(t, rules[types.TileConiferousForest], 1)
	require.Equal(t, "tree_fir", rules[types.TileConiferousForest][0].def.Key)
}

func TestTreePlacementOffsetsStayInsideLargerTiles(t *testing.T) {
	setTreePlacementTestRules(t, []treePlacementRule{{key: "tree_pine", chance: 1}})
	for _, tileSize := range []int{13, 20, 100} {
		t.Run(fmt.Sprint(tileSize), func(t *testing.T) {
			rules, err := resolveTreePlacementRules(objectdefs.NewRegistry(mapgenTreeDefinitions()), tileSize)
			require.NoError(t, err)
			placement := &rules[types.TileConiferousForest][0]
			for _, maximum := range []bool{false, true} {
				x, y := treeSpacingExtremeOffset(t, placement, maximum, maximum)
				collider := placement.def.Components.Collider
				require.GreaterOrEqual(t, float64(x)-collider.W/2, float64(treeMinGap)/2)
				require.GreaterOrEqual(t, float64(y)-collider.H/2, float64(treeMinGap)/2)
				require.GreaterOrEqual(t, float64(tileSize-x)-collider.W/2, float64(treeMinGap)/2)
				require.GreaterOrEqual(t, float64(tileSize-y)-collider.H/2, float64(treeMinGap)/2)
			}
		})
	}
}
