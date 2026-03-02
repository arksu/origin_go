package main

import (
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
