package main

import (
	"crypto/sha256"
	"fmt"
	"image"
	"math"
	"reflect"
	"strings"
	"testing"
)

func lakeTestOptions(seed int64) MapgenOptions {
	opts := DefaultMapgenOptions()
	opts.ChunksX, opts.ChunksY, opts.Threads, opts.Seed = 3, 3, 1, seed
	opts.PerlinWaterEnabled = false
	opts.River = bendTestOptions()
	opts.River.LakeCount, opts.River.MajorRiverCount, opts.River.LakeConnectionLimit = 18, 12, 8
	return opts
}

func TestIrregularLakeNetwork(test *testing.T) {
	for _, seed := range []int64{12345, 67890, 314159} {
		opts := lakeTestOptions(seed)
		baseline, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
		if err != nil {
			test.Fatal(err)
		}
		opts.River.LakeIrregularEnabled = true
		opts.River.LakeIslandMediumChance, opts.River.LakeIslandLargeChance = 1, 1
		terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
		if err != nil {
			test.Fatalf("seed %d: %v", seed, err)
		}
		if !reflect.DeepEqual(baseline.RiverFairways.Lakes, terrain.RiverFairways.Lakes) || !reflect.DeepEqual(baseline.RiverFairways.Routes[:baseline.RiverFairways.MainCount], terrain.RiverFairways.Routes[:terrain.RiverFairways.MainCount]) {
			test.Fatal("lake detail changed main links or placement")
		}
		test.Logf("seed %d: %v", seed, riverRouteStats(terrain.RiverFairways))
		opts.Threads = 4
		opts.PerlinWaterEnabled = true
		wet, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
		if err != nil {
			test.Fatal(err)
		}
		if !reflect.DeepEqual(wet.Elevation, terrain.Elevation) || !reflect.DeepEqual(wet.RiverClass, terrain.RiverClass) {
			test.Fatal("base water changed lake geometry or elevation")
		}
		for _, shape := range wet.RiverFairways.LakeShapes {
			for index, dry := range shape.Land {
				point := shape.point(index)
				if dry && isWaterTileID(wet.Tiles[tileIndex(point.X, point.Y, wet.WidthTiles)]) {
					test.Fatal("reserved lake land flooded")
				}
			}
		}
		opts.PerlinWaterEnabled = false
		repeated, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
		if err != nil {
			test.Fatal(err)
		}
		if !reflect.DeepEqual(repeated.Tiles, terrain.Tiles) {
			test.Fatal("thread count changed terrain")
		}
	}
}

func TestIrregularLakeIslandsAndPeninsulas(test *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.LakeIrregularEnabled, opts.FairwayWidthTiles = true, 3
	opts.LakeIslandLargeChance, opts.LakeIslandSecondChance = 1, 1
	lake := drawLake{ID: 0, X: 128, Y: 128, Radius: 90, RadiusX: 90, RadiusY: 70, SizeClass: lakeSizeLarge, LobeCount: 4, Irregularity: 0.25, DeepRatio: 0.5, PhaseA: 0.7}
	flow := make([]uint32, 256*256)
	inlets := []lakeInlet{{LakeIndex: 0, X: 55, Y: 128, RiverWidth: 5}, {LakeIndex: 0, X: 200, Y: 128, RiverWidth: 5}}
	shape, err := buildIrregularLakeShape(flow, 256, 256, 12345, lake, inlets, opts)
	if err != nil {
		test.Fatal(err)
	}
	if shape.Peninsulas == 0 || len(shape.Islands) == 0 {
		test.Fatalf("missing forced features: peninsulas=%d islands=%d", shape.Peninsulas, len(shape.Islands))
	}
	distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
	for _, island := range shape.Islands {
		if island.Loop[0] != island.Loop[len(island.Loop)-1] {
			test.Fatal("island clearance loop not closed")
		}
		for _, index := range island.Loop {
			if distances[shape.index(index%256, index/256)] <= opts.LakeShallowWidthMax+1 {
				test.Fatal("island loop lacks full deep footprint")
			}
		}
	}
	opts.LakeIslandLargeChance = 0
	disabled, err := buildIrregularLakeShape(flow, 256, 256, 12345, lake, inlets, opts)
	if err != nil {
		test.Fatal(err)
	}
	if len(disabled.Islands) != 0 {
		test.Fatal("zero chance did not disable islands")
	}
	filled := append([]bool(nil), disabled.Water...)
	fillLakeHoles(filled, disabled.Bounds.Dx(), disabled.Bounds.Dy())
	if !reflect.DeepEqual(filled, disabled.Water) {
		test.Fatal("accidental islands remain when disabled")
	}
}

