package main

import (
	"fmt"
	"math"
	"math/rand"

	"origin/internal/objectdefs"
	"origin/internal/types"
)

// Each tile reserves half this clearance at every edge of a tree's trunk.
const treeMinGap = 2.0

type treePlacementRule struct {
	key    string
	chance float64
}

// Chances are absolute probabilities per tile, not relative weights. Each tile
// selects at most one species; the unused probability produces no tree.
var treePlacementRules = [256][]treePlacementRule{
	types.TileConiferousForest: {
		{key: "tree_pine", chance: 0.03 / 3},
		{key: "tree_fir", chance: 0.04 / 3},
		{key: "tree_yew", chance: 0.02 / 40},
	},
	types.TileBroadleafForest: {
		{key: "tree_birch", chance: 0.05 / 6},
		{key: "tree_oak", chance: 0.01 / 6},
		{key: "tree_maple", chance: 0.05 / 6},
		{key: "tree_elm", chance: 0.05 / 6},
		{key: "tree_apple", chance: 0.03 / 6},
		{key: "tree_mulberry", chance: 0.05 / 6},
	},
	types.TileThicket: {{key: "tree_hazel", chance: 0.003}},
	types.TileSwamp1:  {{key: "tree_willow", chance: 0.001}},
	types.TileSwamp2:  {{key: "tree_willow", chance: 0.001}},
	types.TileSwamp3:  {{key: "tree_willow", chance: 0.001}},
	types.TileGrass: {
		{key: "tree_birch", chance: 0.001},
		{key: "tree_oak", chance: 0.00003},
		{key: "tree_elm", chance: 0.0001},
		{key: "tree_apple", chance: 0.0001},
		{key: "tree_mulberry", chance: 0.0001},
	},
}

type resolvedTreePlacementRule struct {
	def       *objectdefs.ObjectDef
	threshold float64
	minX      int
	minY      int
	rangeX    int
	rangeY    int
}

type resolvedTreePlacementRules [256][]resolvedTreePlacementRule

func resolveTreePlacementRules(registry *objectdefs.Registry, tileSize int) (*resolvedTreePlacementRules, error) {
	if registry == nil {
		return nil, fmt.Errorf("mapgen tree placement: object definitions are not initialized")
	}
	if tileSize <= 0 {
		return nil, fmt.Errorf("mapgen tree placement: tile size must be positive")
	}
	rules := &resolvedTreePlacementRules{}
	for tile, choices := range treePlacementRules {
		if len(choices) == 0 {
			continue
		}
		var total float64
		seen := make(map[string]bool, len(choices))
		for _, choice := range choices {
			if math.IsNaN(choice.chance) || math.IsInf(choice.chance, 0) || choice.chance < 0 || choice.chance > 1 {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: %q chance must be finite and within [0,1]", tile, choice.key)
			}
			if seen[choice.key] {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: duplicate tree %q", tile, choice.key)
			}
			seen[choice.key] = true
			total += choice.chance
			if total > 1 {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: total tree chance must not exceed 1", tile)
			}
			if choice.chance == 0 {
				continue
			}
			definition, ok := registry.GetByKey(choice.key)
			if !ok {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: missing object definition %q", tile, choice.key)
			}
			if definition.Components == nil || definition.Components.Collider == nil {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: %q requires a trunk collider", tile, choice.key)
			}
			collider := definition.Components.Collider
			minX, rangeX, err := treeOffsetRange(collider.W, tileSize)
			if err != nil {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: %q width: %w", tile, choice.key, err)
			}
			minY, rangeY, err := treeOffsetRange(collider.H, tileSize)
			if err != nil {
				return nil, fmt.Errorf("mapgen tree placement for tile %d: %q height: %w", tile, choice.key, err)
			}
			rules[tile] = append(rules[tile], resolvedTreePlacementRule{
				def: definition, threshold: total,
				minX: minX, minY: minY, rangeX: rangeX, rangeY: rangeY,
			})
		}
	}
	return rules, nil
}

func treeOffsetRange(trunkSize float64, tileSize int) (int, int, error) {
	if math.IsNaN(trunkSize) || math.IsInf(trunkSize, 0) || trunkSize <= 0 {
		return 0, 0, fmt.Errorf("trunk size must be finite and positive")
	}
	if trunkSize+treeMinGap > float64(tileSize) {
		return 0, 0, fmt.Errorf("trunk size %g with gap %g does not fit tile size %d", trunkSize, treeMinGap, tileSize)
	}
	minimum := int(math.Ceil((trunkSize + treeMinGap) / 2))
	maximum := int(math.Floor(float64(tileSize) - (trunkSize+treeMinGap)/2))
	if minimum > maximum {
		return 0, 0, fmt.Errorf("trunk has no fitting integer position with gap %g in tile size %d", treeMinGap, tileSize)
	}
	return minimum, maximum - minimum + 1, nil
}

func (tree *resolvedTreePlacementRule) offset(rng *rand.Rand) (int, int) {
	return tree.minX + rng.Intn(tree.rangeX), tree.minY + rng.Intn(tree.rangeY)
}

func (g *MapGenerator) treePlacementForTile(tile byte, rng *rand.Rand) *resolvedTreePlacementRule {
	rule := g.treePlacements[tile]
	if len(rule) == 0 {
		return nil
	}
	roll := rng.Float64()
	if roll >= rule[len(rule)-1].threshold {
		return nil
	}
	for index := range rule {
		if roll < rule[index].threshold {
			return &rule[index]
		}
	}
	return nil
}
