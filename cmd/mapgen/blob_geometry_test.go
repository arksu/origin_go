package main

import (
	"image"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func blobTestRandom(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

func TestBlobTreeScalesAndLimits(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		for _, limits := range [][2]int{{4, 3}, {256, 3}, {64, 16}} {
			nodes := genBlobTree(blobPoint{10.5, 20.5}, 12, limits[0], limits[1], blobTestRandom(seed))
			if len(nodes) < 4 || len(nodes) > limits[0] {
				t.Fatalf("node count %d", len(nodes))
			}
			for i, node := range nodes {
				if node.depth > limits[1] {
					t.Fatal("depth exceeded")
				}
				if i == 0 {
					continue
				}
				distance := node.point.sub(nodes[node.adjacent[0]].point).length()
				if distance < 10.8-1e-9 || distance > 13.2+1e-9 {
					t.Fatalf("branch length %g", distance)
				}
			}
			if !reflect.DeepEqual(nodes, genBlobTree(blobPoint{10.5, 20.5}, 12, limits[0], limits[1], blobTestRandom(seed))) {
				t.Fatal("nondeterministic tree")
			}
		}
	}
	if blobWidth(12, 0) != 6 || blobWidth(12, 3) != 3 || blobWidth(120, 3) != 3 {
		t.Fatal("width depends on wrong scale")
	}
}

func blobTestChain(points ...blobPoint) []blobNode {
	nodes := make([]blobNode, len(points))
	for i, point := range points {
		nodes[i].point = point
		nodes[i].depth = i
		if i > 0 {
			nodes[i].adjacent[0] = i - 1
			nodes[i].degree++
		}
		if i+1 < len(points) {
			nodes[i].adjacent[nodes[i].degree] = i + 1
			nodes[i].degree++
		}
	}
	return nodes
}

func TestBlobOutlineWidthAndCaps(t *testing.T) {
	nodes := blobTestChain(blobPoint{0, 0}, blobPoint{20, 0})
	outline := genBlobOutline(nodes, 4, blobTestRandom(1))
	if len(outline) != 6 {
		t.Fatalf("caps: %v", outline)
	}
	if outline[1].X >= 0 || outline[4].X <= 20 {
		t.Fatal("missing outer leaf caps")
	}
	for _, point := range []blobPoint{outline[0], outline[2], outline[3], outline[5]} {
		if math.Abs(point.Y) < 3.2 || math.Abs(point.Y) > 4.8 {
			t.Fatal("width not used for offset")
		}
	}
	wide := genBlobOutline(nodes, 8, blobTestRandom(1))
	if math.Abs(wide[0].Y-2*outline[0].Y) > 1e-9 {
		t.Fatal("independent width")
	}
}

func TestBlobSplineClosureAndRaggedness(t *testing.T) {
	for _, outline := range [][]blobPoint{
		{{0, 0}, {8, 0}, {8, 8}, {0, 8}}, {{1, 1}, {1, 1}, {1, 1}}, {{1, 1}, {1, 1}, {2, 2}, {1, 1}},
	} {
		smooth := blobSpline(outline, 3, 0, blobTestRandom(42))
		jagged := blobSpline(outline, 3, 6, blobTestRandom(42))
		if smooth[0] != smooth[len(smooth)-1] || jagged[0] != jagged[len(jagged)-1] {
			t.Fatal("open contour")
		}
		for i, point := range jagged {
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
				t.Fatal("nonfinite")
			}
			delta := point.sub(smooth[i])
			if math.Abs(delta.X) > 6 || math.Abs(delta.Y) > 6 {
				t.Fatal("raggedness exceeded")
			}
		}
		if !reflect.DeepEqual(smooth, blobSpline(outline, 3, 0, blobTestRandom(99))) {
			t.Fatal("zero jitter consumed randomness")
		}
	}
	outline := []blobPoint{{0, 0}, {6, 0}, {6, 6}, {0, 6}}
	if got := len(blobSpline(outline, 3, 0, blobTestRandom(1))); got != 17 {
		t.Fatalf("subdivisions: %d", got)
	}
}

