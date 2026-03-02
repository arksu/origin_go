package main

import "math"

type BiomeSignals struct {
	Temperature     float64
	Moisture        float64
	Continentalness float64
	Erosion         float64
	Weirdness       float64
	Ruggedness      float64
	Wetness         float64
}

type biomeFamily uint8

const (
	biomeFamilyForest biomeFamily = iota
	biomeFamilyGrassland
	biomeFamilyWetland
	biomeFamilyHeathMoor
	biomeFamilyMountain
)

const (
	biomeMacroShuffleSalt = uint64(0xA77B159C2D4E5F11)
	biomeBlendSalt        = uint64(0xC984E2A58A77DD21)
	biomeVariantSalt      = uint64(0xDEADBEEF1248AA55)
	biomeEdgeNoiseSalt    = uint64(0x1B6F2D7E89AC55D3)
)

type biomeMacroLayout struct {
	gridX      int
	gridY      int
	cellWidth  int
	cellHeight int
	blendWidth float64
	points     []biomeMacroPoint
}

type biomeMacroPoint struct {
	X      float64
	Y      float64
	Family biomeFamily
}

func buildBiomeMacroLayout(width, height int, seed int64, opts BiomeOptions) biomeMacroLayout {
	if !opts.Enabled || opts.RegionCount <= 1 {
		return biomeMacroLayout{
			gridX:      1,
			gridY:      1,
			cellWidth:  maxInt(1, width),
			cellHeight: maxInt(1, height),
			blendWidth: 0,
			points: []biomeMacroPoint{
				{
					X:      float64(width) * 0.5,
					Y:      float64(height) * 0.5,
					Family: biomeFamilyGrassland,
				},
			},
		}
	}

	aspect := float64(width) / float64(maxInt(1, height))
	gridX := int(math.Round(math.Sqrt(float64(opts.RegionCount) * aspect)))
	gridX = maxInt(1, gridX)
	gridY := int(math.Ceil(float64(opts.RegionCount) / float64(gridX)))
	gridY = maxInt(1, gridY)

	cellWidth := int(math.Ceil(float64(width) / float64(gridX)))
	cellHeight := int(math.Ceil(float64(height) / float64(gridY)))
	cellWidth = maxInt(1, cellWidth)
	cellHeight = maxInt(1, cellHeight)

	cellCount := gridX * gridY
	families := assignMacroFamilies(cellCount, seed, opts)

	points := make([]biomeMacroPoint, 0, cellCount)
	jitterXMax := minInt(opts.RegionJitter, maxInt(0, cellWidth/2-2))
	jitterYMax := minInt(opts.RegionJitter, maxInt(0, cellHeight/2-2))
	for gy := 0; gy < gridY; gy++ {
		for gx := 0; gx < gridX; gx++ {
			idx := gy*gridX + gx

			baseX := float64(gx*cellWidth) + float64(cellWidth)*0.5
			baseY := float64(gy*cellHeight) + float64(cellHeight)*0.5

			jitterX := 0.0
			jitterY := 0.0
			if jitterXMax > 0 {
				jitterX = (coordHash01(seed, gx, gy, biomeBlendSalt^0x1111) - 0.5) * 2 * float64(jitterXMax)
			}
			if jitterYMax > 0 {
				jitterY = (coordHash01(seed, gy, gx, biomeBlendSalt^0x2222) - 0.5) * 2 * float64(jitterYMax)
			}

			pointX := clampFloat(baseX+jitterX, 0, float64(maxInt(0, width-1)))
			pointY := clampFloat(baseY+jitterY, 0, float64(maxInt(0, height-1)))
			points = append(points, biomeMacroPoint{
				X:      pointX,
				Y:      pointY,
				Family: families[idx],
			})
		}
	}

	return biomeMacroLayout{
		gridX:      gridX,
		gridY:      gridY,
		cellWidth:  cellWidth,
		cellHeight: cellHeight,
		blendWidth: float64(maxInt(0, opts.BlendWidth)),
		points:     points,
	}
}

