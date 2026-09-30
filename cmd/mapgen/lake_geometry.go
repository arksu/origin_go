package main

import (
	"fmt"
	"image"
	"math"
)

const (
	lakeShoreSalt       = uint64(0x791D938B725C461F)
	lakePeninsulaSalt   = uint64(0x93A725CF189D634B)
	lakeIslandSalt      = uint64(0xB369C2517F4928AD)
	lakeIslandCountSalt = uint64(0xCE87249361AF5B09)
	lakeIslandShapeSalt = uint64(0xEA943C5271B6D08F)
	lakeBankSalt        = uint64(0x53498AEF716C2DB5)
)

type lakeIsland struct {
	Center image.Point
	Radius int
	Tiles  []int
	Loop   []int
}

type lakeShape struct {
	Lake       drawLake
	Bounds     image.Rectangle
	Water      []bool
	Land       []bool
	Baseline   []bool
	Blocked    []bool
	Islands    []lakeIsland
	Routes     []riverRoute
	Peninsulas int
	Fallback   bool
}

type lakeClassStats struct {
	Lakes, Selected, Requested, Placed, Rejected int
}

type lakeGeometryStats struct {
	Classes               [3]lakeClassStats
	Peninsulas, Fallbacks int
}

func (shape *lakeShape) index(column, row int) int {
	return (row-shape.Bounds.Min.Y)*shape.Bounds.Dx() + column - shape.Bounds.Min.X
}

func (shape *lakeShape) point(index int) image.Point {
	return image.Pt(shape.Bounds.Min.X+index%shape.Bounds.Dx(), shape.Bounds.Min.Y+index/shape.Bounds.Dx())
}

func requestedLakeIslands(seed int64, lake drawLake, opts RiverOptions) int {
	chance := opts.LakeIslandSmallChance
	if lake.SizeClass == lakeSizeMedium {
		chance = opts.LakeIslandMediumChance
	} else if lake.SizeClass == lakeSizeLarge {
		chance = opts.LakeIslandLargeChance
	}
	if coordHash01(seed, lake.ID, 0, lakeIslandSalt) >= chance {
		return 0
	}
	if lake.SizeClass == lakeSizeLarge && coordHash01(seed, lake.ID, 0, lakeIslandCountSalt) < opts.LakeIslandSecondChance {
		return 2
	}
	return 1
}

func buildIrregularLakeShape(flow []uint32, width, height int, seed int64, lake drawLake, inlets []lakeInlet, opts RiverOptions) (*lakeShape, error) {
	shape, err := prepareLakeShape(flow, width, height, seed, lake, inlets, opts)
	if err != nil {
		return nil, err
	}
	return detailLakeShape(shape, flow, width, seed, inlets, opts)
}

