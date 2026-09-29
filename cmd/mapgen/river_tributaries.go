package main

import (
	"math"
	"sort"
)

func addRiverTributaries(flow []uint32, plan *riverFairways, width, height int, seed int64, opts RiverOptions) {
	plan.TributaryBudget = int(math.Floor(float64(plan.MainCount) * opts.TributaryRatio))
	if plan.TributaryBudget == 0 {
		return
	}
	parents := make([]int, plan.MainCount)
	for index := range parents {
		parents[index] = index
	}
	sort.Slice(parents, func(first, second int) bool {
		firstHash := splitMix64(uint64(seed) ^ uint64(parents[first]+1)*riverSourceSalt)
		secondHash := splitMix64(uint64(seed) ^ uint64(parents[second]+1)*riverSourceSalt)
		if firstHash == secondHash {
			return parents[first] < parents[second]
		}
		return firstHash < secondHash
	})
	junctions := make([]int, 0, plan.TributaryBudget)
	for _, parentID := range parents {
		if plan.Tributaries >= plan.TributaryBudget {
			break
		}
		parent := plan.Routes[parentID]
		margin := maxInt(9, maxInt(opts.TributarySpacingTiles/2, parent.Width*3))
		if len(parent.Path) <= 2*margin+1 {
			continue
		}
		for attempt := 0; attempt < 16; attempt++ {
			branchSeed := seed + int64(parentID+1)*7919 + int64(attempt+1)*104729
			fraction := coordHash01(branchSeed, parentID, attempt, lakeLinkJitterSalt)
			position := margin + int(fraction*float64(len(parent.Path)-2*margin-1))
			junction := parent.Path[position]
			if !tributaryJunctionSeparated(junction, junctions, plan.Inlets, width, opts.TributarySpacingTiles) {
				plan.RejectedTributaries++
				continue
			}
			before, after := parent.Path[position-8], parent.Path[position+8]
			deltaColumn, deltaRow := float64(after%width-before%width), float64(after/width-before/width)
			tangentLength := math.Hypot(deltaColumn, deltaRow)
			if tangentLength == 0 {
				continue
			}
			length := float64(opts.TributaryLengthMin) + coordHash01(branchSeed, attempt, parentID, riverWidthSalt)*float64(opts.TributaryLengthMax-opts.TributaryLengthMin)
			side := 1.0
			if attempt%2 == 1 {
				side = -1
			}
			startColumn, startRow := junction%width, junction/width
			endColumn := startColumn + int(math.Round(-deltaRow/tangentLength*length*side))
			endRow := startRow + int(math.Round(deltaColumn/tangentLength*length*side))
			edgeMargin := opts.RiverWidthMin + opts.BankRadius
			if endColumn < edgeMargin || endRow < edgeMargin || endColumn >= width-edgeMargin || endRow >= height-edgeMargin {
				plan.RejectedTributaries++
				continue
			}
			branchOpts := opts
			branchOpts.RiverWidthMax = opts.RiverWidthMin
			path := buildNestedBendPath(width, height, startColumn, startRow, endColumn, endRow, branchSeed, branchOpts)
			clearance := opts.RiverWidthMin + opts.BankRadius*2
			startAllowance := parent.Width*2 + clearance
			if len(path) <= startAllowance+opts.FairwayWidthTiles || len(path) < opts.TributaryLengthMin || !riverPathSelfSeparated(path, width, clearance) || riverPathTouchesWater(flow, path, width, height, clearance/2+1, startAllowance, 0, uint32(opts.FlowShallowThreshold)) {
				plan.RejectedTributaries++
				continue
			}
			carveRiverCorridor(flow, width, height, path, opts.RiverWidthMin, opts)
			route := riverRoute{Path: path, Width: opts.RiverWidthMin, Role: "tributary", Parent: parentID}
			plan.Routes = append(plan.Routes, protectRiverRoute(flow, plan, route, width, height, opts))
			plan.Tributaries++
			junctions = append(junctions, junction)
			break
		}
	}
}

func tributaryJunctionSeparated(junction int, existing []int, inlets []lakeInlet, width, spacing int) bool {
	column, row := junction%width, junction/width
	separated := func(otherColumn, otherRow int) bool {
		return math.Hypot(float64(column-otherColumn), float64(row-otherRow)) >= float64(spacing)
	}
	for _, other := range existing {
		if !separated(other%width, other/width) {
			return false
		}
	}
	for _, inlet := range inlets {
		if !separated(inlet.X, inlet.Y) {
			return false
		}
	}
	return true
}