func assignMacroFamilies(cellCount int, seed int64, opts BiomeOptions) []biomeFamily {
	if cellCount <= 0 {
		return []biomeFamily{biomeFamilyGrassland}
	}

	forest := int(math.Floor(opts.ForestShare * float64(cellCount)))
	grass := int(math.Floor(opts.GrasslandShare * float64(cellCount)))
	wetland := int(math.Floor(opts.WetlandShare * float64(cellCount)))
	heathMoor := int(math.Floor(opts.HeathMoorShare * float64(cellCount)))
	mountain := int(math.Floor(opts.MountainShare * float64(cellCount)))

	assigned := forest + grass + wetland + heathMoor + mountain
	if assigned > cellCount {
		assigned = cellCount
	}
	grass += cellCount - assigned

	families := make([]biomeFamily, 0, cellCount)
	appendN := func(family biomeFamily, n int) {
		for i := 0; i < n; i++ {
			families = append(families, family)
		}
	}
	appendN(biomeFamilyForest, forest)
	appendN(biomeFamilyGrassland, grass)
	appendN(biomeFamilyWetland, wetland)
	appendN(biomeFamilyHeathMoor, heathMoor)
	appendN(biomeFamilyMountain, mountain)
	for len(families) < cellCount {
		families = append(families, biomeFamilyGrassland)
	}
	for i := len(families) - 1; i > 0; i-- {
		r := coordHash01(seed, i, len(families), biomeMacroShuffleSalt)
		j := int(math.Floor(r * float64(i+1)))
		families[i], families[j] = families[j], families[i]
	}
	return families
}