func assertBlobConnected(t *testing.T, mask blobMask, seed image.Point) {
	t.Helper()
	if !mask.contains(seed) {
		t.Fatal("seed missing")
	}
	seen := map[image.Point]bool{seed: true}
	queue := []image.Point{seed}
	for head := 0; head < len(queue); head++ {
		for _, direction := range blobDirections {
			next := queue[head].Add(direction)
			if mask.contains(next) && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	total := 0
	mask.each(func(point image.Point) {
		total++
		if !point.In(mask.bounds) {
			t.Fatal("outside mask")
		}
	})
	if len(seen) != total {
		t.Fatalf("disconnected: %d of %d", len(seen), total)
	}
}

func TestBlobMaskConnectedCrossingThinAndWorldEdges(t *testing.T) {
	world := image.Rect(0, 0, 96, 96)
	scratch := blobScratch{}
	for _, points := range [][]blobPoint{
		{{48.5, 48.5}, {70.5, 70.5}, {30.5, 70.5}, {70.5, 30.5}, {30.5, 30.5}},
		{{48.5, 48.5}, {49.1, 49.1}, {49.2, 49.3}},
		{{48.5, 48.5}, {48.5, 48.5}},
		{{0.5, 0.5}, {-50, -20}, {20, 30}, {0, 90}},
		{{95.5, 95.5}, {150, 150}, {5, 80}},
		{{0.5, 95.5}, {-20, 120}, {20, 60}},
		{{95.5, 0.5}, {110, -20}, {60, 30}},
	} {
		nodes := blobTestChain(points...)
		for _, width := range []float64{0.001, 2, 8} {
			contour := blobSpline(genBlobOutline(nodes, width, blobTestRandom(1)), width, 6, blobTestRandom(2))
			mask := scratch.rasterize(nodes, contour, width, world)
			assertBlobConnected(t, mask, points[0].tile())
			if mask.bounds.Intersect(world) != mask.bounds {
				t.Fatal("outside world")
			}
		}
	}
	nodes := blobTestChain(blobPoint{10.5, 10.5}, blobPoint{25.5, 10.5}, blobPoint{40.5, 10.5}, blobPoint{55.5, 10.5})
	contour := blobSpline(genBlobOutline(nodes, 2, blobTestRandom(1)), 2, 0, blobTestRandom(1))
	mask := scratch.rasterize(nodes, contour, 2, world)
	if !mask.contains(image.Pt(55, 10)) {
		t.Fatal("shape normalized to radius")
	}
}

func TestBlobDeterminismIndependentStreamsAndReconstruction(t *testing.T) {
	build := func(column, row int) ([]blobPoint, blobMask) {
		rng := blobRandom(42, blobMainPass, column, row, blobGeometryPurpose)
		nodes := genBlobTree(blobPoint{100.5, 100.5}, 20, 64, 16, rng)
		contour := blobSpline(genBlobOutline(nodes, 10, rng), 10, 6, rng)
		scratch := blobScratch{}
		return contour, scratch.rasterize(nodes, contour, 10, image.Rect(0, 0, 200, 200))
	}
	contour, mask := build(2, 3)
	build(3, 2)
	blobRandom(42, blobMainPass, 2, 3, blobSelectionPurpose).Int63()
	reconstructed, copyMask := build(2, 3)
	if !reflect.DeepEqual(contour, reconstructed) || !reflect.DeepEqual(mask, copyMask) {
		t.Fatal("reconstruction differs")
	}
	values := map[int64]bool{}
	for _, pass := range []uint64{blobMainPass, blobSecondaryPass} {
		for _, purpose := range []uint64{blobPositionPurpose, blobSelectionPurpose, blobSizePurpose, blobGeometryPurpose, blobIsletPurpose} {
			value := blobRandom(42, pass, 2, 3, purpose).Int63()
			if values[value] {
				t.Fatal("stream collision")
			}
			values[value] = true
		}
	}
}
