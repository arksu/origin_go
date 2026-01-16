package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
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

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			packetsSent := r.metrics.packetsSent.Load()
			packetsReceived := r.metrics.packetsReceived.Load()

			r.logger.Info("Packet Statistics (5s interval)",
				zap.Int64("packets_sent", packetsSent),
				zap.Int64("packets_received", packetsReceived),
				zap.Int64("packets_total", packetsSent+packetsReceived),
			)
		}
	}
}
