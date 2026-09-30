package main

import (
	"container/heap"
	"fmt"
)

type riverRoute struct {
	Path   []int
	Width  int
	Role   string
	Parent int
}

type riverFairways struct {
	Routes              []riverRoute
	Protected           []bool
	MainCount           int
	TributaryBudget     int
	Tributaries         int
	RejectedTributaries int
	Lakes               []drawLake
	Inlets              []lakeInlet
	LakeLand            []bool
	LakeWater           []bool
	LakeShapes          []*lakeShape
	LakeStats           lakeGeometryStats
	Junctions           *riverJunctionState
}

func riverRouteStats(plan *riverFairways) map[string]int {
	if plan == nil {
		return nil
	}
	stats := map[string]int{
		"main": plan.MainCount, "tributary_budget": plan.TributaryBudget,
		"tributaries": plan.Tributaries, "rejected_tributary_candidates": plan.RejectedTributaries,
	}
	if state := plan.Junctions; state != nil {
		stats["junction_eligible"], stats["junction_selected"] = state.Eligible, state.Selected
		stats["junction_placed"], stats["junction_fallbacks"] = len(state.Accepted), state.Fallbacks
		stats["junction_attempts"], stats["junction_index_entries"] = state.Attempts, state.Entries
		for reason, count := range state.Rejections {
			stats["junction_rejected_"+reason] = count
		}
	}
	if len(plan.LakeShapes) > 0 {
		for class, name := range []string{"small", "medium", "large"} {
			counts := plan.LakeStats.Classes[class]
			stats["lake_"+name+"_count"] = counts.Lakes
			stats["lake_"+name+"_selected"] = counts.Selected
			stats["lake_"+name+"_requested_islands"] = counts.Requested
			stats["lake_"+name+"_placed_islands"] = counts.Placed
			stats["lake_"+name+"_rejected_islands"] = counts.Rejected
		}
		stats["lake_peninsulas"], stats["lake_shape_fallbacks"] = plan.LakeStats.Peninsulas, plan.LakeStats.Fallbacks
	}
	return stats
}

func recordDrawRiver(plan *riverFairways, path []int, width int, origins ...int) {
	if plan != nil {
		plan.Routes = append(plan.Routes, riverRoute{Path: path, Width: width, Role: "main", Parent: -1})
		if plan.Junctions != nil {
			plan.Junctions.record(len(plan.Routes)-1, plan.Routes[len(plan.Routes)-1], origins...)
		}
	}
}

func protectRiverRoute(flow []uint32, plan *riverFairways, route riverRoute, width, height int, opts RiverOptions) riverRoute {
	radius := opts.FairwayWidthTiles / 2
	placements := make([]int, 0, len(route.Path))
	for _, index := range route.Path {
		column := clampInt(index%width, radius, width-1-radius)
		row := clampInt(index/width, radius, height-1-radius)
		if len(placements) == 0 {
			placements = append(placements, tileIndex(column, row, width))
		} else {
			previous := placements[len(placements)-1]
			placements = appendCardinalRiverSegment(placements, width, previous%width, previous/width, column, row)
		}
	}
	route.Path = placements
	for _, index := range placements {
		column, row := index%width, index/width
		for offsetRow := -radius; offsetRow <= radius; offsetRow++ {
			for offsetColumn := -radius; offsetColumn <= radius; offsetColumn++ {
				protectedIndex := tileIndex(column+offsetColumn, row+offsetRow, width)
				if plan.isLakeLand(protectedIndex) {
					continue
				}
				plan.Protected[protectedIndex] = true
				if flow[protectedIndex] < uint32(opts.FlowDeepThreshold) {
					flow[protectedIndex] = uint32(opts.FlowDeepThreshold)
				}
			}
		}
	}
	return route
}

func finishRiverFairways(flow []uint32, plan *riverFairways, width, height int, seed int64, opts RiverOptions) error {
	plan.Protected = make([]bool, len(flow))
	plan.MainCount = len(plan.Routes)
	for index, route := range plan.Routes {
		plan.Routes[index] = protectRiverRoute(flow, plan, route, width, height, opts)
	}
	if opts.LakeIrregularEnabled {
		if err := finishIrregularLakes(flow, plan, width, height, seed, opts); err != nil {
			return err
		}
		addRiverTributaries(flow, plan, width, height, seed, opts)
		return plan.validate(width, height, opts.FairwayWidthTiles, seed, func(index int) bool { return flow[index] >= uint32(opts.FlowDeepThreshold) })
	}
	hubs := make(map[int]int)
	for _, inlet := range plan.Inlets {
		hub, found := hubs[inlet.LakeIndex]
		if !found {
			lake := plan.Lakes[inlet.LakeIndex]
			column, row, ok := selectLakeInletHub(flow, width, height, lake, buildLakeBasins(lake), lakeShoreThreshold(lake), opts)
			if !ok {
				return fmt.Errorf("seed %d lake %d has no fairway hub", seed, inlet.LakeIndex)
			}
			hub = tileIndex(column, row, width)
			hubs[inlet.LakeIndex] = hub
		}
		path, err := routeLakeFairway(flow, width, height, tileIndex(inlet.X, inlet.Y, width), hub, opts)
		if err != nil {
			return fmt.Errorf("seed %d lake %d inlet (%d,%d): %w", seed, inlet.LakeIndex, inlet.X, inlet.Y, err)
		}
		route := riverRoute{Path: path, Width: inlet.RiverWidth, Role: "inlet", Parent: inlet.LakeIndex}
		plan.Routes = append(plan.Routes, protectRiverRoute(flow, plan, route, width, height, opts))
	}
	addRiverTributaries(flow, plan, width, height, seed, opts)
	return plan.validate(width, height, opts.FairwayWidthTiles, seed, func(index int) bool {
		return flow[index] >= uint32(opts.FlowDeepThreshold)
	})
}

