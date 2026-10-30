package config

import (
	"fmt"
	"math"
)

func validateFarmingQualityVariance(variance int64) error {
	if variance < 0 || variance > math.MaxUint32 {
		return fmt.Errorf("game.farming_quality_variance must be in [0, %d]", uint64(math.MaxUint32))
	}
	return nil
}
