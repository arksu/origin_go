package main

import (
	"context"
	"errors"
	"fmt"
	"origin/internal/persistence/repository"
)

const spotInsertBatchSize = 1000

// The caller owns the transaction. Batches reuse fixed-capacity slices and are
// inserted synchronously, so the insert callback must not retain their contents.
func writeSpotBatches(ctx context.Context, terrain *TerrainPrecompute, config SpotsConfig,
	seed int64, region int, runtimeSeconds int64,
	insert func(context.Context, repository.InsertSpotsParams) error,
) (SpotGenerationStats, error) {
	if runtimeSeconds < 0 {
		return SpotGenerationStats{}, errors.New("spot runtime seconds must be nonnegative")
	}
	if insert == nil {
		return SpotGenerationStats{}, errors.New("spot insert callback is required")
	}
	batch := repository.InsertSpotsParams{
		Region: region, Layer: 0, LastRuntimeSeconds: runtimeSeconds,
		SpotTypes:     make([]string, 0, spotInsertBatchSize),
		DistrictXs:    make([]int, 0, spotInsertBatchSize),
		DistrictYs:    make([]int, 0, spotInsertBatchSize),
		CenterXs:      make([]int, 0, spotInsertBatchSize),
		CenterYs:      make([]int, 0, spotInsertBatchSize),
		Radii:         make([]int, 0, spotInsertBatchSize),
		PeakQualities: make([]int16, 0, spotInsertBatchSize),
	}
	flush := func() error {
		if len(batch.SpotTypes) == 0 {
			return nil
		}
		if err := insert(ctx, batch); err != nil {
			return fmt.Errorf("insert spot batch: %w", err)
		}
		batch.SpotTypes = batch.SpotTypes[:0]
		batch.DistrictXs = batch.DistrictXs[:0]
		batch.DistrictYs = batch.DistrictYs[:0]
		batch.CenterXs = batch.CenterXs[:0]
		batch.CenterYs = batch.CenterYs[:0]
		batch.Radii = batch.Radii[:0]
		batch.PeakQualities = batch.PeakQualities[:0]
		return nil
	}
	stats, err := GenerateSpots(ctx, terrain, config, seed, func(spot GeneratedSpot) error {
		batch.SpotTypes = append(batch.SpotTypes, spot.SpotType)
		batch.DistrictXs = append(batch.DistrictXs, spot.DistrictX)
		batch.DistrictYs = append(batch.DistrictYs, spot.DistrictY)
		batch.CenterXs = append(batch.CenterXs, spot.CenterX)
		batch.CenterYs = append(batch.CenterYs, spot.CenterY)
		batch.Radii = append(batch.Radii, spot.Radius)
		batch.PeakQualities = append(batch.PeakQualities, spot.PeakQuality)
		if len(batch.SpotTypes) == spotInsertBatchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	if err := flush(); err != nil {
		return stats, err
	}
	return stats, nil
}
