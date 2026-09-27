package main

import (
	"bytes"
	"image"
	"reflect"
	"testing"
)

func blobTestPainter(width, height int) *blobPainter {
	tiles := bytes.Repeat([]byte{tileGrass}, width*height)
	return &blobPainter{tiles: tiles, locked: make([]bool, len(tiles)), world: image.Rect(0, 0, width, height), seed: 12345, opts: DefaultMapgenOptions().Biome, signals: func(int, int) BiomeSignals { return BiomeSignals{Temperature: 0.6, Moisture: 0.5, Wetness: 0.7} }}
}

func TestBiomeStructuralMaskProtectsUnresolvedWaterAndGround(t *testing.T) {
	ground := []byte{tileGrass, tileForestLeaf, tileGrass, tileGrass, tileMountain, tileStone, tileSand, tileGrass}
	elevation := []float32{0.2, 0.3, 0.7, 0.7, 0.7, 0.7, 0.7, 0.7}
	rivers := []RiverClass{riverNone, riverDeep, riverShallow, riverDeep, riverNone, riverNone, riverNone, riverNone}
	locked := buildBiomeStructuralMask(ground, elevation, rivers, true)
	if !reflect.DeepEqual(locked, []bool{true, true, true, true, true, true, true, false}) {
		t.Fatal(locked)
	}
	without := buildBiomeStructuralMask(ground, elevation, rivers, false)
	if without[2] || without[3] || !without[0] {
		t.Fatal(without)
	}
}

func TestBlobPlacementGridAndSuitability(t *testing.T) {
	world := image.Rect(0, 0, 23, 17)
	for seed := int64(0); seed < 100; seed++ {
		for row := 0; row < 2; row++ {
			for column := 0; column < 3; column++ {
				point := blobGridPoint(column, row, 10, 0.9999, world, blobTestRandom(seed))
				cell := image.Rect(column*10, row*10, min(23, (column+1)*10), min(17, (row+1)*10))
				if !point.In(cell) {
					t.Fatal(point, cell)
				}
			}
		}
	}
	if point := blobGridPoint(2, 1, 10, 0, world, blobTestRandom(1)); point != image.Pt(21, 13) {
		t.Fatal("partial center", point)
	}
	painter := blobTestPainter(23, 17)
	painter.opts.BlobSeedSpacing = 10
	painter.opts.BlobSeedJitter = 0
	painter.opts.BlobForestWeight = 1
	painter.opts.BlobHeathWeight = 0
	painter.opts.BlobMoorWeight = 0
	painter.opts.BlobSwampWeight = 0
	painter.opts.BlobSkipWeight = 0
	painter.locked[5*23+5] = true
	painter.tiles[5*23+15] = tileSand
	patches := painter.mainPatches()
	if len(patches) != 4 {
		t.Fatalf("unsuitable seeds relocated: %d", len(patches))
	}
	for _, patch := range patches {
		if patch.origin == image.Pt(5, 5) || patch.origin == image.Pt(15, 5) {
			t.Fatal("unsuitable seed")
		}
	}
	painter.opts.BlobForestWeight = 0
	painter.opts.BlobSkipWeight = 1
	before := append([]byte(nil), painter.tiles...)
	painter.paintMain(painter.mainPatches())
	if !bytes.Equal(before, painter.tiles) {
		t.Fatal("skip erased terrain")
	}
}

