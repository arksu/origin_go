package main

import (
	"fmt"
	"math"
)

type RiverClass byte

const (
	riverNone RiverClass = iota
	riverShallow
	riverDeep
)

const (
	riverSourceSalt    = uint64(0x9E3779B185EBCA87)
	riverMeanderSalt   = uint64(0xC2B2AE3D27D4EB4F)
	riverSinkLakeSalt  = uint64(0x165667B19E3779F9)
	riverLakeLinkSalt  = uint64(0x85EBCA77C2B2AE63)
	riverWidthSalt     = uint64(0x27D4EB2F165667C5)
	riverGridSalt      = uint64(0xA24BAED4963EE407)
	voronoiPointXSalt  = uint64(0x94D049BB133111EB)
	voronoiPointYSalt  = uint64(0xBF58476D1CE4E5B9)
	lakeLinkJitterSalt = uint64(0x369DEA0F31A53F85)
	lakeLinkTargetSalt = uint64(0xDB4F0B9175AE2165)
)

type RiverNetwork struct {
	Flow        []uint32
	Class       []RiverClass
	SourceCount int
}

type inlandLake struct {
	ID              int
	TileCount       int
	AnchorX         int
	AnchorY         int
	AnchorElevation float64
	MeanElevation   float64
}

type coastPoint struct {
	X int
	Y int
}

type sourceCandidate struct {
	X         int
	Y         int
	Elevation float64
	Edge      float64
}

type outletPoint struct {
	X         int
	Y         int
	Elevation float64
}

type drawLake struct {
	ID     int
	X      int
	Y      int
	Radius int
}

func BuildRiverNetwork(
	elevation []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
) (*RiverNetwork, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("river network dimensions must be positive, got width=%d height=%d", width, height)
	}
	tileCount, err := checkedMulInt(width, height)
	if err != nil {
		return nil, fmt.Errorf("river network tile count overflow: %w", err)
	}
	if len(elevation) != tileCount {
		return nil, fmt.Errorf("elevation length mismatch: got=%d want=%d", len(elevation), tileCount)
	}

	if opts.LayoutDraw {
		flow, sourceCount := buildDrawLayoutRiverFlow(elevation, width, height, seed, opts)
		classMask := buildRiverClassMask(flow, width, height, opts)
		return &RiverNetwork{
			Flow:        flow,
			Class:       classMask,
			SourceCount: sourceCount,
		}, nil
	}

	flow, sourceCount, err := buildElevationRoutedRiverFlow(elevation, width, height, seed, opts)
	if err != nil {
		return nil, err
	}
	classMask := buildRiverClassMask(flow, width, height, opts)

	return &RiverNetwork{
		Flow:        flow,
		Class:       classMask,
		SourceCount: sourceCount,
	}, nil
}

func buildElevationRoutedRiverFlow(
	elevation []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
) ([]uint32, int, error) {
	tileCount := width * height

	voronoiEdgeStrength, err := buildVoronoiEdgeStrength(width, height, seed, opts.VoronoiCellSize, opts.VoronoiEdgeThreshold)
	if err != nil {
		return nil, 0, err
	}

	flow := make([]uint32, tileCount)
	sourceCount := 0
	maxSteps := (width + height) * 2

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := tileIndex(x, y, width)
			elev := float64(elevation[idx])
			if elev < opts.SourceElevationMin {
				continue
			}

			effectiveSourceChance := opts.SourceChance + float64(voronoiEdgeStrength[idx])*opts.VoronoiSourceBoost
			if effectiveSourceChance > 1 {
				effectiveSourceChance = 1
			}
			if coordHash01(seed, x, y, riverSourceSalt) >= effectiveSourceChance {
				continue
			}

			sourceCount++
			path, sink := traceDownhillPath(
				elevation,
				voronoiEdgeStrength,
				width,
				height,
				x,
				y,
				seed,
				opts.MeanderStrength,
				opts.VoronoiBias,
				maxSteps,
			)
			for _, pathIdx := range path {
				flow[pathIdx]++
			}
			if sink && len(path) > 0 {
				sinkIdx := path[len(path)-1]
				sinkX, sinkY := sinkIdx%width, sinkIdx/width
				if flow[sinkIdx] >= uint32(opts.LakeFlowThreshold) &&
					coordHash01(seed, sinkX, sinkY, riverSinkLakeSalt) < opts.SinkLakeChance {
					carveSinkLake(flow, width, height, sinkX, sinkY, opts)
				}
			}
		}
	}

	if opts.LakeConnectionLimit > 0 && opts.LakeConnectChance > 0 {
		lakes := detectInlandLakes(elevation, width, height, opts.LakeMinSize)
		connectSubsetOfLakes(flow, elevation, voronoiEdgeStrength, width, height, seed, opts, lakes)
	}

	effective := opts
	if effective.TrunkRiverCount > 0 {
		generateMajorTrunkRivers(flow, elevation, voronoiEdgeStrength, width, height, seed, effective)
	}
	if effective.GridEnabled {
		generateUniformGridRivers(flow, elevation, voronoiEdgeStrength, width, height, seed, effective)
	}
	coverage := riverLandCoverage(flow, elevation, effective)
	if coverage < 0.006 && effective.GridEnabled {
		dense := effective
		dense.GridSpacing = maxInt(dense.GridSpacing*3/4, 320)
		maxJitter := maxInt(0, dense.GridSpacing/2-1)
		dense.GridJitter = minInt(maxJitter, maxInt(dense.GridJitter, dense.GridSpacing/6))
		generateUniformGridRivers(flow, elevation, voronoiEdgeStrength, width, height, seed+9173, dense)
	}
	coverage = riverLandCoverage(flow, elevation, effective)
	if coverage < 0.01 && effective.GridEnabled {
		dense := effective
		dense.GridSpacing = maxInt(dense.GridSpacing/2, 220)
		maxJitter := maxInt(0, dense.GridSpacing/2-1)
		dense.GridJitter = minInt(maxJitter, maxInt(dense.GridJitter, dense.GridSpacing/4))
		dense.RiverWidthMin = maxInt(dense.RiverWidthMin, 7)
		dense.RiverWidthMax = maxInt(dense.RiverWidthMax, dense.RiverWidthMin+3)
		generateUniformGridRivers(flow, elevation, voronoiEdgeStrength, width, height, seed+18347, dense)
	}
	coverage = riverLandCoverage(flow, elevation, effective)
	if coverage < 0.01 && effective.TrunkRiverCount > 0 {
		aggressive := effective
		aggressive.TrunkRiverCount = maxInt(aggressive.TrunkRiverCount*2, 18)
		aggressive.TrunkMinLength = maxInt(aggressive.TrunkMinLength/2, 120)
		generateMajorTrunkRivers(flow, elevation, voronoiEdgeStrength, width, height, seed+26599, aggressive)
	}

	return flow, sourceCount, nil
}