func prepareLakeShape(flow []uint32, width, height int, seed int64, lake drawLake, inlets []lakeInlet, opts RiverOptions) (*lakeShape, error) {
	basins := buildLakeBasins(lake)
	padding := opts.LakeShoreVariationTiles + opts.FairwayWidthTiles + 4
	bounds := image.Rect(lake.X, lake.Y, lake.X+1, lake.Y+1)
	for _, basin := range basins {
		bounds = bounds.Union(image.Rect(int(math.Floor(basin.X-basin.RadiusX))-padding, int(math.Floor(basin.Y-basin.RadiusY))-padding, int(math.Ceil(basin.X+basin.RadiusX))+padding+1, int(math.Ceil(basin.Y+basin.RadiusY))+padding+1))
	}
	for _, inlet := range inlets {
		bounds = bounds.Union(image.Rect(inlet.X-padding, inlet.Y-padding, inlet.X+padding+1, inlet.Y+padding+1))
	}
	bounds = bounds.Intersect(image.Rect(0, 0, width, height))
	shape := &lakeShape{Lake: lake, Bounds: bounds, Water: make([]bool, bounds.Dx()*bounds.Dy()), Land: make([]bool, bounds.Dx()*bounds.Dy())}
	baseline := make([]bool, len(shape.Water))
	shape.Baseline = baseline
	scale := clampFloat(float64(lake.Radius)*0.14, 8, 32)
	for index := range shape.Water {
		point := shape.point(index)
		contour, _ := lakeContourValue(lake, basins, float64(point.X), float64(point.Y))
		baseline[index] = contour >= lakeShoreThreshold(lake)
		displacement := func(salt uint64) float64 {
			coarse := smoothHashNoise2D(seed+int64(lake.ID)*137, float64(point.X), float64(point.Y), scale, salt)*2 - 1
			fine := smoothHashNoise2D(seed+int64(lake.ID)*137, float64(point.X), float64(point.Y), maxFloat(3, scale/3), salt^lakeShapeSaltA)*2 - 1
			return float64(opts.LakeShoreVariationTiles) * (coarse*0.7 + fine*0.3)
		}
		warped, _ := lakeContourValue(lake, basins, float64(point.X)+displacement(lakeShoreSalt), float64(point.Y)+displacement(lakeShoreSalt^lakeShapeSaltB))
		shape.Water[index] = warped >= lakeShoreThreshold(lake)
		if baseline[index] && flow[tileIndex(point.X, point.Y, width)] >= uint32(opts.FlowShallowThreshold) {
			shape.Water[index] = true
		}
	}
	fillLakeHoles(shape.Water, bounds.Dx(), bounds.Dy())
	if !lakeWaterConnected(shape.Water, bounds.Dx()) {
		shape.Water = append([]bool(nil), baseline...)
		fillLakeHoles(shape.Water, bounds.Dx(), bounds.Dy())
		shape.Fallback = true
	}
	prepareEntrances := func() error {
		if err := shape.attachInlets(inlets, width, opts); err != nil {
			return err
		}
		_, err := shape.navigation(inlets, width, opts)
		return err
	}
	if err := prepareEntrances(); err != nil {
		shape.Water = append([]bool(nil), baseline...)
		fillLakeHoles(shape.Water, bounds.Dx(), bounds.Dy())
		shape.Fallback = true
		if err := prepareEntrances(); err != nil {
			return nil, err
		}
	}
	fillLakeHoles(shape.Water, bounds.Dx(), bounds.Dy())
	return shape, nil
}

func detailLakeShape(shape *lakeShape, flow []uint32, width int, seed int64, inlets []lakeInlet, opts RiverOptions) (*lakeShape, error) {
	lake := shape.Lake
	if opts.LakePeninsulaDepthRatio > 0 && opts.LakePeninsulaCountMax > 0 {
		count := 1 + int(coordHash01(seed, lake.ID, 0, lakePeninsulaSalt^lakeIslandCountSalt)*float64(opts.LakePeninsulaCountMax))
		for feature := 0; feature < count; feature++ {
			for attempt := 0; attempt < 8; attempt++ {
				cut := shape.peninsula(seed, feature*8+attempt, opts)
				if shape.acceptCut(cut, flow, width, inlets, opts) {
					shape.Peninsulas++
					break
				}
			}
		}
	}
	waterArea := 0
	for _, water := range shape.Water {
		if water {
			waterArea++
		}
	}
	islandArea := 0
	for islandID := 0; islandID < requestedLakeIslands(seed, lake, opts); islandID++ {
		for attempt := 0; attempt < 8; attempt++ {
			island, cut := shape.island(seed, islandID*8+attempt, width, opts)
			if len(island.Tiles)+islandArea > waterArea*8/100 || len(island.Tiles) == 0 {
				continue
			}
			previous := shape.Water
			candidate := append([]bool(nil), previous...)
			valid := true
			for index, removed := range cut {
				if !removed {
					continue
				}
				point := shape.point(index)
				if !previous[index] || (len(shape.Blocked) > 0 && shape.Blocked[index]) || flow[tileIndex(point.X, point.Y, width)] >= uint32(opts.FlowShallowThreshold) {
					valid = false
					break
				}
				candidate[index] = false
			}
			if !valid {
				continue
			}
			shape.Water = candidate
			island.Loop = shape.islandLoop(island, width, opts)
			if len(island.Loop) == 0 || !shape.islandLoopsFit(width, opts) {
				shape.Water = previous
				continue
			}
			if _, err := shape.navigation(inlets, width, opts); err != nil {
				shape.Water = previous
				continue
			}
			for index, removed := range cut {
				if removed {
					shape.Land[index] = true
				}
			}
			shape.Islands = append(shape.Islands, island)
			islandArea += len(island.Tiles)
			break
		}
	}
	for index, wasWater := range shape.Baseline {
		if wasWater && !shape.Water[index] && (len(shape.Blocked) == 0 || !shape.Blocked[index]) {
			shape.Land[index] = true
		}
	}
	routes, err := shape.navigation(inlets, width, opts)
	if err != nil {
		return nil, err
	}
	shape.Routes = routes
	shape.Baseline, shape.Blocked = nil, nil
	for _, island := range shape.Islands {
		shape.Routes = append(shape.Routes, riverRoute{Path: island.Loop, Width: opts.FairwayWidthTiles, Role: "island", Parent: lake.ID})
	}
	return shape, nil
}