func TestBlobPlacementClimateThresholdsAndZeroWeights(t *testing.T) {
	opts := DefaultMapgenOptions().Biome
	opts.BlobHeathWeight = 0
	opts.BlobMoorWeight = 0
	opts.BlobSwampWeight = 0
	opts.BlobSkipWeight = 0
	signals := BiomeSignals{Moisture: 0.5, Temperature: opts.BlobForestColdThreshold}
	if selectMainBlob(signals, opts, 0.5) != tileForestLeaf {
		t.Fatal("forest equality")
	}
	signals.Temperature -= 0.001
	if selectMainBlob(signals, opts, 0.5) != tileForestPine {
		t.Fatal("cold forest")
	}
	opts.BlobForestWeight = 0
	opts.BlobHeathWeight = 1
	opts.BlobMoorWeight = 1
	signals.Moisture = opts.BlobMoorMoistureMax
	signals.Temperature = opts.BlobMoorTemperatureMax
	if selectMainBlob(signals, opts, 0.5) != tileHeath {
		t.Fatal("moor equality")
	}
	signals.Moisture -= 0.001
	if selectMainBlob(signals, opts, 0.5) != tileMoor {
		t.Fatal("dry moor")
	}
	signals.Moisture = 1
	signals.Temperature -= 0.001
	if selectMainBlob(signals, opts, 0.5) != tileMoor {
		t.Fatal("cold moor")
	}
	opts.BlobHeathWeight = 0
	opts.BlobMoorWeight = 0
	opts.BlobSwampWeight = 1
	signals.Wetness = opts.BlobSwampWetnessMin
	signals.Moisture = opts.BlobSwampMoistureMin
	if selectMainBlob(signals, opts, 0.5) != 0 {
		t.Fatal("swamp threshold equality")
	}
	signals.Wetness += 0.001
	if selectMainBlob(signals, opts, 0.5) != tileSwamp {
		t.Fatal("wet swamp")
	}
	signals.Wetness = 0
	signals.Moisture += 0.001
	if selectMainBlob(signals, opts, 0.5) != tileSwamp {
		t.Fatal("moist swamp")
	}
	opts.BlobSwampWeight = 0
	if selectMainBlob(signals, opts, 0) != 0 {
		t.Fatal("zero weights")
	}
}

func TestBlobMainPaintingPrecedenceTieAndStructuralFragments(t *testing.T) {
	painter := blobTestPainter(101, 101)
	patch := blobPatch{origin: image.Pt(50, 50), size: 14, width: 9, tile: tileSwamp}
	forest, heath := patch, patch
	forest.tile = tileForestPine
	heath.tile = tileHeath
	for _, patches := range [][]blobPatch{{patch, forest, heath}, {forest, heath, patch}} {
		painter.tiles = bytes.Repeat([]byte{tileGrass}, 101*101)
		painter.paintMain(patches)
		if painter.tiles[50*101+50] != tileSwamp {
			t.Fatal("swamp must win")
		}
	}
	forest.tile = tileForestLeaf
	painter.paintMain([]blobPatch{{origin: patch.origin, size: 14, width: 9, tile: tileForestPine}, forest})
	if painter.tiles[50*101+50] != tileForestLeaf {
		t.Fatal("equal priority order")
	}
	painter.tiles = bytes.Repeat([]byte{tileGrass}, 101*101)
	for row := 0; row < 101; row++ {
		painter.locked[row*101+50] = true
		painter.tiles[row*101+50] = tileMountain
	}
	mask, _ := painter.geometry(patch, blobMainPass)
	assertBlobConnected(t, mask, patch.origin)
	painter.paintMain([]blobPatch{patch})
	left, right := 0, 0
	for row := 0; row < 101; row++ {
		for column := 0; column < 101; column++ {
			tile := painter.tiles[row*101+column]
			if column == 50 && tile != tileMountain {
				t.Fatal("structural wall overwritten")
			}
			if tile == tileSwamp {
				if column < 50 {
					left++
				}
				if column > 50 {
					right++
				}
			}
		}
	}
	if left == 0 || right == 0 {
		t.Fatal("expected separate fragments")
	}
}

func TestBiomeCleanupLocksVotesAndZeroPasses(t *testing.T) {
	routines := []struct {
		name string
		run  func([]byte, []bool, int, int, int)
	}{
		{"tiles", smoothBiomeTiles}, {"edges", smoothBiomeEdges}, {"border", cleanBiomeBorderArtifacts}, {"tiny", removeTinyBiomePatches},
	}
	for _, routine := range routines {
		t.Run(routine.name, func(t *testing.T) {
			tiles := bytes.Repeat([]byte{tileForestLeaf}, 25)
			locked := make([]bool, 25)
			for i := range locked {
				locked[i] = true
			}
			locked[12] = false
			tiles[12] = tileHeath
			before := append([]byte(nil), tiles...)
			routine.run(tiles, locked, 5, 5, 24)
			if !bytes.Equal(before, tiles) {
				t.Fatal("unresolved water influenced cleanup")
			}
			for _, structural := range []byte{tileMountain, tileStone, tileSand, tileWater, tileWaterDeep} {
				tiles = bytes.Repeat([]byte{tileForestLeaf}, 25)
				tiles[12] = structural
				locked = make([]bool, 25)
				locked[12] = true
				routine.run(tiles, locked, 5, 5, 24)
				if tiles[12] != structural {
					t.Fatal("protected tile erased")
				}
			}
		})
	}
	tiles := bytes.Repeat([]byte{tileForestLeaf}, 25)
	tiles[12] = tileHeath
	locked := make([]bool, 25)
	opts := DefaultMapgenOptions().Biome
	opts.SmoothingPasses = 0
	opts.MinPatchTiles = 1
	before := append([]byte(nil), tiles...)
	cleanupBlobBiomes(tiles, locked, 5, 5, opts)
	if !bytes.Equal(before, tiles) {
		t.Fatal("zero cleanup must preserve spike")
	}
	opts.MinPatchTiles = 2
	cleanupBlobBiomes(tiles, locked, 5, 5, opts)
	if tiles[12] != tileForestLeaf {
		t.Fatal("tiny component not removed")
	}
}

