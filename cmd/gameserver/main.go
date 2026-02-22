package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"origin/internal/config"
	"origin/internal/craftdefs"
	"origin/internal/game"
	"origin/internal/game/behaviors"
	"origin/internal/game/events"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/metrics"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/restapi"
)

func main() {
	// Parse command line flags
	var enableStats bool
	var enableVisionStats bool
	flag.BoolVar(&enableStats, "stats", false, "Enable game tick statistics logging")
	flag.BoolVar(&enableVisionStats, "vision-stats", false, "Enable vision system metrics logging")
	flag.Parse()

	//debug.SetGCPercent(40)
	//debug.SetMemoryLimit(6 * 1024 * 1024 * 1024)

	// Configure logging to both console and file
	logFile, err := os.OpenFile("./log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer logFile.Close()

	// Create encoder config for both console and file
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Create cores for console and file output
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		zapcore.DebugLevel,
	)
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(logFile),
		zapcore.DebugLevel,
	)

	// Combine cores for multi-output logging
	core := zapcore.NewTee(consoleCore, fileCore)
	logger := zap.New(core, zap.AddCaller())
	defer logger.Sync()

	cfg := config.MustLoad(logger)

	// Load item definitions before game loop starts
	itemRegistry, err := itemdefs.LoadFromDirectory("./data/items", logger)
	if err != nil {
		logger.Fatal("Failed to load item definitions", zap.Error(err))
	}
	itemdefs.SetGlobal(itemRegistry)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := persistence.NewPostgres(ctx, &cfg.Database, logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Load object definitions before game loop starts
	behaviorRegistry, err := behaviors.DefaultRegistry()
	if err != nil {
		logger.Fatal("Failed to initialize behavior registry", zap.Error(err))
	}
	objRegistry, err := objectdefs.LoadFromDirectory("./data/objects", behaviorRegistry, logger)
	if err != nil {
		logger.Fatal("Failed to load object definitions", zap.Error(err))
	}
	objectdefs.SetGlobal(objRegistry)

	craftRegistry, err := craftdefs.LoadFromDirectory("./data/crafts", logger)
	if err != nil {
		logger.Fatal("Failed to load craft definitions", zap.Error(err))
	}
	craftdefs.SetGlobal(craftRegistry)

	inventoryLoader := inventory.NewInventoryLoader(logger)
	inventorySnapshotSender := inventory.NewSnapshotSender(logger)

	objectFactory := world.NewObjectFactory(inventory.NewDroppedInventoryLoaderDB(db, inventoryLoader))

	g := game.NewGame(cfg, db, objectFactory, inventoryLoader, inventorySnapshotSender, enableStats, enableVisionStats, logger)

	// Setup event handlers after game creation
	dispatcher := events.NewNetworkVisibilityDispatcher(g.ShardManager(), logger.Named("visibility-dispatcher"))
	dispatcher.Subscribe(g.ShardManager().EventBus())

	mux := http.NewServeMux()

	// Register pprof endpoints if enabled
	if cfg.Game.PprofEnabled {
		mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
		runtime.SetBlockProfileRate(10000)
		runtime.SetMutexProfileFraction(1)
		logger.Info("pprof enabled")
	}

	httpHandler := restapi.NewHandler(db, g.EntityIDManager(), logger, &cfg.Game)
	httpHandler.RegisterRoutes(mux)

	// Register Prometheus metrics
	metricsCollector := metrics.NewCollector(g)
	prometheus.MustRegister(metricsCollector)
	mux.Handle("/metrics", promhttp.Handler())
	logger.Info("Prometheus metrics enabled", zap.String("endpoint", "/metrics"))

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := g.NetworkServer().Start(addr, mux); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}

	g.StartGameLoop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("Shutdown signal received")

	g.Stop()
}