func lakeNeighbors(index, width, count int, visit func(int)) {
	if index%width > 0 {
		visit(index - 1)
	}
	if index%width+1 < width {
		visit(index + 1)
	}
	if index >= width {
		visit(index - width)
	}
	if index+width < count {
		visit(index + width)
	}
}

func lakeWaterConnected(water []bool, width int) bool {
	start, count := -1, 0
	for index, present := range water {
		if present {
			start = index
			count++
		}
	}
	if start < 0 {
		return false
	}
	visited := make([]bool, len(water))
	queue := []int{start}
	visited[start] = true
	for position := 0; position < len(queue); position++ {
		lakeNeighbors(queue[position], width, len(water), func(next int) {
			if water[next] && !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		})
	}
	return len(queue) == count
}

func fillLakeHoles(water []bool, width, height int) {
	visited := make([]bool, len(water))
	queue := make([]int, 0)
	add := func(index int) {
		if !water[index] && !visited[index] {
			visited[index] = true
			queue = append(queue, index)
		}
	}
	for column := 0; column < width; column++ {
		add(column)
		add((height-1)*width + column)
	}
	for row := 0; row < height; row++ {
		add(row * width)
		add(row*width + width - 1)
	}
	for position := 0; position < len(queue); position++ {
		lakeNeighbors(queue[position], width, len(water), add)
	}
	for index := range water {
		if !visited[index] {
			water[index] = true
		}
	}
}

func lakeWaterDistances(water []bool, width int) []int {
	height := len(water) / width
	distances := make([]int, len(water))
	for index, present := range water {
		if !present {
			continue
		}
		distances[index] = minInt(minInt(index%width+1, width-index%width), minInt(index/width+1, height-index/width))
	}
	for index := range water {
		if !water[index] {
			continue
		}
		column := index % width
		if column > 0 {
			distances[index] = minInt(distances[index], distances[index-1]+1)
		}
		if index >= width {
			for offset := -1; offset <= 1; offset++ {
				if column+offset >= 0 && column+offset < width {
					distances[index] = minInt(distances[index], distances[index-width+offset]+1)
				}
			}
		}
	}
	for index := len(water) - 1; index >= 0; index-- {
		if !water[index] {
			continue
		}
		column := index % width
		if column+1 < width {
			distances[index] = minInt(distances[index], distances[index+1]+1)
		}
		if index+width < len(water) {
			for offset := -1; offset <= 1; offset++ {
				if column+offset >= 0 && column+offset < width {
					distances[index] = minInt(distances[index], distances[index+width+offset]+1)
				}
			}
		}
	}
	return distances
}

func (shape *lakeShape) closestPlacement(point image.Point, distances []int, radius int) int {
	best, bestDistance := -1, math.MaxInt
	for index, distance := range distances {
		if distance <= radius {
			continue
		}
		candidate := shape.point(index)
		separation := (candidate.X-point.X)*(candidate.X-point.X) + (candidate.Y-point.Y)*(candidate.Y-point.Y)
		if separation < bestDistance {
			best, bestDistance = index, separation
		}
	}
	return best
}

