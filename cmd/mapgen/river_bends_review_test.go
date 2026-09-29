package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var bendsReviewDirectory = flag.String("bends-review-dir", "", "write river comparison images without DB access")
var bendsReviewFull = flag.Bool("bends-review-full", false, "review the full H&H preset world")

func TestRiverBendsReview(test *testing.T) {
	if *bendsReviewDirectory == "" {
		test.Skip("opt-in image review")
	}
	base, _, err := LoadMapgenOptionsFromYAML("../../etc/mapgen/presets/hnh.yaml", DefaultMapgenOptions())
	if err != nil {
		test.Fatal(err)
	}
	if !*bendsReviewFull {
		base.ChunksX, base.ChunksY = 12, 12
	}
	if err := os.MkdirAll(*bendsReviewDirectory, 0o755); err != nil {
		test.Fatal(err)
	}
	var report strings.Builder
	report.WriteString("# Option B river review\n\nMinimum fairway: 3 deep tiles. No DB writes. Boat-runtime behavior not tested.\n\n")
	report.WriteString("Review settings: wavelength 400, gain 0.5, amplitude 1.5, branch ratio 0.08, junction spacing 300. Visual acceptance is pending.\n\n")
	report.WriteString("The existing lake/link selection policy is retained, not an identical accepted graph. Component increases flag reduced whole-network connectivity for review; validated individual routes do not imply one connected world. Overviews preserve thin river pixels; close-ups show actual final deep/shallow tiles.\n\n")
	for _, seed := range []int64{12345, 67890, 314159} {
		var comparison [2]*TerrainPrecompute
		var timings [2]time.Duration
		var routeCounts [2]int
		var componentCounts [2]int
		var linkedLakeCounts [2]int
		var memory [2]uint64
		for variant := 0; variant < 2; variant++ {
			opts := base
			opts.Seed = seed
			if variant == 0 {
				opts.River.ShapeWavelengthTiles, opts.River.FairwayWidthTiles, opts.River.TributaryRatio = 0, 0, 0
				opts.River.ShapeOctaveGain = 0.06
				opts.River.ShapeAmplitudeScale = 0.5
			}
			started := time.Now()
			terrain, err := BuildTerrainPrecompute(opts, 128, NewNoiseFields(NewPerlinNoise(seed), 12))
			if err != nil {
				test.Fatalf("seed %d variant %d: %v", seed, variant, err)
			}
			timings[variant] = time.Since(started)
			comparison[variant] = terrain
			plan := terrain.RiverFairways
			if plan == nil {
				plan = &riverFairways{}
				buildDrawLayoutRiverFlow(terrain.Elevation, terrain.WidthTiles, terrain.HeightTiles, seed, opts.River, plan)
				plan.MainCount = len(plan.Routes)
			}
			routeCounts[variant] = plan.MainCount
			linkedLakes := make(map[int]bool)
			for _, inlet := range plan.Inlets {
				linkedLakes[inlet.LakeIndex] = true
			}
			linkedLakeCounts[variant] = len(linkedLakes)
			componentCounts[variant] = reviewMainComponents(terrain, plan.Routes[:plan.MainCount])
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			memory[variant] = stats.HeapSys / (1024 * 1024)
			test.Logf("seed=%d variant=%d world=%dx%d main=%d components=%d stats=%v build=%s heap_sys_mib=%d", seed, variant, terrain.WidthTiles, terrain.HeightTiles, plan.MainCount, componentCounts[variant], riverRouteStats(terrain.RiverFairways), timings[variant], memory[variant])
		}
		current := comparison[1]
		if current.RiverFairways.MainCount == 0 {
			test.Fatal("no main routes")
		}
		minimumSpacing := reviewMinimumJunctionSpacing(current.RiverFairways, current.WidthTiles)
		spacingLabel := "n/a (no tributaries)"
		if !math.IsInf(minimumSpacing, 1) {
			spacingLabel = fmt.Sprintf("%.1f tiles (required >= %d)", minimumSpacing, base.River.TributarySpacingTiles)
			if minimumSpacing < float64(base.River.TributarySpacingTiles) {
				test.Fatal("tributary spacing below configured minimum")
			}
		}
		longest := current.RiverFairways.Routes[0]
		for _, route := range current.RiverFairways.Routes[:current.RiverFairways.MainCount] {
			if len(route.Path) > len(longest.Path) {
				longest = route
			}
		}
		center := longest.Path[len(longest.Path)/2]
		junction := center
		for _, route := range current.RiverFairways.Routes {
			if route.Role == "tributary" {
				junction = route.Path[0]
				break
			}
		}
		for variant, terrain := range comparison {
			prefix := fmt.Sprintf("%d-%s", seed, []string{"before", "after"}[variant])
			world := image.Rect(0, 0, terrain.WidthTiles, terrain.HeightTiles)
			for _, view := range []struct {
				name         string
				rectangle    image.Rectangle
				stride, zoom int
				rivers       bool
			}{
				{"map", world, maxInt(1, terrain.WidthTiles/1280), 1, false},
				{"rivers", world, maxInt(1, terrain.WidthTiles/1280), 1, true},
				{"bends", image.Rect(center%terrain.WidthTiles-320, center/terrain.WidthTiles-320, center%terrain.WidthTiles+320, center/terrain.WidthTiles+320).Intersect(world), 1, 1, false},
				{"fairway", image.Rect(junction%terrain.WidthTiles-48, junction/terrain.WidthTiles-48, junction%terrain.WidthTiles+48, junction/terrain.WidthTiles+48).Intersect(world), 1, 6, false},
			} {
				path := filepath.Join(*bendsReviewDirectory, prefix+"-"+view.name+".png")
				if err := writeRiverReviewImage(path, terrain, view.rectangle, view.stride, view.zoom, view.rivers); err != nil {
					test.Fatal(err)
				}
			}
		}
		fmt.Fprintf(&report, "## Seed %d (%dx%d)\n\n- Main routes: %d -> %d\n- Drawn-water components touching main routes: %d -> %d (not a boat-clearance metric)\n- Branch stats: %v\n- Build duration: %s -> %s\n- Go reserved heap snapshot MiB: %d -> %d (same process; not OS peak RSS)\n- Final new fairway validation: passed for every retained route\n- Bend crop center: (%d,%d); fairway crop center: (%d,%d)\n\n", seed, current.WidthTiles, current.HeightTiles, routeCounts[0], routeCounts[1], componentCounts[0], componentCounts[1], riverRouteStats(current.RiverFairways), timings[0], timings[1], memory[0], memory[1], center%current.WidthTiles, center/current.WidthTiles, junction%current.WidthTiles, junction/current.WidthTiles)
		fmt.Fprintf(&report, "- Lakes with accepted main entrances: %d -> %d\n- Minimum tributary-junction distance to other junctions/inlets: %s\n\n", linkedLakeCounts[0], linkedLakeCounts[1], spacingLabel)
		for _, view := range []string{"map", "rivers", "bends", "fairway"} {
			fmt.Fprintf(&report, "- %s: [before](%d-before-%s.png) / [after](%d-after-%s.png)\n", view, seed, view, seed, view)
		}
		report.WriteString("\n")
		comparison = [2]*TerrainPrecompute{}
		runtime.GC()
	}
	if err := os.WriteFile(filepath.Join(*bendsReviewDirectory, "README.md"), []byte(report.String()), 0o644); err != nil {
		test.Fatal(err)
	}
}

