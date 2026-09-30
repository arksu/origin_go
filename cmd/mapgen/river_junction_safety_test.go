package main

import (
	"image"
	"reflect"
	"testing"
)

func acceptedJunctionCandidate(test *testing.T, plan *riverFairways, lake drawLake, opts RiverOptions) ([]int, riverJunctionSample) {
	test.Helper()
	for attempt, target := range plan.Junctions.targets(plan, 0, lake, 100, 600, 12345, opts) {
		path := buildRiverJunctionPath(768, 768, lake, plan.Routes[target.Route].Path, target.Position, 12345+int64(attempt+1)*104729, opts)
		if plan.Junctions.validateCandidate(plan, path, 0, target, opts) == "" {
			return path, target
		}
	}
	test.Fatal("no valid baseline geometry")
	return nil, riverJunctionSample{}
}

func TestRiverJunctionContactSafety(test *testing.T) {
	for _, scenario := range []string{"foreign in blend", "foreign at source", "remote parent", "other lake", "source reentry", "self shortcut", "cross parent"} {
		test.Run(scenario, func(test *testing.T) {
			_, plan, lakes, opts := junctionFixture(false, false, 3)
			path, target := acceptedJunctionCandidate(test, plan, lakes[0], opts)
			clearance := riverCorridorMaximumWidth(opts)
			switch scenario {
			case "foreign in blend":
				tile := path[len(path)-clearance]
				foreign := riverRoute{Path: []int{tile - 1, tile, tile + 1}, Width: 7, Role: "junction", Parent: 0}
				plan.Routes = append(plan.Routes, foreign)
				plan.Junctions.record(1, foreign)
			case "foreign at source":
				foreign := riverRoute{Path: []int{path[0]}, Width: 7, Role: "main", Parent: -1}
				plan.Routes = append(plan.Routes, foreign)
				plan.Junctions.record(1, foreign, 1, 2)
			case "remote parent":
				plan.Junctions.addSample(riverJunctionSample{Route: 0, Position: len(plan.Routes[0].Path) + 1000, Tile: path[len(path)/2]})
			case "other lake":
				tile := path[len(path)/2]
				plan.Junctions.LakeBounds = append(plan.Junctions.LakeBounds, image.Rect(tile%768-10, tile/768-10, tile%768+11, tile/768+11))
			case "source reentry":
				path = append(append(append([]int{}, path[:len(path)/2]...), path[0]), path[len(path)/2:]...)
			case "self shortcut":
				path = append(append(append([]int{}, path[:len(path)-2]...), path[len(path)/2]), path[len(path)-2:]...)
			case "cross parent":
				join := target.Tile
				path = append(path[:len(path)-1], join-768, join)
			}
			if reason := plan.Junctions.validateCandidate(plan, path, 0, target, opts); reason == "" {
				test.Fatal("unsafe contact accepted")
			}
		})
	}
}

func TestRiverJunctionParentEligibilityAndSpacing(test *testing.T) {
	_, plan, lakes, opts := junctionFixture(false, false, 3)
	state := plan.Junctions
	baseline := state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)
	if len(baseline) == 0 || !reflect.DeepEqual(baseline, state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) {
		test.Fatal("target selection not stable")
	}
	parentPath := plan.Routes[0].Path
	plan.Routes[0].Path = parentPath[:20]
	if len(state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) != 0 {
		test.Fatal("short parent accepted")
	}
	plan.Routes[0].Path = parentPath
	recordDrawRiver(plan, parentPath, 7, 1, 2)
	tied := state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)
	if len(tied) < 2 || !reflect.DeepEqual(tied, state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) {
		test.Fatal("equal-distance ordering changed")
	}
	delete(state.Origins, 1)
	for _, origins := range [][2]int{{0, 1}, {2, 0}} {
		state.Origins[0] = origins
		if len(state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) != 0 {
			test.Fatal("source-incident parent accepted")
		}
	}
	delete(state.Origins, 0)
	for _, role := range []string{"junction", "tributary"} {
		plan.Routes[0].Role = role
		state.record(0, plan.Routes[0], 1, 2)
		if len(state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) != 0 {
			test.Fatal("nonordinary parent accepted")
		}
	}
	plan.Routes[0].Role = "main"
	state.Origins[0] = [2]int{1, 2}
	if len(state.targets(plan, 0, lakes[0], 1, 20, 12345, opts)) != 0 {
		test.Fatal("stage distance ignored")
	}
	state.Accepted = []riverJunction{{Route: 1, Parent: 0, Source: 0, Tile: baseline[0].Tile}}
	if len(state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) != 0 {
		test.Fatal("source-parent duplicate accepted")
	}
	state.Accepted[0].Source = 1
	for _, target := range state.targets(plan, 0, lakes[0], 100, 600, 12345, opts) {
		if !state.separated(target.Tile, opts.JunctionSpacingTiles) {
			test.Fatal("spacing violated")
		}
	}
	if state.separated(baseline[0].Tile+1, opts.JunctionSpacingTiles) {
		test.Fatal("nearby tributary allowed")
	}
	state.Accepted = nil
	state.LakeBounds = append(state.LakeBounds, image.Rect(380, 150, 388, 600))
	if len(state.targets(plan, 0, lakes[0], 100, 600, 12345, opts)) != 0 {
		test.Fatal("future lake entrance ignored")
	}
}

