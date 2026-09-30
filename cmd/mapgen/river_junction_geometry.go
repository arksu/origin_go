package main

import (
	"image"
	"math"
)

func buildRiverJunctionPath(width, height int, source drawLake, parent []int, position int, seed int64, opts RiverOptions) []int {
	clearance := riverCorridorMaximumWidth(opts) + riverJunctionSampleStep
	window := maxInt(12, clearance)
	if position < window || position+window >= len(parent) {
		return nil
	}
	join := parent[position]
	before, after := parent[position-window], parent[position+window]
	tangentColumn, tangentRow := float64(after%width-before%width), float64(after/width-before/width)
	tangentLength := math.Hypot(tangentColumn, tangentRow)
	if tangentLength < float64(window)/2 {
		return nil
	}
	tangentColumn, tangentRow = tangentColumn/tangentLength, tangentRow/tangentLength
	joinColumn, joinRow := float64(join%width), float64(join/width)
	deltaColumn, deltaRow := float64(source.X)-joinColumn, float64(source.Y)-joinRow
	normalColumn, normalRow := -tangentRow, tangentColumn
	if deltaColumn*normalColumn+deltaRow*normalRow < 0 {
		normalColumn, normalRow = -normalColumn, -normalRow
	}
	if deltaColumn*tangentColumn+deltaRow*tangentRow < 0 {
		tangentColumn, tangentRow = -tangentColumn, -tangentRow
	}
	angle := (35 + coordHash01(seed, join, 0, riverJunctionSalt)*20) * math.Pi / 180
	directionColumn := normalColumn*math.Sin(angle) + tangentColumn*math.Cos(angle)
	directionRow := normalRow*math.Sin(angle) + tangentRow*math.Cos(angle)
	tailLength := float64(clearance * 3)
	anchorColumn, anchorRow := int(math.Round(joinColumn+directionColumn*tailLength)), int(math.Round(joinRow+directionRow*tailLength))
	if anchorColumn < clearance || anchorRow < clearance || anchorColumn >= width-clearance || anchorRow >= height-clearance {
		return nil
	}
	startColumn, startRow := lakeShorePoint(source, anchorColumn, anchorRow, width, height)
	if math.Hypot(float64(startColumn-anchorColumn), float64(startRow-anchorRow)) < float64(clearance) {
		return nil
	}
	path := buildNestedBendPath(width, height, startColumn, startRow, anchorColumn, anchorRow, seed, opts)
	if len(path) < window {
		return nil
	}
	previous := path[len(path)-window]
	entryColumn, entryRow := float64(anchorColumn-previous%width), float64(anchorRow-previous/width)
	entryLength := math.Hypot(entryColumn, entryRow)
	if entryLength == 0 {
		return nil
	}
	controlFirstColumn := float64(anchorColumn) + entryColumn/entryLength*tailLength*0.3
	controlFirstRow := float64(anchorRow) + entryRow/entryLength*tailLength*0.3
	controlLastColumn, controlLastRow := joinColumn+directionColumn*tailLength*0.35, joinRow+directionRow*tailLength*0.35
	steps := int(math.Ceil(tailLength * 3))
	for step := 1; step <= steps; step++ {
		fraction := float64(step) / float64(steps)
		inverse := 1 - fraction
		column := int(math.Round(inverse*inverse*inverse*float64(anchorColumn) + 3*inverse*inverse*fraction*controlFirstColumn + 3*inverse*fraction*fraction*controlLastColumn + fraction*fraction*fraction*joinColumn))
		row := int(math.Round(inverse*inverse*inverse*float64(anchorRow) + 3*inverse*inverse*fraction*controlFirstRow + 3*inverse*fraction*fraction*controlLastRow + fraction*fraction*fraction*joinRow))
		if column < 0 || row < 0 || column >= width || row >= height {
			return nil
		}
		last := path[len(path)-1]
		path = appendCardinalRiverSegment(path, width, last%width, last/width, column, row)
		if len(path) > 8*(width+height) {
			return nil
		}
	}
	return path
}

func (state *riverJunctionState) validateCandidate(plan *riverFairways, path []int, source int, target riverJunctionSample, opts RiverOptions) string {
	clearance := riverCorridorMaximumWidth(opts) + riverJunctionSampleStep
	if len(path) < clearance*3 || path[len(path)-1] != target.Tile {
		return "shape"
	}
	if !riverPathSelfSeparated(path, state.Width, riverCorridorMaximumWidth(opts)) {
		return "self_contact"
	}
	localArc := clearance * 3
	parentTiles := make(map[int]bool, localArc*2+1)
	parent := plan.Routes[target.Route].Path
	for position := maxInt(0, target.Position-localArc); position <= minInt(len(parent)-1, target.Position+localArc); position++ {
		parentTiles[parent[position]] = true
	}
	start := image.Pt(path[0]%state.Width, path[0]/state.Width)
	join := image.Pt(target.Tile%state.Width, target.Tile/state.Width)
	sourceBounds := state.LakeBounds[source].Inset(-clearance / 2)
	leftSource := false
	for position, tile := range path {
		if position > 0 && absInt(tile%state.Width-path[position-1]%state.Width)+absInt(tile/state.Width-path[position-1]/state.Width) != 1 {
			return "shape"
		}
		if position < len(path)-1 && parentTiles[tile] {
			return "parent_crossing"
		}
		point := image.Pt(tile%state.Width, tile/state.Width)
		if !point.In(image.Rect(clearance/2, clearance/2, state.Width-clearance/2, state.Height-clearance/2)) {
			return "border"
		}
		if !point.In(sourceBounds) {
			leftSource = true
		} else if leftSource {
			return "source_reentry"
		}
		for lakeID, bounds := range state.LakeBounds {
			if lakeID != source && point.In(bounds.Inset(-clearance/2)) {
				return "other_lake"
			}
		}
		if !state.visit(image.Rect(point.X-clearance, point.Y-clearance, point.X+clearance+1, point.Y+clearance+1), func(sample riverJunctionSample) bool {
			if sample.Route == target.Route && absInt(sample.Position-target.Position) <= localArc && len(path)-1-position <= localArc && maxInt(absInt(point.X-join.X), absInt(point.Y-join.Y)) <= localArc {
				return true
			}
			origins, ordinary := state.Origins[sample.Route]
			return ordinary && (origins[0] == source || origins[1] == source) && position <= clearance*2 && maxInt(absInt(point.X-start.X), absInt(point.Y-start.Y)) <= clearance*2 && point.In(sourceBounds)
		}) {
			return "foreign_contact"
		}
	}
	return ""
}
