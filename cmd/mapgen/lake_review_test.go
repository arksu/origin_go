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

var lakeReviewDirectory = flag.String("lake-review-dir", "", "write lake geometry review images without DB access")
var lakeReviewFull = flag.Bool("lake-review-full", false, "review one full current H&H world")

func TestLakeGeometryReview(test *testing.T) {
	if *lakeReviewDirectory == "" {
		test.Skip("opt-in lake image review")
	}
	base, _, err := LoadMapgenOptionsFromYAML("../../etc/mapgen/presets/hnh.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	if err := os.MkdirAll(*lakeReviewDirectory, 0o755); err != nil {
		test.Fatal(err)
	}
	seeds := []int64{12345, 67890, 314159, base.Seed}
	if *lakeReviewFull {
		seeds = []int64{base.Seed}
	} else {
		base.ChunksX, base.ChunksY = 12, 12
	}
	var report strings.Builder
	report.WriteString("# Lake geometry review\n\nBlue: final deep water. Cyan: shallow water. Land: biome colors. Clearance uses the configured square footprint; no runtime boat physics or database writes. Visual acceptance pending.\n\n")
	for _, seed := range seeds {
		var before *TerrainPrecompute
		for _, enabled := range []bool{false, true} {
			opts := base
			opts.Seed, opts.River.LakeIrregularEnabled = seed, enabled
			started := time.Now()
			terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFieldsWithTerrainScale(NewPerlinNoise(seed), 12, opts.TerrainScale))
			if err != nil {
				test.Fatalf("seed %d enabled %v: %v", seed, enabled, err)
			}
			duration := time.Since(started)
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			variant := "before"
			if enabled {
				variant = "after"
			}
			prefix := fmt.Sprintf("%d-%s", seed, variant)
			world := image.Rect(0, 0, terrain.WidthTiles, terrain.HeightTiles)
			if err := writeRiverReviewImage(filepath.Join(*lakeReviewDirectory, prefix+"-map.png"), terrain, world, maxInt(1, terrain.WidthTiles/1600), 1, false); err != nil {
				test.Fatal(err)
			}
			fmt.Fprintf(&report, "## Seed %d, %s (%dx%d)\n\n- Build: %s\n- Go reserved heap: %d MiB (not peak RSS)\n- Statistics: `%v`\n- Final route and dry-land validation: passed\n\n![Overview](%s-map.png)\n\n", seed, variant, terrain.WidthTiles, terrain.HeightTiles, duration, memory.HeapSys/(1024*1024), riverRouteStats(terrain.RiverFairways), prefix)
			test.Logf("seed=%d variant=%s build=%s heap_sys_mib=%d stats=%v", seed, variant, duration, memory.HeapSys/(1024*1024), riverRouteStats(terrain.RiverFairways))
			if !enabled {
				before = terrain
				continue
			}
			if !reflect.DeepEqual(before.RiverFairways.Lakes, terrain.RiverFairways.Lakes) || !reflect.DeepEqual(before.RiverFairways.Routes[:before.RiverFairways.MainCount], terrain.RiverFairways.Routes[:terrain.RiverFairways.MainCount]) {
				test.Fatal("lake detail changed placements or main routes")
			}
			selected := make([]*lakeShape, 0, 3)
			for _, shape := range terrain.RiverFairways.LakeShapes {
				if len(shape.Islands) > 0 && len(selected) < 2 {
					selected = append(selected, shape)
				}
			}
			for _, shape := range terrain.RiverFairways.LakeShapes {
				if shape.Peninsulas > 0 && shape.Lake.SizeClass == lakeSizeLarge {
					selected = append(selected, shape)
					break
				}
			}
			for _, shape := range selected {
				for _, view := range []struct {
					name    string
					terrain *TerrainPrecompute
				}{{"before", before}, {"after", terrain}} {
					name := fmt.Sprintf("%d-lake-%d-%s.png", seed, shape.Lake.ID, view.name)
					if err := writeRiverReviewImage(filepath.Join(*lakeReviewDirectory, name), view.terrain, shape.Bounds.Intersect(world), 1, 2, false); err != nil {
						test.Fatal(err)
					}
					fmt.Fprintf(&report, "![Lake %d %s](%s)\n\n", shape.Lake.ID, view.name, name)
				}
			}
		}
	}
	writeLakeReviewFixture(test, *lakeReviewDirectory)
	report.WriteString("## Forced fixture\n\nTwo entrances, peninsulas, and a forced island request. Deep loops and inlets are validated.\n\n![Fixture](forced-islands.png)\n")
	if err := os.WriteFile(filepath.Join(*lakeReviewDirectory, "README.md"), []byte(report.String()), 0o644); err != nil {
		test.Fatal(err)
	}
}

