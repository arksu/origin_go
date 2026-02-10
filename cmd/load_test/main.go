package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/persistence"
)

type Config struct {
	DBHost     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSchema   string
	DBPort     int
	ServerHost string
	ServerPort int

	Clients    int
	RampUp     int
	Duration   time.Duration
	MoveRadius int
	Period     time.Duration
	Seed       int64
	Scenario   string
}

func main() {
	cfg := parseFlags()

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UnixNano()
	}
	rand.Seed(cfg.Seed)
	logger.Info("Load test starting",
		zap.Int64("seed", cfg.Seed),
		zap.Int("clients", cfg.Clients),
		zap.Int("ramp_up", cfg.RampUp),
		zap.Duration("duration", cfg.Duration),
		zap.String("scenario", cfg.Scenario),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("Received shutdown signal")
		cancel()
	}()

	db, err := persistence.NewPostgres(ctx, &config.DatabaseConfig{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Database: cfg.DBName,
		Schema:   cfg.DBSchema,
		MaxConns: 20,
		MinConns: 5,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	accountPool := NewAccountPool(db, logger)
	if err := accountPool.Init(ctx); err != nil {
		logger.Fatal("Failed to initialize account pool", zap.Error(err))
	}

	metrics := NewMetrics()

	runner := NewRunner(cfg, db, accountPool, metrics, logger)

	if err := runner.Run(ctx); err != nil {
		logger.Error("Load test failed", zap.Error(err))
		os.Exit(1)
	}

	metrics.PrintSummary(logger)
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.DBHost, "db_host", "localhost", "Database host")
	flag.IntVar(&cfg.DBPort, "db_port", 5430, "Database port")
	flag.StringVar(&cfg.DBUser, "db_user", "origin", "Database user")
	flag.StringVar(&cfg.DBPassword, "db_password", "origin", "Database password")
	flag.StringVar(&cfg.DBName, "db_name", "origin", "Database name")
	flag.StringVar(&cfg.DBSchema, "db_schema", "origin", "Database schema")
	flag.StringVar(&cfg.ServerHost, "server_host", "localhost", "Game server host")
	flag.IntVar(&cfg.ServerPort, "server_port", 8080, "Game server port")

	flag.IntVar(&cfg.Clients, "clients", 10, "Number of concurrent clients")
	flag.IntVar(&cfg.RampUp, "ramp-up", 1, "Clients to add per second during ramp-up")
	durationStr := flag.String("duration", "60s", "Test duration")
	flag.IntVar(&cfg.MoveRadius, "move-radius", 100, "Movement radius in world units")
	periodStr := flag.String("period", "1s", "Movement period")
	flag.Int64Var(&cfg.Seed, "seed", 0, "Random seed (0 = use current time)")
	flag.StringVar(&cfg.Scenario, "scenario", "login-move", "Scenario: login-only, login-move")

	flag.Parse()

	var err error
	cfg.Duration, err = time.ParseDuration(*durationStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration: %v\n", err)
		os.Exit(1)
	}

	cfg.Period, err = time.ParseDuration(*periodStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid period: %v\n", err)
		os.Exit(1)
	}

	return cfg
}

// Runner orchestrates the load test
type Runner struct {
	cfg         *Config
	db          *persistence.Postgres
	accountPool *AccountPool
	metrics     *Metrics
	logger      *zap.Logger

	activeClients atomic.Int32
	wg            sync.WaitGroup
}

func NewRunner(cfg *Config, db *persistence.Postgres, pool *AccountPool, metrics *Metrics, logger *zap.Logger) *Runner {
	return &Runner{
		cfg:         cfg,
		db:          db,
		accountPool: pool,
		metrics:     metrics,
		logger:      logger,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, r.cfg.Duration)
	defer cancel()

	// Start packet statistics logger
	go r.startPacketStatsLogger(testCtx)

	rampUpInterval := time.Second / time.Duration(r.cfg.RampUp)
	clientsStarted := 0

	r.logger.Info("Starting ramp-up phase",
		zap.Int("target_clients", r.cfg.Clients),
		zap.Duration("interval", rampUpInterval),
	)

	ticker := time.NewTicker(rampUpInterval)
	defer ticker.Stop()

rampUpLoop:
	for clientsStarted < r.cfg.Clients {
		select {
		case <-testCtx.Done():
			break rampUpLoop
		case <-ticker.C:
			r.wg.Add(1)
			go r.runClient(testCtx, clientsStarted)
			clientsStarted++
			r.logger.Debug("Started client", zap.Int("client_num", clientsStarted))
		}
	}

	r.logger.Info("Ramp-up complete, waiting for test duration",
		zap.Int("active_clients", int(r.activeClients.Load())),
	)

	<-testCtx.Done()

	r.logger.Info("Test duration complete, waiting for clients to finish")
	r.wg.Wait()

	return nil
}

func (r *Runner) runClient(ctx context.Context, clientNum int) {
	defer r.wg.Done()

	vu := NewVirtualClient(r.cfg, r.db, r.accountPool, r.metrics, r.logger.With(zap.Int("vu", clientNum)))

	r.activeClients.Add(1)
	defer r.activeClients.Add(-1)

	if err := vu.Run(ctx); err != nil {
		r.logger.Debug("Virtual client finished with error", zap.Int("vu", clientNum), zap.Error(err))
	}
}

