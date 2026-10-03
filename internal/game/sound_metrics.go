package game

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var soundWorkTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "world_sound_work_total", Help: "World audio work performed, by shard layer and operation.",
}, []string{"layer", "operation"})

var soundDropsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "world_sound_drops_total", Help: "World audio discarded by reason; truncated queries do not scan unheard recipients.",
}, []string{"layer", "reason"})

var soundProcessingSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "world_sound_processing_seconds", Help: "World audio propagation and encoding time per active tick.",
	Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05},
}, []string{"layer", "operation"})

func observeSoundTick(layer int, stats SoundTickStats) {
	if stats.Events == 0 && stats.InvalidEvents == 0 {
		return
	}
	layerLabel := strconv.Itoa(layer)
	for _, operation := range []struct {
		name  string
		count uint64
	}{
		{"events", stats.Events}, {"cells", stats.Cells}, {"candidates", stats.Candidates},
		{"recipients", stats.Recipients}, {"entries", stats.Entries}, {"messages", stats.Messages}, {"bytes", stats.Bytes},
	} {
		soundWorkTotal.WithLabelValues(layerLabel, operation.name).Add(float64(operation.count))
	}
	for _, reason := range []struct {
		name  string
		count uint64
	}{
		{"event_budget", stats.EventDrops}, {"invalid_event", stats.InvalidEvents}, {"stale_event", stats.StaleEvents},
		{"entry_budget", stats.EntryDrops}, {"byte_budget", stats.ByteDrops}, {"work_budget", stats.WorkDrops},
		{"cell_budget_cutoff", stats.CellBudgetCutoffs}, {"candidate_budget_cutoff", stats.CandidateBudgetCutoffs},
		{"global_entry_budget_cutoff", stats.GlobalEntryBudgetCutoffs},
		{"truncated_query", stats.TruncatedQueries}, {"disconnected", stats.Disconnected}, {"audio_admission", stats.TransportDrops},
	} {
		soundDropsTotal.WithLabelValues(layerLabel, reason.name).Add(float64(reason.count))
	}
	soundProcessingSeconds.WithLabelValues(layerLabel, "propagation").Observe(stats.PropagationTime.Seconds())
	soundProcessingSeconds.WithLabelValues(layerLabel, "encoding").Observe(stats.EncodeTime.Seconds())
}