func buildDrawLayoutRiverFlow(
	elevation []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
) ([]uint32, int) {
	_ = elevation
	flow := make([]uint32, width*height)
	lakes := generateDrawLakes(width, height, seed, opts)
	if len(lakes) == 0 {
		return flow, 0
	}
	for _, lake := range lakes {
		carveDrawLakeFootprint(flow, width, height, lake, opts)
	}

	degree := make([]int, len(lakes))
	connected := make([]bool, len(lakes))
	usedPairs := make(map[uint64]struct{}, opts.MajorRiverCount+opts.LakeConnectionLimit+8)
	linksCreated := 0

	majorCount := maxInt(1, opts.MajorRiverCount)
	borderLinks := clampInt(int(math.Round(float64(majorCount)*opts.LakeBorderMix)), 0, majorCount)
	lakeToLakeLinks := majorCount - borderLinks

	for linkID := 0; linkID < lakeToLakeLinks; linkID++ {
		from, to, ok := selectDrawLakePair(lakes, degree, connected, usedPairs, width, height, seed+int64(linkID)*1619, opts)
		if !ok {
			break
		}
		pairKey := makeLakePairKey(lakes[from].ID, lakes[to].ID)
		path, ok := buildDrawPathWithAttempts(flow, width, height, lakes[from].X, lakes[from].Y, lakes[to].X, lakes[to].Y, seed+int64(linkID)*1223, opts)
		if !ok {
			usedPairs[pairKey] = struct{}{}
			continue
		}

		riverWidth := riverWidthForLink(seed, lakes[from].ID, lakes[to].ID, opts)
		carveRiverCorridor(flow, width, height, path, riverWidth, opts)
		degree[from]++
		degree[to]++
		connected[from] = true
		connected[to] = true
		usedPairs[pairKey] = struct{}{}
		linksCreated++
	}

	sideOffset := int(splitMix64(uint64(seed)^riverGridSalt) % 4)
	for linkID := 0; linkID < borderLinks; linkID++ {
		side := (sideOffset + linkID) % 4
		sourceIdx, ok := selectDrawLakeForBorder(lakes, degree, connected, side, width, height, seed+int64(linkID)*2371, opts.MaxLakeDegree)
		if !ok {
			break
		}
		targetX, targetY := chooseDrawBorderTarget(width, height, side, lakes[sourceIdx], seed, int64(linkID))
		path, ok := buildDrawPathWithAttempts(flow, width, height, lakes[sourceIdx].X, lakes[sourceIdx].Y, targetX, targetY, seed+int64(linkID)*3203, opts)
		if !ok {
			continue
		}

		riverWidth := riverWidthForLink(seed, lakes[sourceIdx].ID, 1_000_000+linkID, opts)
		carveRiverCorridor(flow, width, height, path, riverWidth, opts)
		degree[sourceIdx]++
		connected[sourceIdx] = true
		linksCreated++
	}

	desiredConnectedLakes := clampInt(int(math.Round(float64(len(lakes))*opts.LakeConnectChance)), 0, len(lakes))
	extraBudget := maxInt(0, opts.LakeConnectionLimit)
	for extraID := 0; extraID < extraBudget; extraID++ {
		if countTrue(connected) >= desiredConnectedLakes {
			break
		}

		sourceIdx, ok := selectDrawExtraSourceLake(lakes, degree, connected, seed+int64(extraID)*4447, opts.MaxLakeDegree)
		if !ok {
			break
		}

		targetIdx, ok := selectDrawTargetLakeForSource(sourceIdx, lakes, degree, connected, usedPairs, width, height, seed+int64(extraID)*5081, opts)
		if ok {
			pairKey := makeLakePairKey(lakes[sourceIdx].ID, lakes[targetIdx].ID)
			path, ok := buildDrawPathWithAttempts(flow, width, height, lakes[sourceIdx].X, lakes[sourceIdx].Y, lakes[targetIdx].X, lakes[targetIdx].Y, seed+int64(extraID)*5437, opts)
			if !ok {
				usedPairs[pairKey] = struct{}{}
				continue
			}
			riverWidth := riverWidthForLink(seed, lakes[sourceIdx].ID, lakes[targetIdx].ID, opts)
			carveRiverCorridor(flow, width, height, path, riverWidth, opts)
			degree[sourceIdx]++
			degree[targetIdx]++
			connected[sourceIdx] = true
			connected[targetIdx] = true
			usedPairs[pairKey] = struct{}{}
			linksCreated++
			continue
		}

		side := (sideOffset + borderLinks + extraID) % 4
		targetX, targetY := chooseDrawBorderTarget(width, height, side, lakes[sourceIdx], seed+7919, int64(extraID))
		path, ok := buildDrawPathWithAttempts(flow, width, height, lakes[sourceIdx].X, lakes[sourceIdx].Y, targetX, targetY, seed+int64(extraID)*5743, opts)
		if !ok {
			continue
		}
		riverWidth := riverWidthForLink(seed, lakes[sourceIdx].ID, 1_500_000+extraID, opts)
		carveRiverCorridor(flow, width, height, path, riverWidth, opts)
		degree[sourceIdx]++
		connected[sourceIdx] = true
		linksCreated++
	}

	return flow, linksCreated
}

func generateDrawLakes(width int, height int, seed int64, opts RiverOptions) []drawLake {
	if opts.LakeCount <= 0 {
		return nil
	}

	target := opts.LakeCount
	lakes := make([]drawLake, 0, target)
	margin := maxInt(6, opts.RiverWidthMax*2+4)
	if margin*2 >= width || margin*2 >= height {
		margin = 1
	}
	minGap := maxInt(2, opts.RiverWidthMax/2)

	gridCols := int(math.Round(math.Sqrt(float64(target) * float64(width) / float64(maxInt(height, 1)))))
	if gridCols < 1 {
		gridCols = 1
	}
	gridRows := int(math.Ceil(float64(target) / float64(gridCols)))
	if gridRows < 1 {
		gridRows = 1
	}

	cellWidth := maxInt(1, width/gridCols)
	cellHeight := maxInt(1, height/gridRows)
	jitterX := maxInt(2, cellWidth/3)
	jitterY := maxInt(2, cellHeight/3)

	for gy := 0; gy < gridRows && len(lakes) < target; gy++ {
		for gx := 0; gx < gridCols && len(lakes) < target; gx++ {
			baseX := gx*cellWidth + cellWidth/2
			baseY := gy*cellHeight + cellHeight/2

			xOffset := (coordHash01(seed, gx, gy, voronoiPointXSalt) - 0.5) * 2 * float64(jitterX)
			yOffset := (coordHash01(seed, gy, gx, voronoiPointYSalt) - 0.5) * 2 * float64(jitterY)

			x := clampInt(int(math.Round(float64(baseX)+xOffset)), margin, width-margin-1)
			y := clampInt(int(math.Round(float64(baseY)+yOffset)), margin, height-margin-1)
			radius := drawLakeRadiusForID(seed, len(lakes), opts)
			if !canPlaceDrawLake(lakes, x, y, radius, minGap) {
				continue
			}
			lakes = append(lakes, drawLake{
				ID:     len(lakes),
				X:      x,
				Y:      y,
				Radius: radius,
			})
		}
	}

	attemptLimit := target * 24
	for attempt := 0; attempt < attemptLimit && len(lakes) < target; attempt++ {
		xRange := maxInt(1, width-2*margin)
		yRange := maxInt(1, height-2*margin)
		x := margin + int(math.Floor(coordHash01(seed, attempt, target, riverGridSalt)*float64(xRange)))
		y := margin + int(math.Floor(coordHash01(seed, target, attempt, riverGridSalt^0x1248ACED)*float64(yRange)))
		x = clampInt(x, margin, width-margin-1)
		y = clampInt(y, margin, height-margin-1)
		radius := drawLakeRadiusForID(seed, len(lakes)+attempt, opts)
		if !canPlaceDrawLake(lakes, x, y, radius, minGap) {
			continue
		}
		lakes = append(lakes, drawLake{
			ID:     len(lakes),
			X:      x,
			Y:      y,
			Radius: radius,
		})
	}

	return lakes
}