func TestRiverJunctionUnusedDestinationAndDegreeLimit(test *testing.T) {
	flow, plan, lakes, opts := junctionFixture(false, false, 3)
	unused := drawLake{ID: 3, X: 90, Y: 650, Radius: 18, RadiusX: 18, RadiusY: 18, DeepRatio: 0.6}
	lakes = append(lakes, unused)
	plan.Lakes = lakes
	plan.Junctions.LakeBounds = append(plan.Junctions.LakeBounds, newRiverJunctionState(768, 768, []drawLake{unused}, opts).LakeBounds[0])
	degree, connected, used := []int{0, 1, 1, 0}, []bool{false, true, true, false}, []bool{false, true, true, false}
	accepted := false
	for seed := int64(1); seed <= 32; seed++ {
		if junctionSource(0, 3, connected, seed) == 0 && tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 0, 3, 100, 600, seed, opts) {
			accepted = true
			break
		}
	}
	if !accepted || degree[0] != 1 || degree[3] != 0 || connected[3] || used[3] || len(plan.Inlets) != 1 {
		test.Fatal("unused destination gained a fake connection")
	}
	degree[0] = opts.MaxLakeDegree
	eligible, entries := plan.Junctions.Eligible, plan.Junctions.Entries
	if tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 0, -1, 100, 600, 1, opts) || plan.Junctions.Eligible != eligible || plan.Junctions.Entries != entries {
		test.Fatal("degree limit not enforced before attempt")
	}
}

func TestRiverJunctionRepeatedParentAndTributarySpacing(test *testing.T) {
	flow, plan, lakes, opts := junctionFixture(false, false, 3)
	degree, connected, used := []int{0, 1, 1}, []bool{false, true, true}, []bool{false, true, true}
	if !tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 0, 1, 100, 600, 12345, opts) {
		test.Fatal("first junction failed")
	}
	secondLake := drawLake{ID: 3, X: 668, Y: 520, Radius: 18, RadiusX: 18, RadiusY: 18, DeepRatio: 0.6}
	lakes = append(lakes, secondLake)
	plan.Lakes = lakes
	secondBounds := newRiverJunctionState(768, 768, []drawLake{secondLake}, opts).LakeBounds[0]
	plan.Junctions.LakeBounds = append(plan.Junctions.LakeBounds, secondBounds)
	degree = append(degree, 0)
	connected = append(connected, false)
	used = append(used, false)
	if !tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 3, 1, 100, 600, 67890, opts) {
		test.Fatalf("spaced second junction failed: %v", riverRouteStats(plan))
	}
	if plan.Junctions.Accepted[0].Parent != plan.Junctions.Accepted[1].Parent {
		test.Fatal("fixture changed parent")
	}
	opts.TributaryRatio = 1
	opts.TributarySpacingTiles = 180
	for _, lake := range lakes {
		carveDrawLakeFootprint(flow, 768, 768, lake, opts)
	}
	carveLakeInletChannels(flow, 768, 768, lakes, plan.Inlets, opts)
	if err := finishRiverFairways(flow, plan, 768, 768, 12345, opts); err != nil {
		test.Fatal(err)
	}
	for _, route := range plan.Routes {
		if route.Role == "tributary" && !plan.Junctions.separated(route.Path[0], 180) {
			test.Fatal("cross-type spacing violated")
		}
	}
}

func TestRiverJunctionOrdinaryFallbackGolden(test *testing.T) {
	opts := junctionGoldenOptions(12345)
	build := func() *TerrainPrecompute {
		terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(opts.Seed), 12))
		if err != nil {
			test.Fatal(err)
		}
		return terrain
	}
	before := build()
	opts.River.JunctionChance = 1
	opts.River.JunctionSpacingTiles = 8192
	after := build()
	if after.RiverFairways.Junctions.Selected == 0 {
		test.Fatal("no fallback opportunities exercised")
	}
	if !reflect.DeepEqual(before.Tiles, after.Tiles) || !reflect.DeepEqual(before.RiverClass, after.RiverClass) || !reflect.DeepEqual(before.RiverFairways.Routes, after.RiverFairways.Routes) || !reflect.DeepEqual(before.RiverFairways.Inlets, after.RiverFairways.Inlets) {
		test.Fatal("fallback changed original seeds/network")
	}
}

func TestRiverJunctionDenseBudget(test *testing.T) {
	flow, plan, lakes, opts := junctionFixture(false, false, 3)
	opts.MajorRiverCount = 2000
	for len(plan.Routes) < opts.MajorRiverCount {
		recordDrawRiver(plan, plan.Routes[0].Path, 7, 1, 2)
	}
	estimated, err := estimateRiverJunctionBytes(768, 768, opts)
	if err != nil || uint64(plan.Junctions.Entries)*128 > estimated {
		test.Fatalf("index storage estimate: %v", err)
	}
	degree, connected, used := []int{0, 1, 1}, []bool{false, true, true}, []bool{false, true, true}
	before := append([]uint32{}, flow...)
	if tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 0, 1, 100, 600, 12345, opts) {
		test.Fatal("dense foreign overlap accepted")
	}
	if plan.Junctions.Attempts == 0 || plan.Junctions.Attempts > 16 || plan.Junctions.Fallbacks != 1 || !reflect.DeepEqual(before, flow) {
		test.Fatal("dense exhaustion/rollback failed")
	}
}