func TestLakeNavigationRejectsNarrowAndDiagonalGaps(test *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.FairwayWidthTiles = 3
	shape := &lakeShape{Lake: drawLake{ID: 7, X: 4, Y: 4}, Bounds: image.Rect(0, 0, 17, 17), Water: make([]bool, 17*17)}
	for row := 1; row <= 7; row++ {
		for column := 1; column <= 7; column++ {
			shape.Water[row*17+column] = true
			shape.Water[(row+8)*17+column+8] = true
		}
	}
	for _, point := range []image.Point{{8, 7}, {8, 8}, {9, 8}} {
		shape.Water[point.Y*17+point.X] = true
	}
	if !lakeWaterConnected(shape.Water, 17) {
		test.Fatal("fixture must have a connected centerline")
	}
	inlets := []lakeInlet{{X: 4, Y: 4}, {X: 12, Y: 12}}
	if _, err := shape.navigation(inlets, 17, opts); err == nil {
		test.Fatal("accepted a centerline without boat clearance")
	}
	opts.FairwayWidthTiles = 1
	if _, err := shape.navigation(inlets, 17, opts); err != nil {
		test.Fatal(err)
	}
}

func TestLakeBanksAndFinalValidation(test *testing.T) {
	for _, band := range []int{0, 1, 5} {
		opts := DefaultMapgenOptions().River
		opts.LakeIrregularEnabled, opts.FairwayWidthTiles = true, 3
		opts.LakePeninsulaCountMax, opts.LakeShoreVariationTiles = 0, 0
		opts.LakeShallowWidthMin, opts.LakeShallowWidthMax = band, band
		plan := &riverFairways{Protected: make([]bool, 96*96), Lakes: []drawLake{{X: 48, Y: 48, Radius: 30, RadiusX: 30, RadiusY: 25, LobeCount: 4, DeepRatio: 0.5}}}
		flow := make([]uint32, 96*96)
		if err := finishIrregularLakes(flow, plan, 96, 96, 42, opts); err != nil {
			test.Fatal(err)
		}
		classes := buildRiverClassMask(flow, 96, 96, opts, plan)
		shape := plan.LakeShapes[0]
		distances := lakeWaterDistances(shape.Water, shape.Bounds.Dx())
		for index, water := range shape.Water {
			point := shape.point(index)
			class := classes[tileIndex(point.X, point.Y, 96)]
			want := riverNone
			if water {
				want = riverDeep
				if distances[index] <= band {
					want = riverShallow
				}
			}
			if class != want {
				test.Fatalf("band %d at %v: class=%d want=%d", band, point, class, want)
			}
		}
	}
	opts := lakeTestOptions(12345)
	opts.River.LakeIrregularEnabled = true
	opts.River.LakeIslandLargeChance = 1
	terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(opts.Seed), 12))
	if err != nil {
		test.Fatal(err)
	}
	plan := terrain.RiverFairways
	corrupted := -1
	for index, dry := range plan.LakeLand {
		if dry {
			corrupted = index
			break
		}
	}
	if corrupted < 0 {
		test.Fatal("no dry features to validate")
	}
	terrain.Tiles[corrupted] = tileWaterDeep
	if err := plan.validateLakeLand(opts.Seed, func(index int) bool { return isWaterTileID(terrain.Tiles[index]) }, terrain.WidthTiles); err == nil || !strings.Contains(err.Error(), "seed 12345 lake") {
		test.Fatalf("flooded land not detected: %v", err)
	}
	for _, route := range plan.Routes {
		if route.Role != "island" {
			continue
		}
		terrain.Tiles[route.Path[0]] = tileWater
		if err := plan.validate(terrain.WidthTiles, terrain.HeightTiles, 3, opts.Seed, func(index int) bool { return terrain.Tiles[index] == tileWaterDeep }); err == nil {
			test.Fatal("broken island loop not detected")
		}
		return
	}
	test.Fatal("fixture has no island loop")
}