func drawLakeRadiusForID(seed int64, lakeID int, opts RiverOptions) int {
	minRadius := maxInt(2, opts.RiverWidthMin/2+1)
	maxRadius := maxInt(minRadius, opts.RiverWidthMax/2+3)
	if minRadius == maxRadius {
		return minRadius
	}
	span := maxRadius - minRadius + 1
	v := coordHash01(seed, lakeID, maxRadius, riverSinkLakeSalt)
	return minRadius + int(math.Floor(v*float64(span)))
}

func canPlaceDrawLake(lakes []drawLake, x, y, radius, minGap int) bool {
	for _, lake := range lakes {
		required := lake.Radius + radius + minGap
		dx := lake.X - x
		dy := lake.Y - y
		if dx*dx+dy*dy < required*required {
			return false
		}
	}
	return true
}

func carveDrawLakeFootprint(flow []uint32, width, height int, lake drawLake, opts RiverOptions) {
	outerRadius := maxInt(2, lake.Radius)
	innerRadius := maxInt(1, int(math.Round(float64(outerRadius)*0.55)))
	outerSq := outerRadius * outerRadius
	innerSq := innerRadius * innerRadius

	for dy := -outerRadius; dy <= outerRadius; dy++ {
		for dx := -outerRadius; dx <= outerRadius; dx++ {
			d2 := dx*dx + dy*dy
			if d2 > outerSq {
				continue
			}
			x := lake.X + dx
			y := lake.Y + dy
			if x < 0 || y < 0 || x >= width || y >= height {
				continue
			}
			idx := tileIndex(x, y, width)
			if d2 <= innerSq {
				if flow[idx] < uint32(opts.FlowDeepThreshold) {
					flow[idx] = uint32(opts.FlowDeepThreshold)
				}
				continue
			}
			if flow[idx] < uint32(opts.FlowShallowThreshold) {
				flow[idx] = uint32(opts.FlowShallowThreshold)
			}
		}
	}
}

func selectDrawLakePair(
	lakes []drawLake,
	degree []int,
	connected []bool,
	usedPairs map[uint64]struct{},
	width int,
	height int,
	seed int64,
	opts RiverOptions,
) (int, int, bool) {
	if len(lakes) < 2 {
		return 0, 0, false
	}
	bestA := 0
	bestB := 0
	bestScore := 0.0
	found := false

	targetDist := math.Sqrt(float64(width*height)/float64(maxInt(1, opts.MajorRiverCount))) * 1.2
	minDistance := maxFloat(float64(maxInt(24, opts.RiverWidthMax*3)), float64(opts.LakeLinkMinDistance))
	maxDistance := float64(opts.LakeLinkMaxDistance)
	if maxDistance <= 0 {
		maxDistance = float64(maxInt(width, height))
	}
	if maxDistance < minDistance {
		maxDistance = minDistance
	}

	for i := 0; i < len(lakes)-1; i++ {
		if degree[i] >= opts.MaxLakeDegree {
			continue
		}
		for j := i + 1; j < len(lakes); j++ {
			if degree[j] >= opts.MaxLakeDegree {
				continue
			}
			pairKey := makeLakePairKey(lakes[i].ID, lakes[j].ID)
			if _, exists := usedPairs[pairKey]; exists {
				continue
			}

			distance := math.Hypot(float64(lakes[i].X-lakes[j].X), float64(lakes[i].Y-lakes[j].Y))
			if distance < minDistance || distance > maxDistance {
				continue
			}

			score := math.Abs(distance-targetDist) + float64(degree[i]+degree[j])*850
			if connected[i] {
				score += 180
			}
			if connected[j] {
				score += 180
			}

			coarseCellAX := lakes[i].X * 6 / maxInt(1, width)
			coarseCellAY := lakes[i].Y * 6 / maxInt(1, height)
			coarseCellBX := lakes[j].X * 6 / maxInt(1, width)
			coarseCellBY := lakes[j].Y * 6 / maxInt(1, height)
			if coarseCellAX == coarseCellBX && coarseCellAY == coarseCellBY {
				score += 160
			}

			score += coordHash01(seed, lakes[i].ID, lakes[j].ID, riverLakeLinkSalt) * 80
			if !found || score < bestScore {
				found = true
				bestA = i
				bestB = j
				bestScore = score
			}
		}
	}

	return bestA, bestB, found
}

func selectDrawLakeForBorder(
	lakes []drawLake,
	degree []int,
	connected []bool,
	side int,
	width int,
	height int,
	seed int64,
	maxDegree int,
) (int, bool) {
	bestIdx := 0
	bestScore := 0.0
	found := false

	for idx, lake := range lakes {
		if degree[idx] >= maxDegree {
			continue
		}
		distanceToSide := 0.0
		switch side {
		case 0:
			distanceToSide = float64(lake.Y)
		case 1:
			distanceToSide = float64(width - 1 - lake.X)
		case 2:
			distanceToSide = float64(height - 1 - lake.Y)
		default:
			distanceToSide = float64(lake.X)
		}
		score := distanceToSide + float64(degree[idx])*500
		if connected[idx] {
			score += 120
		}
		score += coordHash01(seed, lake.X, lake.Y, riverGridSalt) * 60
		if !found || score < bestScore {
			found = true
			bestIdx = idx
			bestScore = score
		}
	}

	return bestIdx, found
}

func chooseDrawBorderTarget(width int, height int, side int, source drawLake, seed int64, linkID int64) (int, int) {
	switch side {
	case 0:
		jitter := int(math.Round((coordHash01(seed+linkID, source.X, source.Y, riverWidthSalt) - 0.5) * 2 * float64(maxInt(12, width/8))))
		return clampInt(source.X+jitter, 1, width-2), 1
	case 1:
		jitter := int(math.Round((coordHash01(seed+linkID, source.Y, source.X, riverWidthSalt) - 0.5) * 2 * float64(maxInt(12, height/8))))
		return width - 2, clampInt(source.Y+jitter, 1, height-2)
	case 2:
		jitter := int(math.Round((coordHash01(seed+linkID, source.X, source.Y, riverWidthSalt^0x13579) - 0.5) * 2 * float64(maxInt(12, width/8))))
		return clampInt(source.X+jitter, 1, width-2), height - 2
	default:
		jitter := int(math.Round((coordHash01(seed+linkID, source.Y, source.X, riverWidthSalt^0x2468ACE) - 0.5) * 2 * float64(maxInt(12, height/8))))
		return 1, clampInt(source.Y+jitter, 1, height-2)
	}
}

func selectDrawExtraSourceLake(
	lakes []drawLake,
	degree []int,
	connected []bool,
	seed int64,
	maxDegree int,
) (int, bool) {
	bestIdx := 0
	bestScore := 0.0
	found := false

	for idx, lake := range lakes {
		if connected[idx] || degree[idx] >= maxDegree {
			continue
		}
		score := float64(degree[idx])*1000 + coordHash01(seed, lake.X, lake.Y, riverSourceSalt)*100
		if !found || score < bestScore {
			found = true
			bestIdx = idx
			bestScore = score
		}
	}

	if found {
		return bestIdx, true
	}

	for idx, lake := range lakes {
		if degree[idx] >= maxDegree {
			continue
		}
		score := float64(degree[idx])*1000 + coordHash01(seed, lake.X, lake.Y, riverSourceSalt)*100
		if !found || score < bestScore {
			found = true
			bestIdx = idx
			bestScore = score
		}
	}

	return bestIdx, found
}