func TestBlobSecondaryProbabilitiesAndEligibility(t *testing.T) {
	opts := DefaultMapgenOptions().Biome
	opts.BlobDirtDensity = 0.2
	opts.BlobClayDensity = 0.3
	dry := BiomeSignals{Wetness: opts.BlobClayWetnessMin, Moisture: opts.BlobClayMoistureMin}
	wet := dry
	wet.Wetness += 0.001
	for _, signals := range []BiomeSignals{dry, wet} {
		if selectSecondaryBlob(tileGrass, signals, opts, 0.19999) != tileDirt {
			t.Fatal("dirt probability changed")
		}
	}
	if selectSecondaryBlob(tileGrass, dry, opts, 0.2) != 0 || selectSecondaryBlob(tileGrass, wet, opts, 0.2) != tileClay || selectSecondaryBlob(tileGrass, wet, opts, 0.5) != 0 {
		t.Fatal("fixed clay interval")
	}
	opts.BlobThicketDensity = 0
	opts.BlobDirtDensity = 0
	opts.BlobClayDensity = 0
	for _, tile := range []byte{tileForestPine, tileForestLeaf, tileGrass} {
		if selectSecondaryBlob(tile, wet, opts, 0) != 0 {
			t.Fatal("zero density")
		}
	}
	painter := blobTestPainter(5, 1)
	painter.tiles = []byte{tileForestPine, tileForestLeaf, tileGrass, tileSwamp, tileDirt}
	mask := blobMask{painter.world, bytes.Repeat([]byte{1}, 5)}
	painter.paintMask(mask, tileThicket, true)
	if !bytes.Equal(painter.tiles, []byte{tileThicket, tileThicket, tileGrass, tileSwamp, tileDirt}) {
		t.Fatal(painter.tiles)
	}
	painter.paintMask(mask, tileClay, true)
	if !bytes.Equal(painter.tiles, []byte{tileThicket, tileThicket, tileClay, tileSwamp, tileDirt}) {
		t.Fatal("secondary overwrite", painter.tiles)
	}
	painter = blobTestPainter(20, 20)
	painter.opts.BlobSecondarySpacing = 20
	painter.opts.BlobSeedJitter = 0
	painter.opts.BlobDirtDensity = 1
	painter.opts.BlobClayDensity = 0
	painter.locked[10*20+10] = true
	if painter.paintSecondary() != 0 {
		t.Fatal("locked secondary seed relocated")
	}
	painter.locked[10*20+10] = false
	painter.tiles[10*20+10] = tileClay
	if painter.paintSecondary() != 0 {
		t.Fatal("occupied secondary seed relocated")
	}
}

