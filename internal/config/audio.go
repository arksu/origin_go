package config

import (
	"fmt"
	"math"

	"github.com/spf13/viper"
)

type AudioConfig struct {
	BaseHearing          float64 `mapstructure:"base_hearing"`
	ListenerCellSize     float64 `mapstructure:"listener_cell_size"`
	MaxEffectiveRadius   float64 `mapstructure:"max_effective_radius"`
	MaxEventsPerTick     int     `mapstructure:"max_events_per_tick"`
	MaxCellsPerTick      int     `mapstructure:"max_cells_per_tick"`
	MaxCandidatesPerTick int     `mapstructure:"max_candidates_per_tick"`
	MaxEntriesPerTick    int     `mapstructure:"max_entries_per_tick"`
	MaxEntriesPerBatch   int     `mapstructure:"max_entries_per_batch"`
	MaxBatchBytes        int     `mapstructure:"max_batch_bytes"`
	FreshnessMs          int     `mapstructure:"freshness_ms"`
	QueueCapacity        int     `mapstructure:"queue_capacity"`
}

func DefaultAudioConfig() AudioConfig {
	return AudioConfig{BaseHearing: 1, ListenerCellSize: 256, MaxEffectiveRadius: 4096,
		MaxEventsPerTick: 1024, MaxCellsPerTick: 65536, MaxCandidatesPerTick: 65536,
		MaxEntriesPerTick: 16384, MaxEntriesPerBatch: 64, MaxBatchBytes: 16384,
		FreshnessMs: 500, QueueCapacity: 1}
}

func (audio AudioConfig) Validate() error {
	for _, field := range []struct {
		name  string
		value float64
	}{
		{"base_hearing", audio.BaseHearing}, {"listener_cell_size", audio.ListenerCellSize}, {"max_effective_radius", audio.MaxEffectiveRadius},
	} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value <= 0 {
			return fmt.Errorf("game.audio.%s must be finite and positive", field.name)
		}
	}
	if audio.MaxEffectiveRadius > math.Sqrt(math.MaxFloat64) {
		return fmt.Errorf("game.audio.max_effective_radius is too large to square safely")
	}
	for _, field := range []struct {
		name  string
		value int
	}{
		{"max_events_per_tick", audio.MaxEventsPerTick}, {"max_cells_per_tick", audio.MaxCellsPerTick},
		{"max_candidates_per_tick", audio.MaxCandidatesPerTick}, {"max_entries_per_tick", audio.MaxEntriesPerTick},
		{"max_entries_per_batch", audio.MaxEntriesPerBatch}, {"max_batch_bytes", audio.MaxBatchBytes},
		{"freshness_ms", audio.FreshnessMs}, {"queue_capacity", audio.QueueCapacity},
	} {
		if field.value <= 0 || uint64(field.value) > math.MaxUint32 {
			return fmt.Errorf("game.audio.%s must be in 1..%d", field.name, uint64(math.MaxUint32))
		}
	}
	return nil
}

func setAudioDefaults(settings *viper.Viper) {
	audio := DefaultAudioConfig()
	settings.SetDefault("game.audio.base_hearing", audio.BaseHearing)
	settings.SetDefault("game.audio.listener_cell_size", audio.ListenerCellSize)
	settings.SetDefault("game.audio.max_effective_radius", audio.MaxEffectiveRadius)
	settings.SetDefault("game.audio.max_events_per_tick", audio.MaxEventsPerTick)
	settings.SetDefault("game.audio.max_cells_per_tick", audio.MaxCellsPerTick)
	settings.SetDefault("game.audio.max_candidates_per_tick", audio.MaxCandidatesPerTick)
	settings.SetDefault("game.audio.max_entries_per_tick", audio.MaxEntriesPerTick)
	settings.SetDefault("game.audio.max_entries_per_batch", audio.MaxEntriesPerBatch)
	settings.SetDefault("game.audio.max_batch_bytes", audio.MaxBatchBytes)
	settings.SetDefault("game.audio.freshness_ms", audio.FreshnessMs)
	settings.SetDefault("game.audio.queue_capacity", audio.QueueCapacity)
}
