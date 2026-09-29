package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRiverBendsLegacyFixtures(test *testing.T) {
	opts := DefaultMapgenOptions()
	opts.Seed = 42817
	opts.ChunksX, opts.ChunksY, opts.Threads = 2, 3, 1
	opts.River.LakeCount = 12
	opts.River.MajorRiverCount = 10
	opts.River.LakeConnectionLimit = 4
	terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(opts.Seed), 12))
	if err != nil {
		test.Fatal(err)
	}
	classes := make([]byte, len(terrain.RiverClass))
	for index, class := range terrain.RiverClass {
		classes[index] = byte(class)
	}
	for name, fixture := range map[string]struct {
		buffer []byte
		want   string
	}{
		"rivers":  {classes, "ddd9ab01a7a49552d0c6f795a88316bc062b0d2943a79430d755d05463267cef"},
		"terrain": {terrain.Tiles, "ab2e03e58115630e7e20d17b5719ba772f0b14ed4273d527a7f6f35c71c0084b"},
	} {
		if got := fmt.Sprintf("%x", sha256.Sum256(fixture.buffer)); got != fixture.want {
			test.Errorf("%s fixture: got %s want %s", name, got, fixture.want)
		}
	}
}

func bendTestOptions() RiverOptions {
	opts := DefaultMapgenOptions().River
	opts.ShapeWavelengthTiles = 240
	opts.FairwayWidthTiles = 3
	opts.ShapeOctaves = 4
	opts.ShapeOctaveGain = 0.35
	opts.ShapeSegmentLength = 20
	opts.ShapeFrequencyScale = 1
	opts.RiverWidthMin, opts.RiverWidthMax = 5, 9
	opts.TributarySpacingTiles = 90
	opts.TributaryLengthMin, opts.TributaryLengthMax = 50, 100
	return opts
}

func TestRiverBendsConfiguration(test *testing.T) {
	for _, fairwayWidth := range []int{1, 3, 5} {
		opts := DefaultMapgenOptions()
		opts.River = bendTestOptions()
		opts.River.FairwayWidthTiles = fairwayWidth
		if err := opts.Validate(); err != nil {
			test.Fatalf("width %d: %v", fairwayWidth, err)
		}
		encoded, err := yaml.Marshal(opts.River)
		if err != nil {
			test.Fatal(err)
		}
		var decoded RiverOptions
		if err := yaml.Unmarshal(encoded, &decoded); err != nil {
			test.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, opts.River) {
			test.Fatal("YAML options differ")
		}
	}
	for name, mutate := range map[string]func(*RiverOptions){
		"missing width":    func(opts *RiverOptions) { opts.FairwayWidthTiles = 0 },
		"even width":       func(opts *RiverOptions) { opts.FairwayWidthTiles = 2 },
		"large width":      func(opts *RiverOptions) { opts.FairwayWidthTiles = 7 },
		"terrain layout":   func(opts *RiverOptions) { opts.LayoutDraw = false },
		"nan ratio":        func(opts *RiverOptions) { opts.TributaryRatio = math.NaN() },
		"infinite gain":    func(opts *RiverOptions) { opts.ShapeOctaveGain = math.Inf(1) },
		"negative spacing": func(opts *RiverOptions) { opts.TributarySpacingTiles = -1 },
		"zero spacing":     func(opts *RiverOptions) { opts.TributaryRatio = 0.1; opts.TributarySpacingTiles = 0 },
		"reversed length":  func(opts *RiverOptions) { opts.TributaryRatio = 0.1; opts.TributaryLengthMax = 1 },
	} {
		test.Run(name, func(test *testing.T) {
			opts := bendTestOptions()
			mutate(&opts)
			if err := opts.validateBends(); err == nil {
				test.Fatal("expected invalid configuration")
			}
		})
	}
	legacy := DefaultMapgenOptions().River
	if err := yaml.Unmarshal([]byte("shape_wavelength_tiles: 0\nfairway_width_tiles: 0\ntributary_ratio: 0\n"), &legacy); err != nil {
		test.Fatal(err)
	}
	if legacy.ShapeWavelengthTiles != 0 || legacy.FairwayWidthTiles != 0 || legacy.TributaryRatio != 0 {
		test.Fatal("explicit zero replaced")
	}
}

