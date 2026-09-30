package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func junctionGoldenOptions(seed int64) MapgenOptions {
	opts := lakeTestOptions(seed)
	opts.River.LakeIrregularEnabled = true
	opts.River.LakeIslandMediumChance, opts.River.LakeIslandLargeChance = 1, 1
	opts.River.WidthVariationScale = 96
	return opts
}

func TestRiverJunctionOptions(test *testing.T) {
	defaults := DefaultMapgenOptions()
	if defaults.River.JunctionChance != 0 || defaults.River.JunctionSpacingTiles != 180 {
		test.Fatal("junction defaults")
	}
	for _, configuration := range []string{"version: 1\n", "version: 1\nriver:\n  junction_chance: 0\n"} {
		decoded, err := decodeMapgenOptions([]byte(configuration), "junction.yaml", defaults)
		if err != nil || decoded.River != defaults.River {
			test.Fatalf("omitted/zero settings: %v", err)
		}
	}
	if _, err := decodeMapgenOptions([]byte("version: 1\nriver:\n  junction_spacing_tiles: 1.5\n"), "junction.yaml", defaults); err == nil || !strings.Contains(err.Error(), "junction_spacing_tiles") {
		test.Fatalf("fractional YAML spacing accepted: %v", err)
	}
	cases := []struct {
		name    string
		edit    func(*RiverOptions)
		invalid bool
	}{
		{"zero", func(opts *RiverOptions) { opts.JunctionChance = 0; opts.JunctionSpacingTiles = 0 }, false},
		{"one", func(opts *RiverOptions) { opts.JunctionChance = 1 }, false},
		{"negative", func(opts *RiverOptions) { opts.JunctionChance = -0.1 }, true},
		{"over", func(opts *RiverOptions) { opts.JunctionChance = 1.1 }, true},
		{"nan", func(opts *RiverOptions) { opts.JunctionChance = math.NaN() }, true},
		{"infinity", func(opts *RiverOptions) { opts.JunctionChance = math.Inf(1) }, true},
		{"spacing negative", func(opts *RiverOptions) { opts.JunctionSpacingTiles = -1 }, true},
		{"spacing over", func(opts *RiverOptions) { opts.JunctionSpacingTiles = 8193 }, true},
		{"spacing zero", func(opts *RiverOptions) { opts.JunctionSpacingTiles = 0 }, true},
		{"layout", func(opts *RiverOptions) { opts.LayoutDraw = false }, true},
		{"option b", func(opts *RiverOptions) { opts.ShapeWavelengthTiles = 0 }, true},
		{"fairway missing", func(opts *RiverOptions) { opts.FairwayWidthTiles = 0 }, true},
		{"fairway even", func(opts *RiverOptions) { opts.FairwayWidthTiles = 4 }, true},
		{"fairway too wide", func(opts *RiverOptions) { opts.FairwayWidthTiles = 63 }, true},
	}
	for _, fixture := range cases {
		test.Run(fixture.name, func(test *testing.T) {
			opts := bendTestOptions()
			opts.JunctionChance = 0.25
			fixture.edit(&opts)
			if err := opts.validateBends(); (err != nil) != fixture.invalid {
				test.Fatalf("validation: %v", err)
			}
		})
	}
	opts := bendTestOptions()
	if extra, err := estimateRiverJunctionBytes(1024, 1024, opts); err != nil || extra != 0 {
		test.Fatalf("zero memory: %d %v", extra, err)
	}
	opts.JunctionChance = 1
	if extra, err := estimateRiverJunctionBytes(1024, 1024, opts); err != nil || extra == 0 {
		test.Fatalf("active memory: %d %v", extra, err)
	}
	if _, err := estimateRiverJunctionBytes(int(^uint(0)>>1), int(^uint(0)>>1), opts); err == nil {
		test.Fatal("overflow accepted")
	}
	opts.MajorRiverCount = int(^uint(0) >> 1)
	if _, err := BuildRiverNetwork(make([]float32, 128*128), 128, 128, 1, opts); err == nil {
		test.Fatal("overbudget direct allocation accepted")
	}
	opts.MajorRiverCount = 2_000_000
	if _, err := BuildRiverNetwork(make([]float32, 128*128), 128, 128, 1, opts); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		test.Fatalf("direct memory budget: %v", err)
	}
}

