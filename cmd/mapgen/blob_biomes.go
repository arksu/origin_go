package main

import (
	"image"
	"math/rand"
)

func buildBiomeStructuralMask(ground []byte, elevation []float32, rivers []RiverClass, riverEnabled bool) []bool {
	locked := make([]bool, len(ground))
	for i, tile := range ground {
		river := riverNone
		if riverEnabled && len(rivers) == len(ground) {
			river = rivers[i]
		}
		resolved := resolveTileType(float64(elevation[i]), tile, river, riverEnabled)
		locked[i] = resolved == tileWater || resolved == tileWaterDeep || tile == tileMountain || tile == tileStone || tile == tileSand
	}
	return locked
}

type blobPatch struct {
	column, row int
	origin      image.Point
	tile        byte
	size, width float64
}

type blobPainter struct {
	tiles   []byte
	locked  []bool
	world   image.Rectangle
	seed    int64
	opts    BiomeOptions
	signals func(int, int) BiomeSignals
	scratch blobScratch
}

func blobGridPoint(column, row, spacing int, jitter float64, world image.Rectangle, rng *rand.Rand) image.Point {
	left, top := column*spacing, row*spacing
	cellWidth, cellHeight := min(spacing, world.Max.X-left), min(spacing, world.Max.Y-top)
	columnPosition := float64(left) + float64(cellWidth)*0.5*(1+jitter*(2*rng.Float64()-1))
	rowPosition := float64(top) + float64(cellHeight)*0.5*(1+jitter*(2*rng.Float64()-1))
	return image.Pt(max(left, min(left+cellWidth-1, int(columnPosition))), max(top, min(top+cellHeight-1, int(rowPosition))))
}

func (p *blobPainter) grid(pass uint64, spacing int, visit func(int, int, image.Point)) {
	for row := 0; row < (p.world.Dy()-1)/spacing+1; row++ {
		for column := 0; column < (p.world.Dx()-1)/spacing+1; column++ {
			origin := blobGridPoint(column, row, spacing, p.opts.BlobSeedJitter, p.world, blobRandom(p.seed, pass, column, row, blobPositionPurpose))
			visit(column, row, origin)
		}
	}
}

func selectMainBlob(signals BiomeSignals, opts BiomeOptions, draw float64) byte {
	forest := byte(tileForestLeaf)
	if signals.Temperature < opts.BlobForestColdThreshold {
		forest = tileForestPine
	}
	weights := [5]float64{opts.BlobForestWeight * (0.5 + signals.Moisture), 0, 0, 0, opts.BlobSkipWeight}
	if signals.Moisture < opts.BlobMoorMoistureMax || signals.Temperature < opts.BlobMoorTemperatureMax {
		weights[2] = opts.BlobMoorWeight * (1.5 - signals.Moisture)
	} else {
		weights[1] = opts.BlobHeathWeight * (1.5 - signals.Moisture)
	}
	if signals.Wetness > opts.BlobSwampWetnessMin || signals.Moisture > opts.BlobSwampMoistureMin {
		weights[3] = opts.BlobSwampWeight * (0.5 + signals.Wetness)
	}
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	remaining := draw * total
	types := [5]byte{forest, tileHeath, tileMoor, tileSwamp, 0}
	for i, weight := range weights {
		if remaining < weight {
			return types[i]
		}
		remaining -= weight
	}
	return 0
}

func blobPriority(tile byte) int {
	switch tile {
	case tileForestPine, tileForestLeaf:
		return 1
	case tileHeath, tileMoor:
		return 2
	case tileSwamp:
		return 3
	}
	return 0
}

func (p *blobPainter) newPatch(pass uint64, column, row int, origin image.Point, tile byte) blobPatch {
	shapeIndex := 0
	switch tile {
	case tileHeath:
		shapeIndex = 1
	case tileMoor:
		shapeIndex = 2
	case tileSwamp:
		shapeIndex = 3
	case tileThicket:
		shapeIndex = 4
	case tileDirt:
		shapeIndex = 5
	case tileClay:
		shapeIndex = 6
	}
	shape := p.opts.blobShapes()[shapeIndex]
	size := shape.minSize + blobRandom(p.seed, pass, column, row, blobSizePurpose).Float64()*(shape.maxSize-shape.minSize)
	return blobPatch{column: column, row: row, origin: origin, tile: tile, size: size, width: blobWidth(size, shape.width)}
}