func TestRiverBendsGeometry(test *testing.T) {
	opts := bendTestOptions()
	for _, endColumn := range []int{220, 1100, 1900} {
		path := buildDrawPath(2048, 512, 30, 256, endColumn, 256, 42817, opts)
		if len(path) < endColumn-30 || path[0] != tileIndex(30, 256, 2048) || path[len(path)-1] != tileIndex(endColumn, 256, 2048) {
			test.Fatal("lost endpoints or nontrivial path")
		}
		if !reflect.DeepEqual(path, buildDrawPath(2048, 512, 30, 256, endColumn, 256, 42817, opts)) {
			test.Fatal("nondeterministic geometry")
		}
		for position := 1; position < len(path); position++ {
			previous, current := path[position-1], path[position]
			if absInt(previous%2048-current%2048)+absInt(previous/2048-current/2048) != 1 {
				test.Fatal("non-cardinal raster step")
			}
		}
		broadOpts := opts
		broadOpts.ShapeOctaves = 1
		if reflect.DeepEqual(path, buildDrawPath(2048, 512, 30, 256, endColumn, 256, 42817, broadOpts)) {
			test.Fatal("fine bends missing")
		}
	}
	loop := []int{11, 12, 22, 21, 11}
	if riverPathSelfSeparated(loop, 10, 2) {
		test.Fatal("accepted self crossing")
	}
	flow := make([]uint32, 256*256)
	for index := range flow {
		flow[index] = uint32(opts.FlowDeepThreshold)
	}
	before := append([]uint32(nil), flow...)
	if _, ok := buildDrawPathWithAttempts(flow, 256, 256, 5, 128, 250, 128, 1, opts); ok {
		test.Fatal("accepted crowded route")
	}
	if !reflect.DeepEqual(flow, before) {
		test.Fatal("rejected geometry painted tiles")
	}
}

func TestRiverBendsMemoryBudget(test *testing.T) {
	opts := bendTestOptions()
	extra, err := estimateRiverBendsBytes(6400, 6400, opts)
	if err != nil || extra <= 6400*6400 {
		test.Fatalf("missing route/scratch budget: %d, %v", extra, err)
	}
	if _, err := estimateRiverBendsBytes(math.MaxInt, 1, opts); err == nil {
		test.Fatal("accepted overflowing route allocation")
	}
	opts.MajorRiverCount, opts.LakeConnectionLimit = math.MaxInt, math.MaxInt
	if _, err := estimateRiverBendsBytes(128, 128, opts); err == nil {
		test.Fatal("accepted overflowing route count")
	}
}

func TestRiverFairwayFootprints(test *testing.T) {
	for _, fairwayWidth := range []int{1, 3, 5} {
		opts := bendTestOptions()
		opts.FairwayWidthTiles = fairwayWidth
		flow := make([]uint32, 64*64)
		plan := &riverFairways{Protected: make([]bool, len(flow))}
		path := appendCardinalRiverSegment(nil, 64, 0, 2, 30, 30)
		path = appendCardinalRiverSegment(path, 64, 30, 30, 63, 50)
		route := protectRiverRoute(flow, plan, riverRoute{Path: path, Role: "fixture"}, 64, 64, opts)
		plan.Routes = []riverRoute{route}
		deep := func(index int) bool { return flow[index] >= uint32(opts.FlowDeepThreshold) }
		if err := plan.validate(64, 64, fairwayWidth, 42, deep); err != nil {
			test.Fatal(err)
		}
		middle := route.Path[len(route.Path)/2]
		flow[middle+fairwayWidth/2] = 0
		if err := plan.validate(64, 64, fairwayWidth, 42, deep); err == nil || !strings.Contains(err.Error(), "seed 42 route 0") {
			test.Fatalf("missing contextual failure: %v", err)
		}
	}
}

