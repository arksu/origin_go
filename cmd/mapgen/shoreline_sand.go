package main

const (
	shoreSandNoiseSaltA = uint64(0xF1E2D3C4B5A69788)
	shoreSandNoiseSaltB = uint64(0x9A8B7C6D5E4F3021)
)

func applyShorelineSand(
	tiles []byte,
	baseTiles []byte,
	riverClass []RiverClass,
	elevation []float32,
	width int,
	height int,
	seed int64,
) {
	if len(tiles) != width*height || len(baseTiles) != len(tiles) || len(elevation) != len(tiles) {
		return
	}

	// Step 1: remove deterministic inland sand rings around non-ocean water.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := tileIndex(x, y, width)
			if tiles[idx] != tileSand {
				continue
			}
			hasOcean, hasInland, _ := classifyAdjacentWater(tiles, riverClass, elevation, width, height, x, y)
			if hasInland && !hasOcean {
				replacement := baseTiles[idx]
				if replacement == tileSand || isWaterTileID(replacement) {
					replacement = dominantNearbyLandTile(tiles, width, height, x, y)
				}
				if replacement == tileSand || isWaterTileID(replacement) {
					replacement = tileGrass
				}
				tiles[idx] = replacement
			}
		}
	}

	// Step 2: add occasional shoreline sand patches near ocean/lakes/rivers.
	shoreCandidates := make([]int, 0, len(tiles)/8)
	addedShoreSand := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := tileIndex(x, y, width)
			tile := tiles[idx]
			if !isShoreSandCandidate(tile) {
				continue
			}

			hasOcean, hasInland, hasDeep := classifyAdjacentWater(tiles, riverClass, elevation, width, height, x, y)
			if !hasOcean && !hasInland {
				continue
			}
			shoreCandidates = append(shoreCandidates, idx)

			chance := 0.0
			if hasOcean {
				chance = 0.17
			}
			if hasInland {
				if chance < 0.11 {
					chance = 0.11
				}
				if hasDeep {
					chance += 0.04
				}
			}
			if hasOcean && hasInland {
				chance += 0.03
			}

			clusterScale := 14.0
			if hasOcean {
				clusterScale = 20.0
			}
			cluster := smoothHashNoise2D(seed+911, float64(x), float64(y), clusterScale, shoreSandNoiseSaltA)
			modulation := 0.75 + smoothHashNoise2D(seed+1777, float64(x), float64(y), clusterScale*2, shoreSandNoiseSaltB)*0.5
			target := chance * modulation
			if cluster < target {
				tiles[idx] = tileSand
				addedShoreSand++
			}
		}
	}

	// Keep behavior "sometimes sand near shores" but avoid all-zero outcomes
	// on seeds where coherent-noise threshold misses every candidate.
	if addedShoreSand == 0 && len(shoreCandidates) > 0 {
		fallbackBudget := maxInt(1, len(shoreCandidates)/140)
		for _, idx := range shoreCandidates {
			x := idx % width
			y := idx / width
			if coordHash01(seed+4099, x, y, shoreSandNoiseSaltA^shoreSandNoiseSaltB) < 0.08 {
				tiles[idx] = tileSand
				addedShoreSand++
				if addedShoreSand >= fallbackBudget {
					break
				}
			}
		}
	}
}

func classifyAdjacentWater(
	tiles []byte,
	riverClass []RiverClass,
	elevation []float32,
	width int,
	height int,
	x int,
	y int,
) (hasOcean bool, hasInland bool, hasDeep bool) {
	dirs := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for _, d := range dirs {
		nx := x + d[0]
		ny := y + d[1]
		if nx < 0 || ny < 0 || nx >= width || ny >= height {
			continue
		}
		nIdx := tileIndex(nx, ny, width)
		nTile := tiles[nIdx]
		if !isWaterTileID(nTile) {
			continue
		}

		isRiverWater := len(riverClass) == len(tiles) && riverClass[nIdx] != riverNone
		isOceanWater := !isRiverWater && float64(elevation[nIdx]) < shallowWaterThreshold
		if isOceanWater {
			hasOcean = true
		} else {
			hasInland = true
		}
		if nTile == tileWaterDeep || isRiverWater && riverClass[nIdx] == riverDeep {
			hasDeep = true
		}
	}

	return hasOcean, hasInland, hasDeep
}

func dominantNearbyLandTile(tiles []byte, width, height, x, y int) byte {
	var counts [256]int
	bestTile := byte(tileGrass)
	bestCount := 0
	dirs := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for _, d := range dirs {
		nx := x + d[0]
		ny := y + d[1]
		if nx < 0 || ny < 0 || nx >= width || ny >= height {
			continue
		}
		nTile := tiles[tileIndex(nx, ny, width)]
		if isWaterTileID(nTile) || nTile == tileSand {
			continue
		}
		counts[nTile]++
		if counts[nTile] > bestCount {
			bestCount = counts[nTile]
			bestTile = nTile
		}
	}

	return bestTile
}

func isWaterTileID(tile byte) bool {
	return tile == tileWater || tile == tileWaterDeep
}

func isShoreSandCandidate(tile byte) bool {
	switch tile {
	case tileGrass, tileDirt, tileClay, tileHeath, tileMoor, tileForestLeaf, tileForestPine, tileThicket:
		return true
	default:
		return false
	}
}