func (p *blobPainter) mainPatches() []blobPatch {
	capacity := ((p.world.Dx()-1)/p.opts.BlobSeedSpacing + 1) * ((p.world.Dy()-1)/p.opts.BlobSeedSpacing + 1)
	patches := make([]blobPatch, 0, capacity)
	// Complete selection before painting so earlier patches cannot change seed suitability.
	p.grid(blobMainPass, p.opts.BlobSeedSpacing, func(column, row int, origin image.Point) {
		index := origin.Y*p.world.Dx() + origin.X
		if p.locked[index] || p.tiles[index] != tileGrass {
			return
		}
		tile := selectMainBlob(p.signals(origin.X, origin.Y), p.opts, blobRandom(p.seed, blobMainPass, column, row, blobSelectionPurpose).Float64())
		if tile != 0 {
			patches = append(patches, p.newPatch(blobMainPass, column, row, origin, tile))
		}
	})
	return patches
}

func (p *blobPainter) geometry(patch blobPatch, pass uint64) (blobMask, []blobPoint) {
	rng := blobRandom(p.seed, pass, patch.column, patch.row, blobGeometryPurpose)
	root := blobPoint{float64(patch.origin.X) + 0.5, float64(patch.origin.Y) + 0.5}
	nodes := genBlobTree(root, patch.size, p.opts.BlobMaxNodes, p.opts.BlobMaxDepth, rng)
	contour := blobSpline(genBlobOutline(nodes, patch.width, rng), patch.width, p.opts.BlobRaggedness, rng)
	return p.scratch.rasterize(nodes, contour, patch.width, p.world), contour
}

func (p *blobPainter) paintMain(patches []blobPatch) {
	for priority := 1; priority <= 3; priority++ {
		for _, patch := range patches {
			if blobPriority(patch.tile) != priority {
				continue
			}
			mask, _ := p.geometry(patch, blobMainPass)
			p.paintMask(mask, patch.tile, false)
		}
	}
}

func (p *blobPainter) paintMask(mask blobMask, tile byte, secondary bool) {
	mask.each(func(point image.Point) {
		index := point.Y*p.world.Dx() + point.X
		if p.locked[index] {
			return
		}
		current := p.tiles[index]
		if secondary {
			if tile == tileThicket {
				if current != tileForestPine && current != tileForestLeaf {
					return
				}
			} else if current != tileGrass {
				return
			}
		}
		p.tiles[index] = tile
	})
}

func selectSecondaryBlob(current byte, signals BiomeSignals, opts BiomeOptions, draw float64) byte {
	switch current {
	case tileForestPine, tileForestLeaf:
		if draw < opts.BlobThicketDensity {
			return tileThicket
		}
	case tileGrass:
		if draw < opts.BlobDirtDensity {
			return tileDirt
		}
		if draw < opts.BlobDirtDensity+opts.BlobClayDensity && (signals.Wetness > opts.BlobClayWetnessMin || signals.Moisture > opts.BlobClayMoistureMin) {
			return tileClay
		}
	}
	return 0
}

func (p *blobPainter) paintSecondary() int {
	count := 0
	p.grid(blobSecondaryPass, p.opts.BlobSecondarySpacing, func(column, row int, origin image.Point) {
		index := origin.Y*p.world.Dx() + origin.X
		if p.locked[index] {
			return
		}
		tile := selectSecondaryBlob(p.tiles[index], p.signals(origin.X, origin.Y), p.opts, blobRandom(p.seed, blobSecondaryPass, column, row, blobSelectionPurpose).Float64())
		if tile == 0 {
			return
		}
		patch := p.newPatch(blobSecondaryPass, column, row, origin, tile)
		mask, _ := p.geometry(patch, blobSecondaryPass)
		p.paintMask(mask, tile, true)
		count++
	})
	return count
}