func junctionFixture(curved, right bool, fairway int) ([]uint32, *riverFairways, []drawLake, RiverOptions) {
	const size = 768
	opts := bendTestOptions()
	opts.JunctionChance, opts.JunctionSpacingTiles = 1, 120
	opts.FairwayWidthTiles = fairway
	opts.RiverWidthMin = maxInt(5, fairway)
	opts.RiverWidthMax = maxInt(9, fairway)
	sourceColumn := 100
	if right {
		sourceColumn = 668
	}
	lakes := []drawLake{{ID: 0, X: sourceColumn, Y: 280, Radius: 18, RadiusX: 18, RadiusY: 18, DeepRatio: 0.6}, {ID: 1, X: 384, Y: 45, Radius: 18, RadiusX: 18, RadiusY: 18, DeepRatio: 0.6}, {ID: 2, X: 384, Y: 723, Radius: 18, RadiusX: 18, RadiusY: 18, DeepRatio: 0.6}}
	plan := &riverFairways{Lakes: lakes, Junctions: newRiverJunctionState(size, size, lakes, opts)}
	path := []int{tileIndex(384, 65, size)}
	for row := 66; row <= 703; row++ {
		column := 384
		if curved {
			column += int(math.Round(35 * math.Sin(float64(row-65)/638*math.Pi*2)))
		}
		last := path[len(path)-1]
		path = appendCardinalRiverSegment(path, size, last%size, last/size, column, row)
	}
	flow := make([]uint32, size*size)
	carveRiverCorridor(flow, size, size, path, 7, 12345, opts)
	recordDrawRiver(plan, path, 7, 1, 2)
	return flow, plan, lakes, opts
}

func TestRiverJunctionGeometryAndAtomicCommit(test *testing.T) {
	for _, curved := range []bool{false, true} {
		for _, right := range []bool{false, true} {
			for _, fairway := range []int{3, 5} {
				flow, plan, lakes, opts := junctionFixture(curved, right, fairway)
				before := append([]uint32(nil), flow...)
				degree, connected, used := []int{0, 1, 1}, []bool{false, true, true}, []bool{false, true, true}
				if !tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 0, 1, 100, 600, 12345, opts) {
					test.Fatalf("curved %v right %v fairway %d: %v", curved, right, fairway, riverRouteStats(plan))
				}
				if !reflect.DeepEqual(degree, []int{1, 1, 1}) || len(plan.Routes) != 2 || len(plan.Inlets) != 1 || plan.Inlets[0].LakeIndex != 0 || !connected[0] || !used[0] {
					test.Fatal("source-only atomic commit failed")
				}
				branch := plan.Routes[1]
				carveRiverCorridor(before, 768, 768, branch.Path, branch.Width, 12345+int64(plan.Junctions.Attempts)*104729, opts)
				if !reflect.DeepEqual(before, flow) {
					test.Fatal("junction carving changed the configured width/bank profile")
				}
				for _, lake := range lakes {
					carveDrawLakeFootprint(flow, 768, 768, lake, opts)
				}
				carveLakeInletChannels(flow, 768, 768, lakes, plan.Inlets, opts)
				if err := finishRiverFairways(flow, plan, 768, 768, 12345, opts); err != nil {
					test.Fatal(err)
				}
				junction := plan.Junctions.Accepted[0]
				if len(plan.Junctions.Accepted) != 1 || plan.Junctions.Attempts > 16 {
					test.Fatal("attempt/budget limit")
				}
				flow[junction.Tile] = 0
				err := plan.validate(768, 768, fairway, 12345, func(index int) bool { return flow[index] >= uint32(opts.FlowDeepThreshold) })
				if err == nil || !strings.Contains(err.Error(), "seed 12345") || !strings.Contains(err.Error(), "junction branch 1 parent 0") {
					test.Fatalf("corruption diagnostic: %v", err)
				}
			}
		}
	}
}