func TestRiverFairwayLakeRouting(test *testing.T) {
	opts := bendTestOptions()
	flow := make([]uint32, 64*64)
	for row := 5; row < 60; row++ {
		for column := 5; column < 60; column++ {
			flow[tileIndex(column, row, 64)] = uint32(opts.FlowShallowThreshold)
		}
	}
	for row := 12; row < 52; row++ {
		for column := 25; column < 38; column++ {
			flow[tileIndex(column, row, 64)] = 0
		}
	}
	plan := &riverFairways{Protected: make([]bool, len(flow))}
	for _, start := range []int{tileIndex(5, 20, 64), tileIndex(58, 45, 64)} {
		path, err := routeLakeFairway(flow, 64, 64, start, tileIndex(45, 30, 64), opts)
		if err != nil {
			test.Fatal(err)
		}
		for _, index := range path {
			if flow[index] == 0 {
				test.Fatal("cut island despite water route")
			}
		}
		plan.Routes = append(plan.Routes, protectRiverRoute(flow, plan, riverRoute{Path: path, Role: "inlet"}, 64, 64, opts))
	}
	if err := plan.validate(64, 64, 3, 1, func(index int) bool { return flow[index] >= uint32(opts.FlowDeepThreshold) }); err != nil {
		test.Fatal(err)
	}
	for index := range flow {
		flow[index] = 0
	}
	path, err := routeLakeFairway(flow, 64, 64, tileIndex(20, 30, 64), tileIndex(25, 30, 64), opts)
	if err != nil || len(path) != 6 {
		test.Fatalf("local neck widening: %d, %v", len(path), err)
	}
}

func TestRiverTributariesSparse(test *testing.T) {
	opts := bendTestOptions()
	opts.TributaryRatio, opts.TributarySpacingTiles = 0.08, 40
	opts.TributaryLengthMin, opts.TributaryLengthMax = 40, 55
	const width, height = 1800, 1800
	flow := make([]uint32, width*height)
	plan := &riverFairways{Protected: make([]bool, len(flow)), MainCount: 100}
	for index := 0; index < 100; index++ {
		column, row := 40+(index%10)*170, 70+(index/10)*170
		path := appendCardinalRiverSegment(nil, width, column, row, column+110, row)
		carveRiverCorridor(flow, width, height, path, 5, opts)
		plan.Routes = append(plan.Routes, protectRiverRoute(flow, plan, riverRoute{Path: path, Width: 5, Role: "main"}, width, height, opts))
	}
	parents := append([]riverRoute(nil), plan.Routes...)
	originalFlow := append([]uint32(nil), flow...)
	addRiverTributaries(flow, plan, width, height, 12345, opts)
	if plan.TributaryBudget != 8 || plan.Tributaries == 0 || plan.Tributaries > 8 {
		test.Fatalf("branch stats: %+v", riverRouteStats(plan))
	}
	if !reflect.DeepEqual(parents, plan.Routes[:100]) {
		test.Fatal("branches changed main routes")
	}
	if reviewMinimumJunctionSpacing(plan, width) < float64(opts.TributarySpacingTiles) {
		test.Fatal("tributary junctions too close")
	}
	for _, route := range plan.Routes[100:] {
		startAllowance := parents[route.Parent].Width*2 + opts.RiverWidthMin + opts.BankRadius*2
		if riverPathTouchesWater(originalFlow, route.Path, width, height, 1, startAllowance, 0, uint32(opts.FlowShallowThreshold)) {
			test.Fatal("tributary touches unrelated water")
		}
	}
	for index, value := range originalFlow {
		if flow[index] < value {
			test.Fatal("tributary removed existing water")
		}
	}
	used := map[int]bool{}
	for _, route := range plan.Routes[100:] {
		if used[route.Parent] {
			test.Fatal("more than one branch on parent")
		}
		used[route.Parent] = true
		found := false
		for _, index := range parents[route.Parent].Path {
			if index == route.Path[0] {
				found = true
				break
			}
		}
		if !found {
			test.Fatal("branch detached from parent")
		}
	}
	if err := plan.validate(width, height, 3, 12345, func(index int) bool { return flow[index] >= uint32(opts.FlowDeepThreshold) }); err != nil {
		test.Fatal(err)
	}
	repeatedFlow := append([]uint32(nil), originalFlow...)
	repeatedPlan := &riverFairways{Protected: make([]bool, len(flow)), MainCount: 100, Routes: append([]riverRoute(nil), parents...)}
	addRiverTributaries(repeatedFlow, repeatedPlan, width, height, 12345, opts)
	if !reflect.DeepEqual(repeatedFlow, flow) || !reflect.DeepEqual(repeatedPlan.Routes, plan.Routes) {
		test.Fatal("nondeterministic tributaries")
	}
	opts.ShapeOctaves = 1
	coarsePlan := &riverFairways{Protected: make([]bool, len(flow)), MainCount: 100, Routes: append([]riverRoute(nil), parents...)}
	addRiverTributaries(append([]uint32(nil), originalFlow...), coarsePlan, width, height, 12345, opts)
	if coarsePlan.TributaryBudget != plan.TributaryBudget {
		test.Fatal("curve detail changed branch budget")
	}
	opts.TributaryRatio = 0
	before := len(plan.Routes)
	addRiverTributaries(flow, plan, width, height, 3, opts)
	if len(plan.Routes) != before {
		test.Fatal("zero ratio added branches")
	}
}