// Perimeter samples use arc length, so spline subdivision does not change frequency.
func sampleBlobPerimeter(contour []blobPoint, spacing float64, visit func(blobPoint, blobPoint)) {
	untilNext := spacing
	for i := 1; i < len(contour); i++ {
		start, end := contour[i-1], contour[i]
		delta := end.sub(start)
		length := delta.length()
		if length == 0 {
			continue
		}
		direction := delta.scale(1 / length)
		for untilNext <= length {
			visit(start.add(direction.scale(untilNext)), blobPoint{-direction.Y, direction.X})
			untilNext += spacing
		}
		untilNext -= length
	}
}

func growBlobIslet(origin image.Point, size int, rng *rand.Rand) []image.Point {
	cluster := make([]image.Point, 1, 15)
	cluster[0] = origin
	frontier := make([]image.Point, 1, 61)
	frontier[0] = origin
	for proposals := 0; len(cluster) < size && len(frontier) > 0 && proposals < 64; proposals++ {
		frontierIndex := rng.Intn(len(frontier))
		point := frontier[frontierIndex]
		available := [4]image.Point{}
		availableCount := 0
		for _, direction := range blobDirections {
			neighbor := point.Add(direction)
			occupied := false
			for _, existing := range cluster {
				if neighbor == existing {
					occupied = true
					break
				}
			}
			if !occupied {
				available[availableCount] = neighbor
				availableCount++
			}
		}
		if availableCount == 0 {
			frontier[frontierIndex] = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
			continue
		}
		next := available[rng.Intn(availableCount)]
		cluster = append(cluster, next)
		frontier = append(frontier, next)
	}
	if len(cluster) != size {
		return nil
	}
	return cluster
}

func (p *blobPainter) acceptIslet(cluster []image.Point, parent blobMask, tile byte) bool {
	if len(cluster) == 0 {
		return false
	}
	for _, point := range cluster {
		if !point.In(p.world) || parent.contains(point) {
			return false
		}
		index := point.Y*p.world.Dx() + point.X
		if p.locked[index] || p.tiles[index] != tileGrass {
			return false
		}
		for offsetY := -1; offsetY <= 1; offsetY++ {
			for offsetX := -1; offsetX <= 1; offsetX++ {
				neighbor := point.Add(image.Pt(offsetX, offsetY))
				if neighbor.In(p.world) && p.tiles[neighbor.Y*p.world.Dx()+neighbor.X] == tile {
					return false
				}
			}
		}
	}
	return true
}

func (p *blobPainter) exposedParent(point blobPoint, mask blobMask, tile byte) bool {
	center := point.tile()
	for offsetY := -1; offsetY <= 1; offsetY++ {
		for offsetX := -1; offsetX <= 1; offsetX++ {
			neighbor := center.Add(image.Pt(offsetX, offsetY))
			if !mask.contains(neighbor) {
				continue
			}
			index := neighbor.Y*p.world.Dx() + neighbor.X
			if !p.locked[index] && p.tiles[index] == tile {
				return true
			}
		}
	}
	return false
}

func (p *blobPainter) paintIslets(patches []blobPatch) (int, int) {
	if p.opts.BlobIsletChance == 0 {
		return 0, 0
	}
	count, tiles := 0, 0
	for _, patch := range patches {
		mask, contour := p.geometry(patch, blobMainPass)
		rng := blobRandom(p.seed, blobMainPass, patch.column, patch.row, blobIsletPurpose)
		sampleBlobPerimeter(contour, float64(p.opts.BlobIsletSpacing), func(point, normal blobPoint) {
			if rng.Float64() >= p.opts.BlobIsletChance || !p.exposedParent(point, mask, patch.tile) {
				return
			}
			distance := 5 + 2*rng.Float64()
			first := normal.scale(distance)
			candidate := point.add(first).tile()
			if mask.contains(candidate) {
				candidate = point.sub(first).tile()
				if mask.contains(candidate) {
					return
				}
			}
			cluster := growBlobIslet(candidate, 1+rng.Intn(15), rng)
			if !p.acceptIslet(cluster, mask, patch.tile) {
				return
			}
			for _, tile := range cluster {
				p.tiles[tile.Y*p.world.Dx()+tile.X] = patch.tile
			}
			count++
			tiles += len(cluster)
		})
	}
	return count, tiles
}
