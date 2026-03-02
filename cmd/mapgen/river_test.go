package main

import (
	"math"
	"reflect"
	"testing"
)

func newTestRiverOptions() RiverOptions {
	return RiverOptions{
		Enabled:              true,
		SourceElevationMin:   0.7,
		SourceChance:         0.25,
		MeanderStrength:      0.003,
		VoronoiCellSize:      8,
		VoronoiEdgeThreshold: 0.14,
		VoronoiSourceBoost:   0.003,
		VoronoiBias:          0.01,
		SinkLakeChance:       1.0,
		LakeMinSize:          2,
		LakeConnectChance:    0.5,
		LakeConnectionLimit:  4,
		LakeLinkMinDistance:  0,
		LakeLinkMaxDistance:  64,
		RiverWidthMin:        5,
		RiverWidthMax:        15,
		FlowShallowThreshold: 4,
		FlowDeepThreshold:    8,
		BankRadius:           1,
		LakeFlowThreshold:    10,
	}
}

func TestBuildRiverNetworkDeterministic(t *testing.T) {
	width := 32
	height := 32
	elevation := make([]float32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			elevation[tileIndex(x, y, width)] = 0.95 - float32(y)*0.01 - float32(x)*0.001
		}
	}

	opts := newTestRiverOptions()

	networkA, err := BuildRiverNetwork(elevation, width, height, 777, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork A error: %v", err)
	}
	networkB, err := BuildRiverNetwork(elevation, width, height, 777, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork B error: %v", err)
	}

	if !reflect.DeepEqual(networkA.Flow, networkB.Flow) {
		t.Fatalf("flow accumulation differs for same seed/options")
	}
	if !reflect.DeepEqual(networkA.Class, networkB.Class) {
		t.Fatalf("river class mask differs for same seed/options")
	}
}

func TestTraceDownhillPathStrictlyDecreasesElevation(t *testing.T) {
	width := 6
	height := 6
	elevation := make([]float32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			elevation[tileIndex(x, y, width)] = 0.95 - float32(x+y)*0.05
		}
	}

	path, _ := traceDownhillPath(elevation, nil, width, height, 2, 2, 99, 0.003, 0, 64)
	if len(path) < 2 {
		t.Fatalf("expected a path with at least 2 points, got %d", len(path))
	}

	for i := 0; i < len(path)-1; i++ {
		current := float64(elevation[path[i]])
		next := float64(elevation[path[i+1]])
		if next >= current {
			t.Fatalf("path must strictly decrease elevation at step %d: current=%.6f next=%.6f", i, current, next)
		}
	}
}

func TestSinkLakeThresholdControlsLakeCarving(t *testing.T) {
	width := 5
	height := 5
	elevation := make([]float32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			elevation[tileIndex(x, y, width)] = 0.95
		}
	}
	centerX, centerY := 2, 2
	centerIdx := tileIndex(centerX, centerY, width)
	elevation[centerIdx] = 0.5
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x := centerX + dx
			y := centerY + dy
			elevation[tileIndex(x, y, width)] = 0.9
		}
	}

	baseOpts := newTestRiverOptions()
	baseOpts.SourceElevationMin = 0.8
	baseOpts.SourceChance = 1
	baseOpts.MeanderStrength = 0
	baseOpts.FlowShallowThreshold = 10
	baseOpts.FlowDeepThreshold = 20
	baseOpts.LakeFlowThreshold = 9

	withoutLake, err := BuildRiverNetwork(elevation, width, height, 11, baseOpts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork without lake error: %v", err)
	}
	if withoutLake.Class[centerIdx] != riverNone {
		t.Fatalf("expected no center river without lake carve, got class=%d", withoutLake.Class[centerIdx])
	}

	withLakeOpts := baseOpts
	withLakeOpts.LakeFlowThreshold = 8
	withLake, err := BuildRiverNetwork(elevation, width, height, 11, withLakeOpts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork with lake error: %v", err)
	}
	if withLake.Class[centerIdx] != riverDeep {
		t.Fatalf("expected deep center river after lake carve, got class=%d", withLake.Class[centerIdx])
	}
}