func reviewMinimumJunctionSpacing(plan *riverFairways, width int) float64 {
	minimum := math.Inf(1)
	junctions := make([]int, 0, len(plan.Inlets)+plan.Tributaries)
	for _, inlet := range plan.Inlets {
		junctions = append(junctions, tileIndex(inlet.X, inlet.Y, width))
	}
	for _, route := range plan.Routes {
		if route.Role != "tributary" {
			continue
		}
		junction := route.Path[0]
		for _, other := range junctions {
			minimum = math.Min(minimum, math.Hypot(float64(junction%width-other%width), float64(junction/width-other/width)))
		}
		junctions = append(junctions, junction)
	}
	return minimum
}

func writeRiverReviewImage(path string, terrain *TerrainPrecompute, rectangle image.Rectangle, stride, zoom int, riversOnly bool) error {
	result := image.NewRGBA(image.Rect(0, 0, ((rectangle.Dx()+stride-1)/stride)*zoom, ((rectangle.Dy()+stride-1)/stride)*zoom))
	for row := rectangle.Min.Y; row < rectangle.Max.Y; row += stride {
		for column := rectangle.Min.X; column < rectangle.Max.X; column += stride {
			index := tileIndex(column, row, terrain.WidthTiles)
			class := riverNone
			for sampleRow := row; sampleRow < minInt(row+stride, rectangle.Max.Y); sampleRow++ {
				for sampleColumn := column; sampleColumn < minInt(column+stride, rectangle.Max.X); sampleColumn++ {
					candidate := terrain.RiverClass[tileIndex(sampleColumn, sampleRow, terrain.WidthTiles)]
					if candidate > class {
						class = candidate
					}
				}
			}
			paint := tileColor(terrain.Tiles[index], class, true)
			if riversOnly && class == riverNone {
				paint = color.RGBA{R: 6, G: 12, B: 8, A: 255}
			}
			if stride == 1 && class != riverNone {
				if terrain.Tiles[index] == tileWaterDeep {
					paint = color.RGBA{R: 25, G: 85, B: 205, A: 255}
				} else {
					paint = color.RGBA{R: 98, G: 210, B: 241, A: 255}
				}
			}
			drawScaledPixel(result, ((column-rectangle.Min.X)/stride)*zoom, ((row-rectangle.Min.Y)/stride)*zoom, zoom, paint)
		}
	}
	return writePNG(path, result)
}

func reviewMainComponents(terrain *TerrainPrecompute, routes []riverRoute) int {
	anchors := make(map[int]bool)
	for _, route := range routes {
		if len(route.Path) > 0 {
			anchors[route.Path[0]] = true
		}
	}
	visited := make([]bool, len(terrain.RiverClass))
	queue := []int{}
	components := 0
	for index := range terrain.RiverClass {
		if visited[index] || terrain.RiverClass[index] == riverNone {
			continue
		}
		visited[index] = true
		queue = append(queue[:0], index)
		hasRoute := false
		for position := 0; position < len(queue); position++ {
			current := queue[position]
			hasRoute = hasRoute || anchors[current]
			column, row := current%terrain.WidthTiles, current/terrain.WidthTiles
			for _, offset := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nextColumn, nextRow := column+offset[0], row+offset[1]
				if nextColumn < 0 || nextRow < 0 || nextColumn >= terrain.WidthTiles || nextRow >= terrain.HeightTiles {
					continue
				}
				next := tileIndex(nextColumn, nextRow, terrain.WidthTiles)
				if !visited[next] && terrain.RiverClass[next] != riverNone {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		if hasRoute {
			components++
		}
	}
	return components
}