func (shape *lakeShape) attachInlets(inlets []lakeInlet, worldWidth int, opts RiverOptions) error {
	radius := opts.FairwayWidthTiles / 2
	distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
	for _, inlet := range inlets {
		nearest := shape.closestPlacement(image.Pt(inlet.X, inlet.Y), distances, radius)
		if nearest < 0 {
			return fmt.Errorf("lake %d has no water footprint for inlet (%d,%d)", shape.Lake.ID, inlet.X, inlet.Y)
		}
		target := shape.point(nearest)
		path := appendCardinalRiverSegment([]int{tileIndex(inlet.X, inlet.Y, worldWidth)}, worldWidth, inlet.X, inlet.Y, target.X, target.Y)
		for _, index := range path {
			point := image.Pt(index%worldWidth, index/worldWidth)
			for offsetRow := -radius; offsetRow <= radius; offsetRow++ {
				for offsetColumn := -radius; offsetColumn <= radius; offsetColumn++ {
					candidate := point.Add(image.Pt(offsetColumn, offsetRow))
					if !candidate.In(shape.Bounds) {
						return fmt.Errorf("lake %d inlet footprint outside local bounds", shape.Lake.ID)
					}
					shape.Water[shape.index(candidate.X, candidate.Y)] = true
				}
			}
		}
	}
	return nil
}

func (shape *lakeShape) navigation(inlets []lakeInlet, worldWidth int, opts RiverOptions) ([]riverRoute, error) {
	distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
	radius := opts.FairwayWidthTiles / 2
	hub := shape.closestPlacement(image.Pt(shape.Lake.X, shape.Lake.Y), distances, radius)
	if hub < 0 {
		return nil, fmt.Errorf("lake %d has no valid water hub", shape.Lake.ID)
	}
	parents := make([]int, len(shape.Water))
	for index := range parents {
		parents[index] = -1
	}
	parents[hub] = hub
	queue := []int{hub}
	for position := 0; position < len(queue); position++ {
		current := queue[position]
		lakeNeighbors(current, shape.Bounds.Dx(), len(shape.Water), func(next int) {
			if parents[next] == -1 && distances[next] > radius {
				parents[next] = current
				queue = append(queue, next)
			}
		})
	}
	routes := make([]riverRoute, 0, len(inlets))
	for _, inlet := range inlets {
		start := shape.index(inlet.X, inlet.Y)
		if parents[start] == -1 {
			return nil, fmt.Errorf("lake %d inlet (%d,%d) has no full-footprint water route", shape.Lake.ID, inlet.X, inlet.Y)
		}
		path := make([]int, 0)
		for current := start; ; current = parents[current] {
			point := shape.point(current)
			path = append(path, tileIndex(point.X, point.Y, worldWidth))
			if current == hub {
				break
			}
		}
		routes = append(routes, riverRoute{Path: path, Width: inlet.RiverWidth, Role: "inlet", Parent: shape.Lake.ID})
	}
	return routes, nil
}

func (shape *lakeShape) peninsula(seed int64, attempt int, opts RiverOptions) []bool {
	cut := make([]bool, len(shape.Water))
	angle := coordHash01(seed, shape.Lake.ID, attempt, lakePeninsulaSalt) * 2 * math.Pi
	unitColumn, unitRow := math.Cos(angle), math.Sin(angle)
	distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
	hub := shape.closestPlacement(image.Pt(shape.Lake.X, shape.Lake.Y), distances, opts.FairwayWidthTiles/2)
	if hub < 0 {
		return cut
	}
	center := shape.point(hub)
	shoreDistance := 0.0
	for distance := 1.0; distance < float64(shape.Bounds.Dx()+shape.Bounds.Dy()); distance++ {
		point := image.Pt(center.X+int(math.Round(unitColumn*distance)), center.Y+int(math.Round(unitRow*distance)))
		if !point.In(shape.Bounds) || !shape.Water[shape.index(point.X, point.Y)] {
			shoreDistance = distance
			break
		}
	}
	localRadius := float64(minInt(shape.Lake.RadiusX, shape.Lake.RadiusY))
	depth := minFloat(shoreDistance*0.55, localRadius*opts.LakePeninsulaDepthRatio*(0.7+0.3*coordHash01(seed, attempt, shape.Lake.ID, lakePeninsulaSalt^lakeShapeSaltA)))
	rootWidth := maxFloat(3, localRadius*(0.10+0.10*coordHash01(seed, shape.Lake.ID, attempt, lakePeninsulaSalt^lakeShapeSaltB)))
	for index := range cut {
		point := shape.point(index)
		deltaColumn, deltaRow := float64(point.X-center.X), float64(point.Y-center.Y)
		along := deltaColumn*unitColumn + deltaRow*unitRow - shoreDistance
		if along < -depth || along > rootWidth*1.5 {
			continue
		}
		fraction := clamp01((along + depth) / (depth + rootWidth))
		bend := math.Sin(fraction*math.Pi) * rootWidth * 0.3
		across := -deltaColumn*unitRow + deltaRow*unitColumn - bend
		halfWidth := rootWidth * (0.25 + 0.75*math.Sqrt(fraction))
		roughness := (smoothHashNoise2D(seed+int64(shape.Lake.ID), float64(point.X), float64(point.Y), maxFloat(4, rootWidth/2), lakePeninsulaSalt) - 0.5) * 2
		cut[index] = math.Abs(across) <= halfWidth+roughness
	}
	return cut
}