func TestBuildRiverClassMaskAddsShallowBanksAroundDeep(t *testing.T) {
	width := 5
	height := 5
	flow := make([]uint32, width*height)
	flow[tileIndex(2, 2, width)] = 100

	opts := RiverOptions{
		FlowShallowThreshold: 10,
		FlowDeepThreshold:    50,
		BankRadius:           1,
	}
	classMask := buildRiverClassMask(flow, width, height, opts)

	if classMask[tileIndex(2, 2, width)] != riverDeep {
		t.Fatalf("center should remain deep")
	}
	if classMask[tileIndex(2, 1, width)] != riverShallow {
		t.Fatalf("north neighbor should become shallow bank")
	}
	if classMask[tileIndex(0, 0, width)] != riverNone {
		t.Fatalf("far corner should stay non-river")
	}
}

func TestBuildVoronoiEdgeStrengthDeterministicAndNonZero(t *testing.T) {
	width := 64
	height := 64
	a, err := buildVoronoiEdgeStrength(width, height, 123, 16, 0.14)
	if err != nil {
		t.Fatalf("buildVoronoiEdgeStrength A error: %v", err)
	}
	b, err := buildVoronoiEdgeStrength(width, height, 123, 16, 0.14)
	if err != nil {
		t.Fatalf("buildVoronoiEdgeStrength B error: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("voronoi edge strength must be deterministic")
	}
	nonZero := 0
	for _, v := range a {
		if v > 0.01 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Fatalf("expected non-zero voronoi edge strength values")
	}
}

func TestRiverWidthForLinkWithinConfiguredRange(t *testing.T) {
	opts := newTestRiverOptions()
	opts.RiverWidthMin = 5
	opts.RiverWidthMax = 15
	for i := 0; i < 200; i++ {
		width := riverWidthForLink(12345, i, i+17, opts)
		if width < 5 || width > 15 {
			t.Fatalf("river width out of range: %d", width)
		}
	}
}

func TestLakeLinkingCreatesRiverOutsideLakeBodies(t *testing.T) {
	width := 80
	height := 40
	elevation := make([]float32, width*height)
	for i := range elevation {
		elevation[i] = 0.8
	}

	lakeCells := make(map[int]struct{})
	carveLake := func(cx, cy, radius int) {
		for y := cy - radius; y <= cy+radius; y++ {
			for x := cx - radius; x <= cx+radius; x++ {
				if x < 0 || y < 0 || x >= width || y >= height {
					continue
				}
				dx := x - cx
				dy := y - cy
				if dx*dx+dy*dy > radius*radius {
					continue
				}
				idx := tileIndex(x, y, width)
				elevation[idx] = 0.2
				lakeCells[idx] = struct{}{}
			}
		}
	}
	carveLake(18, 20, 4)
	carveLake(62, 20, 4)

	opts := newTestRiverOptions()
	opts.SourceChance = 0
	opts.SourceElevationMin = 1
	opts.SinkLakeChance = 0
	opts.LakeMinSize = 20
	opts.LakeConnectChance = 1
	opts.LakeConnectionLimit = 1
	opts.LakeLinkMinDistance = 10
	opts.LakeLinkMaxDistance = 80
	opts.RiverWidthMin = 5
	opts.RiverWidthMax = 5
	opts.FlowShallowThreshold = 2
	opts.FlowDeepThreshold = 3
	opts.LakeFlowThreshold = 3

	network, err := BuildRiverNetwork(elevation, width, height, 777, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork error: %v", err)
	}

	deepOutsideLakes := 0
	for idx, class := range network.Class {
		if class != riverDeep {
			continue
		}
		if _, inLake := lakeCells[idx]; inLake {
			continue
		}
		deepOutsideLakes++
	}
	if deepOutsideLakes == 0 {
		t.Fatalf("expected lake-link stage to create deep river tiles outside original lakes")
	}
}

func TestMajorTrunkRiversCarveVisibleChannels(t *testing.T) {
	width := 120
	height := 120
	elevation := make([]float32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			elev := 0.7
			if x < 6 || y < 6 || x > width-7 || y > height-7 {
				elev = 0.2 // coastline/ocean border
			}
			elevation[tileIndex(x, y, width)] = float32(elev)
		}
	}

	opts := newTestRiverOptions()
	opts.SourceChance = 0
	opts.LakeConnectChance = 0
	opts.GridEnabled = false
	opts.TrunkRiverCount = 4
	opts.TrunkMinLength = 20
	opts.TrunkSourceElevation = 0.65
	opts.CoastSampleChance = 0.2
	opts.RiverWidthMin = 5
	opts.RiverWidthMax = 7
	opts.SinkLakeChance = 0

	network, err := BuildRiverNetwork(elevation, width, height, 9981, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork error: %v", err)
	}

	deepCount := 0
	for _, class := range network.Class {
		if class == riverDeep {
			deepCount++
		}
	}
	if deepCount == 0 {
		t.Fatalf("expected trunk rivers to produce deep river tiles")
	}
}

