package main

import "math"

type riverCrossSection struct {
	deepWidth, leftShallow, rightShallow float64
}

func hasRiverWidthProfile(opts RiverOptions) bool {
	return opts.LayoutDraw && opts.FairwayWidthTiles > 0
}

func riverCorridorMaximumWidth(opts RiverOptions) int {
	if hasRiverWidthProfile(opts) {
		return maxInt(opts.RiverWidthMax, opts.FairwayWidthTiles) + 2*opts.ShallowWidthMax
	}
	return opts.RiverWidthMax + 2*opts.BankRadius
}

func riverCrossSectionAt(distance float64, nominalWidth int, seed int64, opts RiverOptions) riverCrossSection {
	minimum := maxInt(opts.FairwayWidthTiles, opts.RiverWidthMin)
	maximum := maxInt(minimum, opts.RiverWidthMax)
	deepWidth := float64(clampInt(nominalWidth, minimum, maximum))
	if opts.WidthVariationScale > 0 && maximum > minimum {
		variation := smoothHashNoise2D(seed, distance, 0, float64(opts.WidthVariationScale), riverWidthSalt)
		deepWidth = float64(minimum) + float64(maximum-minimum)*variation
	}
	bankWidth := func(salt uint64) float64 {
		if opts.ShallowWidthMin == opts.ShallowWidthMax {
			return float64(opts.ShallowWidthMin)
		}
		variation := smoothHashNoise2D(seed, distance, 0, float64(opts.ShallowVariationScale), salt)
		return math.Round(float64(opts.ShallowWidthMin) + float64(opts.ShallowWidthMax-opts.ShallowWidthMin)*variation)
	}
	return riverCrossSection{
		deepWidth:    deepWidth,
		leftShallow:  bankWidth(riverWidthSalt ^ 0xA24BAED4963EE407),
		rightShallow: bankWidth(riverWidthSalt ^ 0x9FB21C651E98DF25),
	}
}

func carveVariableRiverCorridor(flow []uint32, width, height int, path []int, nominalWidth int, seed int64, opts RiverOptions) {
	if len(path) == 0 {
		return
	}
	profileSeed := seed ^ int64(splitMix64(uint64(path[0])^uint64(path[len(path)-1])*riverSourceSalt))
	coreRadius := opts.FairwayWidthTiles / 2
	distance := 0.0
	for position, index := range path {
		if position > 0 {
			previous := path[position-1]
			distance += math.Hypot(float64(index%width-previous%width), float64(index/width-previous/width))
		}
		section := riverCrossSectionAt(distance, nominalWidth, profileSeed, opts)
		deepRadius := maxFloat(0, math.Round((section.deepWidth-float64(opts.FairwayWidthTiles))/2))
		deepRadius = minFloat(deepRadius, float64(maxInt(opts.FairwayWidthTiles, opts.RiverWidthMax)-opts.FairwayWidthTiles)/2)
		radius := coreRadius + int(math.Ceil(deepRadius+maxFloat(section.leftShallow, section.rightShallow)))
		column := clampInt(index%width, coreRadius, width-1-coreRadius)
		row := clampInt(index/width, coreRadius, height-1-coreRadius)
		before, after := path[maxInt(0, position-4)], path[minInt(len(path)-1, position+4)]
		deltaColumn, deltaRow := float64(after%width-before%width), float64(after/width-before/width)
		tangentLength := math.Hypot(deltaColumn, deltaRow)
		normalColumn, normalRow := 0.0, 1.0
		if tangentLength > 0 {
			normalColumn, normalRow = -deltaRow/tangentLength, deltaColumn/tangentLength
		}
		for nearbyRow := maxInt(0, row-radius); nearbyRow <= minInt(height-1, row+radius); nearbyRow++ {
			for nearbyColumn := maxInt(0, column-radius); nearbyColumn <= minInt(width-1, column+radius); nearbyColumn++ {
				offsetColumn, offsetRow := nearbyColumn-column, nearbyRow-row
				outsideColumn := maxInt(0, absInt(offsetColumn)-coreRadius)
				outsideRow := maxInt(0, absInt(offsetRow)-coreRadius)
				distanceSquared := float64(outsideColumn*outsideColumn + outsideRow*outsideRow)
				nearbyIndex := tileIndex(nearbyColumn, nearbyRow, width)
				if distanceSquared <= deepRadius*deepRadius {
					flow[nearbyIndex] = max(flow[nearbyIndex], uint32(opts.FlowDeepThreshold))
					continue
				}
				offsetLength := math.Hypot(float64(offsetColumn), float64(offsetRow))
				side := (float64(offsetColumn)*normalColumn + float64(offsetRow)*normalRow) / offsetLength
				bankWidth := section.leftShallow*(1+side)/2 + section.rightShallow*(1-side)/2
				outerRadius := deepRadius + bankWidth
				if distanceSquared <= outerRadius*outerRadius {
					flow[nearbyIndex] = max(flow[nearbyIndex], uint32(opts.FlowShallowThreshold))
				}
			}
		}
	}
}
