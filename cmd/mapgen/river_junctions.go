package main

import (
	"fmt"
	"image"
	"math"
	"sort"
)

const (
	riverJunctionCell       = 64
	riverJunctionSampleStep = 8
	riverJunctionAttempts   = 16
	riverJunctionSalt       = uint64(0x752AD907EA416BC3)
)

type riverJunctionSample struct {
	Route, Position, Tile int
}

type riverJunction struct {
	Route, Parent, Source, Tile int
}

type riverJunctionState struct {
	Width, Height                                                   int
	Origins                                                         map[int][2]int
	Grid                                                            map[image.Point][]riverJunctionSample
	LakeBounds                                                      []image.Rectangle
	Accepted                                                        []riverJunction
	Eligible, Selected, Fallbacks, Attempts, BackboneCount, Entries int
	Rejections                                                      map[string]int
}

func newRiverJunctionState(width, height int, lakes []drawLake, opts RiverOptions) *riverJunctionState {
	state := &riverJunctionState{Width: width, Height: height, Origins: make(map[int][2]int), Grid: make(map[image.Point][]riverJunctionSample), Rejections: make(map[string]int)}
	padding := opts.LakeShoreVariationTiles + opts.FairwayWidthTiles + 4
	for _, lake := range lakes {
		bounds := image.Rect(lake.X, lake.Y, lake.X+1, lake.Y+1)
		for _, basin := range buildLakeBasins(lake) {
			bounds = bounds.Union(image.Rect(int(math.Floor(basin.X-basin.RadiusX))-padding, int(math.Floor(basin.Y-basin.RadiusY))-padding, int(math.Ceil(basin.X+basin.RadiusX))+padding+1, int(math.Ceil(basin.Y+basin.RadiusY))+padding+1))
		}
		state.LakeBounds = append(state.LakeBounds, bounds)
	}
	return state
}

func (state *riverJunctionState) record(routeID int, route riverRoute, origins ...int) {
	if route.Role == "main" && len(origins) == 2 {
		state.Origins[routeID] = [2]int{origins[0], origins[1]}
	}
	for position := 0; position < len(route.Path); position += riverJunctionSampleStep {
		state.addSample(riverJunctionSample{routeID, position, route.Path[position]})
	}
	if len(route.Path) > 1 && (len(route.Path)-1)%riverJunctionSampleStep != 0 {
		position := len(route.Path) - 1
		state.addSample(riverJunctionSample{routeID, position, route.Path[position]})
	}
}

func (state *riverJunctionState) addSample(sample riverJunctionSample) {
	cell := image.Pt(sample.Tile%state.Width/riverJunctionCell, sample.Tile/state.Width/riverJunctionCell)
	state.Grid[cell] = append(state.Grid[cell], sample)
	state.Entries++
}

func (state *riverJunctionState) visit(bounds image.Rectangle, visit func(riverJunctionSample) bool) bool {
	bounds = bounds.Intersect(image.Rect(0, 0, state.Width, state.Height))
	if bounds.Empty() {
		return true
	}
	for row := bounds.Min.Y / riverJunctionCell; row <= (bounds.Max.Y-1)/riverJunctionCell; row++ {
		for column := bounds.Min.X / riverJunctionCell; column <= (bounds.Max.X-1)/riverJunctionCell; column++ {
			for _, sample := range state.Grid[image.Pt(column, row)] {
				if image.Pt(sample.Tile%state.Width, sample.Tile/state.Width).In(bounds) && !visit(sample) {
					return false
				}
			}
		}
	}
	return true
}

func junctionSource(from, to int, connected []bool, seed int64) int {
	if to < 0 || connected[from] != connected[to] && !connected[from] {
		return from
	}
	if connected[from] != connected[to] || splitMix64(uint64(seed)^riverJunctionSalt)&1 != 0 {
		return to
	}
	return from
}

func (state *riverJunctionState) selectOpportunity(source int, seed int64, chance float64) bool {
	if len(state.Origins) == 0 || chance == 0 {
		return false
	}
	state.Eligible++
	if coordHash01(seed, source, 0, riverJunctionSalt) >= chance {
		return false
	}
	state.Selected++
	return true
}

func (state *riverJunctionState) separated(tile, spacing int) bool {
	for _, junction := range state.Accepted {
		if math.Hypot(float64(tile%state.Width-junction.Tile%state.Width), float64(tile/state.Width-junction.Tile/state.Width)) < float64(spacing) {
			return false
		}
	}
	return true
}