func selectDrawTargetLakeForSource(
	sourceIdx int,
	lakes []drawLake,
	degree []int,
	connected []bool,
	usedPairs map[uint64]struct{},
	width int,
	height int,
	seed int64,
	opts RiverOptions,
) (int, bool) {
	bestIdx := 0
	bestScore := 0.0
	found := false

	source := lakes[sourceIdx]
	targetDist := math.Sqrt(float64(width*height)/float64(maxInt(1, opts.MajorRiverCount))) * 1.1
	minDistance := maxFloat(float64(maxInt(20, opts.RiverWidthMax*2)), float64(opts.LakeLinkMinDistance))
	maxDistance := float64(opts.LakeLinkMaxDistance)
	if maxDistance <= 0 {
		maxDistance = float64(maxInt(width, height))
	}
	if maxDistance < minDistance {
		maxDistance = minDistance
	}

	for idx, candidate := range lakes {
		if idx == sourceIdx || degree[idx] >= opts.MaxLakeDegree {
			continue
		}
		pairKey := makeLakePairKey(source.ID, candidate.ID)
		if _, exists := usedPairs[pairKey]; exists {
			continue
		}

		distance := math.Hypot(float64(source.X-candidate.X), float64(source.Y-candidate.Y))
		if distance < minDistance || distance > maxDistance {
			continue
		}

		score := math.Abs(distance-targetDist) + float64(degree[idx])*600
		if connected[idx] {
			score -= 140
		}
		score += coordHash01(seed, source.ID, candidate.ID, lakeLinkTargetSalt) * 60
		if !found || score < bestScore {
			found = true
			bestIdx = idx
			bestScore = score
		}
	}

	return bestIdx, found
}

func buildDrawPathWithAttempts(
	flow []uint32,
	width int,
	height int,
	startX int,
	startY int,
	targetX int,
	targetY int,
	seed int64,
	opts RiverOptions,
) ([]int, bool) {
	bestOverlap := math.MaxFloat64
	var bestPath []int
	minLen := maxInt(8, opts.RiverWidthMax)

	for attempt := 0; attempt < 6; attempt++ {
		path := buildDrawPath(width, height, startX, startY, targetX, targetY, seed+int64(attempt)*8191, opts)
		if len(path) < minLen {
			continue
		}
		overlap := pathOverlapRatio(flow, path, opts)
		if overlap < bestOverlap {
			bestOverlap = overlap
			bestPath = path
		}
		if overlap <= 0.30 {
			break
		}
	}

	if len(bestPath) == 0 {
		return nil, false
	}
	return bestPath, true
}

func buildDrawPath(
	width int,
	height int,
	startX int,
	startY int,
	targetX int,
	targetY int,
	seed int64,
	opts RiverOptions,
) []int {
	if startX == targetX && startY == targetY {
		return []int{tileIndex(startX, startY, width)}
	}

	dx := float64(targetX - startX)
	dy := float64(targetY - startY)
	distance := math.Hypot(dx, dy)
	if distance < 1 {
		return []int{tileIndex(startX, startY, width)}
	}

	segments := maxInt(4, minInt(32, int(distance/70)+3))
	dirX := dx / distance
	dirY := dy / distance
	perpX := -dirY
	perpY := dirX

	baseAmplitude := float64(maxInt(10, opts.RiverWidthMax*3))
	meanderAmplitude := float64(minInt(width, height)) * opts.MeanderStrength * 8.5
	amplitude := maxFloat(baseAmplitude, meanderAmplitude)
	amplitude = minFloat(amplitude, distance*0.40)
	if amplitude < 2 {
		amplitude = 2
	}
	waveCount := 1.25 + coordHash01(seed, startX, startY, riverMeanderSalt)*1.75
	phase := coordHash01(seed, targetX, targetY, lakeLinkJitterSalt) * 2 * math.Pi

	controlX := make([]int, 0, segments+1)
	controlY := make([]int, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		px := float64(startX) + dx*t
		py := float64(startY) + dy*t

		if i > 0 && i < segments {
			falloff := math.Sin(math.Pi * t)
			wave := math.Sin((t*waveCount*2*math.Pi)+phase) * amplitude * falloff
			noise := (coordHash01(seed+int64(i)*37, int(math.Round(px)), int(math.Round(py)), riverMeanderSalt) - 0.5) * 2 * (amplitude * 0.30) * falloff
			cross := wave + noise
			along := (coordHash01(seed+int64(i)*59, int(math.Round(py)), int(math.Round(px)), lakeLinkJitterSalt) - 0.5) * 2 * (amplitude * 0.16) * falloff
			px += perpX*cross + dirX*along
			py += perpY*cross + dirY*along
		}

		cx := clampInt(int(math.Round(px)), 1, width-2)
		cy := clampInt(int(math.Round(py)), 1, height-2)
		controlX = append(controlX, cx)
		controlY = append(controlY, cy)
	}

	path := rasterizeSmoothControlPath(width, height, controlX, controlY)
	if len(path) == 0 {
		path = append(path, tileIndex(startX, startY, width))
		path = append(path, tileIndex(targetX, targetY, width))
	}
	return path
}

func rasterizeSmoothControlPath(width int, height int, controlX []int, controlY []int) []int {
	if len(controlX) == 0 || len(controlX) != len(controlY) {
		return nil
	}
	if len(controlX) == 1 {
		return []int{tileIndex(controlX[0], controlY[0], width)}
	}

	seen := make(map[int]struct{}, len(controlX)*8)
	path := make([]int, 0, len(controlX)*8)
	prevX := controlX[0]
	prevY := controlY[0]
	path = appendRasterizedPathSegment(path, seen, width, prevX, prevY, prevX, prevY)

	for segment := 0; segment < len(controlX)-1; segment++ {
		p0x, p0y := controlPointAt(controlX, controlY, segment-1)
		p1x, p1y := controlPointAt(controlX, controlY, segment)
		p2x, p2y := controlPointAt(controlX, controlY, segment+1)
		p3x, p3y := controlPointAt(controlX, controlY, segment+2)

		segmentLength := math.Hypot(float64(p2x-p1x), float64(p2y-p1y))
		steps := maxInt(6, minInt(120, int(math.Ceil(segmentLength*1.35))))
		for step := 1; step <= steps; step++ {
			t := float64(step) / float64(steps)
			sx := catmullRom(float64(p0x), float64(p1x), float64(p2x), float64(p3x), t)
			sy := catmullRom(float64(p0y), float64(p1y), float64(p2y), float64(p3y), t)
			x := clampInt(int(math.Round(sx)), 1, width-2)
			y := clampInt(int(math.Round(sy)), 1, height-2)
			path = appendRasterizedPathSegment(path, seen, width, prevX, prevY, x, y)
			prevX = x
			prevY = y
		}
	}

	return path
}

func controlPointAt(controlX []int, controlY []int, index int) (int, int) {
	if index < 0 {
		return controlX[0], controlY[0]
	}
	if index >= len(controlX) {
		last := len(controlX) - 1
		return controlX[last], controlY[last]
	}
	return controlX[index], controlY[index]
}

func catmullRom(p0, p1, p2, p3, t float64) float64 {
	t2 := t * t
	t3 := t2 * t
	return 0.5 * ((2 * p1) +
		(-p0+p2)*t +
		(2*p0-5*p1+4*p2-p3)*t2 +
		(-p0+3*p1-3*p2+p3)*t3)
}

func appendRasterizedPathSegment(path []int, seen map[int]struct{}, width int, x0 int, y0 int, x1 int, y1 int) []int {
	dx := absInt(x1 - x0)
	dy := absInt(y1 - y0)
	stepX := signInt(x1 - x0)
	stepY := signInt(y1 - y0)
	err := dx - dy

	for {
		idx := tileIndex(x0, y0, width)
		if _, exists := seen[idx]; !exists {
			seen[idx] = struct{}{}
			path = append(path, idx)
		}
		if x0 == x1 && y0 == y1 {
			break
		}

		e2 := err * 2
		if e2 > -dy {
			err -= dy
			x0 += stepX
		}
		if e2 < dx {
			err += dx
			y0 += stepY
		}
	}

	return path
}