func TestUniformGridRiversCoverMapBands(t *testing.T) {
	width := 180
	height := 180
	flow := make([]uint32, width*height)
	elevation := make([]float32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			elev := 0.85
			if x == 0 || y == 0 || x == width-1 || y == height-1 {
				elev = 0.2
			}
			elevation[tileIndex(x, y, width)] = float32(elev)
		}
	}
	voronoi, err := buildVoronoiEdgeStrength(width, height, 123, 16, 0.2)
	if err != nil {
		t.Fatalf("buildVoronoiEdgeStrength error: %v", err)
	}

	opts := newTestRiverOptions()
	opts.GridEnabled = true
	opts.GridSpacing = 40
	opts.GridJitter = 0
	opts.RiverWidthMin = 5
	opts.RiverWidthMax = 5
	opts.TrunkSourceElevation = 0.65
	opts.TrunkMinLength = 40
	opts.CoastSampleChance = 1
	opts.FlowShallowThreshold = 2
	opts.FlowDeepThreshold = 3

	generateUniformGridRivers(flow, elevation, voronoi, width, height, 123, opts)
	classMask := buildRiverClassMask(flow, width, height, opts)

	leftBand := 0
	centerBand := 0
	rightBand := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rc := classMask[tileIndex(x, y, width)]
			if rc == riverNone {
				continue
			}
			switch {
			case x < width/3:
				leftBand++
			case x < 2*width/3:
				centerBand++
			default:
				rightBand++
			}
		}
	}

	if leftBand == 0 || centerBand == 0 || rightBand == 0 {
		t.Fatalf("expected grid rivers across all map bands: left=%d center=%d right=%d", leftBand, centerBand, rightBand)
	}
}

func TestDefaultRiverCoverageOnPerlinTerrainIsVisible(t *testing.T) {
	width := 512
	height := 512
	fields := NewNoiseFields(NewPerlinNoise(123), 12)
	elevation := make([]float32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			elevation[tileIndex(x, y, width)] = float32(fields.Elevation(x, y))
		}
	}

	opts := DefaultMapgenOptions().River
	opts.GridEnabled = false
	network, err := BuildRiverNetwork(elevation, width, height, 123, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork error: %v", err)
	}

	landTiles := 0
	riverTiles := 0
	for idx, rc := range network.Class {
		if float64(elevation[idx]) < shallowWaterThreshold {
			continue
		}
		landTiles++
		if rc != riverNone {
			riverTiles++
		}
	}
	if landTiles == 0 {
		t.Fatalf("expected land tiles")
	}
	coverage := float64(riverTiles) / float64(landTiles)
	if coverage < 0.004 {
		voronoi, _ := buildVoronoiEdgeStrength(width, height, 123, opts.VoronoiCellSize, opts.VoronoiEdgeThreshold)
		sources := collectTrunkSourceCandidates(elevation, voronoi, width, height, opts)
		outlets := collectBorderOutletPoints(elevation, width, height, 123, opts.CoastSampleChance)
		deep := 0
		shallow := 0
		for _, rc := range network.Class {
			if rc == riverDeep {
				deep++
			} else if rc == riverShallow {
				shallow++
			}
		}
		t.Fatalf(
			"expected visible river coverage on land, got %.6f (sources=%d outlets=%d trunks=%d deep=%d shallow=%d)",
			coverage,
			len(sources),
			len(outlets),
			opts.TrunkRiverCount,
			deep,
			shallow,
		)
	}
}