func TestRiverTributariesCrowdingAndSmallSettings(test *testing.T) {
	opts := bendTestOptions()
	opts.TributaryRatio = 1
	const width, height = 256, 256
	flow := make([]uint32, width*height)
	for index := range flow {
		flow[index] = uint32(opts.FlowDeepThreshold)
	}
	before := append([]uint32(nil), flow...)
	path := appendCardinalRiverSegment(nil, width, 20, 128, 230, 128)
	plan := &riverFairways{Protected: make([]bool, len(flow)), MainCount: 1, Routes: []riverRoute{{Path: path, Width: 5, Role: "main"}}}
	addRiverTributaries(flow, plan, width, height, 12345, opts)
	if plan.Tributaries != 0 || plan.RejectedTributaries == 0 || !reflect.DeepEqual(flow, before) {
		test.Fatal("crowded candidate was painted")
	}
	if tributaryJunctionSeparated(tileIndex(30, 30, width), nil, []lakeInlet{{X: 31, Y: 30}}, width, 5) {
		test.Fatal("accepted junction beside lake inlet")
	}
	opts.FairwayWidthTiles, opts.RiverWidthMin, opts.RiverWidthMax, opts.BankRadius = 1, 1, 1, 0
	opts.TributarySpacingTiles, opts.TributaryLengthMin, opts.TributaryLengthMax = 1, 1, 1
	if err := opts.validateBends(); err != nil {
		test.Fatal(err)
	}
	plan.Routes[0] = riverRoute{Path: appendCardinalRiverSegment(nil, width, 20, 128, 40, 128), Width: 1, Role: "main"}
	addRiverTributaries(flow, plan, width, height, 12345, opts)
	if plan.Tributaries != 0 || !reflect.DeepEqual(flow, before) {
		test.Fatal("undersized branch escaped rejection")
	}
}

func TestRiverBendsFinalTerrain(test *testing.T) {
	opts := DefaultMapgenOptions()
	opts.ChunksX, opts.ChunksY, opts.Threads, opts.Seed = 4, 4, 1, 12345
	opts.River = bendTestOptions()
	opts.River.MajorRiverCount, opts.River.LakeCount = 24, 30
	opts.River.TributaryRatio = 0.08
	fields := NewNoiseFields(NewPerlinNoise(opts.Seed), 12)
	first, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		test.Fatal(err)
	}
	if first.RiverFairways.MainCount == 0 {
		test.Fatal("no main routes")
	}
	opts.Threads = 4
	second, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(first.Tiles, second.Tiles) || !reflect.DeepEqual(first.RiverFairways.Routes, second.RiverFairways.Routes) {
		test.Fatal("worker count changed output")
	}
	protectedShallow := 0
	for index, protected := range first.RiverFairways.Protected {
		if protected && first.Tiles[index] != tileWaterDeep {
			test.Fatal("lost final deep tile")
		}
		if protected && first.Elevation[index] >= deepWaterThreshold && first.Elevation[index] < shallowWaterThreshold {
			protectedShallow++
		}
	}
	if protectedShallow == 0 {
		test.Fatal("fixture does not exercise shallow terrain precedence")
	}
	opts.River.Enabled = false
	without, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(first.Elevation, without.Elevation) {
		test.Fatal("river style changed elevation")
	}
	opts.River.ShapeWavelengthTiles, opts.River.FairwayWidthTiles, opts.River.TributaryRatio = 0, 0, 0
	legacyWithout, err := BuildTerrainPrecompute(opts, 128, fields)
	if err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(without.Tiles, legacyWithout.Tiles) {
		test.Fatal("disabled rivers changed land")
	}
	for index := range first.Tiles {
		column, row := index%first.WidthTiles, index/first.WidthTiles
		if column != 0 && row != 0 && column != first.WidthTiles-1 && row != first.HeightTiles-1 {
			continue
		}
		if first.RiverClass[index] == riverNone && isWaterTileID(first.Tiles[index]) != isWaterTileID(without.Tiles[index]) {
			test.Fatal("introduced coastal water outside drawn rivers")
		}
	}
	corrupted := first.RiverFairways.Routes[0].Path[0]
	first.Tiles[corrupted] = tileWater
	if err := first.RiverFairways.validate(first.WidthTiles, first.HeightTiles, 3, opts.Seed, func(index int) bool {
		return first.Tiles[index] == tileWaterDeep
	}); err == nil || !strings.Contains(err.Error(), "seed 12345 route 0") {
		test.Fatalf("final-tile corruption not detected: %v", err)
	}
}