func pathOverlapRatio(flow []uint32, path []int, opts RiverOptions) float64 {
	if len(path) == 0 {
		return 1
	}
	overlap := 0
	for _, idx := range path {
		if idx < 0 || idx >= len(flow) {
			continue
		}
		if flow[idx] >= uint32(opts.FlowShallowThreshold) {
			overlap++
		}
	}
	return float64(overlap) / float64(len(path))
}

func countTrue(values []bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func traceDownhillPath(
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	startX int,
	startY int,
	seed int64,
	meanderStrength float64,
	voronoiBias float64,
	maxSteps int,
) ([]int, bool) {
	path := make([]int, 0, maxSteps)
	x := startX
	y := startY

	for step := 0; step < maxSteps; step++ {
		idx := tileIndex(x, y, width)
		path = append(path, idx)

		currentElevation := float64(elevation[idx])
		if isOceanTileByElevation(currentElevation) {
			return path, false
		}
		if x == 0 || y == 0 || x == width-1 || y == height-1 {
			return path, false
		}

		nextX, nextY, ok := chooseLowerNeighbor(
			elevation,
			voronoiEdgeStrength,
			width,
			height,
			x,
			y,
			seed,
			meanderStrength,
			voronoiBias,
		)
		if !ok {
			return path, true
		}
		x = nextX
		y = nextY
	}

	return path, false
}

func chooseLowerNeighbor(
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	x int,
	y int,
	seed int64,
	meanderStrength float64,
	voronoiBias float64,
) (int, int, bool) {
	currentIdx := tileIndex(x, y, width)
	currentElevation := float64(elevation[currentIdx])

	bestX := 0
	bestY := 0
	bestElevation := 0.0
	bestScore := 0.0
	found := false

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := x + dx
			ny := y + dy
			if nx < 0 || ny < 0 || nx >= width || ny >= height {
				continue
			}
			nIdx := tileIndex(nx, ny, width)
			nElevation := float64(elevation[nIdx])
			if nElevation >= currentElevation {
				continue
			}

			score := nElevation
			if meanderStrength > 0 {
				meander := (coordHash01(seed, nx, ny, riverMeanderSalt) - 0.5) * meanderStrength
				score += meander
			}
			if voronoiBias > 0 && len(voronoiEdgeStrength) == len(elevation) {
				score -= float64(voronoiEdgeStrength[nIdx]) * voronoiBias
			}

			if !found || score < bestScore || (nearlyEqual(score, bestScore) && nElevation < bestElevation) {
				found = true
				bestX = nx
				bestY = ny
				bestElevation = nElevation
				bestScore = score
			}
		}
	}

	return bestX, bestY, found
}

func carveSinkLake(flow []uint32, width, height, sinkX, sinkY int, opts RiverOptions) {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x := sinkX + dx
			y := sinkY + dy
			if x < 0 || y < 0 || x >= width || y >= height {
				continue
			}
			idx := tileIndex(x, y, width)
			if dx == 0 && dy == 0 {
				if flow[idx] < uint32(opts.FlowDeepThreshold) {
					flow[idx] = uint32(opts.FlowDeepThreshold)
				}
				continue
			}
			if flow[idx] < uint32(opts.FlowShallowThreshold) {
				flow[idx] = uint32(opts.FlowShallowThreshold)
			}
		}
	}
}

func detectInlandLakes(elevation []float32, width, height, minSize int) []inlandLake {
	visited := make([]bool, len(elevation))
	queue := make([]int, 0, 1024)
	lakes := make([]inlandLake, 0, 64)
	lakeID := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			startIdx := tileIndex(x, y, width)
			if visited[startIdx] || !isOceanTileByElevation(float64(elevation[startIdx])) {
				continue
			}

			queue = queue[:0]
			queue = append(queue, startIdx)
			visited[startIdx] = true

			tileCount := 0
			touchesEdge := false
			sumX := 0.0
			sumY := 0.0
			sumElevation := 0.0
			anchorIdx := startIdx
			anchorElevation := float64(elevation[startIdx])

			for head := 0; head < len(queue); head++ {
				idx := queue[head]
				cx := idx % width
				cy := idx / width
				tileCount++
				sumX += float64(cx)
				sumY += float64(cy)
				ce := float64(elevation[idx])
				sumElevation += ce
				if ce < anchorElevation {
					anchorElevation = ce
					anchorIdx = idx
				}

				if cx == 0 || cy == 0 || cx == width-1 || cy == height-1 {
					touchesEdge = true
				}

				for _, n := range []struct{ x, y int }{{cx - 1, cy}, {cx + 1, cy}, {cx, cy - 1}, {cx, cy + 1}} {
					if n.x < 0 || n.y < 0 || n.x >= width || n.y >= height {
						continue
					}
					nIdx := tileIndex(n.x, n.y, width)
					if visited[nIdx] || !isOceanTileByElevation(float64(elevation[nIdx])) {
						continue
					}
					visited[nIdx] = true
					queue = append(queue, nIdx)
				}
			}

			if touchesEdge || tileCount < minSize {
				continue
			}

			ax := anchorIdx % width
			ay := anchorIdx / width
			lakes = append(lakes, inlandLake{
				ID:              lakeID,
				TileCount:       tileCount,
				AnchorX:         ax,
				AnchorY:         ay,
				AnchorElevation: anchorElevation,
				MeanElevation:   sumElevation / float64(tileCount),
			})
			lakeID++
		}
	}

	return lakes
}

func connectSubsetOfLakes(
	flow []uint32,
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
	lakes []inlandLake,
) {
	if len(lakes) < 2 || opts.LakeConnectionLimit <= 0 {
		return
	}

	connections := 0
	usedPairs := make(map[uint64]struct{}, opts.LakeConnectionLimit)

	for _, source := range lakes {
		if connections >= opts.LakeConnectionLimit {
			break
		}
		if coordHash01(seed, source.AnchorX, source.AnchorY, riverLakeLinkSalt) >= opts.LakeConnectChance {
			continue
		}

		target, ok := selectLakeTarget(seed, source, lakes, opts)
		if !ok {
			continue
		}

		pairKey := makeLakePairKey(source.ID, target.ID)
		if _, exists := usedPairs[pairKey]; exists {
			continue
		}

		path, success := buildLakeLinkPath(
			elevation,
			voronoiEdgeStrength,
			width,
			height,
			seed,
			opts,
			source.AnchorX,
			source.AnchorY,
			target.AnchorX,
			target.AnchorY,
		)
		if !success || len(path) < 2 {
			continue
		}

		riverWidth := riverWidthForLink(seed, source.ID, target.ID, opts)
		carveRiverCorridor(flow, width, height, path, riverWidth, opts)
		usedPairs[pairKey] = struct{}{}
		connections++
	}
}

