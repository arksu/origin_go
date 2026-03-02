package main

import "fmt"

type TerrainPrecompute struct {
	WidthTiles        int
	HeightTiles       int
	Elevation         []float32
	RiverClass        []RiverClass
	Tiles             []byte
	RiverSources      int
	RiverShallowTiles int
	RiverDeepTiles    int
}

func BuildTerrainPrecompute(opts MapgenOptions, chunkSize int, fields *NoiseFields) (*TerrainPrecompute, error) {
	widthTiles, heightTiles, err := opts.WorldTileDimensions()
	if err != nil {
		return nil, err
	}
	tileCount, err := checkedMulInt(widthTiles, heightTiles)
	if err != nil {
		return nil, fmt.Errorf("tile count overflow: %w", err)
	}

	elevation := make([]float32, tileCount)
	parallelForRows(heightTiles, opts.Threads, func(y int) {
		for x := 0; x < widthTiles; x++ {
			idx := tileIndex(x, y, widthTiles)
			elevation[idx] = float32(fields.Elevation(x, y))
		}
	})

	var riverClass []RiverClass
	riverSources := 0
	if opts.River.Enabled {
		riverNetwork, riverErr := BuildRiverNetwork(elevation, widthTiles, heightTiles, opts.Seed, opts.River)
		if riverErr != nil {
			return nil, riverErr
		}
		riverClass = riverNetwork.Class
		riverSources = riverNetwork.SourceCount
	}

	tiles := make([]byte, tileCount)
	parallelForRows(heightTiles, opts.Threads, func(y int) {
		for x := 0; x < widthTiles; x++ {
			idx := tileIndex(x, y, widthTiles)
			elevationValue := float64(elevation[idx])
			moisture, temperature := fields.MoistureTemperature(x, y)
			rc := riverNone
			if opts.River.Enabled && len(riverClass) == tileCount {
				rc = riverClass[idx]
			}
			tiles[idx] = resolveTileType(elevationValue, moisture, temperature, rc, opts.River.Enabled)
		}
	})

	riverShallowTiles := 0
	riverDeepTiles := 0
	if opts.River.Enabled && len(riverClass) == tileCount {
		for idx := range riverClass {
			if float64(elevation[idx]) < shallowWaterThreshold {
				continue
			}
			switch riverClass[idx] {
			case riverShallow:
				riverShallowTiles++
			case riverDeep:
				riverDeepTiles++
			}
		}
	}

	return &TerrainPrecompute{
		WidthTiles:        widthTiles,
		HeightTiles:       heightTiles,
		Elevation:         elevation,
		RiverClass:        riverClass,
		Tiles:             tiles,
		RiverSources:      riverSources,
		RiverShallowTiles: riverShallowTiles,
		RiverDeepTiles:    riverDeepTiles,
	}, nil
}

func parallelForRows(height int, threads int, fn func(y int)) {
	if threads <= 1 || height <= 1 {
		for y := 0; y < height; y++ {
			fn(y)
		}
		return
	}

	jobs := make(chan int, threads*2)
	done := make(chan struct{}, threads)

	for worker := 0; worker < threads; worker++ {
		go func() {
			for y := range jobs {
				fn(y)
			}
			done <- struct{}{}
		}()
	}

	for y := 0; y < height; y++ {
		jobs <- y
	}
	close(jobs)

	for worker := 0; worker < threads; worker++ {
		<-done
	}
}

func resolveTileType(elevation, moisture, temperature float64, rc RiverClass, riverEnabled bool) byte {
	if elevation < deepWaterThreshold {
		return tileWaterDeep
	}
	if elevation < shallowWaterThreshold {
		return tileWater
	}
	if riverEnabled {
		switch rc {
		case riverDeep:
			return tileWaterDeep
		case riverShallow:
			return tileWater
		}
	}
	return classifyBaseTile(elevation, moisture, temperature)
}
