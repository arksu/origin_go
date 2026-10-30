package config

import (
	"math"
	"testing"

	"github.com/spf13/viper"
)

func TestFarmingQualityVariance(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	if actual := v.GetInt64("game.farming_quality_variance"); actual != 2 {
		t.Fatalf("default variance = %d, want 2", actual)
	}
	for _, value := range []int64{0, 2, math.MaxUint32} {
		if err := validateFarmingQualityVariance(value); err != nil {
			t.Fatalf("variance %d rejected: %v", value, err)
		}
	}
	for _, value := range []int64{-1, math.MaxUint32 + 1} {
		if err := validateFarmingQualityVariance(value); err == nil {
			t.Fatalf("variance %d accepted", value)
		}
	}
}
