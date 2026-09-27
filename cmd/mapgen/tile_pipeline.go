package main

import (
	"fmt"
	"image"
	"time"
)

type TerrainTimings struct {
	Elevation, Rivers, Ground, Main, Cleanup, Secondary, Satellites, Hydrology time.Duration
}

type TerrainPrecompute struct {
	Timings                                           TerrainTimings
	MainPatches, SecondaryPatches, Islets, IsletTiles int

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
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	widthTiles, heightTiles, err := opts.WorldTileDimensions()
	if err != nil {
		return nil, err
	}
	tileCount, err := checkedMulInt(widthTiles, heightTiles)
	if err != nil {
		return nil, fmt.Errorf("tile count overflow: %w", err)
	}

	timings := TerrainTimings{}
	started := time.Now()
	elevation := make([]float32, tileCount)
	parallelForRows(heightTiles, opts.Threads, func(y int) {
		for x := 0; x < widthTiles; x++ {
			idx := tileIndex(x, y, widthTiles)
			elevation[idx] = float32(fields.Elevation(x, y))
		}
	})

	timings.Elevation = time.Since(started)
	started = time.Now()
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

	timings.Rivers = time.Since(started)
	started = time.Now()
	baseTiles := make([]byte, tileCount)

	parallelForRows(heightTiles, opts.Threads, func(y int) {
		for x := 0; x < widthTiles; x++ {
			idx := tileIndex(x, y, widthTiles)
			elevationValue := float64(elevation[idx])
			signals := fields.BiomeSignals(x, y, opts.Biome)

			baseTiles[idx] = classifyBiomeGround(elevationValue, signals, opts.Biome, opts.Seed, x, y)
		}
	})

	timings.Ground = time.Since(started)
	mainCount, secondaryCount, islets, isletTiles := 0, 0, 0, 0
	if opts.Biome.Enabled && opts.Biome.BlobEnabled {
		locked := buildBiomeStructuralMask(baseTiles, elevation, riverClass, opts.River.Enabled)
		painter := blobPainter{tiles: baseTiles, locked: locked, world: image.Rect(0, 0, widthTiles, heightTiles), seed: opts.Seed, opts: opts.Biome, signals: func(column, row int) BiomeSignals { return fields.BiomeSignals(column, row, opts.Biome) }}
		started = time.Now()
		patches := painter.mainPatches()
		mainCount = len(patches)
		painter.paintMain(patches)
		timings.Main = time.Since(started)
		started = time.Now()
		cleanupBlobBiomes(baseTiles, locked, widthTiles, heightTiles, opts.Biome)
		timings.Cleanup = time.Since(started)
		started = time.Now()
		secondaryCount = painter.paintSecondary()
		timings.Secondary = time.Since(started)
		started = time.Now()
		islets, isletTiles = painter.paintIslets(patches)
		timings.Satellites = time.Since(started)
	}
	started = time.Now()

	tiles := make([]byte, tileCount)
	parallelForRows(heightTiles, opts.Threads, func(y int) {
		for x := 0; x < widthTiles; x++ {
			idx := tileIndex(x, y, widthTiles)
			elevationValue := float64(elevation[idx])
			rc := riverNone
			if opts.River.Enabled && len(riverClass) == tileCount {
				rc = riverClass[idx]
			}
			tiles[idx] = resolveTileType(elevationValue, baseTiles[idx], rc, opts.River.Enabled)
		}
	})
	applyShorelineSand(tiles, baseTiles, riverClass, elevation, widthTiles, heightTiles, opts.Seed)

	timings.Hydrology = time.Since(started)
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
		Timings: timings, MainPatches: mainCount, SecondaryPatches: secondaryCount, Islets: islets, IsletTiles: isletTiles,
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

func resolveTileType(elevation float64, baseTile byte, rc RiverClass, riverEnabled bool) byte {
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
	return baseTile
}
