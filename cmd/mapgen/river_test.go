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
	base.MajorRiverCount = 24
	base.LakeCount = 90
	base.LakeBorderMix = 0.35
	base.MaxLakeDegree = 3
	base.LakeConnectionLimit = 120
	base.RiverWidthMin = 5
	base.RiverWidthMax = 5
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