func (shape *lakeShape) acceptCut(cut []bool, flow []uint32, worldWidth int, inlets []lakeInlet, opts RiverOptions) bool {
	previous := shape.Water
	candidate := append([]bool(nil), previous...)
	removed := 0
	for index, present := range cut {
		if !present {
			continue
		}
		point := shape.point(index)
		if (len(shape.Blocked) > 0 && shape.Blocked[index]) || flow[tileIndex(point.X, point.Y, worldWidth)] >= uint32(opts.FlowShallowThreshold) {
			return false
		}
		if candidate[index] {
			candidate[index] = false
			removed++
		}
	}
	if removed < maxInt(6, opts.FairwayWidthTiles*2) || !lakeWaterConnected(candidate, shape.Bounds.Dx()) {
		return false
	}
	filled := append([]bool(nil), candidate...)
	fillLakeHoles(filled, shape.Bounds.Dx(), shape.Bounds.Dy())
	for index := range candidate {
		if candidate[index] != filled[index] {
			return false
		}
	}
	shape.Water = candidate
	if _, err := shape.navigation(inlets, worldWidth, opts); err != nil {
		shape.Water = previous
		return false
	}
	for index, present := range cut {
		if present && !candidate[index] {
			shape.Land[index] = true
		}
	}
	return true
}

func (shape *lakeShape) island(seed int64, attempt, worldWidth int, opts RiverOptions) (lakeIsland, []bool) {
	radius := opts.LakeIslandRadiusMin + int(coordHash01(seed, shape.Lake.ID, attempt, lakeIslandSalt^lakeShapeSaltA)*float64(opts.LakeIslandRadiusMax-opts.LakeIslandRadiusMin+1))
	center := image.Pt(shape.Bounds.Min.X+int(coordHash01(seed, shape.Lake.ID, attempt, lakeIslandSalt^lakeShapeSaltB)*float64(shape.Bounds.Dx())), shape.Bounds.Min.Y+int(coordHash01(seed, shape.Lake.ID, attempt, lakeIslandSalt^lakeShapeSaltC)*float64(shape.Bounds.Dy())))
	island := lakeIsland{Center: center, Radius: radius}
	cut := make([]bool, len(shape.Water))
	sample := func(channel int) float64 {
		return coordHash01(seed, shape.Lake.ID, attempt*8+channel, lakeIslandShapeSalt)
	}
	rotation := sample(0) * 2 * math.Pi
	sinRotation, cosRotation := math.Sincos(rotation)
	minorRatio := 0.65 + sample(1)*0.35
	phaseA, phaseB := sample(2)*2*math.Pi, sample(3)*2*math.Pi
	bayAngle := sample(4) * 2 * math.Pi
	bayDepth, bayWidth := 0.20+sample(5)*0.18, 0.45+sample(6)*0.35
	noiseAmplitude := minFloat(0.07, 0.65/float64(radius))
	extent := int(math.Ceil(float64(radius)*1.25)) + 1
	diameter := extent*2 + 1
	outline := make([]bool, diameter*diameter)
	for row := center.Y - extent; row <= center.Y+extent; row++ {
		for column := center.X - extent; column <= center.X+extent; column++ {
			point := image.Pt(column, row)
			deltaColumn, deltaRow := float64(column-center.X), float64(row-center.Y)
			major := (deltaColumn*cosRotation + deltaRow*sinRotation) / float64(radius)
			minor := (-deltaColumn*sinRotation + deltaRow*cosRotation) / (float64(radius) * minorRatio)
			angle := math.Atan2(minor, major)
			bayDistance := math.Remainder(angle-bayAngle, 2*math.Pi) / bayWidth
			shore := 0.88 + 0.13*math.Sin(angle+phaseA) + 0.17*math.Sin(3*angle+phaseB) + 0.07*math.Sin(5*angle+phaseA-phaseB)
			shore -= bayDepth * math.Exp(-0.5*bayDistance*bayDistance)
			shore += (smoothHashNoise2D(seed+int64(shape.Lake.ID), float64(column), float64(row), maxFloat(3, float64(radius)/3), lakeIslandShapeSalt)*2 - 1) * noiseAmplitude
			shore = clampFloat(shore, 0.48, 1.25)
			if major*major+minor*minor > shore*shore {
				continue
			}
			if !point.In(shape.Bounds) {
				return lakeIsland{}, cut
			}
			cut[shape.index(column, row)] = true
			outline[(row-center.Y+extent)*diameter+column-center.X+extent] = true
			island.Tiles = append(island.Tiles, tileIndex(column, row, worldWidth))
		}
	}
	if !lakeWaterConnected(outline, diameter) {
		return lakeIsland{}, cut
	}
	for index := range outline {
		outline[index] = !outline[index]
	}
	if !lakeWaterConnected(outline, diameter) {
		return lakeIsland{}, cut
	}
	return island, cut
}

