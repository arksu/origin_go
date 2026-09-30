package main

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestRiverWidthProfileRasterBounds(test *testing.T) {
	opts := bendTestOptions()
	opts.RiverWidthMin, opts.RiverWidthMax = 3, 19
	const width, height, centerRow = 2048, 80, 40
	path := appendCardinalRiverSegment(nil, width, 24, centerRow, width-25, centerRow)
	flow := make([]uint32, width*height)
	carveRiverCorridor(flow, width, height, path, 9, 12345, opts)
	classes := buildRiverClassMask(flow, width, height, opts)
	deepWidths, bankWidths := map[int]bool{}, map[int]bool{}
	asymmetric := false
	previousWidth := 0
	for column := 50; column < width-50; column++ {
		deepMin, deepMax := centerRow, centerRow
		for classes[tileIndex(column, deepMin-1, width)] == riverDeep {
			deepMin--
		}
		for classes[tileIndex(column, deepMax+1, width)] == riverDeep {
			deepMax++
		}
		deepWidth := deepMax - deepMin + 1
		if deepWidth < opts.FairwayWidthTiles || deepWidth > opts.RiverWidthMax {
			test.Fatalf("deep width %d outside configured range at %d", deepWidth, column)
		}
		if previousWidth > 0 && absInt(previousWidth-deepWidth) > 2 {
			test.Fatalf("abrupt width change at %d: %d -> %d", column, previousWidth, deepWidth)
		}
		previousWidth = deepWidth
		shallowMin, shallowMax := deepMin, deepMax
		for classes[tileIndex(column, shallowMin-1, width)] == riverShallow {
			shallowMin--
		}
		for classes[tileIndex(column, shallowMax+1, width)] == riverShallow {
			shallowMax++
		}
		leftWidth, rightWidth := deepMin-shallowMin, shallowMax-deepMax
		for _, bankWidth := range []int{leftWidth, rightWidth} {
			if bankWidth < opts.ShallowWidthMin || bankWidth > opts.ShallowWidthMax {
				test.Fatalf("bank width %d outside [%d,%d] at %d", bankWidth, opts.ShallowWidthMin, opts.ShallowWidthMax, column)
			}
			bankWidths[bankWidth] = true
		}
		asymmetric = asymmetric || leftWidth != rightWidth
		deepWidths[deepWidth] = true
	}
	if len(deepWidths) < 4 || len(bankWidths) < 4 || !asymmetric {
		test.Fatalf("insufficient variation: deep=%v banks=%v asymmetric=%v", deepWidths, bankWidths, asymmetric)
	}
	repeated := make([]uint32, len(flow))
	carveRiverCorridor(repeated, width, height, path, 9, 12345, opts)
	if !reflect.DeepEqual(flow, repeated) {
		test.Fatal("same seed changed widths")
	}
	different := make([]uint32, len(flow))
	carveRiverCorridor(different, width, height, path, 9, 67890, opts)
	if reflect.DeepEqual(flow, different) {
		test.Fatal("width variation ignored seed")
	}
}

func TestRiverWidthProfileKeepsBoatFootprintThroughBends(test *testing.T) {
	const width, height = 256, 256
	opts := bendTestOptions()
	opts.RiverWidthMin, opts.RiverWidthMax = 3, 15
	for _, seed := range []int64{12345, 67890, 314159} {
		path := buildNestedBendPath(width, height, 0, 0, width-1, height-1, seed, opts)
		flow := make([]uint32, width*height)
		carveRiverCorridor(flow, width, height, path, 5, seed, opts)
		plan := &riverFairways{Protected: make([]bool, len(flow))}
		route := protectRiverRoute(flow, plan, riverRoute{Path: path, Role: "main"}, width, height, opts)
		plan.Routes = []riverRoute{route}
		classes := buildRiverClassMask(flow, width, height, opts)
		if len(route.Path) < width || plan.validate(width, height, 3, seed, func(index int) bool { return classes[index] == riverDeep }) != nil {
			test.Fatalf("seed %d has broken boat clearance", seed)
		}
	}
}

func TestRiverWidthProfileNarrowChannelStillVaries(test *testing.T) {
	opts := bendTestOptions()
	opts.RiverWidthMin, opts.RiverWidthMax = 3, 5
	const width, height, centerRow = 1024, 32, 16
	path := appendCardinalRiverSegment(nil, width, 16, centerRow, width-17, centerRow)
	flow := make([]uint32, width*height)
	carveRiverCorridor(flow, width, height, path, 5, 12345, opts)
	widths := map[int]bool{}
	for column := 32; column < width-32; column++ {
		deepCount := 0
		for row := 0; row < height; row++ {
			if flow[tileIndex(column, row, width)] >= uint32(opts.FlowDeepThreshold) {
				deepCount++
			}
		}
		if deepCount < 3 || deepCount > 5 {
			test.Fatalf("narrow channel width outside [3,5]: %d", deepCount)
		}
		widths[deepCount] = true
	}
	if !widths[3] || !widths[5] {
		test.Fatalf("tile rounding erased narrow-channel width variation: %v", widths)
	}
}