func TestRiverJunctionSelectionAndRollback(test *testing.T) {
	if junctionSource(0, 1, []bool{true, false}, 12) != 1 || junctionSource(0, 1, []bool{false, true}, 12) != 0 {
		test.Fatal("unconnected source preference")
	}
	for _, chance := range []float64{0, 0.25, 1} {
		state := &riverJunctionState{Origins: map[int][2]int{0: {1, 2}}}
		for seed := int64(0); seed < 1000; seed++ {
			state.selectOpportunity(0, seed, chance)
		}
		if chance == 0 && state.Selected != 0 || chance == 1 && state.Selected != 1000 || chance == 0.25 && (state.Selected < 200 || state.Selected > 300) {
			test.Fatalf("one roll distribution: %v", state)
		}
	}
	flow, plan, lakes, opts := junctionFixture(false, false, 3)
	before := append([]uint32(nil), flow...)
	degree, connected, used := []int{0, 1, 1}, []bool{false, true, true}, []bool{false, true, true}
	opts.JunctionSpacingTiles = 8192
	entries := plan.Junctions.Entries
	if tryRiverJunction(flow, 768, 768, lakes, degree, connected, used, &plan.Inlets, plan, 0, 1, 100, 600, 12345, opts) {
		test.Fatal("impossible junction placed")
	}
	if !reflect.DeepEqual(flow, before) || len(plan.Routes) != 1 || len(plan.Inlets) != 0 || degree[0] != 0 || connected[0] || used[0] || plan.Junctions.Entries != entries {
		test.Fatal("rejection mutated network")
	}
	if plan.Junctions.Selected != 1 || plan.Junctions.Fallbacks != 1 {
		test.Fatal("retry rerolled probability")
	}
}

func TestRiverJunctionGeneratedNetwork(test *testing.T) {
	base, _, err := LoadMapgenOptionsFromYAML("../../etc/mapgen/presets/hnh.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	base.ChunksX, base.ChunksY = 12, 12
	base.River.JunctionChance = 1
	for _, seed := range []int64{12345, 67890, 314159, base.Seed} {
		base.Seed = seed
		terrain, err := BuildTerrainPrecompute(base, 128, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(seed), 12, base.TerrainScale))
		if err != nil {
			test.Fatal(err)
		}
		test.Logf("seed %d: %v", seed, riverRouteStats(terrain.RiverFairways))
		state := terrain.RiverFairways.Junctions
		if state.Selected != state.Eligible || state.Selected != len(state.Accepted)+state.Fallbacks || state.Attempts > 16*state.Selected {
			test.Fatal("opportunity accounting")
		}
	}
}

func TestRiverJunctionDisabledGolden(test *testing.T) {
	fixtures := map[int64][3]string{
		12345: {"be608b49b1386ac801dd03ebbd3987ddc6a1acd659d87b501afb0214969aa51d", "260b2929184958f9ab87023577ac5458ad0f71c6d6cdfc82099e8eddbc0229e7", "145b62af1518e337437b3e5934863c4a70788a8867eef039df21b8fcc6a5d18d"},
		67890: {"30220ca6c12a3e273baeb60a3e405a9ba4b0ee70cedbc72e5c006dfd5ddc09de", "6f33120025e1f56b107639d5f02f1eb16924bf76edfdf5ac5cda89f1ea6fef60", "2986bdb020d5c426e8d6c40e9611a0709f9ec2d305a0082d9d726a771875bee3"},
	}
	for seed, expected := range fixtures {
		opts := junctionGoldenOptions(seed)
		terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
		if err != nil {
			test.Fatal(err)
		}
		classes := make([]byte, len(terrain.RiverClass))
		for index, class := range terrain.RiverClass {
			classes[index] = byte(class)
		}
		routes, err := json.Marshal(terrain.RiverFairways.Routes)
		if err != nil {
			test.Fatal(err)
		}
		actual := [3]string{fmt.Sprintf("%x", sha256.Sum256(terrain.Tiles)), fmt.Sprintf("%x", sha256.Sum256(classes)), fmt.Sprintf("%x", sha256.Sum256(routes))}
		if actual != expected {
			test.Fatalf("seed %d changed disabled junction output: %v", seed, actual)
		}
	}
}