func (shape *lakeShape) islandLoop(island lakeIsland, worldWidth int, opts RiverOptions) []int {
	if len(island.Tiles) == 0 {
		return nil
	}
	first := island.Tiles[0]
	bounds := image.Rect(first%worldWidth, first/worldWidth, first%worldWidth+1, first/worldWidth+1)
	for _, index := range island.Tiles {
		bounds = bounds.Union(image.Rect(index%worldWidth, index/worldWidth, index%worldWidth+1, index/worldWidth+1))
	}
	padding := opts.LakeShallowWidthMax + opts.FairwayWidthTiles/2 + 1
	bounds = bounds.Inset(-padding)
	if !bounds.Min.In(shape.Bounds) || !bounds.Max.Sub(image.Pt(1, 1)).In(shape.Bounds) {
		return nil
	}
	distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
	path := []int{tileIndex(bounds.Min.X, bounds.Min.Y, worldWidth)}
	for _, point := range []image.Point{image.Pt(bounds.Max.X-1, bounds.Min.Y), bounds.Max.Sub(image.Pt(1, 1)), image.Pt(bounds.Min.X, bounds.Max.Y-1), bounds.Min} {
		previous := path[len(path)-1]
		path = appendCardinalRiverSegment(path, worldWidth, previous%worldWidth, previous/worldWidth, point.X, point.Y)
	}
	for _, index := range path {
		if distances[shape.index(index%worldWidth, index/worldWidth)] <= opts.LakeShallowWidthMax+opts.FairwayWidthTiles/2 {
			return nil
		}
	}
	return path
}

func (shape *lakeShape) islandLoopsFit(worldWidth int, opts RiverOptions) bool {
	if len(shape.Islands) == 0 {
		return true
	}
	distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
	for _, island := range shape.Islands {
		for _, index := range island.Loop {
			if distances[shape.index(index%worldWidth, index/worldWidth)] <= opts.LakeShallowWidthMax+opts.FairwayWidthTiles/2 {
				return false
			}
		}
	}
	return true
}

func lakeBankWidth(seed int64, lake drawLake, column, row int, opts RiverOptions) int {
	noise := smoothHashNoise2D(seed+int64(lake.ID)*137, float64(column), float64(row), clampFloat(float64(lake.Radius)*0.3, 16, 64), lakeBankSalt)
	return opts.LakeShallowWidthMin + int(math.Round(noise*float64(opts.LakeShallowWidthMax-opts.LakeShallowWidthMin)))
}