func writeLakeReviewFixture(test *testing.T, directory string) {
	test.Helper()
	opts := DefaultMapgenOptions().River
	opts.LakeIrregularEnabled, opts.FairwayWidthTiles = true, 3
	opts.LakeIslandLargeChance, opts.LakeIslandSecondChance = 1, 1
	plan := &riverFairways{Protected: make([]bool, 256*256), Lakes: []drawLake{{ID: 0, X: 128, Y: 128, Radius: 90, RadiusX: 90, RadiusY: 70, SizeClass: lakeSizeLarge, LobeCount: 4, Irregularity: 0.25, DeepRatio: 0.5, PhaseA: 0.7}}, Inlets: []lakeInlet{{LakeIndex: 0, X: 55, Y: 128, RiverWidth: 5}, {LakeIndex: 0, X: 200, Y: 128, RiverWidth: 5}}}
	flow := make([]uint32, 256*256)
	if err := finishIrregularLakes(flow, plan, 256, 256, 12345, opts); err != nil {
		test.Fatal(err)
	}
	classes := buildRiverClassMask(flow, 256, 256, opts, plan)
	tiles := make([]byte, len(flow))
	for index, class := range classes {
		if plan.isLakeLand(index) {
			classes[index] = riverNone
			class = riverNone
		}
		tiles[index] = resolveTileType(1, tileGrass, class, false, true)
	}
	if err := plan.validate(256, 256, 3, 12345, func(index int) bool { return tiles[index] == tileWaterDeep }); err != nil {
		test.Fatal(err)
	}
	if err := plan.validateLakeLand(12345, func(index int) bool { return isWaterTileID(tiles[index]) }, 256); err != nil {
		test.Fatal(err)
	}
	terrain := &TerrainPrecompute{WidthTiles: 256, HeightTiles: 256, Tiles: tiles, RiverClass: classes, RiverFairways: plan}
	if err := writeRiverReviewImage(filepath.Join(directory, "forced-islands.png"), terrain, image.Rect(0, 0, 256, 256), 1, 3, false); err != nil {
		test.Fatal(err)
	}
}

func TestLakeIslandOutlineReview(test *testing.T) {
	if *lakeReviewDirectory == "" {
		test.Skip("opt-in island outline review")
	}
	if err := os.MkdirAll(*lakeReviewDirectory, 0o755); err != nil {
		test.Fatal(err)
	}
	const cellSize, width, height = 64, 256, 192
	water := make([]bool, width*height)
	for index := range water {
		water[index] = true
	}
	opts := DefaultMapgenOptions().River
	for row, radius := range []int{4, 8, 12} {
		opts.LakeIslandRadiusMin, opts.LakeIslandRadiusMax = radius, radius
		for column := 0; column < 4; column++ {
			shape := &lakeShape{Lake: drawLake{ID: row*4 + column}, Bounds: image.Rect(0, 0, cellSize, cellSize), Water: make([]bool, cellSize*cellSize)}
			var island lakeIsland
			for attempt := 0; attempt < 8 && len(island.Tiles) == 0; attempt++ {
				island, _ = shape.island(12345, attempt, cellSize, opts)
			}
			if len(island.Tiles) == 0 {
				test.Fatal("gallery island did not fit")
			}
			center := image.Pt(column*cellSize+cellSize/2, row*cellSize+cellSize/2)
			for _, index := range island.Tiles {
				point := center.Add(image.Pt(index%cellSize, index/cellSize).Sub(island.Center))
				water[tileIndex(point.X, point.Y, width)] = false
			}
		}
	}
	distances := lakeWaterDistances(water, width)
	tiles := make([]byte, len(water))
	for index, wet := range water {
		switch {
		case !wet:
			tiles[index] = tileGrass
		case distances[index] <= 2:
			tiles[index] = tileWater
		default:
			tiles[index] = tileWaterDeep
		}
	}
	terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: height, Tiles: tiles, RiverClass: make([]RiverClass, len(tiles))}
	if err := writeRiverReviewImage(filepath.Join(*lakeReviewDirectory, "island-shapes.png"), terrain, image.Rect(0, 0, width, height), 1, 4, false); err != nil {
		test.Fatal(err)
	}
}