func generateMajorTrunkRivers(
	flow []uint32,
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
) {
	sources := collectTrunkSourceCandidates(elevation, voronoiEdgeStrength, width, height, opts)
	if len(sources) == 0 {
		return
	}
	outlets := collectBorderOutletPoints(elevation, width, height, seed, opts.CoastSampleChance)
	if len(outlets) == 0 {
		return
	}

	usedSources := make(map[int]struct{}, opts.TrunkRiverCount)
	selectedSources := make([]coastPoint, 0, opts.TrunkRiverCount)
	for trunkID := 0; trunkID < opts.TrunkRiverCount; trunkID++ {
		source, sourceIdx, ok := selectTrunkSource(seed, trunkID, sources, usedSources, selectedSources)
		if !ok {
			break
		}
		usedSources[sourceIdx] = struct{}{}
		selectedSources = append(selectedSources, source)

		target, ok := selectOutletTarget(seed, trunkID, source, outlets, opts)
		if !ok {
			continue
		}
		path, success := buildTrunkPath(
			elevation,
			voronoiEdgeStrength,
			width,
			height,
			seed+int64(trunkID)*7919,
			opts,
			source.X,
			source.Y,
			target.X,
			target.Y,
		)
		if !success || len(path) < opts.TrunkMinLength {
			continue
		}
		riverWidth := riverWidthForLink(seed, trunkID, trunkID+10_000, opts)
		carveRiverCorridor(flow, width, height, path, riverWidth, opts)
	}
}

func collectTrunkSourceCandidates(
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	opts RiverOptions,
) []sourceCandidate {
	candidates := make([]sourceCandidate, 0, 8192)
	minElevation := opts.TrunkSourceElevation
	edgeMinStrength := 0.2
	margin := maxInt(32, minInt(width, height)/12)
	for y := margin; y < height-margin; y++ {
		for x := margin; x < width-margin; x++ {
			idx := tileIndex(x, y, width)
			elev := float64(elevation[idx])
			if elev < minElevation {
				continue
			}
			if isOceanTileByElevation(elev) {
				continue
			}
			edgeStrength := 0.0
			if len(voronoiEdgeStrength) == len(elevation) {
				edgeStrength = float64(voronoiEdgeStrength[idx])
			}
			if edgeStrength < edgeMinStrength {
				continue
			}
			candidates = append(candidates, sourceCandidate{
				X:         x,
				Y:         y,
				Elevation: elev,
				Edge:      edgeStrength,
			})
		}
	}
	return candidates
}

func collectBorderOutletPoints(
	elevation []float32,
	width int,
	height int,
	seed int64,
	sampleChance float64,
) []outletPoint {
	points := make([]outletPoint, 0, 8192)
	addIfSelected := func(x, y int) {
		idx := tileIndex(x, y, width)
		elev := float64(elevation[idx])
		if elev > 0.45 && coordHash01(seed, x, y, riverLakeLinkSalt) > sampleChance {
			return
		}
		points = append(points, outletPoint{X: x, Y: y, Elevation: elev})
	}

	for x := 1; x < width-1; x++ {
		addIfSelected(x, 0)
		addIfSelected(x, height-1)
	}
	for y := 1; y < height-1; y++ {
		addIfSelected(0, y)
		addIfSelected(width-1, y)
	}

	if len(points) == 0 {
		for x := 1; x < width-1; x++ {
			points = append(points, outletPoint{X: x, Y: 0, Elevation: float64(elevation[tileIndex(x, 0, width)])})
			points = append(points, outletPoint{X: x, Y: height - 1, Elevation: float64(elevation[tileIndex(x, height-1, width)])})
		}
		for y := 1; y < height-1; y++ {
			points = append(points, outletPoint{X: 0, Y: y, Elevation: float64(elevation[tileIndex(0, y, width)])})
			points = append(points, outletPoint{X: width - 1, Y: y, Elevation: float64(elevation[tileIndex(width-1, y, width)])})
		}
	}
	return points
}

func selectTrunkSource(seed int64, trunkID int, sources []sourceCandidate, used map[int]struct{}, selected []coastPoint) (coastPoint, int, bool) {
	if len(sources) == 0 {
		return coastPoint{}, 0, false
	}
	scoreBest := 0.0
	best := coastPoint{}
	bestIdx := 0
	found := false
	centerX := float64(sources[0].X)
	centerY := float64(sources[0].Y)
	if len(sources) > 1 {
		centerX = 0
		centerY = 0
		for _, s := range sources {
			centerX += float64(s.X)
			centerY += float64(s.Y)
		}
		centerX /= float64(len(sources))
		centerY /= float64(len(sources))
	}

	for idx, source := range sources {
		if _, exists := used[idx]; exists {
			continue
		}
		spread := 0.0
		if len(selected) == 0 {
			dx := float64(source.X) - centerX
			dy := float64(source.Y) - centerY
			spread = math.Sqrt(dx*dx + dy*dy)
		} else {
			minDist := math.MaxFloat64
			for _, prior := range selected {
				dx := float64(source.X - prior.X)
				dy := float64(source.Y - prior.Y)
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < minDist {
					minDist = dist
				}
			}
			spread = minDist
		}
		jitter := coordHash01(seed+int64(trunkID)*104729, source.X, source.Y, riverSourceSalt) * 60
		score := spread*1.5 + source.Elevation*500 + source.Edge*260 + jitter
		if !found || score > scoreBest {
			found = true
			scoreBest = score
			best = coastPoint{X: source.X, Y: source.Y}
			bestIdx = idx
		}
	}
	return best, bestIdx, found
}

func selectOutletTarget(seed int64, trunkID int, source coastPoint, outlets []outletPoint, opts RiverOptions) (coastPoint, bool) {
	if len(outlets) == 0 {
		return coastPoint{}, false
	}
	best := coastPoint{}
	bestScore := 0.0
	found := false
	for _, outlet := range outlets {
		dx := outlet.X - source.X
		dy := outlet.Y - source.Y
		distance := math.Sqrt(float64(dx*dx + dy*dy))
		if distance < float64(opts.TrunkMinLength)/2 {
			continue
		}
		jitter := (coordHash01(seed+int64(trunkID)*8191, outlet.X, outlet.Y, lakeLinkTargetSalt) - 0.5) * 100
		coastPreference := (0.45 - outlet.Elevation) * 700
		score := distance*1.4 + coastPreference + jitter
		if !found || score > bestScore {
			found = true
			best = coastPoint{X: outlet.X, Y: outlet.Y}
			bestScore = score
		}
	}
	return best, found
}

func riverLandCoverage(flow []uint32, elevation []float32, opts RiverOptions) float64 {
	landTiles := 0
	riverTiles := 0
	for idx, elev := range elevation {
		if float64(elev) < shallowWaterThreshold {
			continue
		}
		landTiles++
		if flow[idx] >= uint32(opts.FlowShallowThreshold) {
			riverTiles++
		}
	}
	if landTiles == 0 {
		return 0
	}
	return float64(riverTiles) / float64(landTiles)
}

func generateUniformGridRivers(
	flow []uint32,
	elevation []float32,
	voronoiEdgeStrength []float32,
	width, height int,
	seed int64,
	opts RiverOptions,
) {
	if opts.GridSpacing <= 0 {
		return
	}
	outlets := collectBorderOutletPoints(elevation, width, height, seed+43117, maxFloat(opts.CoastSampleChance, 0.02))
	if len(outlets) == 0 {
		return
	}

	cellID := 0
	minPathLength := maxInt(60, opts.TrunkMinLength/2)
	minSourceElevation := maxFloat(0.45, opts.TrunkSourceElevation*0.8)
	searchRadius := maxInt(24, opts.GridSpacing/3)

	for yBase := opts.GridSpacing / 2; yBase < height-1; yBase += opts.GridSpacing {
		for xBase := opts.GridSpacing / 2; xBase < width-1; xBase += opts.GridSpacing {
			jitterX := int(math.Round((coordHash01(seed, xBase, yBase, riverGridSalt) - 0.5) * 2 * float64(opts.GridJitter)))
			jitterY := int(math.Round((coordHash01(seed, yBase, xBase, riverGridSalt^0xABCDEF) - 0.5) * 2 * float64(opts.GridJitter)))
			sx := clampInt(xBase+jitterX, 1, width-2)
			sy := clampInt(yBase+jitterY, 1, height-2)

			sourceX, sourceY, ok := snapToNearestLandSource(
				elevation,
				voronoiEdgeStrength,
				width,
				height,
				sx,
				sy,
				searchRadius,
				minSourceElevation,
			)
			if !ok {
				cellID++
				continue
			}

			source := coastPoint{X: sourceX, Y: sourceY}
			target, ok := selectOutletTarget(seed+int64(cellID)*29, cellID, source, outlets, opts)
			if !ok {
				cellID++
				continue
			}

			path, success := buildTrunkPath(
				elevation,
				voronoiEdgeStrength,
				width,
				height,
				seed+int64(cellID)*7919,
				opts,
				source.X,
				source.Y,
				target.X,
				target.Y,
			)
			if !success || len(path) < minPathLength {
				cellID++
				continue
			}

			riverWidth := riverWidthForLink(seed, 40000+cellID, 60000+cellID, opts)
			carveRiverCorridor(flow, width, height, path, riverWidth, opts)
			cellID++
		}
	}
}