func TestLakeCandidateRejectionIsAtomic(test *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.FairwayWidthTiles = 3
	shape := &lakeShape{Lake: drawLake{ID: 0, X: 12, Y: 12}, Bounds: image.Rect(0, 0, 25, 25), Water: make([]bool, 625), Land: make([]bool, 625)}
	for row := 2; row < 23; row++ {
		for column := 2; column < 23; column++ {
			shape.Water[row*25+column] = true
		}
	}
	before := append([]bool(nil), shape.Water...)
	cut := make([]bool, 625)
	for row := 0; row < 25; row++ {
		for column := 11; column <= 13; column++ {
			cut[row*25+column] = true
		}
	}
	if shape.acceptCut(cut, make([]uint32, 625), 25, nil, opts) {
		test.Fatal("accepted a cut splitting the lake")
	}
	if !reflect.DeepEqual(before, shape.Water) {
		test.Fatal("rejected cut left partial edits")
	}
	for _, dry := range shape.Land {
		if dry {
			test.Fatal("rejected cut reserved land")
		}
	}
}

func TestLakeWiderFairwayAndIslandAreaLimit(test *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.LakeIrregularEnabled, opts.FairwayWidthTiles = true, 5
	opts.RiverWidthMin = 5
	opts.LakeIslandLargeChance, opts.LakeIslandSecondChance = 1, 1
	plan := &riverFairways{Protected: make([]bool, 256*256), Lakes: []drawLake{{ID: 0, X: 128, Y: 128, Radius: 90, RadiusX: 90, RadiusY: 70, SizeClass: lakeSizeLarge, LobeCount: 4, Irregularity: 0.25, DeepRatio: 0.5, PhaseA: 0.7}}, Inlets: []lakeInlet{{LakeIndex: 0, X: 55, Y: 128, RiverWidth: 5}, {LakeIndex: 0, X: 200, Y: 128, RiverWidth: 5}}}
	flow := make([]uint32, 256*256)
	if err := finishIrregularLakes(flow, plan, 256, 256, 12345, opts); err != nil {
		test.Fatal(err)
	}
	if err := plan.validate(256, 256, 5, 12345, func(index int) bool { return flow[index] >= uint32(opts.FlowDeepThreshold) }); err != nil {
		test.Fatal(err)
	}
	shape := plan.LakeShapes[0]
	waterArea, islandArea := 0, 0
	for _, water := range shape.Water {
		if water {
			waterArea++
		}
	}
	for _, island := range shape.Islands {
		if island.Radius < opts.LakeIslandRadiusMin || island.Radius > opts.LakeIslandRadiusMax {
			test.Fatal("island radius outside configured limits")
		}
		islandArea += len(island.Tiles)
	}
	if islandArea == 0 || islandArea > (waterArea+islandArea)*8/100 {
		test.Fatal("missing islands or area cap exceeded")
	}
}

func TestLakeMemoryBudget(test *testing.T) {
	opts := DefaultMapgenOptions()
	opts.River.LakeIrregularEnabled, opts.River.FairwayWidthTiles = true, 3
	opts.River.LakeCount = math.MaxInt
	if _, err := estimateLakeGeometryBytes(6400, 6400, opts.River); err == nil {
		test.Fatal("lake memory arithmetic did not reject overflow")
	}
	opts.River.LakeCount = 100000
	if err := validatePreviewMemory(opts); err == nil {
		test.Fatal("preview ignored extra lake memory")
	}
	if err := opts.Validate(); err == nil {
		test.Fatal("terrain ignored extra lake memory")
	}
}

func TestLakeShapesWithoutIslandsHaveNoHoles(test *testing.T) {
	for seed := int64(1); seed <= 16; seed++ {
		opts := lakeTestOptions(seed).River
		opts.LakeIrregularEnabled = true
		network, err := BuildRiverNetwork(make([]float32, 384*384), 384, 384, seed, opts)
		if err != nil {
			test.Fatalf("seed %d: %v", seed, err)
		}
		for _, shape := range network.Fairways.LakeShapes {
			filled := append([]bool(nil), shape.Water...)
			fillLakeHoles(filled, shape.Bounds.Dx(), shape.Bounds.Dy())
			if !reflect.DeepEqual(filled, shape.Water) {
				test.Fatalf("seed %d lake %d: incidental island with chances zero", seed, shape.Lake.ID)
			}
			if !lakeWaterConnected(shape.Water, shape.Bounds.Dx()) {
				test.Fatalf("seed %d lake %d: detached pool", seed, shape.Lake.ID)
			}
		}
	}
}