func TestRiverBendsElevationIndependent(test *testing.T) {
	opts := bendTestOptions()
	opts.LakeCount, opts.MajorRiverCount = 15, 10
	elevation := make([]float32, 384*384)
	first, err := BuildRiverNetwork(elevation, 384, 384, 71, opts)
	if err != nil {
		test.Fatal(err)
	}
	for index := range elevation {
		elevation[index] = float32(index%317) / 317
	}
	second, err := BuildRiverNetwork(elevation, 384, 384, 71, opts)
	if err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		test.Fatal("drawn rivers depend on elevation")
	}
	if err := second.Fairways.validate(384, 384, 3, 71, func(index int) bool { return second.Class[index] == riverDeep }); err != nil {
		test.Fatal(err)
	}
}

func TestRiverBendsPreviewAndPresets(test *testing.T) {
	server := newTestPreviewServer(test)
	server.base.River = bendTestOptions()
	server.base.River.LakeCount, server.base.River.MajorRiverCount = 15, 10
	request := renderRequest{Seed: 71, ChunksX: 3, ChunksY: 3, Layers: []string{"rivers"}}
	encoded, err := server.buildPreviewPNG(request)
	if err != nil {
		test.Fatal(err)
	}
	preview, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		test.Fatal(err)
	}
	network, err := BuildRiverNetwork(make([]float32, 384*384), 384, 384, 71, server.base.River)
	if err != nil {
		test.Fatal(err)
	}
	for index, class := range network.Class {
		want := previewBackgroundColor
		if class == riverDeep {
			want = previewDeepColor
		} else if class == riverShallow {
			want = previewShallowColor
		}
		if preview.At(index%384, index/384) != want {
			test.Fatalf("preview differs from production at %d", index)
		}
	}
	request.Params = map[string]json.RawMessage{previewParamRivers: json.RawMessage(`{"fairway_width_tiles":2}`)}
	if _, err := server.buildPreviewPNG(request); err == nil {
		test.Fatal("preview accepted even clearance")
	}
	keys := map[string]bool{}
	for _, group := range riversLayerSchema(server.base).Groups {
		for _, field := range group.Fields {
			keys[field.Key] = true
		}
	}
	for _, key := range []string{"shape_wavelength_tiles", "fairway_width_tiles", "tributary_ratio", "tributary_spacing_tiles", "tributary_length_min", "tributary_length_max"} {
		if !keys[key] {
			test.Fatalf("missing preview control %s", key)
		}
	}
	for _, name := range []string{"default", "hnh", "minecraft_like"} {
		opts, _, err := LoadMapgenOptionsFromYAML("../../etc/mapgen/presets/"+name+".yaml", DefaultMapgenOptions())
		if err != nil {
			test.Fatal(err)
		}
		if err := opts.Validate(); err != nil {
			test.Fatal(err)
		}
		if name == "hnh" {
			if opts.River.FairwayWidthTiles != 3 || opts.River.ShapeWavelengthTiles == 0 {
				test.Fatal("H&H did not enable selected mode")
			}
		} else if opts.River.ShapeWavelengthTiles != 0 || opts.River.FairwayWidthTiles != 0 || opts.River.TributaryRatio != 0 {
			test.Fatalf("changed legacy preset %s", name)
		}
	}
}