func TestDrawLayoutSpreadsRiversAcrossMapBands(t *testing.T) {
	width := 480
	height := 480
	elevation := make([]float32, width*height)
	for idx := range elevation {
		elevation[idx] = 0.8
	}

	opts := DefaultMapgenOptions().River
	opts.LayoutDraw = true
	opts.MajorRiverCount = 28
	opts.LakeCount = 120
	opts.LakeBorderMix = 0.4
	opts.MaxLakeDegree = 2
	opts.LakeConnectChance = 0.7
	opts.LakeConnectionLimit = 80
	opts.RiverWidthMin = 7
	opts.RiverWidthMax = 7
	opts.FlowShallowThreshold = 2
	opts.FlowDeepThreshold = 3

	network, err := BuildRiverNetwork(elevation, width, height, 99123, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork draw layout error: %v", err)
	}

	left := 0
	center := 0
	right := 0
	top := 0
	middle := 0
	bottom := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rc := network.Class[tileIndex(x, y, width)]
			if rc == riverNone {
				continue
			}
			switch {
			case x < width/3:
				left++
			case x < (2*width)/3:
				center++
			default:
				right++
			}
			switch {
			case y < height/3:
				top++
			case y < (2*height)/3:
				middle++
			default:
				bottom++
			}
		}
	}

	if left == 0 || center == 0 || right == 0 {
		t.Fatalf("expected draw layout rivers across horizontal bands: left=%d center=%d right=%d", left, center, right)
	}
	if top == 0 || middle == 0 || bottom == 0 {
		t.Fatalf("expected draw layout rivers across vertical bands: top=%d middle=%d bottom=%d", top, middle, bottom)
	}
}

func TestDrawLayoutLakeConnectivityChanceAffectsRiverDensity(t *testing.T) {
	width := 360
	height := 360
	elevation := make([]float32, width*height)
	for idx := range elevation {
		elevation[idx] = 0.8
	}

	base := DefaultMapgenOptions().River
	base.LayoutDraw = true
	base.MajorRiverCount = 10
	base.LakeCount = 90
	base.LakeBorderMix = 0.35
	base.MaxLakeDegree = 5
	base.LakeConnectionLimit = 120
	base.RiverWidthMin = 5
	base.RiverWidthMax = 5
	base.LakeSizeSmallMin = 8
	base.LakeSizeSmallMax = 14
	base.LakeSizeMediumMin = 16
	base.LakeSizeMediumMax = 22
	base.LakeSizeLargeMin = 24
	base.LakeSizeLargeMax = 30
	base.LakeSizeMediumChance = 0.22
	base.LakeSizeLargeChance = 0.02
	base.FlowShallowThreshold = 2
	base.FlowDeepThreshold = 3

	low := base
	low.LakeConnectChance = 0.2
	lowNetwork, err := BuildRiverNetwork(elevation, width, height, 7001, low)
	if err != nil {
		t.Fatalf("BuildRiverNetwork low connectivity error: %v", err)
	}

	high := base
	high.LakeConnectChance = 0.85
	highNetwork, err := BuildRiverNetwork(elevation, width, height, 7001, high)
	if err != nil {
		t.Fatalf("BuildRiverNetwork high connectivity error: %v", err)
	}

	lowCount := 0
	highCount := 0
	for idx := range lowNetwork.Class {
		if lowNetwork.Class[idx] != riverNone {
			lowCount++
		}
		if highNetwork.Class[idx] != riverNone {
			highCount++
		}
	}
	if highCount <= lowCount {
		t.Fatalf("expected higher lake-connect-chance to increase river density: low=%d high=%d", lowCount, highCount)
	}
}