func (state *riverJunctionState) targets(plan *riverFairways, source int, lake drawLake, minimum, maximum int, seed int64, opts RiverOptions) []riverJunctionSample {
	clearance := riverCorridorMaximumWidth(opts) + riverJunctionSampleStep
	margin := maxInt(opts.JunctionSpacingTiles/2, clearance*3)
	searchRadius := maximum + maxInt(state.LakeBounds[source].Dx(), state.LakeBounds[source].Dy())
	targets := make([]riverJunctionSample, 0, riverJunctionAttempts)
	distance := func(sample riverJunctionSample) float64 {
		return math.Hypot(float64(sample.Tile%state.Width-lake.X), float64(sample.Tile/state.Width-lake.Y))
	}
	less := func(first, second riverJunctionSample) bool {
		firstDistance, secondDistance := distance(first), distance(second)
		if firstDistance != secondDistance {
			return firstDistance < secondDistance
		}
		firstHash := splitMix64(uint64(seed) ^ uint64(first.Tile) ^ uint64(first.Route)*riverJunctionSalt)
		secondHash := splitMix64(uint64(seed) ^ uint64(second.Tile) ^ uint64(second.Route)*riverJunctionSalt)
		if firstHash != secondHash {
			return firstHash < secondHash
		}
		if first.Route != second.Route {
			return first.Route < second.Route
		}
		return first.Position < second.Position
	}
	state.visit(image.Rect(lake.X-searchRadius, lake.Y-searchRadius, lake.X+searchRadius+1, lake.Y+searchRadius+1), func(sample riverJunctionSample) bool {
		origins, ordinary := state.Origins[sample.Route]
		if !ordinary || origins[0] == source || origins[1] == source {
			return true
		}
		for _, junction := range state.Accepted {
			if junction.Source == source && junction.Parent == sample.Route {
				return true
			}
		}
		parent := plan.Routes[sample.Route]
		if sample.Position < margin || sample.Position >= len(parent.Path)-margin {
			return true
		}
		point := image.Pt(sample.Tile%state.Width, sample.Tile/state.Width)
		startColumn, startRow := lakeShorePoint(lake, point.X, point.Y, state.Width, state.Height)
		length := math.Hypot(float64(point.X-startColumn), float64(point.Y-startRow))
		if length < float64(minimum) || length > float64(maximum) {
			return true
		}
		spacing := maxInt(opts.JunctionSpacingTiles, clearance*2)
		if !state.separated(sample.Tile, spacing) {
			return true
		}
		for _, bounds := range state.LakeBounds {
			if point.In(bounds.Inset(-spacing)) {
				return true
			}
		}
		for index, other := range targets {
			if other.Route == sample.Route && absInt(other.Position-sample.Position) < margin {
				if less(sample, other) {
					targets[index] = sample
				}
				return true
			}
		}
		targets = append(targets, sample)
		sort.Slice(targets, func(first, second int) bool { return less(targets[first], targets[second]) })
		if len(targets) > riverJunctionAttempts {
			targets = targets[:riverJunctionAttempts]
		}
		return true
	})
	sort.Slice(targets, func(first, second int) bool { return less(targets[first], targets[second]) })
	return targets
}

func tryRiverJunction(flow []uint32, width, height int, lakes []drawLake, degree []int, connected, lakeUsed []bool, inlets *[]lakeInlet, plan *riverFairways, from, to, minimum, maximum int, seed int64, opts RiverOptions) bool {
	if plan == nil || plan.Junctions == nil {
		return false
	}
	state := plan.Junctions
	source := junctionSource(from, to, connected, seed)
	if source < 0 || source >= len(lakes) || degree[source] >= opts.MaxLakeDegree || !state.selectOpportunity(source, seed, opts.JunctionChance) {
		return false
	}
	targets := state.targets(plan, source, lakes[source], minimum, maximum, seed, opts)
	if len(targets) == 0 {
		state.Rejections["no_target"]++
	}
	for attempt, target := range targets {
		state.Attempts++
		branchSeed := seed + int64(attempt+1)*104729
		path := buildRiverJunctionPath(width, height, lakes[source], plan.Routes[target.Route].Path, target.Position, branchSeed, opts)
		if len(path) > 0 {
			length := math.Hypot(float64(path[0]%width-target.Tile%width), float64(path[0]/width-target.Tile/width))
			if length < float64(minimum) || length > float64(maximum) {
				state.Rejections["distance"]++
				continue
			}
		}
		if reason := state.validateCandidate(plan, path, source, target, opts); reason != "" {
			state.Rejections[reason]++
			continue
		}
		riverWidth := riverWidthForLink(seed, lakes[source].ID, target.Route+3_000_000, opts)
		carveRiverCorridor(flow, width, height, path, riverWidth, branchSeed, opts)
		routeID := len(plan.Routes)
		route := riverRoute{Path: path, Width: riverWidth, Role: "junction", Parent: target.Route}
		plan.Routes = append(plan.Routes, route)
		state.record(routeID, route)
		state.Accepted = append(state.Accepted, riverJunction{Route: routeID, Parent: target.Route, Source: source, Tile: target.Tile})
		start := path[0]
		recordLakeInlet(inlets, source, start%width, start/width, riverWidth)
		degree[source]++
		connected[source], lakeUsed[source] = true, true
		return true
	}
	state.Fallbacks++
	return false
}

func (state *riverJunctionState) validateAttachments(plan *riverFairways, seed int64) error {
	for _, junction := range state.Accepted {
		problem := func() error {
			return fmt.Errorf("seed %d junction branch %d parent %d source lake %d disconnected at (%d,%d)", seed, junction.Route, junction.Parent, junction.Source, junction.Tile%state.Width, junction.Tile/state.Width)
		}
		if junction.Route >= len(plan.Routes) || junction.Parent >= len(plan.Routes) {
			return problem()
		}
		branch, parent := plan.Routes[junction.Route], plan.Routes[junction.Parent]
		if len(branch.Path) == 0 || branch.Path[len(branch.Path)-1] != junction.Tile {
			return problem()
		}
		found := false
		for position, tile := range parent.Path {
			if tile == junction.Tile && position > 0 && position < len(parent.Path)-1 {
				found = true
				break
			}
		}
		if !found {
			return problem()
		}
		found = false
		for _, inlet := range plan.Inlets {
			if inlet.LakeIndex == junction.Source && tileIndex(inlet.X, inlet.Y, state.Width) == branch.Path[0] {
				found = true
				break
			}
		}
		if !found {
			return problem()
		}
		found = false
		for _, route := range plan.Routes {
			if route.Role == "inlet" && route.Parent == plan.Lakes[junction.Source].ID && len(route.Path) > 0 && route.Path[0] == branch.Path[0] {
				found = true
				break
			}
		}
		if !found {
			return problem()
		}
	}
	return nil
}