func (m biomeMacroLayout) familyAt(x, y int, seed int64, jitter int) biomeFamily {
	if len(m.points) == 0 {
		return biomeFamilyGrassland
	}

	centerGX := minInt(maxInt(0, x/m.cellWidth), m.gridX-1)
	centerGY := minInt(maxInt(0, y/m.cellHeight), m.gridY-1)

	nearestDist2 := math.MaxFloat64
	secondDist2 := math.MaxFloat64
	nearestFamily := biomeFamilyGrassland
	secondFamily := biomeFamilyGrassland

	// Points are stored per grid cell, so 3x3 neighbor lookup gives near-Voronoi
	// regions without expensive all-point scans.
	for gy := centerGY - 1; gy <= centerGY+1; gy++ {
		if gy < 0 || gy >= m.gridY {
			continue
		}
		for gx := centerGX - 1; gx <= centerGX+1; gx++ {
			if gx < 0 || gx >= m.gridX {
				continue
			}
			idx := gy*m.gridX + gx
			if idx < 0 || idx >= len(m.points) {
				continue
			}
			point := m.points[idx]
			dx := float64(x) - point.X
			dy := float64(y) - point.Y
			dist2 := dx*dx + dy*dy
			if dist2 < nearestDist2 {
				secondDist2 = nearestDist2
				secondFamily = nearestFamily
				nearestDist2 = dist2
				nearestFamily = point.Family
			} else if dist2 < secondDist2 {
				secondDist2 = dist2
				secondFamily = point.Family
			}
		}
	}

	if nearestFamily == secondFamily || m.blendWidth <= 0 {
		return nearestFamily
	}

	nearestDist := math.Sqrt(nearestDist2)
	secondDist := math.Sqrt(secondDist2)
	if secondDist <= nearestDist {
		return nearestFamily
	}

	// Approximate distance to Voronoi boundary between nearest and second site.
	boundaryDistance := (secondDist - nearestDist) * 0.5
	blendWidth := m.blendWidth
	if blendWidth == 0 || boundaryDistance >= blendWidth {
		return nearestFamily
	}

	noiseScale := maxFloat64(12.0, blendWidth*0.65)
	if jitter > 0 {
		noiseScale = maxFloat64(8.0, noiseScale-float64(jitter)*0.08)
	}
	coherentNoise := smoothHashNoise2D(seed, float64(x), float64(y), noiseScale, biomeEdgeNoiseSalt)
	offset := (coherentNoise - 0.5) * blendWidth * 0.9
	if nearestDist+offset > secondDist {
		return secondFamily
	}
	return nearestFamily
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func smoothHashNoise2D(seed int64, x, y, scale float64, salt uint64) float64 {
	if scale <= 0 {
		scale = 1
	}
	sx := x / scale
	sy := y / scale

	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	x1 := x0 + 1
	y1 := y0 + 1

	tx := sx - float64(x0)
	ty := sy - float64(y0)
	ux := fade(tx)
	uy := fade(ty)

	n00 := coordHash01(seed, x0, y0, salt)
	n10 := coordHash01(seed, x1, y0, salt)
	n01 := coordHash01(seed, x0, y1, salt)
	n11 := coordHash01(seed, x1, y1, salt)

	row0 := lerp(ux, n00, n10)
	row1 := lerp(ux, n01, n11)
	return lerp(uy, row0, row1)
}

func classifyBaseTileFromBiome(
	elevation float64,
	signals BiomeSignals,
	family biomeFamily,
	opts BiomeOptions,
	seed int64,
	x int,
	y int,
) byte {
	// Inland shoreline sand is handled in a dedicated post-pass so lakes/rivers
	// do not get a hard sand ring from elevation alone.
	if signals.Moisture < 0.22 && signals.Temperature > 0.62 && signals.Continentalness > 0.45 {
		return tileSand
	}
	if opts.HNHEnabled && signals.Wetness > 0.68 && coordHash01(seed, x, y, biomeVariantSalt^0x01) < opts.VariantDensity*0.18 {
		return tileClay
	}

	if !opts.Enabled {
		return classifyBaseTile(elevation, signals.Moisture, signals.Temperature)
	}

	if signals.Ruggedness >= opts.MountainRuggedThreshold {
		if coordHash01(seed, x, y, biomeVariantSalt^0x02) < 0.2 {
			return tileStone
		}
		return tileMountain
	}

	switch family {
	case biomeFamilyForest:
		if opts.HNHEnabled && coordHash01(seed, x, y, biomeVariantSalt^0x03) < opts.VariantDensity*0.18 && signals.Moisture > 0.62 {
			return tileThicket
		}
		if signals.Temperature < 0.45 {
			return tileForestPine
		}
		return tileForestLeaf
	case biomeFamilyWetland:
		if signals.Wetness > 0.55 || signals.Moisture > 0.72 {
			return tileSwamp
		}
		if coordHash01(seed, x, y, biomeVariantSalt^0x04) < 0.35 {
			return tileClay
		}
		return tileDirt
	case biomeFamilyHeathMoor:
		if signals.Moisture < 0.36 || signals.Temperature < 0.42 {
			return tileMoor
		}
		return tileHeath
	case biomeFamilyMountain:
		if opts.HNHEnabled && coordHash01(seed, x, y, biomeVariantSalt^0x05) < opts.VariantDensity*0.22 {
			return tileStone
		}
		return tileMountain
	default:
		if opts.HNHEnabled {
			v := coordHash01(seed, x, y, biomeVariantSalt^0x06)
			if signals.Moisture < 0.30 && v < opts.VariantDensity*0.18 {
				return tileHeath
			}
			if signals.Moisture > 0.64 && v < opts.VariantDensity*0.10 {
				return tileForestLeaf
			}
			if v < opts.VariantDensity*0.09 {
				return tileDirt
			}
		}
		return tileGrass
	}
}

func smoothBiomeTiles(tiles []byte, width, height, passes int) {
	if passes <= 0 || len(tiles) == 0 {
		return
	}

	neighbors := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for pass := 0; pass < passes; pass++ {
		next := make([]byte, len(tiles))
		copy(next, tiles)

		var counts [256]int
		touched := make([]int, 0, 8)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := tileIndex(x, y, width)
				current := tiles[idx]
				if isLockedCoastTile(current) {
					continue
				}

				bestTile := current
				bestCount := 0
				touched = touched[:0]

				for _, d := range neighbors {
					nx := x + d[0]
					ny := y + d[1]
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					nTile := tiles[tileIndex(nx, ny, width)]
					if isLockedCoastTile(nTile) {
						continue
					}
					cIndex := int(nTile)
					if counts[cIndex] == 0 {
						touched = append(touched, cIndex)
					}
					counts[cIndex]++
					if counts[cIndex] > bestCount {
						bestCount = counts[cIndex]
						bestTile = nTile
					}
				}

				if bestTile != current && bestCount >= 5 {
					next[idx] = bestTile
				}
				for _, cIndex := range touched {
					counts[cIndex] = 0
				}
			}
		}

		copy(tiles, next)
	}
}