func TestBuildDrawPathProducesWindingCurve(t *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.MeanderStrength = 0.01
	opts.RiverWidthMin = 8
	opts.RiverWidthMax = 8

	startX, startY := 24, 24
	targetX, targetY := 220, 180
	path := buildDrawPath(256, 256, startX, startY, targetX, targetY, 424242, opts)
	if len(path) < 16 {
		t.Fatalf("expected non-trivial draw path length, got %d", len(path))
	}

	lineDX := float64(targetX - startX)
	lineDY := float64(targetY - startY)
	lineLen := math.Hypot(lineDX, lineDY)
	if lineLen == 0 {
		t.Fatalf("invalid zero line length")
	}

	maxDeviation := 0.0
	for _, idx := range path {
		x := float64(idx % 256)
		y := float64(idx / 256)
		// Perpendicular distance from point to start-target line.
		numerator := math.Abs(lineDY*x - lineDX*y + float64(targetX*startY-targetY*startX))
		deviation := numerator / lineLen
		if deviation > maxDeviation {
			maxDeviation = deviation
		}
	}
	if maxDeviation < 6.0 {
		t.Fatalf("expected visibly winding path, max perpendicular deviation too small: %.3f", maxDeviation)
	}
}

func TestDrawLayoutAvoidsTinyIsolatedRiverDots(t *testing.T) {
	width := 320
	height := 320
	elevation := make([]float32, width*height)
	for idx := range elevation {
		elevation[idx] = 0.8
	}

	opts := DefaultMapgenOptions().River
	opts.LayoutDraw = true
	opts.MajorRiverCount = 26
	opts.LakeCount = 120
	opts.LakeBorderMix = 0.35
	opts.MaxLakeDegree = 2
	opts.LakeConnectChance = 0.8
	opts.LakeConnectionLimit = 100
	opts.RiverWidthMin = 5
	opts.RiverWidthMax = 9
	opts.FlowShallowThreshold = 2
	opts.FlowDeepThreshold = 3

	network, err := BuildRiverNetwork(elevation, width, height, 5151, opts)
	if err != nil {
		t.Fatalf("BuildRiverNetwork error: %v", err)
	}

	smallComponents := countRiverComponentsAtMostSize(network.Class, width, height, 24)
	if smallComponents > 0 {
		t.Fatalf("expected no tiny isolated river components, got %d", smallComponents)
	}
}

func TestGenerateDrawLakesProducesVariedSizesAndShapes(t *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.LayoutDraw = true
	opts.LakeCount = 120
	opts.LakeSizeSmallMin = 8
	opts.LakeSizeSmallMax = 18
	opts.LakeSizeMediumMin = 22
	opts.LakeSizeMediumMax = 44
	opts.LakeSizeLargeMin = 52
	opts.LakeSizeLargeMax = 72
	opts.LakeSizeMediumChance = 0.30
	opts.LakeSizeLargeChance = 0.08

	lakes := generateDrawLakes(1024, 1024, 777, opts)
	if len(lakes) < 80 {
		t.Fatalf("expected many lakes, got %d", len(lakes))
	}

	minRadius := 1 << 30
	maxRadius := 0
	nonCircular := 0
	largeCount := 0
	lobeVariety := make(map[int]struct{})
	leftBand := 0
	centerBand := 0
	rightBand := 0
	topBand := 0
	middleBand := 0
	bottomBand := 0
	for _, lake := range lakes {
		if lake.Radius < minRadius {
			minRadius = lake.Radius
		}
		if lake.Radius > maxRadius {
			maxRadius = lake.Radius
		}
		if lake.RadiusX != lake.RadiusY {
			nonCircular++
		}
		if lake.Radius >= opts.LakeSizeLargeMin {
			largeCount++
		}
		lobeVariety[lake.LobeCount] = struct{}{}
		switch {
		case lake.X < 1024/3:
			leftBand++
		case lake.X < (1024*2)/3:
			centerBand++
		default:
			rightBand++
		}
		switch {
		case lake.Y < 1024/3:
			topBand++
		case lake.Y < (1024*2)/3:
			middleBand++
		default:
			bottomBand++
		}
	}

	if maxRadius-minRadius < 3 {
		t.Fatalf("expected varied lake sizes, min=%d max=%d", minRadius, maxRadius)
	}
	if nonCircular == 0 {
		t.Fatalf("expected non-circular lakes")
	}
	if len(lobeVariety) < 2 {
		t.Fatalf("expected variety in lake shape lobes")
	}
	if largeCount == 0 {
		t.Fatalf("expected at least one large lake")
	}
	if leftBand == 0 || centerBand == 0 || rightBand == 0 {
		t.Fatalf("expected lakes across horizontal bands: left=%d center=%d right=%d", leftBand, centerBand, rightBand)
	}
	if topBand == 0 || middleBand == 0 || bottomBand == 0 {
		t.Fatalf("expected lakes across vertical bands: top=%d middle=%d bottom=%d", topBand, middleBand, bottomBand)
	}
}