func TestRiverWidthProfileConstantAndDisabledShallows(test *testing.T) {
	opts := bendTestOptions()
	opts.WidthVariationScale = 0
	opts.ShallowVariationScale = 0
	opts.BankRadius = 8
	const width, height, centerRow = 256, 64, 32
	path := appendCardinalRiverSegment(nil, width, 16, centerRow, width-17, centerRow)
	for _, shallowWidth := range []int{0, 3} {
		opts.ShallowWidthMin, opts.ShallowWidthMax = shallowWidth, shallowWidth
		flow := make([]uint32, width*height)
		carveRiverCorridor(flow, width, height, path, 9, 99, opts)
		classes := buildRiverClassMask(flow, width, height, opts)
		for column := 32; column < width-32; column++ {
			deepCount, shallowCount := 0, 0
			for row := 0; row < height; row++ {
				switch classes[tileIndex(column, row, width)] {
				case riverDeep:
					deepCount++
				case riverShallow:
					shallowCount++
				}
			}
			if deepCount != 9 || shallowCount != shallowWidth*2 {
				test.Fatalf("constant profile differs: deep=%d shallow=%d want 9/%d", deepCount, shallowCount, shallowWidth*2)
			}
		}
	}
}

func TestRiverWidthConfigurationAndPreview(test *testing.T) {
	opts, err := decodeMapgenOptions([]byte("version: 1\nriver:\n  fairway_width_tiles: 3\n"), "partial.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	if err := opts.Validate(); err != nil {
		test.Fatal(err)
	}
	if opts.River.ShallowWidthMin != 1 || opts.River.ShallowWidthMax != 5 || opts.River.WidthVariationScale != 160 || opts.River.ShallowVariationScale != 64 {
		test.Fatal("omitted width settings did not inherit defaults")
	}
	for _, raw := range []string{
		`{"width_variation_scale":-1}`, `{"width_variation_scale":15}`,
		`{"shallow_width_min":6,"shallow_width_max":5}`,
		`{"shallow_width_min":-1}`, `{"shallow_width_max":33}`,
		`{"shallow_variation_scale":0}`, `{"shallow_variation_scale":8193}`,
	} {
		_, err := resolvePreviewOptions(renderRequest{Seed: 12345, Params: map[string]json.RawMessage{"river": json.RawMessage(raw)}}, opts)
		if err == nil || !strings.Contains(err.Error(), "river.") {
			test.Fatalf("invalid width settings accepted: %s (%v)", raw, err)
		}
	}
	resolved, err := resolvePreviewOptions(renderRequest{Seed: 12345, Params: map[string]json.RawMessage{"river": json.RawMessage(`{"width_variation_scale":0,"shallow_width_min":0,"shallow_width_max":0,"shallow_variation_scale":0}`)}}, opts)
	if err != nil {
		test.Fatal(err)
	}
	encoded, err := encodePreviewPreset([]byte("version: 1\n"), resolved)
	if err != nil {
		test.Fatal(err)
	}
	reloaded, err := decodeMapgenOptions(encoded, "saved.yaml", DefaultMapgenOptions())
	if err != nil || reloaded.River != resolved.River {
		test.Fatalf("width settings lost in save round-trip: %v", err)
	}
	fields := map[string]bool{}
	for _, group := range riversLayerSchema(opts).Groups {
		for _, field := range group.Fields {
			fields[field.Key] = true
		}
	}
	for _, key := range []string{"width_variation_scale", "shallow_width_min", "shallow_width_max", "shallow_variation_scale"} {
		if !fields[key] {
			test.Fatalf("missing river control %s", key)
		}
	}
}

func TestRiverCrossSectionSmoothAndBounded(test *testing.T) {
	opts := bendTestOptions()
	opts.RiverWidthMin, opts.RiverWidthMax = 3, 19
	previous := riverCrossSectionAt(0, 9, 12345, opts)
	for distance := 1; distance < 2000; distance++ {
		section := riverCrossSectionAt(float64(distance), 9, 12345, opts)
		if section.deepWidth < 3 || section.deepWidth > 19 || math.Abs(section.deepWidth-previous.deepWidth) > 0.2 {
			test.Fatalf("abrupt or out-of-range deep width at %d: %+v", distance, section)
		}
		if math.Abs(section.leftShallow-previous.leftShallow) > 1 || math.Abs(section.rightShallow-previous.rightShallow) > 1 {
			test.Fatalf("abrupt shallow width at %d: %+v", distance, section)
		}
		previous = section
	}
}