func finishIrregularLakes(flow []uint32, plan *riverFairways, width, height int, seed int64, opts RiverOptions) error {
	plan.LakeLand = make([]bool, len(flow))
	plan.LakeWater = make([]bool, len(flow))
	shapes := make([]*lakeShape, len(plan.Lakes))
	initialWater := make([][]bool, len(plan.Lakes))
	lakeInlets := make([][]lakeInlet, len(plan.Lakes))
	for _, inlet := range plan.Inlets {
		lakeInlets[inlet.LakeIndex] = append(lakeInlets[inlet.LakeIndex], inlet)
	}
	for index, lake := range plan.Lakes {
		shape, err := prepareLakeShape(flow, width, height, seed, lake, lakeInlets[index], opts)
		if err != nil {
			return fmt.Errorf("seed %d: %w", seed, err)
		}
		shapes[index], initialWater[index] = shape, shape.Water
	}
	for index, shape := range shapes {
		shape.Blocked = make([]bool, len(shape.Water))
		for otherIndex, other := range shapes {
			if index == otherIndex {
				continue
			}
			intersection := shape.Bounds.Intersect(other.Bounds)
			for row := intersection.Min.Y; row < intersection.Max.Y; row++ {
				for column := intersection.Min.X; column < intersection.Max.X; column++ {
					if initialWater[otherIndex][other.index(column, row)] {
						shape.Blocked[shape.index(column, row)] = true
					}
				}
			}
		}
	}
	for _, lake := range plan.Lakes {
		shape, err := detailLakeShape(shapes[lake.ID], flow, width, seed, lakeInlets[lake.ID], opts)
		if err != nil {
			return fmt.Errorf("seed %d: %w", seed, err)
		}
		distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
		for index, present := range shape.Water {
			point := shape.point(index)
			worldIndex := tileIndex(point.X, point.Y, width)
			if shape.Land[index] {
				if flow[worldIndex] >= uint32(opts.FlowShallowThreshold) || plan.Protected[worldIndex] {
					return fmt.Errorf("seed %d lake %d reserved land conflicts with water at (%d,%d)", seed, lake.ID, point.X, point.Y)
				}
				plan.LakeLand[worldIndex] = true
			}
			if !present {
				continue
			}
			plan.LakeWater[worldIndex] = true
			if plan.LakeLand[worldIndex] {
				return fmt.Errorf("seed %d lake %d overlaps another lake's reserved land at (%d,%d)", seed, lake.ID, point.X, point.Y)
			}
			value := uint32(opts.FlowDeepThreshold)
			if distances[index] <= lakeBankWidth(seed, lake, point.X, point.Y, opts) {
				value = uint32(opts.FlowShallowThreshold)
			}
			if flow[worldIndex] < value {
				flow[worldIndex] = value
			}
		}
		plan.LakeShapes = append(plan.LakeShapes, shape)
		stats := &plan.LakeStats.Classes[lake.SizeClass]
		stats.Lakes++
		requested := requestedLakeIslands(seed, lake, opts)
		if requested > 0 {
			stats.Selected++
		}
		stats.Requested += requested
		stats.Placed += len(shape.Islands)
		stats.Rejected += requested - len(shape.Islands)
		plan.LakeStats.Peninsulas += shape.Peninsulas
		if shape.Fallback {
			plan.LakeStats.Fallbacks++
		}
		for _, route := range shape.Routes {
			plan.Routes = append(plan.Routes, protectRiverRoute(flow, plan, route, width, height, opts))
		}
	}
	return nil
}

func (plan *riverFairways) isLakeLand(index int) bool {
	return plan != nil && len(plan.LakeLand) > 0 && plan.LakeLand[index]
}

func (plan *riverFairways) validateLakeLand(seed int64, isWater func(int) bool, width int) error {
	if plan == nil {
		return nil
	}
	for _, shape := range plan.LakeShapes {
		for index, land := range shape.Land {
			if !land {
				continue
			}
			point := shape.point(index)
			worldIndex := tileIndex(point.X, point.Y, width)
			if plan.Protected[worldIndex] || isWater(worldIndex) {
				return fmt.Errorf("seed %d lake %d reserved land became water at (%d,%d)", seed, shape.Lake.ID, point.X, point.Y)
			}
		}
	}
	return nil
}

func riverPathTouchesLakeLand(plan *riverFairways, path []int, width, height, radius int) bool {
	if len(plan.LakeLand) == 0 {
		return false
	}
	for _, index := range path {
		column, row := index%width, index/width
		for offsetRow := -radius; offsetRow <= radius; offsetRow++ {
			for offsetColumn := -radius; offsetColumn <= radius; offsetColumn++ {
				candidateColumn, candidateRow := column+offsetColumn, row+offsetRow
				if candidateColumn >= 0 && candidateRow >= 0 && candidateColumn < width && candidateRow < height && plan.LakeLand[tileIndex(candidateColumn, candidateRow, width)] {
					return true
				}
			}
		}
	}
	return false
}