func TestGenerateBlueNoiseLakeCentersSpacing(t *testing.T) {
	minDistance := 22.0
	centers := generateBlueNoiseLakeCenters(600, 600, 20, 180, minDistance, 77)
	if len(centers) < 80 {
		t.Fatalf("expected many blue-noise centers, got %d", len(centers))
	}

	minPairDistance := math.MaxFloat64
	for i := 0; i < len(centers)-1; i++ {
		for j := i + 1; j < len(centers); j++ {
			distance := math.Hypot(float64(centers[i].X-centers[j].X), float64(centers[i].Y-centers[j].Y))
			if distance < minPairDistance {
				minPairDistance = distance
			}
		}
	}

	if minPairDistance < minDistance-3.0 {
		t.Fatalf("expected center spacing near minDistance, got minPairDistance=%.2f minDistance=%.2f", minPairDistance, minDistance)
	}
}

func TestGenerateDrawLakesDefaultLargeMapDistribution(t *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.LayoutDraw = true

	width := 6400
	height := 6400
	lakes := generateDrawLakes(width, height, 123, opts)
	if len(lakes) < opts.LakeCount/3 {
		t.Fatalf("expected substantial lake count, got=%d want_at_least=%d", len(lakes), opts.LakeCount/3)
	}

	left := 0
	center := 0
	right := 0
	top := 0
	middle := 0
	bottom := 0
	for _, lake := range lakes {
		switch {
		case lake.X < width/3:
			left++
		case lake.X < (2*width)/3:
			center++
		default:
			right++
		}
		switch {
		case lake.Y < height/3:
			top++
		case lake.Y < (2*height)/3:
			middle++
		default:
			bottom++
		}
	}

	if left == 0 || center == 0 || right == 0 {
		t.Fatalf("expected default lake placement across horizontal bands: left=%d center=%d right=%d total=%d", left, center, right, len(lakes))
	}
	if top == 0 || middle == 0 || bottom == 0 {
		t.Fatalf("expected default lake placement across vertical bands: top=%d middle=%d bottom=%d total=%d", top, middle, bottom, len(lakes))
	}
}

func TestGenerateBlueNoiseLakeCentersLargeMapDistribution(t *testing.T) {
	width := 6400
	height := 6400
	margin := 246
	minDistance := 39.0
	centers := generateBlueNoiseLakeCenters(width, height, margin, 1540, minDistance, 123)
	if len(centers) < 500 {
		t.Fatalf("expected many centers, got %d", len(centers))
	}

	left := 0
	center := 0
	right := 0
	for _, c := range centers {
		switch {
		case c.X < width/3:
			left++
		case c.X < (2*width)/3:
			center++
		default:
			right++
		}
	}
	if left == 0 || center == 0 || right == 0 {
		t.Fatalf("expected blue-noise centers across bands: left=%d center=%d right=%d total=%d", left, center, right, len(centers))
	}
}

