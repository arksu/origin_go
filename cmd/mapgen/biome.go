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

func classifyBiomeGround(elevation float64, signals BiomeSignals, opts BiomeOptions, seed int64, column, row int) byte {
	if !opts.Enabled {
		return classifyBaseTile(elevation, signals.Moisture, signals.Temperature)
	}
	if signals.Moisture < 0.22 && signals.Temperature > 0.62 && signals.Continentalness > 0.45 {
		return tileSand
	}
	if signals.Ruggedness >= opts.MountainRuggedThreshold {
		const stoneSalt = uint64(0xDEADBEEF1248AA55) ^ 0x02
		// Coherent outcrops preserve the massif's visual continuity at tile scale.
		if smoothHashNoise2D(seed, float64(column), float64(row), opts.MountainStoneScale, stoneSalt) < 0.2 {
			return tileStone
		}
		return tileMountain
	}
	return tileGrass
}

func smoothBiomeTiles(tiles []byte, locked []bool, width, height, passes int) {
	if passes <= 0 || len(tiles) == 0 {
		return
	}

	neighbors := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	next := make([]byte, len(tiles))
	for pass := 0; pass < passes; pass++ {
		copy(next, tiles)

		var counts [256]int
		touched := make([]int, 0, 8)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := tileIndex(x, y, width)
				current := tiles[idx]
				if locked[idx] {
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
					neighborIndex := tileIndex(nx, ny, width)
					nTile := tiles[neighborIndex]
					if locked[neighborIndex] {
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

func smoothBiomeEdges(tiles []byte, locked []bool, width, height, passes int) {
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

	next := make([]byte, len(tiles))
	for pass := 0; pass < passes; pass++ {
		copy(next, tiles)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := tileIndex(x, y, width)
				current := tiles[idx]
				if locked[idx] {
					continue
				}

				sameOrth := 0
				for _, d := range dirs4 {
					nx := x + d[0]
					ny := y + d[1]
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					neighborIndex := tileIndex(nx, ny, width)
					nTile := tiles[neighborIndex]
					if !locked[neighborIndex] && nTile == current {
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
					neighborIndex := tileIndex(nx, ny, width)
					nTile := tiles[neighborIndex]
					if locked[neighborIndex] {
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

func cleanBiomeBorderArtifacts(tiles []byte, locked []bool, width, height, passes int) {
	if passes <= 0 || len(tiles) == 0 {
		return
	}

	neighbors := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	var counts [256]int
	touched := make([]int, 0, 8)

	next := make([]byte, len(tiles))
	for pass := 0; pass < passes; pass++ {
		copy(next, tiles)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := tileIndex(x, y, width)
				current := tiles[idx]
				if locked[idx] {
					continue
				}

				touched = touched[:0]
				sameCount := 0
				dominantTile := current
				dominantCount := 0

				for _, d := range neighbors {
					nx := x + d[0]
					ny := y + d[1]
					if nx < 0 || ny < 0 || nx >= width || ny >= height {
						continue
					}
					neighborIndex := tileIndex(nx, ny, width)
					nTile := tiles[neighborIndex]
					if locked[neighborIndex] {
						continue
					}
					if !locked[neighborIndex] && nTile == current {
						sameCount++
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

				shouldReplace := dominantTile != current && ((sameCount <= 2 && dominantCount >= 4) || (sameCount <= 1 && dominantCount >= 3))
				if shouldReplace {
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

func removeTinyBiomePatches(tiles []byte, locked []bool, width, height, minPatchTiles int) {
	if minPatchTiles <= 1 || len(tiles) == 0 {
		return
	}

	visited := make([]bool, len(tiles))
	// A full-capacity queue bounds allocation and also stores the component.
	queue := make([]int, 0, len(tiles))
	dirs := [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

	for idx := range tiles {
		if visited[idx] {
			continue
		}
		tile := tiles[idx]
		if locked[idx] {
			visited[idx] = true
			continue
		}

		queue = queue[:0]
		queue = append(queue, idx)
		visited[idx] = true

		for head := 0; head < len(queue); head++ {
			current := queue[head]
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
				if tiles[nIdx] != tile || locked[nIdx] {
					continue
				}
				visited[nIdx] = true
				queue = append(queue, nIdx)
			}
		}

		if len(queue) >= minPatchTiles {
			continue
		}

		replacement := dominantNeighborTile(queue, tiles, locked, width, height, tile)
		for _, cIdx := range queue {
			tiles[cIdx] = replacement
		}
	}
}

func dominantNeighborTile(component []int, tiles []byte, locked []bool, width, height int, oldTile byte) byte {
	var counts [256]int
	bestTile := oldTile
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
			neighborIndex := tileIndex(nx, ny, width)
			nTile := tiles[neighborIndex]
			if nTile == oldTile || locked[neighborIndex] {
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

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func cleanupBlobBiomes(tiles []byte, locked []bool, width, height int, opts BiomeOptions) {
	smoothBiomeTiles(tiles, locked, width, height, opts.SmoothingPasses)
	smoothBiomeEdges(tiles, locked, width, height, opts.SmoothingPasses)
	cleanBiomeBorderArtifacts(tiles, locked, width, height, opts.SmoothingPasses)
	removeTinyBiomePatches(tiles, locked, width, height, opts.MinPatchTiles)
}