func (plan *riverFairways) validate(width, height, fairwayWidth int, seed int64, isDeep func(int) bool) error {
	if plan == nil {
		return nil
	}
	if plan.Junctions != nil {
		if err := plan.Junctions.validateAttachments(plan, seed); err != nil {
			return err
		}
	}
	radius := fairwayWidth / 2
	for routeID, route := range plan.Routes {
		context := fmt.Sprintf("route %d (%s) parent %d", routeID, route.Role, route.Parent)
		if plan.Junctions != nil {
			for _, junction := range plan.Junctions.Accepted {
				origins, ordinary := plan.Junctions.Origins[routeID]
				if routeID == junction.Route || routeID == junction.Parent || route.Role == "inlet" && route.Parent == plan.Lakes[junction.Source].ID || ordinary && (origins[0] == junction.Source || origins[1] == junction.Source) {
					context += fmt.Sprintf(" junction branch %d parent %d at (%d,%d)", junction.Route, junction.Parent, junction.Tile%width, junction.Tile/width)
				}
			}
		}
		if len(route.Path) == 0 {
			return fmt.Errorf("seed %d %s has no fairway placements", seed, context)
		}
		for position, index := range route.Path {
			column, row := index%width, index/width
			if column < radius || row < radius || column >= width-radius || row >= height-radius {
				return fmt.Errorf("seed %d %s fairway out of bounds at (%d,%d)", seed, context, column, row)
			}
			if position > 0 {
				previous := route.Path[position-1]
				if absInt(column-previous%width)+absInt(row-previous/width) != 1 {
					return fmt.Errorf("seed %d %s disconnected fairway at (%d,%d)", seed, context, column, row)
				}
			}
			for offsetRow := -radius; offsetRow <= radius; offsetRow++ {
				for offsetColumn := -radius; offsetColumn <= radius; offsetColumn++ {
					footprintIndex := tileIndex(column+offsetColumn, row+offsetRow, width)
					if plan.isLakeLand(footprintIndex) {
						return fmt.Errorf("seed %d %s crosses lake land at (%d,%d)", seed, context, column+offsetColumn, row+offsetRow)
					}
					if !isDeep(footprintIndex) {
						return fmt.Errorf("seed %d %s broken deep fairway at (%d,%d)", seed, context, column+offsetColumn, row+offsetRow)
					}
				}
			}
		}
	}
	return nil
}

type lakeFairwayNode struct {
	index, cost, priority int
}

type lakeFairwayQueue []lakeFairwayNode

func (queue lakeFairwayQueue) Len() int { return len(queue) }
func (queue lakeFairwayQueue) Less(first, second int) bool {
	if queue[first].priority != queue[second].priority {
		return queue[first].priority < queue[second].priority
	}
	if queue[first].cost != queue[second].cost {
		return queue[first].cost > queue[second].cost
	}
	return queue[first].index < queue[second].index
}
func (queue lakeFairwayQueue) Swap(first, second int) {
	queue[first], queue[second] = queue[second], queue[first]
}
func (queue *lakeFairwayQueue) Push(value any) { *queue = append(*queue, value.(lakeFairwayNode)) }
func (queue *lakeFairwayQueue) Pop() any {
	last := len(*queue) - 1
	value := (*queue)[last]
	*queue = (*queue)[:last]
	return value
}

func routeLakeFairway(flow []uint32, width, height, start, target int, opts RiverOptions) ([]int, error) {
	queue := &lakeFairwayQueue{{index: start}}
	costs, parents := map[int]int{start: 0}, map[int]int{start: start}
	limit := 8 * (width + height)
	for queue.Len() > 0 {
		current := heap.Pop(queue).(lakeFairwayNode)
		if current.cost != costs[current.index] {
			continue
		}
		if current.index == target {
			path := []int{target}
			for path[len(path)-1] != start {
				path = append(path, parents[path[len(path)-1]])
			}
			for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
				path[left], path[right] = path[right], path[left]
			}
			return path, nil
		}
		column, row := current.index%width, current.index/width
		for _, offset := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			nextColumn, nextRow := column+offset[0], row+offset[1]
			if nextColumn < 0 || nextRow < 0 || nextColumn >= width || nextRow >= height {
				continue
			}
			next := tileIndex(nextColumn, nextRow, width)
			stepCost := 1
			if flow[next] < uint32(opts.FlowShallowThreshold) {
				stepCost = 12
			}
			cost := current.cost + stepCost
			previous, visited := costs[next]
			if visited && previous <= cost {
				continue
			}
			if !visited && len(costs) >= limit {
				return nil, fmt.Errorf("lake fairway search exceeds %d cells", limit)
			}
			costs[next], parents[next] = cost, current.index
			heuristic := absInt(nextColumn-target%width) + absInt(nextRow-target/width)
			heap.Push(queue, lakeFairwayNode{index: next, cost: cost, priority: cost + heuristic})
		}
	}
	return nil, fmt.Errorf("lake fairway has no route")
}
