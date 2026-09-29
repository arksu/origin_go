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
}

func riverRouteStats(plan *riverFairways) map[string]int {
	if plan == nil {
		return nil
	}
	return map[string]int{
		"main": plan.MainCount, "tributary_budget": plan.TributaryBudget,
		"tributaries": plan.Tributaries, "rejected_tributary_candidates": plan.RejectedTributaries,
	}
}

func recordDrawRiver(plan *riverFairways, path []int, width int) {
	if plan != nil {
		plan.Routes = append(plan.Routes, riverRoute{Path: path, Width: width, Role: "main", Parent: -1})
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
	radius := fairwayWidth / 2
	for routeID, route := range plan.Routes {
		if len(route.Path) == 0 {
			return fmt.Errorf("seed %d route %d (%s) has no fairway placements", seed, routeID, route.Role)
		}
		for position, index := range route.Path {
			column, row := index%width, index/width
			if column < radius || row < radius || column >= width-radius || row >= height-radius {
				return fmt.Errorf("seed %d route %d (%s) fairway out of bounds at (%d,%d)", seed, routeID, route.Role, column, row)
			}
			if position > 0 {
				previous := route.Path[position-1]
				if absInt(column-previous%width)+absInt(row-previous/width) != 1 {
					return fmt.Errorf("seed %d route %d (%s) disconnected fairway at (%d,%d)", seed, routeID, route.Role, column, row)
				}
			}
			for offsetRow := -radius; offsetRow <= radius; offsetRow++ {
				for offsetColumn := -radius; offsetColumn <= radius; offsetColumn++ {
					footprintIndex := tileIndex(column+offsetColumn, row+offsetRow, width)
					if !isDeep(footprintIndex) {
						return fmt.Errorf("seed %d route %d (%s) broken deep fairway at (%d,%d)", seed, routeID, route.Role, column+offsetColumn, row+offsetRow)
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
