package main

import (
	"context"
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

	"origin/internal/config"
	"origin/internal/game"
	"origin/internal/game/events"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/metrics"
	"origin/internal/persistence"
	"origin/internal/restapi"
)

func main() {
	//debug.SetGCPercent(40)
	//debug.SetMemoryLimit(6 * 1024 * 1024 * 1024)

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
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

	objectFactory := world.NewObjectFactory()
	objectFactory.RegisterBuilder(&world.TreeBuilder{})
	objectFactory.RegisterBuilder(&world.PlayerBuilder{})

	inventoryLoader := inventory.NewInventoryLoader(itemRegistry, logger)
	inventorySnapshotSender := inventory.NewSnapshotSender(logger, itemRegistry)

	g := game.NewGame(cfg, db, objectFactory, inventoryLoader, inventorySnapshotSender, logger)

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
