package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

var junctionReviewDirectory = flag.String("junction-review-dir", "", "write Y-junction comparisons without database access")
var junctionReviewFull = flag.Bool("junction-review-full", false, "review current full-size H&H world")

func TestRiverJunctionReview(test *testing.T) {
	if *junctionReviewDirectory == "" {
		test.Skip("opt-in Y-junction review")
	}
	base, _, err := LoadMapgenOptionsFromYAML("../../etc/mapgen/presets/hnh.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	if err := os.MkdirAll(*junctionReviewDirectory, 0o755); err != nil {
		test.Fatal(err)
	}
	seeds := []int64{12345, 67890, 314159, base.Seed}
	if *junctionReviewFull {
		seeds = []int64{base.Seed}
	} else {
		base.ChunksX, base.ChunksY = 12, 12
	}
	var report strings.Builder
	report.WriteString("# Y-junction review\n\nBlue: deep water. Cyan: shallow water. All reported passages use the configured square deep-tile footprint, not runtime boat physics. No database writes. Visual acceptance pending.\n\n")
	for _, seed := range seeds {
		var baselineRoutes []riverRoute
		var baselineLakes []drawLake
		for _, chance := range []float64{0, 0.25, 1} {
			opts := base
			opts.Seed, opts.River.JunctionChance = seed, chance
			if err := opts.Validate(); err != nil {
				test.Fatal(err)
			}
			if err := validatePreviewMemory(opts); err != nil {
				test.Fatal(err)
			}
			started := time.Now()
			terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(seed), 12, opts.TerrainScale))
			if err != nil {
				test.Fatal(err)
			}
			duration := time.Since(started)
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			plan := terrain.RiverFairways
			if chance == 0 {
				baselineRoutes = plan.Routes
				baselineLakes = plan.Lakes
			} else if !reflect.DeepEqual(baselineLakes, plan.Lakes) || !reflect.DeepEqual(baselineRoutes[:plan.Junctions.BackboneCount], plan.Routes[:plan.Junctions.BackboneCount]) {
				test.Fatal("lake placement or regional backbone changed")
			}
			connected := map[int]bool{}
			for _, inlet := range plan.Inlets {
				connected[inlet.LakeIndex] = true
			}
			components := reviewMainComponents(terrain, plan.Routes[:plan.MainCount])
			prefix := fmt.Sprintf("%d-chance-%g", seed, chance)
			world := image.Rect(0, 0, terrain.WidthTiles, terrain.HeightTiles)
			if err := writeRiverReviewImage(filepath.Join(*junctionReviewDirectory, prefix+"-map.png"), terrain, world, maxInt(1, terrain.WidthTiles/1600), 1, false); err != nil {
				test.Fatal(err)
			}
			fmt.Fprintf(&report, "## Seed %d, chance %g (%dx%d)\n\n- Generation: %s\n- Go HeapAlloc: %d MiB; HeapSys: %d MiB (snapshot, not peak RSS; shared process retains previous allocations)\n- Connected lakes: %d/%d; water components containing main routes: %d\n- Final configured deep footprint and lake land validation: passed\n- Statistics: `%v`\n\n![Overview](%s-map.png)\n\n", seed, chance, terrain.WidthTiles, terrain.HeightTiles, duration, memory.HeapAlloc>>20, memory.HeapSys>>20, len(connected), len(plan.Lakes), components, riverRouteStats(plan), prefix)
			test.Logf("seed=%d chance=%g build=%s heap_alloc_mib=%d heap_sys_mib=%d connected=%d components=%d stats=%v", seed, chance, duration, memory.HeapAlloc>>20, memory.HeapSys>>20, len(connected), components, riverRouteStats(plan))
			if plan.Junctions != nil {
				for index, junction := range plan.Junctions.Accepted {
					if index >= 6 {
						break
					}
					point := image.Pt(junction.Tile%terrain.WidthTiles, junction.Tile/terrain.WidthTiles)
					name := fmt.Sprintf("%s-junction-%d.png", prefix, index)
					if err := writeRiverReviewImage(filepath.Join(*junctionReviewDirectory, name), terrain, image.Rect(point.X-120, point.Y-120, point.X+121, point.Y+121).Intersect(world), 1, 3, false); err != nil {
						test.Fatal(err)
					}
					fmt.Fprintf(&report, "![Branch %d, parent %d, lake %d](%s)\n\n", junction.Route, junction.Parent, junction.Source, name)
				}
			}
		}
	}
	for _, curved := range []bool{false, true} {
		for _, right := range []bool{false, true} {
			flow, plan, lakes, opts := junctionFixture(curved, right, 3)
			if !tryRiverJunction(flow, 768, 768, lakes, []int{0, 1, 1}, []bool{false, true, true}, []bool{false, true, true}, &plan.Inlets, plan, 0, 1, 100, 600, 12345, opts) {
				test.Fatal("forced fixture failed")
			}
			for _, lake := range lakes {
				carveDrawLakeFootprint(flow, 768, 768, lake, opts)
			}
			carveLakeInletChannels(flow, 768, 768, lakes, plan.Inlets, opts)
			if err := finishRiverFairways(flow, plan, 768, 768, 12345, opts); err != nil {
				test.Fatal(err)
			}
			classes := buildRiverClassMask(flow, 768, 768, opts, plan)
			tiles := make([]byte, len(flow))
			for index, class := range classes {
				tiles[index] = resolveTileType(1, tileGrass, class, false, true)
			}
			terrain := &TerrainPrecompute{WidthTiles: 768, HeightTiles: 768, Tiles: tiles, RiverClass: classes, RiverFairways: plan}
			name := fmt.Sprintf("forced-curved-%v-right-%v.png", curved, right)
			if err := writeRiverReviewImage(filepath.Join(*junctionReviewDirectory, name), terrain, image.Rect(0, 0, 768, 768), 1, 1, false); err != nil {
				test.Fatal(err)
			}
			fmt.Fprintf(&report, "![Forced geometry](%s)\n\n", name)
		}
	}
	flow, plan, lakes, opts := junctionFixture(false, false, 3)
	for _, offset := range []int{-20, 20} {
		path := make([]int, len(plan.Routes[0].Path))
		for index, tile := range plan.Routes[0].Path {
			path[index] = tile + offset
		}
		carveRiverCorridor(flow, 768, 768, path, 7, 12345, opts)
		recordDrawRiver(plan, path, 7, 1, 2)
	}
	if tryRiverJunction(flow, 768, 768, lakes, []int{0, 1, 1}, []bool{false, true, true}, []bool{false, true, true}, &plan.Inlets, plan, 0, 1, 100, 600, 12345, opts) {
		test.Fatal("dense review fixture should reject foreign contact")
	}
	for _, lake := range lakes {
		carveDrawLakeFootprint(flow, 768, 768, lake, opts)
	}
	classes := buildRiverClassMask(flow, 768, 768, opts, plan)
	tiles := make([]byte, len(flow))
	for index, class := range classes {
		tiles[index] = resolveTileType(1, tileGrass, class, false, true)
	}
	terrain := &TerrainPrecompute{WidthTiles: 768, HeightTiles: 768, Tiles: tiles, RiverClass: classes, RiverFairways: plan}
	if err := writeRiverReviewImage(filepath.Join(*junctionReviewDirectory, "forced-dense-rejected.png"), terrain, image.Rect(0, 0, 768, 768), 1, 1, false); err != nil {
		test.Fatal(err)
	}
	fmt.Fprintf(&report, "## Dense-water rejection fixture\n\nNo unsafe Y was carved next to foreign corridors. Statistics: `%v`\n\n![Dense water](forced-dense-rejected.png)\n", riverRouteStats(plan))
	if err := os.WriteFile(filepath.Join(*junctionReviewDirectory, "README.md"), []byte(report.String()), 0o644); err != nil {
		test.Fatal(err)
	}
}