func TestBuildLakeBasinsLargeLakeCreatesElongatedSpine(t *testing.T) {
	sizeProfile := lakeSizeProfile{
		SmallMin:  8,
		SmallMax:  14,
		MediumMin: 20,
		MediumMax: 34,
		LargeMin:  56,
		LargeMax:  72,
	}

	largeLake := buildDrawLakeProfileForClass(91, 7, sizeProfile, lakeSizeLarge)
	largeLake.X = 500
	largeLake.Y = 500
	largeBasins := buildLakeBasins(largeLake)
	if len(largeBasins) < 7 {
		t.Fatalf("expected many basins for large lake, got=%d", len(largeBasins))
	}

	smallLake := buildDrawLakeProfileForClass(91, 7, sizeProfile, lakeSizeSmall)
	smallLake.X = 500
	smallLake.Y = 500
	smallBasins := buildLakeBasins(smallLake)
	if len(largeBasins) <= len(smallBasins) {
		t.Fatalf("expected large lake to use richer basin structure: large=%d small=%d", len(largeBasins), len(smallBasins))
	}

	cosR := math.Cos(largeLake.Rotation)
	sinR := math.Sin(largeLake.Rotation)
	minAlong, maxAlong := math.MaxFloat64, -math.MaxFloat64
	minCross, maxCross := math.MaxFloat64, -math.MaxFloat64
	for _, basin := range largeBasins {
		dx := basin.X - float64(largeLake.X)
		dy := basin.Y - float64(largeLake.Y)
		along := dx*cosR + dy*sinR
		cross := -dx*sinR + dy*cosR
		if along < minAlong {
			minAlong = along
		}
		if along > maxAlong {
			maxAlong = along
		}
		if cross < minCross {
			minCross = cross
		}
		if cross > maxCross {
			maxCross = cross
		}
	}

	alongSpan := maxAlong - minAlong
	crossSpan := maxCross - minCross
	if alongSpan <= crossSpan*1.15 {
		t.Fatalf("expected elongated large-lake basin envelope, along=%.2f cross=%.2f", alongSpan, crossSpan)
	}
}

func TestPathCrowdingRatioPenalizesNearbyRivers(t *testing.T) {
	width := 64
	height := 64
	flow := make([]uint32, width*height)

	// Existing straight river corridor around x=24.
	for y := 5; y < height-5; y++ {
		flow[tileIndex(24, y, width)] = 20
	}

	opts := DefaultMapgenOptions().River
	opts.FlowShallowThreshold = 6
	opts.RiverWidthMin = 6

	nearPath := make([]int, 0, height-10)
	farPath := make([]int, 0, height-10)
	for y := 5; y < height-5; y++ {
		nearPath = append(nearPath, tileIndex(27, y, width))
		farPath = append(farPath, tileIndex(44, y, width))
	}

	nearCrowding := pathCrowdingRatio(flow, width, height, nearPath, opts)
	farCrowding := pathCrowdingRatio(flow, width, height, farPath, opts)
	if nearCrowding <= farCrowding {
		t.Fatalf("expected near path to be more crowded: near=%.3f far=%.3f", nearCrowding, farCrowding)
	}
}

func TestPathCrossingCountDetectsInteriorCrossings(t *testing.T) {
	width := 64
	height := 64
	flow := make([]uint32, width*height)

	// Existing vertical river at x=32.
	for y := 0; y < height; y++ {
		flow[tileIndex(32, y, width)] = 20
	}

	path := make([]int, 0, 45)
	for x := 10; x <= 54; x++ {
		path = append(path, tileIndex(x, 32, width))
	}

	opts := DefaultMapgenOptions().River
	opts.FlowShallowThreshold = 6
	opts.RiverWidthMax = 3

	crossings := pathCrossingCount(flow, width, path, opts)
	if crossings == 0 {
		t.Fatalf("expected crossing count > 0 for interior crossing")
	}
}