func smoothBiomeEdges(tiles []byte, width, height, passes int) {
	if passes <= 0 || len(tiles) == 0 {
		return
	}

	dirs8 := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}
	dirs4 := [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

	var counts [256]int
	touched := make([]int, 0, 8)

	for pass := 0; pass < passes; pass++ {
		next := make([]byte, len(tiles))
		copy(next, tiles)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := tileIndex(x, y, width)
				current := tiles[idx]
				if isLockedCoastTile(current) {
					continue
				}

				sameOrth := 0
				for _, d := range dirs4 {
					nx := x + d[0]
					ny := y + d[1]
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					nTile := tiles[tileIndex(nx, ny, width)]
					if nTile == current {
						sameOrth++
					}
				}
				if sameOrth >= 3 {
					continue
				}

				touched = touched[:0]
				dominantTile := current
				dominantCount := 0

				for _, d := range dirs8 {
					nx := x + d[0]
					ny := y + d[1]
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					nTile := tiles[tileIndex(nx, ny, width)]
					if isLockedCoastTile(nTile) {
						continue
					}
					cIndex := int(nTile)
					if counts[cIndex] == 0 {
						touched = append(touched, cIndex)
					}
					counts[cIndex]++
					if counts[cIndex] > dominantCount {
						dominantCount = counts[cIndex]
						dominantTile = nTile
					}
				}

				replace := false
				if dominantTile != current && dominantCount >= 6 {
					replace = true
				} else if dominantTile != current && sameOrth <= 1 && dominantCount >= 4 {
					replace = true
				}
				if replace {
					next[idx] = dominantTile
				}

				for _, cIndex := range touched {
					counts[cIndex] = 0
				}
			}
		}

		copy(tiles, next)
	}
}

func removeTinyBiomePatches(tiles []byte, width, height, minPatchTiles int) {
	if minPatchTiles <= 1 || len(tiles) == 0 {
		return
	}

	visited := make([]bool, len(tiles))
	component := make([]int, 0, minPatchTiles)
	queue := make([]int, 0, minPatchTiles)
	dirs := [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

	for idx := range tiles {
		if visited[idx] {
			continue
		}
		tile := tiles[idx]
		if isLockedCoastTile(tile) {
			visited[idx] = true
			continue
		}

		component = component[:0]
		queue = queue[:0]
		queue = append(queue, idx)
		visited[idx] = true

		for head := 0; head < len(queue); head++ {
			current := queue[head]
			component = append(component, current)
			cx := current % width
			cy := current / width

			for _, d := range dirs {
				nx := cx + d[0]
				ny := cy + d[1]
				if nx < 0 || ny < 0 || nx >= width || ny >= height {
					continue
				}
				nIdx := tileIndex(nx, ny, width)
				if visited[nIdx] {
					continue
				}
				if tiles[nIdx] != tile || isLockedCoastTile(tiles[nIdx]) {
					continue
				}
				visited[nIdx] = true
				queue = append(queue, nIdx)
			}
		}

		if len(component) >= minPatchTiles {
			continue
		}

		replacement := dominantNeighborTile(component, tiles, width, height, tile)
		for _, cIdx := range component {
			tiles[cIdx] = replacement
		}
	}
}

func dominantNeighborTile(component []int, tiles []byte, width, height int, oldTile byte) byte {
	var counts [256]int
	bestTile := byte(tileGrass)
	bestCount := 0
	dirs := [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

	for _, idx := range component {
		x := idx % width
		y := idx / width
		for _, d := range dirs {
			nx := x + d[0]
			ny := y + d[1]
			if nx < 0 || ny < 0 || nx >= width || ny >= height {
				continue
			}
			nTile := tiles[tileIndex(nx, ny, width)]
			if nTile == oldTile || isLockedCoastTile(nTile) {
				continue
			}
			cIndex := int(nTile)
			counts[cIndex]++
			if counts[cIndex] > bestCount {
				bestCount = counts[cIndex]
				bestTile = nTile
			}
		}
	}

	return bestTile
}

func isLockedCoastTile(tile byte) bool {
	switch tile {
	case tileWaterDeep, tileWater, tileSand:
		return true
	default:
		return false
	}
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