func TestBlobIsletsGrowthAcceptanceAndSpacing(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		for size := 1; size <= 15; size++ {
			cluster := growBlobIslet(image.Pt(30, 30), size, blobTestRandom(seed))
			if len(cluster) != size {
				t.Fatalf("growth incomplete for seed %d size %d", seed, size)
			}
			cells := make(map[image.Point]bool)
			for _, point := range cluster {
				if cells[point] {
					t.Fatal("duplicate")
				}
				cells[point] = true
			}
			seen := map[image.Point]bool{cluster[0]: true}
			queue := cluster[:1:1]
			for head := 0; head < len(queue); head++ {
				for _, direction := range blobDirections {
					next := queue[head].Add(direction)
					if cells[next] && !seen[next] {
						seen[next] = true
						queue = append(queue, next)
					}
				}
			}
			if len(seen) != size {
				t.Fatal("disconnected islet")
			}
		}
	}
	painter := blobTestPainter(40, 40)
	parent := blobMask{image.Rect(10, 10, 20, 20), bytes.Repeat([]byte{1}, 100)}
	parent.each(func(point image.Point) { painter.tiles[point.Y*40+point.X] = tileForestLeaf })
	for _, point := range []image.Point{{20, 15}, {19, 15}, {40, 15}, {-1, 15}} {
		if painter.acceptIslet([]image.Point{point}, parent, tileForestLeaf) {
			t.Fatal("accepted invalid islet", point)
		}
	}
	candidate := []image.Point{{21, 15}, {22, 15}}
	if !painter.acceptIslet(candidate, parent, tileForestLeaf) {
		t.Fatal("one tile separation rejected")
	}
	painter.locked[15*40+22] = true
	if painter.acceptIslet(candidate, parent, tileForestLeaf) {
		t.Fatal("locked candidate")
	}
	painter.locked[15*40+22] = false
	painter.tiles[15*40+22] = tileDirt
	if painter.acceptIslet(candidate, parent, tileForestLeaf) {
		t.Fatal("occupied candidate")
	}
	painter.opts.BlobIsletChance = 0
	if count, tiles := painter.paintIslets([]blobPatch{{origin: image.Pt(15, 15), tile: tileForestLeaf, size: 8, width: 4}}); count != 0 || tiles != 0 {
		t.Fatal("chance zero")
	}
	samples := func(contour []blobPoint) []blobPoint {
		var points []blobPoint
		sampleBlobPerimeter(contour, 7, func(point, normal blobPoint) { points = append(points, point) })
		return points
	}
	simple := samples([]blobPoint{{0, 0}, {20, 0}, {20, 20}, {0, 20}, {0, 0}})
	divided := samples([]blobPoint{{0, 0}, {10, 0}, {20, 0}, {20, 10}, {20, 20}, {10, 20}, {0, 20}, {0, 10}, {0, 0}})
	if !reflect.DeepEqual(simple, divided) {
		t.Fatal("subdivision changed perimeter frequency")
	}
}

func TestBlobPipelineSmallDetailSurvivesCleanup(t *testing.T) {
	painter := blobTestPainter(240, 240)
	painter.opts.SmoothingPasses = 3
	painter.opts.MinPatchTiles = 24
	painter.opts.BlobRaggedness = 0
	painter.opts.BlobIsletChance = 1
	patch := blobPatch{origin: image.Pt(120, 120), tile: tileForestLeaf, size: 25, width: 12}
	painter.paintMain([]blobPatch{patch})
	cleanupBlobBiomes(painter.tiles, painter.locked, 240, 240, painter.opts)
	painter.opts.BlobSecondarySpacing = 20
	painter.opts.BlobDirtDensity = 1
	painter.opts.BlobClayDensity = 0
	painter.opts.BlobThicketDensity = 1
	painter.opts.BlobDirtSizeMin = 1
	painter.opts.BlobDirtSizeMax = 1
	painter.opts.BlobDirtWidth = 0.1
	painter.opts.BlobThicketSizeMin = 1
	painter.opts.BlobThicketSizeMax = 1
	painter.opts.BlobThicketWidth = 0.1
	if painter.paintSecondary() == 0 {
		t.Fatal("no secondary detail")
	}
	before := append([]byte(nil), painter.tiles...)
	count, tiles := painter.paintIslets([]blobPatch{patch})
	if count == 0 || tiles < count || tiles > 15*count {
		t.Fatal("no valid satellites", count, tiles)
	}
	newlyPainted := 0
	for i, tile := range painter.tiles {
		if tile != before[i] {
			if before[i] != tileGrass || tile != tileForestLeaf {
				t.Fatal("invalid satellite overwrite")
			}
			newlyPainted++
		}
		if before[i] == tileDirt || before[i] == tileThicket {
			if tile != before[i] {
				t.Fatal("detail erased")
			}
		}
	}
	if newlyPainted != tiles {
		t.Fatal("partial candidate painting")
	}
	// Final hydrology remains authoritative even for an intentional single tile.
	if resolveTileType(0.7, tileForestLeaf, riverDeep, true) != tileWaterDeep {
		t.Fatal("hydrology lost precedence")
	}
}