func snapToNearestLandSource(
	elevation []float32,
	voronoiEdgeStrength []float32,
	width, height int,
	startX, startY int,
	searchRadius int,
	minElevation float64,
) (int, int, bool) {
	startIdx := tileIndex(startX, startY, width)
	startElevation := float64(elevation[startIdx])
	if startElevation >= minElevation && startElevation >= shallowWaterThreshold {
		return startX, startY, true
	}

	bestX := 0
	bestY := 0
	bestScore := -math.MaxFloat64
	found := false

	for dy := -searchRadius; dy <= searchRadius; dy++ {
		for dx := -searchRadius; dx <= searchRadius; dx++ {
			x := startX + dx
			y := startY + dy
			if x < 1 || y < 1 || x >= width-1 || y >= height-1 {
				continue
			}
			idx := tileIndex(x, y, width)
			elev := float64(elevation[idx])
			if elev < minElevation || elev < shallowWaterThreshold {
				continue
			}
			dist2 := dx*dx + dy*dy
			edge := 0.0
			if len(voronoiEdgeStrength) == len(elevation) {
				edge = float64(voronoiEdgeStrength[idx])
			}
			score := elev*200 + edge*120 - float64(dist2)
			if score > bestScore {
				bestScore = score
				bestX = x
				bestY = y
				found = true
			}
		}
	}

	return bestX, bestY, found
}

func selectLakeTarget(seed int64, source inlandLake, lakes []inlandLake, opts RiverOptions) (inlandLake, bool) {
	best := inlandLake{}
	bestScore := 0.0
	found := false

	for _, candidate := range lakes {
		if candidate.ID == source.ID {
			continue
		}
		dx := candidate.AnchorX - source.AnchorX
		dy := candidate.AnchorY - source.AnchorY
		distance := math.Sqrt(float64(dx*dx + dy*dy))
		if distance < float64(opts.LakeLinkMinDistance) || distance > float64(opts.LakeLinkMaxDistance) {
			continue
		}

		elevationPenalty := 0.0
		if candidate.MeanElevation > source.MeanElevation {
			elevationPenalty = (candidate.MeanElevation - source.MeanElevation) * 4000
		}
		targetJitter := (coordHash01(seed, candidate.AnchorX, candidate.AnchorY, lakeLinkTargetSalt) - 0.5) * 50
		score := distance + elevationPenalty + targetJitter

		if !found || score < bestScore {
			found = true
			best = candidate
			bestScore = score
		}
	}

	return best, found
}

func buildLakeLinkPath(
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
	startX int,
	startY int,
	targetX int,
	targetY int,
) ([]int, bool) {
	dx := targetX - startX
	dy := targetY - startY
	baseDistance := int(math.Sqrt(float64(dx*dx + dy*dy)))
	if baseDistance <= 0 {
		return []int{tileIndex(startX, startY, width)}, true
	}
	maxSteps := baseDistance*5 + 256
	if maxSteps < 256 {
		maxSteps = 256
	}

	path := make([]int, 0, maxSteps)
	visited := make(map[int]struct{}, maxSteps)
	cx := startX
	cy := startY

	for step := 0; step < maxSteps; step++ {
		idx := tileIndex(cx, cy, width)
		path = append(path, idx)
		visited[idx] = struct{}{}

		if absInt(cx-targetX) <= 1 && absInt(cy-targetY) <= 1 {
			return path, true
		}

		bestX := 0
		bestY := 0
		bestScore := 0.0
		found := false
		currentElevation := float64(elevation[idx])

		for dyN := -1; dyN <= 1; dyN++ {
			for dxN := -1; dxN <= 1; dxN++ {
				if dxN == 0 && dyN == 0 {
					continue
				}
				nx := cx + dxN
				ny := cy + dyN
				if nx <= 0 || ny <= 0 || nx >= width-1 || ny >= height-1 {
					continue
				}
				nIdx := tileIndex(nx, ny, width)
				if _, seen := visited[nIdx]; seen {
					continue
				}
				nElevation := float64(elevation[nIdx])

				dtx := targetX - nx
				dty := targetY - ny
				distanceToTarget := math.Sqrt(float64(dtx*dtx + dty*dty))

				elevationDelta := nElevation - currentElevation
				uphillPenalty := 0.0
				if elevationDelta > 0 {
					uphillPenalty = elevationDelta * 5000
				}

				voronoiBonus := 0.0
				if len(voronoiEdgeStrength) == len(elevation) {
					voronoiBonus = float64(voronoiEdgeStrength[nIdx]) * opts.VoronoiBias * 400
				}

				jitter := (coordHash01(seed+int64(step), nx, ny, lakeLinkJitterSalt) - 0.5) * opts.MeanderStrength * 300

				score := distanceToTarget + uphillPenalty + nElevation*20 - voronoiBonus + jitter
				if !found || score < bestScore {
					found = true
					bestX = nx
					bestY = ny
					bestScore = score
				}
			}
		}

		if !found {
			return path, false
		}

		cx = bestX
		cy = bestY
	}

	return path, false
}