func (r *Runner) startPacketStatsLogger(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	prev := r.metrics.Snapshot()
	var prevMem runtime.MemStats
	runtime.ReadMemStats(&prevMem)
	lastTick := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			intervalSec := now.Sub(lastTick).Seconds()
			lastTick = now

			curr := r.metrics.Snapshot()

			dSent := curr.PacketsSent - prev.PacketsSent
			dRecv := curr.PacketsReceived - prev.PacketsReceived
			dBytesSent := curr.BytesSent - prev.BytesSent
			dBytesRecv := curr.BytesReceived - prev.BytesReceived

			dReadNs := curr.ReadWaitNs - prev.ReadWaitNs
			dReadSamples := curr.ReadWaitSamples - prev.ReadWaitSamples
			dUnmarshalNs := curr.UnmarshalNs - prev.UnmarshalNs
			dUnmarshalSamples := curr.UnmarshalSamples - prev.UnmarshalSamples
			dHandleNs := curr.HandleNs - prev.HandleNs
			dHandleSamples := curr.HandleSamples - prev.HandleSamples
			dMarshalNs := curr.SendMarshalNs - prev.SendMarshalNs
			dMarshalCount := curr.SendMarshalCount - prev.SendMarshalCount
			dWriteNs := curr.SendWriteNs - prev.SendWriteNs
			dWriteCount := curr.SendWriteCount - prev.SendWriteCount

			dAuth := curr.MsgAuthResult - prev.MsgAuthResult
			dEnter := curr.MsgEnterWorld - prev.MsgEnterWorld
			dSpawn := curr.MsgObjectSpawn - prev.MsgObjectSpawn
			dMove := curr.MsgObjectMove - prev.MsgObjectMove
			dSrvErr := curr.MsgServerError - prev.MsgServerError
			dOther := curr.MsgOther - prev.MsgOther
			dUnmarshalErr := curr.MsgUnmarshalErr - prev.MsgUnmarshalErr

			prev = curr

			readAvgUs := avgMicros(dReadNs, dReadSamples)
			unmarshalAvgUs := avgMicros(dUnmarshalNs, dUnmarshalSamples)
			handleAvgUs := avgMicros(dHandleNs, dHandleSamples)
			marshalAvgUs := avgMicros(dMarshalNs, dMarshalCount)
			writeAvgUs := avgMicros(dWriteNs, dWriteCount)

			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)

			dTotalAlloc := mem.TotalAlloc - prevMem.TotalAlloc
			dMallocs := mem.Mallocs - prevMem.Mallocs
			dFrees := mem.Frees - prevMem.Frees
			dPauseNs := mem.PauseTotalNs - prevMem.PauseTotalNs
			dNumGC := mem.NumGC - prevMem.NumGC
			prevMem = mem

			allocRateMBs := 0.0
			mallocRate := 0.0
			freeRate := 0.0
			if intervalSec > 0 {
				allocRateMBs = float64(dTotalAlloc) / (1024.0 * 1024.0) / intervalSec
				mallocRate = float64(dMallocs) / intervalSec
				freeRate = float64(dFrees) / intervalSec
			}

			gcPauseMs := float64(dPauseNs) / 1_000_000.0
			gcPausePct := 0.0
			if intervalSec > 0 {
				gcPausePct = (float64(dPauseNs) / (intervalSec * float64(time.Second))) * 100.0
			}

			r.logger.Info("LoadTest Diagnostics (5s interval)",
				zap.Int64("pkt_sent_5s", dSent),
				zap.Int64("pkt_recv_5s", dRecv),
				zap.Int64("bytes_sent_5s", dBytesSent),
				zap.Int64("bytes_recv_5s", dBytesRecv),
				zap.Float64("read_wait_avg_us_sampled", readAvgUs),
				zap.Float64("unmarshal_avg_us_sampled", unmarshalAvgUs),
				zap.Float64("handle_avg_us_sampled", handleAvgUs),
				zap.Float64("send_marshal_avg_us", marshalAvgUs),
				zap.Float64("send_write_avg_us", writeAvgUs),
				zap.Int64("msg_auth_result_5s", dAuth),
				zap.Int64("msg_enter_world_5s", dEnter),
				zap.Int64("msg_object_spawn_5s", dSpawn),
				zap.Int64("msg_object_move_5s", dMove),
				zap.Int64("msg_error_5s", dSrvErr),
				zap.Int64("msg_other_5s", dOther),
				zap.Int64("msg_unmarshal_err_5s", dUnmarshalErr),
				zap.Int("goroutines", runtime.NumGoroutine()),
				zap.Uint32("num_gc", mem.NumGC),
				zap.Uint32("num_gc_5s", dNumGC),
				zap.Uint64("heap_alloc_mb", mem.Alloc/1024/1024),
				zap.Uint64("total_alloc_mb_5s", dTotalAlloc/1024/1024),
				zap.Float64("alloc_rate_mb_s", allocRateMBs),
				zap.Uint64("mallocs_5s", dMallocs),
				zap.Float64("mallocs_s", mallocRate),
				zap.Uint64("frees_5s", dFrees),
				zap.Float64("frees_s", freeRate),
				zap.Float64("gc_pause_ms_5s", gcPauseMs),
				zap.Float64("gc_pause_pct_5s", gcPausePct),
			)
		}
	}
}

func avgMicros(totalNs, samples int64) float64 {
	if samples <= 0 {
		return 0
	}
	return float64(totalNs) / float64(samples) / 1000.0
}