func TestLakeIslandSelection(test *testing.T) {
	opts := DefaultMapgenOptions().River
	for _, class := range []lakeSizeClass{lakeSizeSmall, lakeSizeMedium, lakeSizeLarge} {
		lake := drawLake{ID: 8, SizeClass: class}
		if requestedLakeIslands(42, lake, opts) != 0 {
			test.Fatal("default island chance is not zero")
		}
	}
	opts.LakeIslandMediumChance, opts.LakeIslandLargeChance = 0.15, 0.35
	opts.LakeIslandSecondChance = 0.2
	selected, doubles := 0, 0
	for seed := int64(1); seed <= 10000; seed++ {
		requested := requestedLakeIslands(seed, drawLake{ID: 0, SizeClass: lakeSizeLarge}, opts)
		if requested > 0 {
			selected++
		}
		if requested == 2 {
			doubles++
		}
	}
	if selected < 3200 || selected > 3800 || doubles < 550 || doubles > 850 {
		test.Fatalf("unexpected selection distribution: selected=%d doubles=%d", selected, doubles)
	}
	opts.LakeIslandLargeChance, opts.LakeIslandSecondChance = 1, 0
	if requestedLakeIslands(1, drawLake{SizeClass: lakeSizeLarge}, opts) != 1 {
		test.Fatal("one chance or zero second chance is ineffective")
	}
	opts.LakeIslandSecondChance = 1
	if requestedLakeIslands(1, drawLake{SizeClass: lakeSizeLarge}, opts) != 2 {
		test.Fatal("second chance one is ineffective")
	}
}

func TestLakeOptionsValidation(test *testing.T) {
	for name, mutate := range map[string]func(*RiverOptions){
		"nan":       func(opts *RiverOptions) { opts.LakeIslandMediumChance = math.NaN() },
		"chance":    func(opts *RiverOptions) { opts.LakeIslandLargeChance = 1.1 },
		"radius":    func(opts *RiverOptions) { opts.LakeIslandRadiusMax = 3 },
		"banks":     func(opts *RiverOptions) { opts.LakeShallowWidthMin = 6 },
		"depth":     func(opts *RiverOptions) { opts.LakePeninsulaDepthRatio = math.Inf(1) },
		"count":     func(opts *RiverOptions) { opts.LakePeninsulaCountMax = 9 },
		"roughness": func(opts *RiverOptions) { opts.LakeShoreVariationTiles = -1 },
		"clearance": func(opts *RiverOptions) { opts.LakeIrregularEnabled = true },
	} {
		test.Run(name, func(test *testing.T) {
			opts := DefaultMapgenOptions().River
			mutate(&opts)
			if err := opts.validateLakeOptions(); err == nil || !strings.Contains(err.Error(), "river.lake_") {
				test.Fatalf("invalid option not rejected: %v", err)
			}
		})
	}
}

func TestLakeLegacyFixtures(test *testing.T) {
	fixtures := map[int64][2]string{
		12345: {"165189cb83a78b52026021cfbc0b35f88699db1351bd4d9ebbd779872deaa959", "428d5bd9a324b8f21021481a728e4231c9317e26421039904c8639831dbab4a5"},
		67890: {"691e204a28ff7f9faeb2bb7d05944692ca171dfe385e1404bace86347d3fef39", "657311c54d8b5522356167c859e04ae173b90ca9e64dee7cd237d95d2f581828"},
	}
	for seed, expected := range fixtures {
		opts := lakeTestOptions(seed)
		terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
		if err != nil {
			test.Fatal(err)
		}
		classes := make([]byte, len(terrain.RiverClass))
		for index, class := range terrain.RiverClass {
			classes[index] = byte(class)
		}
		for index, buffer := range [][]byte{terrain.Tiles, classes} {
			if actual := fmt.Sprintf("%x", sha256.Sum256(buffer)); actual != expected[index] {
				test.Errorf("seed %d fixture %d: got %s want %s", seed, index, actual, expected[index])
			}
		}
	}
}