func buildTrunkPath(
	elevation []float32,
	voronoiEdgeStrength []float32,
	width int,
	height int,
	seed int64,
	opts RiverOptions,
	startX int,
	startY int,
	targetX int,
	targetY int,
) ([]int, bool) {
	maxSteps := maxInt(width, height) * 4
	path := make([]int, 0, maxSteps)
	visited := make(map[int]struct{}, maxSteps)

	x := startX
	y := startY
	for step := 0; step < maxSteps; step++ {
		if x < 1 {
			x = 1
		}
		if y < 1 {
			y = 1
		}
		if x > width-2 {
			x = width - 2
		}
		if y > height-2 {
			y = height - 2
		}

		idx := tileIndex(x, y, width)
		if len(path) == 0 || path[len(path)-1] != idx {
			path = append(path, idx)
		}
		if absInt(x-targetX) <= 1 && absInt(y-targetY) <= 1 {
			return path, true
		}
		visited[idx] = struct{}{}

		currentElevation := float64(elevation[idx])
		currentDistance := math.Hypot(float64(targetX-x), float64(targetY-y))

		bestX := 0
		bestY := 0
		bestScore := math.MaxFloat64
		found := false

		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx := x + dx
				ny := y + dy
				if nx < 1 || ny < 1 || nx >= width-1 || ny >= height-1 {
					continue
				}
				nIdx := tileIndex(nx, ny, width)
				if _, seen := visited[nIdx]; seen && step < maxSteps-8 {
					continue
				}

				dist := math.Hypot(float64(targetX-nx), float64(targetY-ny))
				progress := currentDistance - dist
				if progress < -0.4 {
					continue
				}

				nElevation := float64(elevation[nIdx])
				uphillPenalty := 0.0
				if nElevation > currentElevation {
					uphillPenalty = (nElevation - currentElevation) * 1800
				}

				voronoiBonus := 0.0
				if len(voronoiEdgeStrength) == len(elevation) {
					voronoiBonus = float64(voronoiEdgeStrength[nIdx]) * opts.VoronoiBias * 260
				}

				meander := (coordHash01(seed+int64(step), nx, ny, lakeLinkJitterSalt) - 0.5) * opts.MeanderStrength * 180
				score := dist + uphillPenalty - voronoiBonus + meander
				if score < bestScore {
					bestScore = score
					bestX = nx
					bestY = ny
					found = true
				}
			}
		}

		if !found {
			// Guaranteed progress fallback: step toward outlet.
			nextX := x + signInt(targetX-x)
			nextY := y + signInt(targetY-y)
			if nextX < 1 {
				nextX = 1
			}
			if nextY < 1 {
				nextY = 1
			}
			if nextX > width-2 {
				nextX = width - 2
			}
			if nextY > height-2 {
				nextY = height - 2
			}
			if nextX == x && nextY == y {
				break
			}
			x = nextX
			y = nextY
			continue
		}

		x = bestX
		y = bestY
	}

	return path, len(path) >= opts.TrunkMinLength
}

func carveRiverCorridor(flow []uint32, width, height int, path []int, riverWidth int, opts RiverOptions) {
	if riverWidth < 1 {
		riverWidth = 1
	}
	radius := riverWidth / 2
	if radius < 1 {
		radius = 1
	}
	innerRadius := int(math.Round(float64(radius) * 0.45))
	if innerRadius < 1 {
		innerRadius = 1
	}
	radiusSq := radius * radius
	innerSq := innerRadius * innerRadius

	for _, idx := range path {
		cx := idx % width
		cy := idx / width
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dy*dy > radiusSq {
					continue
				}
				nx := cx + dx
				ny := cy + dy
				if nx < 0 || ny < 0 || nx >= width || ny >= height {
					continue
				}
				nIdx := tileIndex(nx, ny, width)
				if dx*dx+dy*dy <= innerSq {
					if flow[nIdx] < uint32(opts.FlowDeepThreshold) {
						flow[nIdx] = uint32(opts.FlowDeepThreshold)
					}
					continue
				}
				if flow[nIdx] < uint32(opts.FlowShallowThreshold) {
					flow[nIdx] = uint32(opts.FlowShallowThreshold)
				}
			}
		}
	}
}

func riverWidthForLink(seed int64, lakeAID, lakeBID int, opts RiverOptions) int {
	if opts.RiverWidthMin == opts.RiverWidthMax {
		return opts.RiverWidthMin
	}
	if opts.RiverWidthMin > opts.RiverWidthMax {
		return opts.RiverWidthMin
	}
	minID := lakeAID
	maxID := lakeBID
	if minID > maxID {
		minID, maxID = maxID, minID
	}
	span := opts.RiverWidthMax - opts.RiverWidthMin + 1
	r := coordHash01(seed, minID, maxID, riverWidthSalt)
	return opts.RiverWidthMin + int(math.Floor(r*float64(span)))
}

func makeLakePairKey(a, b int) uint64 {
	if a > b {
		a, b = b, a
	}
	return (uint64(uint32(a)) << 32) | uint64(uint32(b))
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func signInt(value int) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func buildRiverClassMask(flow []uint32, width, height int, opts RiverOptions) []RiverClass {
	classMask := make([]RiverClass, len(flow))

	for idx, value := range flow {
		if value >= uint32(opts.FlowDeepThreshold) {
			classMask[idx] = riverDeep
			continue
		}
		if value >= uint32(opts.FlowShallowThreshold) {
			classMask[idx] = riverShallow
		}
	}

	if opts.BankRadius <= 0 {
		return classMask
	}

	radiusSquared := opts.BankRadius * opts.BankRadius
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := tileIndex(x, y, width)
			if classMask[idx] != riverDeep {
				continue
			}
			for dy := -opts.BankRadius; dy <= opts.BankRadius; dy++ {
				for dx := -opts.BankRadius; dx <= opts.BankRadius; dx++ {
					if dx*dx+dy*dy > radiusSquared {
						continue
					}
					nx := x + dx
					ny := y + dy
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					nIdx := tileIndex(nx, ny, width)
					if classMask[nIdx] == riverNone {
						classMask[nIdx] = riverShallow
					}
				}
			}
		}
	}

	return classMask
}

func buildVoronoiEdgeStrength(width, height int, seed int64, cellSize int, edgeThreshold float64) ([]float32, error) {
	if cellSize <= 1 {
		return nil, fmt.Errorf("river-voronoi-cell-size must be > 1, got %d", cellSize)
	}
	if edgeThreshold <= 0 {
		return nil, fmt.Errorf("river-voronoi-edge-threshold must be > 0, got %.6f", edgeThreshold)
	}

	edge := make([]float32, width*height)
	for y := 0; y < height; y++ {
		cellY := y / cellSize
		sy := float64(y) + 0.5
		for x := 0; x < width; x++ {
			cellX := x / cellSize
			sx := float64(x) + 0.5

			closest := math.MaxFloat64
			second := math.MaxFloat64

			for oy := -1; oy <= 1; oy++ {
				for ox := -1; ox <= 1; ox++ {
					nx := cellX + ox
					ny := cellY + oy
					fx, fy := voronoiFeaturePoint(nx, ny, cellSize, seed)
					dx := sx - fx
					dy := sy - fy
					d2 := dx*dx + dy*dy
					if d2 < closest {
						second = closest
						closest = d2
					} else if d2 < second {
						second = d2
					}
				}
			}

			if !isFinitePositive(second) || !isFinitePositive(closest) {
				edge[tileIndex(x, y, width)] = 0
				continue
			}

			gap := math.Sqrt(second) - math.Sqrt(closest)
			normalizedGap := gap / float64(cellSize)
			strength := 1.0 - clamp01(normalizedGap/edgeThreshold)
			edge[tileIndex(x, y, width)] = float32(strength)
		}
	}

	return edge, nil
}

func voronoiFeaturePoint(cellX, cellY int, cellSize int, seed int64) (float64, float64) {
	baseX := float64(cellX * cellSize)
	baseY := float64(cellY * cellSize)
	jitterX := coordHash01(seed, cellX, cellY, voronoiPointXSalt)
	jitterY := coordHash01(seed, cellX, cellY, voronoiPointYSalt)
	return baseX + jitterX*float64(cellSize), baseY + jitterY*float64(cellSize)
}

func isFinitePositive(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func tileIndex(x, y, width int) int {
	return y*width + x
}

func coordHash01(seed int64, x int, y int, salt uint64) float64 {
	mix := uint64(seed)
	mix ^= uint64(int64(x)) * 0x9E3779B185EBCA87
	mix ^= uint64(int64(y)) * 0xC2B2AE3D27D4EB4F
	mix ^= salt
	mix = splitMix64(mix)
	const denom = 1.0 / (1 << 53)
	return float64(mix>>11) * denom
}

func splitMix64(value uint64) uint64 {
	value += 0x9E3779B97F4A7C15
	value = (value ^ (value >> 30)) * 0xBF58476D1CE4E5B9
	value = (value ^ (value >> 27)) * 0x94D049BB133111EB
	return value ^ (value >> 31)
}