func TestCarveLakeInletChannelsExtendsDeepFlowIntoLake(t *testing.T) {
	width := 96
	height := 96
	opts := DefaultMapgenOptions().River
	opts.FlowShallowThreshold = 6
	opts.FlowDeepThreshold = 20

	lake := drawLake{
		ID:           77,
		X:            48,
		Y:            48,
		Radius:       16,
		RadiusX:      18,
		RadiusY:      14,
		SizeClass:    lakeSizeMedium,
		Rotation:     0.35,
		LobeCount:    4,
		Irregularity: 0.14,
		DeepRatio:    0.46,
		PhaseA:       1.2,
		PhaseB:       3.4,
	}

	flow := make([]uint32, width*height)
	carveDrawLakeFootprint(flow, width, height, lake, opts)

	inletX, inletY := lakeShorePoint(lake, width-1, lake.Y, width, height)
	inletIdx := tileIndex(inletX, inletY, width)
	flow[inletIdx] = uint32(opts.FlowDeepThreshold)

	beforeDeep := countDeepTilesInLake(flow, width, height, lake, opts)
	carveLakeInletChannels(
		flow,
		width,
		height,
		[]drawLake{lake},
		[]lakeInlet{{LakeIndex: 0, X: inletX, Y: inletY, RiverWidth: 12}},
		opts,
	)
	afterDeep := countDeepTilesInLake(flow, width, height, lake, opts)

	if afterDeep <= beforeDeep {
		t.Fatalf("expected inlet channel pass to increase deep lake tiles: before=%d after=%d", beforeDeep, afterDeep)
	}
}

func TestCarveLakeInletChannelsDoesNotCreateWaterOnDryTiles(t *testing.T) {
	width := 48
	height := 48
	opts := DefaultMapgenOptions().River
	opts.FlowShallowThreshold = 6
	opts.FlowDeepThreshold = 20

	flow := make([]uint32, width*height)
	lake := drawLake{ID: 9, X: 24, Y: 24, Radius: 10, RadiusX: 11, RadiusY: 9, SizeClass: lakeSizeSmall, DeepRatio: 0.45}
	inletX, inletY := 30, 24

	carveLakeInletChannels(
		flow,
		width,
		height,
		[]drawLake{lake},
		[]lakeInlet{{LakeIndex: 0, X: inletX, Y: inletY, RiverWidth: 9}},
		opts,
	)

	for idx, value := range flow {
		if value != 0 {
			t.Fatalf("expected inlet channel pass to skip dry maps, found non-zero flow at idx=%d value=%d", idx, value)
		}
	}
}

func countDeepTilesInLake(flow []uint32, width, height int, lake drawLake, opts RiverOptions) int {
	basins := buildLakeBasins(lake)
	if len(basins) == 0 {
		return 0
	}
	shoreThreshold := lakeShoreThreshold(lake)
	deepCount := 0

	minX := clampInt(lake.X-maxInt(lake.RadiusX, lake.Radius)-4, 0, width-1)
	maxX := clampInt(lake.X+maxInt(lake.RadiusX, lake.Radius)+4, 0, width-1)
	minY := clampInt(lake.Y-maxInt(lake.RadiusY, lake.Radius)-4, 0, height-1)
	maxY := clampInt(lake.Y+maxInt(lake.RadiusY, lake.Radius)+4, 0, height-1)

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			contour, _ := lakeContourValue(lake, basins, float64(x), float64(y))
			if contour < shoreThreshold {
				continue
			}
			idx := tileIndex(x, y, width)
			if flow[idx] >= uint32(opts.FlowDeepThreshold) {
				deepCount++
			}
		}
	}

	return deepCount
}

func countRiverComponentsAtMostSize(classMask []RiverClass, width, height, maxSize int) int {
	visited := make([]bool, len(classMask))
	queue := make([]int, 0, 256)
	smallCount := 0
	neighbors := [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			start := tileIndex(x, y, width)
			if visited[start] || classMask[start] == riverNone {
				continue
			}

			queue = queue[:0]
			queue = append(queue, start)
			visited[start] = true
			componentSize := 0

			for head := 0; head < len(queue); head++ {
				idx := queue[head]
				componentSize++
				cx := idx % width
				cy := idx / width

				for _, offset := range neighbors {
					nx := cx + offset[0]
					ny := cy + offset[1]
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					nIdx := tileIndex(nx, ny, width)
					if visited[nIdx] || classMask[nIdx] == riverNone {
						continue
					}
					visited[nIdx] = true
					queue = append(queue, nIdx)
				}
			}

			if componentSize <= maxSize {
				smallCount++
			}
		}
	}

	return smallCount
}
