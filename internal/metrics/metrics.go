package metrics

import (
	"origin/internal/game"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Collector struct {
	game *game.Game

	connectedClients *prometheus.Desc
	totalPlayers     *prometheus.Desc
	currentTick      *prometheus.Desc
	tickRate         *prometheus.Desc
	avgTickDuration  *prometheus.Desc

	chunkActiveCount    *prometheus.Desc
	chunkPreloadedCount *prometheus.Desc
	chunkInactiveCount  *prometheus.Desc
	chunkLoadRequests   *prometheus.Desc
	chunkSaveRequests   *prometheus.Desc
	chunkCacheHits      *prometheus.Desc
	chunkCacheMisses    *prometheus.Desc
}

func NewCollector(g *game.Game) *Collector {
	return &Collector{
		game: g,
		connectedClients: prometheus.NewDesc(
			"game_connected_clients",
			"Number of connected clients",
			nil, nil,
		),
		totalPlayers: prometheus.NewDesc(
			"game_total_players",
			"Total number of players in all shards",
			nil, nil,
		),
		currentTick: prometheus.NewDesc(
			"game_current_tick",
			"Current game tick number",
			nil, nil,
		),
		tickRate: prometheus.NewDesc(
			"game_tick_rate",
			"Game tick rate in Hz",
			nil, nil,
		),
		avgTickDuration: prometheus.NewDesc(
			"game_avg_tick_duration_seconds",
			"Average tick duration in seconds",
			nil, nil,
		),
		chunkActiveCount: prometheus.NewDesc(
			"game_chunk_active_count",
			"Number of active chunks",
			[]string{"layer"}, nil,
		),
		chunkPreloadedCount: prometheus.NewDesc(
			"game_chunk_preloaded_count",
			"Number of preloaded chunks",
			[]string{"layer"}, nil,
		),
		chunkInactiveCount: prometheus.NewDesc(
			"game_chunk_inactive_count",
			"Number of inactive chunks",
			[]string{"layer"}, nil,
		),
		chunkLoadRequests: prometheus.NewDesc(
			"game_chunk_load_requests_total",
			"Total number of chunk load requests",
			[]string{"layer"}, nil,
		),
		chunkSaveRequests: prometheus.NewDesc(
			"game_chunk_save_requests_total",
			"Total number of chunk save requests",
			[]string{"layer"}, nil,
		),
		chunkCacheHits: prometheus.NewDesc(
			"game_chunk_cache_hits_total",
			"Total number of chunk cache hits",
			[]string{"layer"}, nil,
		),
		chunkCacheMisses: prometheus.NewDesc(
			"game_chunk_cache_misses_total",
			"Total number of chunk cache misses",
			[]string{"layer"}, nil,
		),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connectedClients
	ch <- c.totalPlayers
	ch <- c.currentTick
	ch <- c.tickRate
	ch <- c.avgTickDuration
	ch <- c.chunkActiveCount
	ch <- c.chunkPreloadedCount
	ch <- c.chunkInactiveCount
	ch <- c.chunkLoadRequests
	ch <- c.chunkSaveRequests
	ch <- c.chunkCacheHits
	ch <- c.chunkCacheMisses
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	gameStats := c.game.Stats()

	ch <- prometheus.MustNewConstMetric(
		c.connectedClients,
		prometheus.GaugeValue,
		float64(gameStats.ConnectedClients),
	)
	ch <- prometheus.MustNewConstMetric(
		c.totalPlayers,
		prometheus.GaugeValue,
		float64(gameStats.TotalPlayers),
	)
	ch <- prometheus.MustNewConstMetric(
		c.currentTick,
		prometheus.CounterValue,
		float64(gameStats.CurrentTick),
	)
	ch <- prometheus.MustNewConstMetric(
		c.tickRate,
		prometheus.GaugeValue,
		float64(gameStats.TickRate),
	)
	ch <- prometheus.MustNewConstMetric(
		c.avgTickDuration,
		prometheus.GaugeValue,
		gameStats.AvgTickDuration.Seconds(),
	)

	shardManager := c.game.ShardManager()
	shards := shardManager.GetShards()
	var wg sync.WaitGroup

	for layer, shard := range shards {
		wg.Add(1)
		go func(l int, s *game.Shard) {
			defer wg.Done()
			if s == nil {
				return
			}

			chunkStats := s.ChunkManager().Stats()
			layerLabel := prometheus.Labels{"layer": string(rune('0' + l))}

			ch <- prometheus.MustNewConstMetric(
				c.chunkActiveCount,
				prometheus.GaugeValue,
				float64(chunkStats.ActiveCount),
				layerLabel["layer"],
			)
			ch <- prometheus.MustNewConstMetric(
				c.chunkPreloadedCount,
				prometheus.GaugeValue,
				float64(chunkStats.PreloadedCount),
				layerLabel["layer"],
			)
			ch <- prometheus.MustNewConstMetric(
				c.chunkInactiveCount,
				prometheus.GaugeValue,
				float64(chunkStats.InactiveCount),
				layerLabel["layer"],
			)
			ch <- prometheus.MustNewConstMetric(
				c.chunkLoadRequests,
				prometheus.CounterValue,
				float64(chunkStats.LoadRequests),
				layerLabel["layer"],
			)
			ch <- prometheus.MustNewConstMetric(
				c.chunkSaveRequests,
				prometheus.CounterValue,
				float64(chunkStats.SaveRequests),
				layerLabel["layer"],
			)
			ch <- prometheus.MustNewConstMetric(
				c.chunkCacheHits,
				prometheus.CounterValue,
				float64(chunkStats.CacheHits),
				layerLabel["layer"],
			)
			ch <- prometheus.MustNewConstMetric(
				c.chunkCacheMisses,
				prometheus.CounterValue,
				float64(chunkStats.CacheMisses),
				layerLabel["layer"],
			)
		}(layer, shard)
	}

	wg.Wait()
}
